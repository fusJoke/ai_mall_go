package admin

import (
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"ai-go-mall/internal/model/mall"
	"ai-go-mall/internal/repository"
	adminSvc "ai-go-mall/internal/service/admin"
)

// mockMallSeckillService 实现 adminSvc.SeckillService。
type mockMallSeckillService struct {
	listFunc   func(c *gin.Context, opts repository.ListOptions) ([]mall.MallSeckillActivity, int64, error)
	createFunc func(c *gin.Context, sec *mall.MallSeckillActivity) (*mall.MallSeckillActivity, error)
	toggleFunc func(c *gin.Context, id int64) error
}

func (m *mockMallSeckillService) List(c *gin.Context, opts repository.ListOptions) ([]mall.MallSeckillActivity, int64, error) {
	if m.listFunc != nil {
		return m.listFunc(c, opts)
	}
	return []mall.MallSeckillActivity{}, 0, nil
}

func (m *mockMallSeckillService) Create(c *gin.Context, sec *mall.MallSeckillActivity) (*mall.MallSeckillActivity, error) {
	if m.createFunc != nil {
		return m.createFunc(c, sec)
	}
	sec.ID = 1
	return sec, nil
}

func (m *mockMallSeckillService) Toggle(c *gin.Context, id int64) error {
	if m.toggleFunc != nil {
		return m.toggleFunc(c, id)
	}
	return nil
}

var _ adminSvc.SeckillService = (*mockMallSeckillService)(nil)

// =============================================================================
// List
// =============================================================================

func TestMallSeckillList_HappyPath(t *testing.T) {
	want := []mall.MallSeckillActivity{
		{ID: 1, SupplierID: 3, BlindBoxID: 5, Status: mall.StatusActive, RedisInitialized: true},
		{ID: 2, SupplierID: 4, BlindBoxID: 6, Status: mall.StatusDisabled},
	}
	svc := &mockMallSeckillService{
		listFunc: func(c *gin.Context, opts repository.ListOptions) ([]mall.MallSeckillActivity, int64, error) {
			if opts.Page != 1 || opts.PageSize != 10 {
				t.Errorf("opts = %+v, want Page=1 PageSize=10", opts)
			}
			return want, 2, nil
		},
	}
	h := NewMallSeckillHandler(svc)

	code, resp := doMallAdminJSON(t, http.MethodGet, "/admin/seckill/list?page=1&page_size=10", nil, func(r *gin.Engine) {
		r.GET("/admin/seckill/list", h.List)
	})

	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%v", code, resp)
	}
	if resp["total"] != float64(2) {
		t.Errorf("total = %v, want 2", resp["total"])
	}
	items, ok := resp["items"].([]any)
	if !ok || len(items) != 2 {
		t.Fatalf("items = %v, want 2 items", resp["items"])
	}
}

func TestMallSeckillList_ServiceError_500(t *testing.T) {
	svc := &mockMallSeckillService{
		listFunc: func(c *gin.Context, opts repository.ListOptions) ([]mall.MallSeckillActivity, int64, error) {
			return nil, 0, errors.New("db: down")
		},
	}
	h := NewMallSeckillHandler(svc)

	code, body := doMallAdminJSON(t, http.MethodGet, "/admin/seckill/list", nil, func(r *gin.Engine) {
		r.GET("/admin/seckill/list", h.List)
	})
	if code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", code)
	}
	if body["code"] != "admin.seckill.list.internal" {
		t.Errorf("code = %v, want admin.seckill.list.internal", body["code"])
	}
}

// =============================================================================
// Create
// =============================================================================

func TestMallSeckillCreate_HappyPath(t *testing.T) {
	var captured *mall.MallSeckillActivity
	svc := &mockMallSeckillService{
		createFunc: func(c *gin.Context, sec *mall.MallSeckillActivity) (*mall.MallSeckillActivity, error) {
			captured = sec
			sec.ID = 100
			return sec, nil
		},
	}
	h := NewMallSeckillHandler(svc)

	body := map[string]any{
		"supplier_id":    3,
		"blind_box_id":   5,
		"seckill_price":  79.0,
		"total_stock":    100,
		"per_user_limit": 1,
		"start_at":       time.Now().Add(time.Hour),
		"end_at":         time.Now().Add(2 * time.Hour),
	}
	code, resp := doMallAdminJSON(t, http.MethodPost, "/admin/seckill", body, func(r *gin.Engine) {
		r.POST("/admin/seckill", h.Create)
	})
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%v", code, resp)
	}
	if captured == nil {
		t.Fatal("svc.Create not called")
	}
	if captured.SupplierID != 3 || captured.BlindBoxID != 5 || captured.SeckillPrice != 79.0 {
		t.Errorf("captured = %+v", captured)
	}
}

func TestMallSeckillCreate_InvalidBody_400(t *testing.T) {
	svc := &mockMallSeckillService{}
	h := NewMallSeckillHandler(svc)

	code, body := doMallAdminJSON(t, http.MethodPost, "/admin/seckill", map[string]any{}, func(r *gin.Engine) {
		r.POST("/admin/seckill", h.Create)
	})
	if code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", code)
	}
	if body["code"] != "admin.seckill.create.invalid_input" {
		t.Errorf("code = %v", body["code"])
	}
}

func TestMallSeckillCreate_ServiceError_500(t *testing.T) {
	svc := &mockMallSeckillService{
		createFunc: func(c *gin.Context, sec *mall.MallSeckillActivity) (*mall.MallSeckillActivity, error) {
			return nil, errors.New("redis init failed")
		},
	}
	h := NewMallSeckillHandler(svc)

	body := map[string]any{
		"supplier_id":    3,
		"blind_box_id":   5,
		"seckill_price":  79.0,
		"total_stock":    100,
		"per_user_limit": 1,
		"start_at":       time.Now().Add(time.Hour),
		"end_at":         time.Now().Add(2 * time.Hour),
	}
	code, resp := doMallAdminJSON(t, http.MethodPost, "/admin/seckill", body, func(r *gin.Engine) {
		r.POST("/admin/seckill", h.Create)
	})
	if code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", code)
	}
	if resp["code"] != "admin.seckill.create.internal" {
		t.Errorf("code = %v", resp["code"])
	}
}

// =============================================================================
// Toggle
// =============================================================================

func TestMallSeckillToggle_HappyPath(t *testing.T) {
	var capturedID int64
	svc := &mockMallSeckillService{
		toggleFunc: func(c *gin.Context, id int64) error {
			capturedID = id
			return nil
		},
	}
	h := NewMallSeckillHandler(svc)

	code, body := doMallAdminJSON(t, http.MethodPost, "/admin/seckill/7/toggle", nil, func(r *gin.Engine) {
		r.POST("/admin/seckill/:id/toggle", h.Toggle)
	})
	if code != http.StatusOK {
		t.Errorf("status = %d, want 200", code)
	}
	if body["code"] != "admin.seckill.toggle.ok" {
		t.Errorf("code = %v", body["code"])
	}
	if capturedID != 7 {
		t.Errorf("capturedID = %d, want 7", capturedID)
	}
}

func TestMallSeckillToggle_InvalidID_400(t *testing.T) {
	svc := &mockMallSeckillService{}
	h := NewMallSeckillHandler(svc)

	cases := []string{"abc", "0", "-1"}
	for _, idStr := range cases {
		code, body := doMallAdminJSON(t, http.MethodPost, "/admin/seckill/"+idStr+"/toggle", nil, func(r *gin.Engine) {
			r.POST("/admin/seckill/:id/toggle", h.Toggle)
		})
		if code != http.StatusBadRequest {
			t.Errorf("%q: status = %d, want 400", idStr, code)
		}
		if body["code"] != "admin.seckill.toggle.invalid_id" {
			t.Errorf("%q: code = %v", idStr, body["code"])
		}
	}
}

func TestMallSeckillToggle_NotFound_404(t *testing.T) {
	svc := &mockMallSeckillService{
		toggleFunc: func(c *gin.Context, id int64) error {
			return adminSvc.ErrSeckillNotFound
		},
	}
	h := NewMallSeckillHandler(svc)

	code, body := doMallAdminJSON(t, http.MethodPost, "/admin/seckill/999/toggle", nil, func(r *gin.Engine) {
		r.POST("/admin/seckill/:id/toggle", h.Toggle)
	})
	if code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", code)
	}
	if body["code"] != "admin.seckill.toggle.not_found" {
		t.Errorf("code = %v", body["code"])
	}
}