// Package supplier — seckill_test.go 覆盖供应商秒杀管理 handler
// （List / Create / Edit / Toggle + sentinel → HTTP 错误码映射）。
package supplier

import (
	"errors"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"

	"ai-go-mall/internal/model/mall"
	"ai-go-mall/internal/repository"
	supplierSvc "ai-go-mall/internal/service/supplier"
)

// mockSeckillService 实现 supplierSvc.SeckillService。
type mockSeckillService struct {
	listFunc   func(c *gin.Context, supplierID int64, opts repository.ListOptions) ([]mall.MallSeckillActivity, int64, error)
	createFunc func(c *gin.Context, supplierID int64, s *mall.MallSeckillActivity) (*mall.MallSeckillActivity, error)
	updateFunc func(c *gin.Context, supplierID, seckillID int64, patch supplierSvc.SeckillPatch) error
	toggleFunc func(c *gin.Context, supplierID, seckillID int64) error

	lastSupplierID int64
	lastSeckill    *mall.MallSeckillActivity
	lastUpdateID   int64
	lastPatch      supplierSvc.SeckillPatch
	lastTargetID   int64
}

func (m *mockSeckillService) List(c *gin.Context, supplierID int64, opts repository.ListOptions) ([]mall.MallSeckillActivity, int64, error) {
	if m.listFunc != nil {
		return m.listFunc(c, supplierID, opts)
	}
	return []mall.MallSeckillActivity{}, 0, nil
}

func (m *mockSeckillService) Create(c *gin.Context, supplierID int64, s *mall.MallSeckillActivity) (*mall.MallSeckillActivity, error) {
	m.lastSupplierID = supplierID
	m.lastSeckill = s
	if m.createFunc != nil {
		return m.createFunc(c, supplierID, s)
	}
	s.ID = 66
	return s, nil
}

func (m *mockSeckillService) Update(c *gin.Context, supplierID, seckillID int64, patch supplierSvc.SeckillPatch) error {
	m.lastSupplierID = supplierID
	m.lastUpdateID = seckillID
	m.lastPatch = patch
	if m.updateFunc != nil {
		return m.updateFunc(c, supplierID, seckillID, patch)
	}
	return nil
}

func (m *mockSeckillService) Toggle(c *gin.Context, supplierID, seckillID int64) error {
	m.lastSupplierID = supplierID
	m.lastTargetID = seckillID
	if m.toggleFunc != nil {
		return m.toggleFunc(c, supplierID, seckillID)
	}
	return nil
}

var _ supplierSvc.SeckillService = (*mockSeckillService)(nil)

func TestSeckillList_HappyPath(t *testing.T) {
	svc := &mockSeckillService{
		listFunc: func(c *gin.Context, supplierID int64, opts repository.ListOptions) ([]mall.MallSeckillActivity, int64, error) {
			if supplierID != 10 {
				t.Errorf("supplierID = %d, want 10", supplierID)
			}
			return []mall.MallSeckillActivity{{ID: 66, SupplierID: supplierID, BlindBoxID: 7}}, 1, nil
		},
	}
	h := NewSeckillHandler(svc)

	code, resp := doSupplierJSON(t, http.MethodGet, "/supplier/seckill/list", nil, func(r *gin.Engine) {
		r.GET("/supplier/seckill/list", withSupplier(10), h.List)
	})

	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%v", code, resp)
	}
	if resp["total"] != float64(1) {
		t.Errorf("total = %v", resp["total"])
	}
	items, ok := resp["items"].([]any)
	if !ok || len(items) != 1 {
		t.Fatalf("items = %v", resp["items"])
	}
}

func TestSeckillList_Unauthorized(t *testing.T) {
	h := NewSeckillHandler(&mockSeckillService{})

	code, body := doSupplierJSON(t, http.MethodGet, "/supplier/seckill/list", nil, func(r *gin.Engine) {
		r.GET("/supplier/seckill/list", withSupplier(0), h.List)
	})

	if code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", code)
	}
	if body["code"] != "seckill.unauthorized" {
		t.Errorf("code = %v, want seckill.unauthorized", body["code"])
	}
}

func TestSeckillCreate_HappyPath(t *testing.T) {
	svc := &mockSeckillService{
		createFunc: func(c *gin.Context, supplierID int64, s *mall.MallSeckillActivity) (*mall.MallSeckillActivity, error) {
			if supplierID != 10 {
				t.Errorf("supplierID = %d, want 10", supplierID)
			}
			if s.BlindBoxID != 7 || s.SeckillPrice != 79 || s.TotalStock != 100 || s.PerUserLimit != 1 {
				t.Errorf("seckill = %+v", s)
			}
			if s.StartAt.IsZero() || s.EndAt.IsZero() {
				t.Errorf("time window not parsed: %+v", s)
			}
			s.ID = 66
			return s, nil
		},
	}
	h := NewSeckillHandler(svc)

	code, resp := doSupplierJSON(t, http.MethodPost, "/supplier/seckill/create", map[string]any{
		"blind_box_id":   7,
		"seckill_price":  79,
		"total_stock":    100,
		"per_user_limit": 1,
		"start_at":       "2026-10-01T00:00:00Z",
		"end_at":         "2026-10-08T00:00:00Z",
	}, func(r *gin.Engine) {
		r.POST("/supplier/seckill/create", withSupplier(10), h.Create)
	})

	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%v", code, resp)
	}
	if resp["id"] != float64(66) {
		t.Errorf("id = %v", resp["id"])
	}
}

func TestSeckillCreate_InvalidInput_400(t *testing.T) {
	h := NewSeckillHandler(&mockSeckillService{})

	// 缺必填字段。
	code, body := doSupplierJSON(t, http.MethodPost, "/supplier/seckill/create", map[string]any{
		"blind_box_id": 7,
	}, func(r *gin.Engine) {
		r.POST("/supplier/seckill/create", withSupplier(10), h.Create)
	})

	if code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", code)
	}
	if body["code"] != "seckill.invalid_input" {
		t.Errorf("code = %v", body["code"])
	}
}

func TestSeckillEdit_AppliesPatch(t *testing.T) {
	svc := &mockSeckillService{}
	h := NewSeckillHandler(svc)

	price := 69.9
	limit := 3
	code, _ := doSupplierJSON(t, http.MethodPost, "/supplier/seckill/edit", map[string]any{
		"id":             66,
		"seckill_price":  price,
		"per_user_limit": limit,
	}, func(r *gin.Engine) {
		r.POST("/supplier/seckill/edit", withSupplier(10), h.Edit)
	})

	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200", code)
	}
	if svc.lastUpdateID != 66 || svc.lastSupplierID != 10 {
		t.Errorf("update id = %d, supplier = %d", svc.lastUpdateID, svc.lastSupplierID)
	}
	if svc.lastPatch.SeckillPrice == nil || *svc.lastPatch.SeckillPrice != price {
		t.Errorf("patch price = %v", svc.lastPatch.SeckillPrice)
	}
	if svc.lastPatch.PerUserLimit == nil || *svc.lastPatch.PerUserLimit != limit {
		t.Errorf("patch limit = %v", svc.lastPatch.PerUserLimit)
	}
}

func TestSeckillToggle(t *testing.T) {
	svc := &mockSeckillService{}
	h := NewSeckillHandler(svc)

	code, _ := doSupplierJSON(t, http.MethodPost, "/supplier/seckill/toggle", map[string]any{"id": 66}, func(r *gin.Engine) {
		r.POST("/supplier/seckill/toggle", withSupplier(10), h.Toggle)
	})

	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200", code)
	}
	if svc.lastTargetID != 66 || svc.lastSupplierID != 10 {
		t.Errorf("toggle id = %d, supplier = %d", svc.lastTargetID, svc.lastSupplierID)
	}
}

func TestSeckillErrorMapping(t *testing.T) {
	cases := []struct {
		name     string
		err      error
		wantCode int
		wantStr  string
	}{
		{"not_found", supplierSvc.ErrSeckillNotFound, http.StatusNotFound, "seckill.not_found"},
		{"forbidden", supplierSvc.ErrSeckillForbidden, http.StatusForbidden, "seckill.forbidden"},
		{"stock_exceeds_pool", supplierSvc.ErrSeckillStockExceedsPool, http.StatusUnprocessableEntity, "seckill.stock_exceeds_pool"},
		{"invalid_time_window", supplierSvc.ErrInvalidSeckillWindow, http.StatusUnprocessableEntity, "seckill.invalid_time_window"},
		{"invalid_price", supplierSvc.ErrInvalidSeckillPrice, http.StatusUnprocessableEntity, "seckill.invalid_price"},
		{"redis_init_failed", supplierSvc.ErrRedisInitFailed, http.StatusInternalServerError, "seckill.redis_init_failed"},
		{"internal", errors.New("db down"), http.StatusInternalServerError, "seckill.internal"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := &mockSeckillService{
				createFunc: func(c *gin.Context, supplierID int64, s *mall.MallSeckillActivity) (*mall.MallSeckillActivity, error) {
					return nil, tc.err
				},
			}
			h := NewSeckillHandler(svc)
			code, body := doSupplierJSON(t, http.MethodPost, "/supplier/seckill/create", map[string]any{
				"blind_box_id":   7,
				"seckill_price":  79,
				"total_stock":    100,
				"per_user_limit": 1,
				"start_at":       "2026-10-01T00:00:00Z",
				"end_at":         "2026-10-08T00:00:00Z",
			}, func(r *gin.Engine) {
				r.POST("/supplier/seckill/create", withSupplier(10), h.Create)
			})
			if code != tc.wantCode {
				t.Errorf("status = %d, want %d", code, tc.wantCode)
			}
			if body["code"] != tc.wantStr {
				t.Errorf("code = %v, want %v", body["code"], tc.wantStr)
			}
		})
	}
}
