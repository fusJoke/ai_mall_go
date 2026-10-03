package user

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	userSvc "ai-go-mall/internal/service/user"
)

// mockFeedService 实现 userSvc.FeedService。
type mockFeedService struct {
	feedFunc func(ctx context.Context) (*userSvc.Feed, error)

	feedCalls int
}

func (m *mockFeedService) Feed(ctx context.Context) (*userSvc.Feed, error) {
	m.feedCalls++
	if m.feedFunc != nil {
		return m.feedFunc(ctx)
	}
	return &userSvc.Feed{
		BlindBoxes: []userSvc.BlindBoxFeedItem{},
		Suppliers:  []userSvc.SupplierFeedItem{},
		FetchedAt:  time.Now(),
	}, nil
}

var _ userSvc.FeedService = (*mockFeedService)(nil)

// doFeedGET 构造 GET /user/home/feed 请求。
func doFeedGET(t *testing.T, h *HomeHandler) (int, map[string]any) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/user/home/feed", h.Feed)
	req := httptest.NewRequest(http.MethodGet, "/user/home/feed", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	return w.Code, resp
}

func TestFeed_HappyPath(t *testing.T) {
	svc := &mockFeedService{
		feedFunc: func(ctx context.Context) (*userSvc.Feed, error) {
			return &userSvc.Feed{
				BlindBoxes: []userSvc.BlindBoxFeedItem{
					{ID: 1, Name: "NBA 盲盒", Price: 99, HotScore: 95},
				},
				Suppliers: []userSvc.SupplierFeedItem{
					{ID: 10, Name: "Panini 旗舰店", BlindBoxCount: 3},
				},
				FetchedAt: time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC),
			}, nil
		},
	}
	h := NewHomeHandler(svc)

	code, resp := doFeedGET(t, h)

	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200", code)
	}
	if svc.feedCalls != 1 {
		t.Errorf("Feed called %d times, want 1", svc.feedCalls)
	}
	bbs, ok := resp["blind_boxes"].([]any)
	if !ok || len(bbs) != 1 {
		t.Fatalf("blind_boxes missing: %v", resp["blind_boxes"])
	}
	bb := bbs[0].(map[string]any)
	if bb["name"] != "NBA 盲盒" || bb["hot_score"] != float64(95) {
		t.Errorf("blind box item = %v", bb)
	}
	sups, ok := resp["suppliers"].([]any)
	if !ok || len(sups) != 1 {
		t.Fatalf("suppliers missing: %v", resp["suppliers"])
	}
	if sups[0].(map[string]any)["name"] != "Panini 旗舰店" {
		t.Errorf("supplier item = %v", sups[0])
	}
}

// TestFeed_EmptyArrays ES 返回 0 条时 JSON 必须是 []（非 null），前端免判空。
func TestFeed_EmptyArrays(t *testing.T) {
	h := NewHomeHandler(&mockFeedService{})

	code, resp := doFeedGET(t, h)
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200", code)
	}
	if arr, ok := resp["blind_boxes"].([]any); !ok || len(arr) != 0 {
		t.Errorf("blind_boxes = %v, want []", resp["blind_boxes"])
	}
	if arr, ok := resp["suppliers"].([]any); !ok || len(arr) != 0 {
		t.Errorf("suppliers = %v, want []", resp["suppliers"])
	}
}

// TestFeed_SearchUnavailable ES 不可用 → 500 + code=search_unavailable（spec 不 fallback）。
func TestFeed_SearchUnavailable(t *testing.T) {
	svc := &mockFeedService{
		feedFunc: func(ctx context.Context) (*userSvc.Feed, error) {
			// 模拟 service 层 wrapSearchErr 的包装形态（errors.Join 挂 sentinel）。
			return nil, fmt.Errorf("blind_box index: %w", errors.Join(userSvc.ErrFeedSearchUnavailable, errors.New("timeout")))
		},
	}
	h := NewHomeHandler(svc)

	code, body := doFeedGET(t, h)
	if code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", code)
	}
	if body["code"] != "search_unavailable" {
		t.Errorf("code = %v, want search_unavailable", body["code"])
	}
}

// TestFeed_OtherError_500 非 ES sentinel 的未知错误 → 500 home.internal。
func TestFeed_OtherError_500(t *testing.T) {
	svc := &mockFeedService{
		feedFunc: func(ctx context.Context) (*userSvc.Feed, error) {
			return nil, errors.New("boom")
		},
	}
	h := NewHomeHandler(svc)

	code, body := doFeedGET(t, h)
	if code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", code)
	}
	if body["code"] != "home.internal" {
		t.Errorf("code = %v, want home.internal", body["code"])
	}
}
