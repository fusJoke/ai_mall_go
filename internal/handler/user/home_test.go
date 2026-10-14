package user

import (
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
	feedFunc func(c *gin.Context) (*userSvc.Feed, error)

	feedCalls int
}

func (m *mockFeedService) Feed(c *gin.Context) (*userSvc.Feed, error) {
	m.feedCalls++
	if m.feedFunc != nil {
		return m.feedFunc(c)
	}
	return &userSvc.Feed{
		BlindBoxes: []userSvc.BlindBoxFeedItem{},
		Suppliers:  []userSvc.SupplierFeedItem{},
		FetchedAt:  time.Now(),
		Source:     userSvc.FeedSourceES,
	}, nil
}

var _ userSvc.FeedService = (*mockFeedService)(nil)

// doFeedGET 构造 GET /user/home/feed 请求，返回 (状态码, 响应体, 响应头)。
func doFeedGET(t *testing.T, h *HomeHandler) (int, map[string]any, http.Header) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/user/home/feed", h.Feed)
	req := httptest.NewRequest(http.MethodGet, "/user/home/feed", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	return w.Code, resp, w.Header()
}

func TestFeed_HappyPath(t *testing.T) {
	svc := &mockFeedService{
		feedFunc: func(c *gin.Context) (*userSvc.Feed, error) {
			return &userSvc.Feed{
				BlindBoxes: []userSvc.BlindBoxFeedItem{
					{ID: 1, Name: "NBA 盲盒", Price: 99, HotScore: 95},
				},
				Suppliers: []userSvc.SupplierFeedItem{
					{ID: 10, Name: "Panini 旗舰店", BlindBoxCount: 3},
				},
				FetchedAt: time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC),
				Source:    userSvc.FeedSourceES,
			}, nil
		},
	}
	h := NewHomeHandler(svc)

	code, resp, header := doFeedGET(t, h)

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
	// 主路径响应头标识来源为 es。
	if got := header.Get(userSvc.FeedSourceHeader); got != userSvc.FeedSourceES {
		t.Errorf("%s = %q, want %q", userSvc.FeedSourceHeader, got, userSvc.FeedSourceES)
	}
}

// TestFeed_SourceHeader_Mysql 降级路径响应头标识来源为 mysql。
//
// spec Requirement "降级可观测性" Scenario "来源标识"。
func TestFeed_SourceHeader_Mysql(t *testing.T) {
	svc := &mockFeedService{
		feedFunc: func(c *gin.Context) (*userSvc.Feed, error) {
			return &userSvc.Feed{
				BlindBoxes: []userSvc.BlindBoxFeedItem{},
				Suppliers:  []userSvc.SupplierFeedItem{},
				FetchedAt:  time.Now(),
				Source:     userSvc.FeedSourceMySQL,
			}, nil
		},
	}
	h := NewHomeHandler(svc)

	code, _, header := doFeedGET(t, h)
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200", code)
	}
	if got := header.Get(userSvc.FeedSourceHeader); got != userSvc.FeedSourceMySQL {
		t.Errorf("%s = %q, want %q", userSvc.FeedSourceHeader, got, userSvc.FeedSourceMySQL)
	}
}

// TestFeed_SourceHeader_EmptyNormalizedToES 验证 source 为空时兜底为 es
// （缓存项由 source 字段上线前的版本写入时可能出现空串）。
func TestFeed_SourceHeader_EmptyNormalizedToES(t *testing.T) {
	svc := &mockFeedService{
		feedFunc: func(c *gin.Context) (*userSvc.Feed, error) {
			return &userSvc.Feed{
				BlindBoxes: []userSvc.BlindBoxFeedItem{},
				Suppliers:  []userSvc.SupplierFeedItem{},
				FetchedAt:  time.Now(),
				// Source 留空
			}, nil
		},
	}
	h := NewHomeHandler(svc)

	_, _, header := doFeedGET(t, h)
	if got := header.Get(userSvc.FeedSourceHeader); got != userSvc.FeedSourceES {
		t.Errorf("%s = %q, want %q (blank should fall back to es)", userSvc.FeedSourceHeader, got, userSvc.FeedSourceES)
	}
}

// TestFeed_EmptyArrays ES 返回 0 条时 JSON 必须是 []（非 null），前端免判空。
func TestFeed_EmptyArrays(t *testing.T) {
	h := NewHomeHandler(&mockFeedService{})

	code, resp, _ := doFeedGET(t, h)
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

// TestFeed_SearchUnavailable ES 与 MySQL 降级**均**失败 → 500 + code=search_unavailable。
//
// 单侧 ES 失败不再返回该错误（会先降级）；这里断言的是双层失败的下游契约。
func TestFeed_SearchUnavailable(t *testing.T) {
	svc := &mockFeedService{
		feedFunc: func(c *gin.Context) (*userSvc.Feed, error) {
			// 模拟 service 层 wrapSearchErr 的包装形态（errors.Join 挂 sentinel）。
			return nil, fmt.Errorf("blind_box index: %w", errors.Join(userSvc.ErrFeedSearchUnavailable, errors.New("timeout")))
		},
	}
	h := NewHomeHandler(svc)

	code, body, header := doFeedGET(t, h)
	if code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", code)
	}
	if body["code"] != "search_unavailable" {
		t.Errorf("code = %v, want search_unavailable", body["code"])
	}
	// 失败响应不设来源头（无数据来源可言）。
	if got := header.Get(userSvc.FeedSourceHeader); got != "" {
		t.Errorf("%s = %q, want empty on failure", userSvc.FeedSourceHeader, got)
	}
}

// TestFeed_OtherError_500 非 ES sentinel 的未知错误 → 500 home.internal。
func TestFeed_OtherError_500(t *testing.T) {
	svc := &mockFeedService{
		feedFunc: func(c *gin.Context) (*userSvc.Feed, error) {
			return nil, errors.New("boom")
		},
	}
	h := NewHomeHandler(svc)

	code, body, _ := doFeedGET(t, h)
	if code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", code)
	}
	if body["code"] != "home.internal" {
		t.Errorf("code = %v, want home.internal", body["code"])
	}
}
