package user

import (
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

// mockSeckillService 实现 userSvc.SeckillService。
type mockSeckillService struct {
	drawFunc func(c *gin.Context, userID, seckillID int64) (*userSvc.DrawResult, error)

	drawCalls        int
	lastUserID       int64
	lastSeckillID    int64
	lastUserInCtx    *model.User
}

func (m *mockSeckillService) DrawSeckill(c *gin.Context, userID, seckillID int64) (*userSvc.DrawResult, error) {
	m.drawCalls++
	m.lastUserID = userID
	m.lastSeckillID = seckillID
	m.lastUserInCtx = middleware.UserFromContext(c)
	if m.drawFunc != nil {
		return m.drawFunc(c, userID, seckillID)
	}
	return &userSvc.DrawResult{OrderID: 1, OrderNo: "NO1", ActualPrice: 79}, nil
}

var _ userSvc.SeckillService = (*mockSeckillService)(nil)

// doSeckillDraw 构造 POST /user/seckill/:id/draw 请求。
//
// userID > 0 时预置 UserAuth 写入的 context 值（模拟中间件已完成鉴权）。
// seckillIDStr 写到 path param :id。
func doSeckillDraw(t *testing.T, h *SeckillHandler, userID int64, seckillIDStr string) (int, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/user/seckill/"+seckillIDStr+"/draw", nil)
	w := httptest.NewRecorder()

	gin.SetMode(gin.TestMode)
	r := gin.New()
	// 模拟 UserAuth 的 context 写入（不走真实 token 校验）。
	r.POST("/user/seckill/:id/draw", func(c *gin.Context) {
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

func TestSeckillDraw_HappyPath(t *testing.T) {
	svc := &mockSeckillService{
		drawFunc: func(c *gin.Context, userID, seckillID int64) (*userSvc.DrawResult, error) {
			if userID != 42 {
				t.Errorf("userID = %d, want 42", userID)
			}
			if seckillID != 7 {
				t.Errorf("seckillID = %d, want 7", seckillID)
			}
			return &userSvc.DrawResult{
				OrderID:     200,
				OrderNo:     "S202610020001",
				ActualPrice: 79,
				Cards: []userSvc.DrawnCard{
					{ItemID: 9, CardID: 5, Rarity: mall.RaritySSR, SnapshotName: "勒布朗 签名卡", SnapshotImage: "https://cdn/x.jpg"},
				},
			}, nil
		},
	}
	h := NewSeckillHandler(svc, nil)

	code, resp := doSeckillDraw(t, h, 42, "7")

	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%v", code, resp)
	}
	if svc.drawCalls != 1 || svc.lastUserID != 42 || svc.lastSeckillID != 7 {
		t.Errorf("svc calls = %d, uid=%d, sid=%d", svc.drawCalls, svc.lastUserID, svc.lastSeckillID)
	}
	if svc.lastUserInCtx == nil || svc.lastUserInCtx.ID != 42 {
		t.Errorf("UserFromContext not visible to service: %v", svc.lastUserInCtx)
	}
	if resp["order_id"] != float64(200) || resp["order_no"] != "S202610020001" || resp["actual_price"] != float64(79) {
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

func TestSeckillDraw_NoUserInContext_401(t *testing.T) {
	svc := &mockSeckillService{}
	h := NewSeckillHandler(svc, nil)

	code, body := doSeckillDraw(t, h, 0, "7")

	if code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", code)
	}
	if body["code"] != "seckill.unauthorized" {
		t.Errorf("code = %v, want seckill.unauthorized", body["code"])
	}
	if svc.drawCalls != 0 {
		t.Errorf("DrawSeckill should NOT be called without user")
	}
}

func TestSeckillDraw_InvalidID_400(t *testing.T) {
	svc := &mockSeckillService{}
	h := NewSeckillHandler(svc, nil)

	// 非数字。
	code, body := doSeckillDraw(t, h, 42, "abc")
	if code != http.StatusBadRequest {
		t.Errorf("non-numeric: status = %d, want 400", code)
	}
	if body["code"] != "seckill.invalid_input" {
		t.Errorf("code = %v, want seckill.invalid_input", body["code"])
	}
	// 零。
	code, _ = doSeckillDraw(t, h, 42, "0")
	if code != http.StatusBadRequest {
		t.Errorf("zero id: status = %d, want 400", code)
	}
	// 负数。
	code, _ = doSeckillDraw(t, h, 42, "-1")
	if code != http.StatusBadRequest {
		t.Errorf("negative id: status = %d, want 400", code)
	}
	if svc.drawCalls != 0 {
		t.Errorf("DrawSeckill should NOT be called on invalid input")
	}
}

func TestSeckillDraw_ErrorMapping(t *testing.T) {
	cases := []struct {
		name     string
		err      error
		wantCode int
		wantStr  string
	}{
		{"user_limit_exceeded", userSvc.ErrUserLimitExceeded, http.StatusTooManyRequests, "seckill.user_limit_exceeded"},
		{"sold_out", userSvc.ErrSoldOut, http.StatusConflict, "seckill.sold_out"},
		{"insufficient_balance", userSvc.ErrInsufficientBalance, http.StatusPaymentRequired, "seckill.insufficient_balance"},
		{"not_in_window", userSvc.ErrSeckillNotInWindow, http.StatusForbidden, "seckill.not_in_window"},
		{"seckill_not_available", userSvc.ErrSeckillNotAvailable, http.StatusNotFound, "seckill.not_available"},
		{"blindbox_not_available", userSvc.ErrBlindBoxNotAvailable, http.StatusNotFound, "seckill.blindbox_not_available"},
		{"user_not_available", userSvc.ErrUserNotAvailable, http.StatusForbidden, "seckill.user_not_available"},
		{"internal", errors.New("db: down"), http.StatusInternalServerError, "seckill.internal"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := &mockSeckillService{
				drawFunc: func(c *gin.Context, userID, seckillID int64) (*userSvc.DrawResult, error) {
					return nil, tc.err
				},
			}
			h := NewSeckillHandler(svc, nil)
			code, body := doSeckillDraw(t, h, 42, "7")
			if code != tc.wantCode {
				t.Errorf("status = %d, want %d", code, tc.wantCode)
			}
			if body["code"] != tc.wantStr {
				t.Errorf("code = %v, want %v", body["code"], tc.wantStr)
			}
		})
	}
}