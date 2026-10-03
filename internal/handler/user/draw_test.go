package user

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"ai-go-mall/internal/middleware"
	"ai-go-mall/internal/model"
	"ai-go-mall/internal/model/mall"
	userSvc "ai-go-mall/internal/service/user"
)

// mockDrawService 实现 userSvc.DrawService。
type mockDrawService struct {
	drawFunc func(c *gin.Context, userID, blindBoxID int64) (*userSvc.DrawResult, error)

	drawCalls    int
	lastUserID   int64
	lastBoxID    int64
	lastUserInCt *model.User
}

func (m *mockDrawService) Draw(c *gin.Context, userID, blindBoxID int64) (*userSvc.DrawResult, error) {
	m.drawCalls++
	m.lastUserID = userID
	m.lastBoxID = blindBoxID
	m.lastUserInCt = middleware.UserFromContext(c)
	if m.drawFunc != nil {
		return m.drawFunc(c, userID, blindBoxID)
	}
	return &userSvc.DrawResult{OrderID: 1, OrderNo: "NO1", ActualPrice: 99}, nil
}

var _ userSvc.DrawService = (*mockDrawService)(nil)

// doDraw 构造 POST /user/blindbox/draw 请求。userID > 0 时预置 UserAuth 写入的
// context 值（模拟中间件已完成鉴权）。
func doDraw(t *testing.T, h *DrawHandler, userID int64, body any) (int, map[string]any) {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/user/blindbox/draw", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	gin.SetMode(gin.TestMode)
	r := gin.New()
	// 模拟 UserAuth 的 context 写入（不走真实 token 校验）。
	r.POST("/user/blindbox/draw", func(c *gin.Context) {
		if userID > 0 {
			u := &model.User{ID: userID, Username: "alice", Status: 1}
			c.Set("user.current", u)
		}
		c.Next()
	}, h.Draw)
	r.ServeHTTP(w, req)

	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	return w.Code, resp
}

func TestDraw_HappyPath(t *testing.T) {
	svc := &mockDrawService{
		drawFunc: func(c *gin.Context, userID, blindBoxID int64) (*userSvc.DrawResult, error) {
			if userID != 42 {
				t.Errorf("userID = %d, want 42", userID)
			}
			if blindBoxID != 7 {
				t.Errorf("blindBoxID = %d, want 7", blindBoxID)
			}
			return &userSvc.DrawResult{
				OrderID:     100,
				OrderNo:     "D202610020001",
				ActualPrice: 79,
				Cards: []userSvc.DrawnCard{
					{ItemID: 9, CardID: 5, Rarity: mall.RaritySSR, SnapshotName: "勒布朗 签名卡", SnapshotImage: "https://cdn/x.jpg"},
				},
			}, nil
		},
	}
	h := NewDrawHandler(svc)

	code, resp := doDraw(t, h, 42, map[string]any{"blind_box_id": 7})

	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%v", code, resp)
	}
	if svc.drawCalls != 1 || svc.lastUserID != 42 || svc.lastBoxID != 7 {
		t.Errorf("svc calls = %d, uid=%d, box=%d", svc.drawCalls, svc.lastUserID, svc.lastBoxID)
	}
	if svc.lastUserInCt == nil || svc.lastUserInCt.ID != 42 {
		t.Errorf("UserFromContext not visible to service: %v", svc.lastUserInCt)
	}
	if resp["order_id"] != float64(100) || resp["order_no"] != "D202610020001" || resp["actual_price"] != float64(79) {
		t.Errorf("resp = %v", resp)
	}
	cards, ok := resp["cards"].([]any)
	if !ok || len(cards) != 1 {
		t.Fatalf("cards missing: %v", resp["cards"])
	}
	card := cards[0].(map[string]any)
	if card["card_id"] != float64(5) || card["rarity"] != "SSR" || card["snapshot_name"] != "勒布朗 签名卡" {
		t.Errorf("card = %v", card)
	}
}

func TestDraw_NoUserInContext_401(t *testing.T) {
	svc := &mockDrawService{}
	h := NewDrawHandler(svc)

	code, body := doDraw(t, h, 0, map[string]any{"blind_box_id": 7})

	if code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", code)
	}
	if body["code"] != "draw.unauthorized" {
		t.Errorf("code = %v, want draw.unauthorized", body["code"])
	}
	if svc.drawCalls != 0 {
		t.Errorf("Draw should NOT be called without user")
	}
}

func TestDraw_InvalidBody_400(t *testing.T) {
	svc := &mockDrawService{}
	h := NewDrawHandler(svc)

	// 缺 blind_box_id。
	code, _ := doDraw(t, h, 42, map[string]any{})
	if code != http.StatusBadRequest {
		t.Errorf("missing field: status = %d, want 400", code)
	}
	// 非正数。
	code, _ = doDraw(t, h, 42, map[string]any{"blind_box_id": 0})
	if code != http.StatusBadRequest {
		t.Errorf("zero id: status = %d, want 400", code)
	}
	if svc.drawCalls != 0 {
		t.Errorf("Draw should NOT be called on invalid input")
	}
}

func TestDraw_ErrorMapping(t *testing.T) {
	cases := []struct {
		name     string
		err      error
		wantCode int
		wantStr  string
	}{
		{"sold_out", userSvc.ErrSoldOut, http.StatusConflict, "draw.sold_out"},
		{"insufficient_balance", userSvc.ErrInsufficientBalance, http.StatusPaymentRequired, "draw.insufficient_balance"},
		{"blindbox_not_available", userSvc.ErrBlindBoxNotAvailable, http.StatusNotFound, "draw.blindbox_not_available"},
		{"user_not_available", userSvc.ErrUserNotAvailable, http.StatusForbidden, "draw.user_not_available"},
		{"internal", errors.New("db: down"), http.StatusInternalServerError, "draw.internal"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := &mockDrawService{
				drawFunc: func(c *gin.Context, userID, blindBoxID int64) (*userSvc.DrawResult, error) {
					return nil, tc.err
				},
			}
			h := NewDrawHandler(svc)
			code, body := doDraw(t, h, 42, map[string]any{"blind_box_id": 7})
			if code != tc.wantCode {
				t.Errorf("status = %d, want %d", code, tc.wantCode)
			}
			if body["code"] != tc.wantStr {
				t.Errorf("code = %v, want %v", body["code"], tc.wantStr)
			}
		})
	}
}

// TestDraw_ErrPoolNotFound_Internal 数据异常（卡池缺失）映射 500，而非业务码。
func TestDraw_ErrPoolNotFound_Internal(t *testing.T) {
	svc := &mockDrawService{
		drawFunc: func(c *gin.Context, userID, blindBoxID int64) (*userSvc.DrawResult, error) {
			return nil, userSvc.ErrPoolNotFound
		},
	}
	h := NewDrawHandler(svc)
	code, body := doDraw(t, h, 42, map[string]any{"blind_box_id": 7})
	if code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", code)
	}
	if body["code"] != "draw.internal" {
		t.Errorf("code = %v, want draw.internal", body["code"])
	}
}
