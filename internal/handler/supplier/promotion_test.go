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

// mockPromotionService 实现 supplierSvc.PromotionService。
type mockPromotionService struct {
	listFunc   func(c *gin.Context, supplierID int64, opts repository.ListOptions) ([]mall.MallPromotion, int64, error)
	createFunc func(c *gin.Context, supplierID int64, p *mall.MallPromotion) (*mall.MallPromotion, error)
	updateFunc func(c *gin.Context, supplierID, promotionID int64, patch supplierSvc.PromotionPatch) error
	toggleFunc func(c *gin.Context, supplierID, promotionID int64) error
	deleteFunc func(c *gin.Context, supplierID, promotionID int64) error

	lastSupplierID int64
	lastPromotion  *mall.MallPromotion
	lastTargetID   int64
	lastUpdateID   int64
	lastPatch      supplierSvc.PromotionPatch
}

func (m *mockPromotionService) List(c *gin.Context, supplierID int64, opts repository.ListOptions) ([]mall.MallPromotion, int64, error) {
	if m.listFunc != nil {
		return m.listFunc(c, supplierID, opts)
	}
	return []mall.MallPromotion{}, 0, nil
}

func (m *mockPromotionService) Create(c *gin.Context, supplierID int64, p *mall.MallPromotion) (*mall.MallPromotion, error) {
	m.lastSupplierID = supplierID
	m.lastPromotion = p
	if m.createFunc != nil {
		return m.createFunc(c, supplierID, p)
	}
	p.ID = 88
	return p, nil
}

func (m *mockPromotionService) Update(c *gin.Context, supplierID, promotionID int64, patch supplierSvc.PromotionPatch) error {
	m.lastSupplierID = supplierID
	m.lastUpdateID = promotionID
	m.lastPatch = patch
	if m.updateFunc != nil {
		return m.updateFunc(c, supplierID, promotionID, patch)
	}
	return nil
}

func (m *mockPromotionService) Toggle(c *gin.Context, supplierID, promotionID int64) error {
	m.lastSupplierID = supplierID
	m.lastTargetID = promotionID
	if m.toggleFunc != nil {
		return m.toggleFunc(c, supplierID, promotionID)
	}
	return nil
}

func (m *mockPromotionService) Delete(c *gin.Context, supplierID, promotionID int64) error {
	m.lastSupplierID = supplierID
	m.lastTargetID = promotionID
	if m.deleteFunc != nil {
		return m.deleteFunc(c, supplierID, promotionID)
	}
	return nil
}

var _ supplierSvc.PromotionService = (*mockPromotionService)(nil)

func TestPromotionCreate_HappyPath(t *testing.T) {
	svc := &mockPromotionService{
		createFunc: func(c *gin.Context, supplierID int64, p *mall.MallPromotion) (*mall.MallPromotion, error) {
			if supplierID != 10 {
				t.Errorf("supplierID = %d, want 10", supplierID)
			}
			// SupplierID 必须被 handler 强制为 context 身份，防越权赋值。
			if p.SupplierID != 10 {
				t.Errorf("promo.SupplierID = %d, want forced 10", p.SupplierID)
			}
			if p.BlindBoxID != 7 || p.PromoPrice != 79 || p.OriginalPrice != 99 {
				t.Errorf("promo = %+v", p)
			}
			if p.StartAt.IsZero() || p.EndAt.IsZero() {
				t.Errorf("time window not parsed: %+v", p)
			}
			p.ID = 88
			return p, nil
		},
	}
	h := NewPromotionHandler(svc)

	code, resp := doSupplierJSON(t, http.MethodPost, "/supplier/promotions/create", map[string]any{
		"blind_box_id":   7,
		"original_price": 99,
		"promo_price":    79,
		"start_at":       "2026-10-01T00:00:00Z",
		"end_at":         "2026-10-08T00:00:00Z",
	}, func(r *gin.Engine) {
		r.POST("/supplier/promotions/create", withSupplier(10), h.Create)
	})

	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%v", code, resp)
	}
	if resp["id"] != float64(88) {
		t.Errorf("id = %v", resp["id"])
	}
}

func TestPromotionCreate_ClientSupplierIDIgnored(t *testing.T) {
	svc := &mockPromotionService{
		createFunc: func(c *gin.Context, supplierID int64, p *mall.MallPromotion) (*mall.MallPromotion, error) {
			// 即使 body 里伪造 supplier_id=999，也必须被覆盖为 context 身份 10。
			if p.SupplierID != 10 {
				t.Errorf("promo.SupplierID = %d, want 10 (forced)", p.SupplierID)
			}
			return p, nil
		},
	}
	h := NewPromotionHandler(svc)

	code, _ := doSupplierJSON(t, http.MethodPost, "/supplier/promotions/create", map[string]any{
		"blind_box_id": 7,
		"supplier_id":  999,
		"start_at":     "2026-10-01T00:00:00Z",
		"end_at":       "2026-10-08T00:00:00Z",
	}, func(r *gin.Engine) {
		r.POST("/supplier/promotions/create", withSupplier(10), h.Create)
	})
	if code != http.StatusOK {
		t.Fatalf("status = %d", code)
	}
}

func TestPromotionCreate_InvalidTimeFormat_400(t *testing.T) {
	svc := &mockPromotionService{}
	h := NewPromotionHandler(svc)

	code, _ := doSupplierJSON(t, http.MethodPost, "/supplier/promotions/create", map[string]any{
		"blind_box_id": 7,
		"start_at":     "not-a-time",
		"end_at":       "2026-10-08T00:00:00Z",
	}, func(r *gin.Engine) {
		r.POST("/supplier/promotions/create", withSupplier(10), h.Create)
	})
	if code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", code)
	}
	if svc.lastSupplierID != 0 {
		t.Errorf("Create should NOT be called")
	}
}

func TestPromotionCreate_ServiceErrors(t *testing.T) {
	cases := []struct {
		name     string
		err      error
		wantCode int
		wantStr  string
	}{
		{"time_overlap", supplierSvc.ErrPromotionTimeOverlap, http.StatusUnprocessableEntity, "promotion.time_overlap"},
		{"invalid_time_window", supplierSvc.ErrInvalidTimeWindow, http.StatusUnprocessableEntity, "promotion.invalid_time_window"},
		{"invalid_price", supplierSvc.ErrInvalidPrice, http.StatusUnprocessableEntity, "promotion.invalid_price"},
		{"forbidden", supplierSvc.ErrPromotionForbidden, http.StatusForbidden, "promotion.forbidden"},
		{"internal", errors.New("db: down"), http.StatusInternalServerError, "promotion.internal"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := &mockPromotionService{
				createFunc: func(c *gin.Context, supplierID int64, p *mall.MallPromotion) (*mall.MallPromotion, error) {
					return nil, tc.err
				},
			}
			h := NewPromotionHandler(svc)
			code, body := doSupplierJSON(t, http.MethodPost, "/supplier/promotions/create", map[string]any{
				"blind_box_id": 7,
				"start_at":     "2026-10-01T00:00:00Z",
				"end_at":       "2026-10-08T00:00:00Z",
			}, func(r *gin.Engine) {
				r.POST("/supplier/promotions/create", withSupplier(10), h.Create)
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

func TestPromotionList_HappyPath(t *testing.T) {
	svc := &mockPromotionService{
		listFunc: func(c *gin.Context, supplierID int64, opts repository.ListOptions) ([]mall.MallPromotion, int64, error) {
			if supplierID != 10 {
				t.Errorf("supplierID = %d", supplierID)
			}
			return []mall.MallPromotion{{ID: 1, PromoPrice: 79}}, 1, nil
		},
	}
	h := NewPromotionHandler(svc)

	code, resp := doSupplierJSON(t, http.MethodGet, "/supplier/promotions/list", nil, func(r *gin.Engine) {
		r.GET("/supplier/promotions/list", withSupplier(10), h.List)
	})
	if code != http.StatusOK {
		t.Fatalf("status = %d", code)
	}
	if resp["total"] != float64(1) {
		t.Errorf("total = %v", resp["total"])
	}
}

func TestPromotionEdit_HappyPath(t *testing.T) {
	svc := &mockPromotionService{
		updateFunc: func(c *gin.Context, supplierID, promotionID int64, patch supplierSvc.PromotionPatch) error {
			if supplierID != 10 || promotionID != 8 {
				t.Errorf("supplierID=%d promotionID=%d, want 10/8", supplierID, promotionID)
			}
			if patch.PromoPrice == nil || *patch.PromoPrice != 69 {
				t.Errorf("patch.PromoPrice = %v, want 69", patch.PromoPrice)
			}
			if patch.OriginalPrice == nil || *patch.OriginalPrice != 99 {
				t.Errorf("patch.OriginalPrice = %v, want 99", patch.OriginalPrice)
			}
			return nil
		},
	}
	h := NewPromotionHandler(svc)

	code, _ := doSupplierJSON(t, http.MethodPost, "/supplier/promotions/edit", map[string]any{
		"id": 8, "promo_price": 69, "original_price": 99,
	}, func(r *gin.Engine) {
		r.POST("/supplier/promotions/edit", withSupplier(10), h.Edit)
	})
	if code != http.StatusOK {
		t.Fatalf("status = %d", code)
	}
}

func TestPromotionToggle_HappyPath(t *testing.T) {
	svc := &mockPromotionService{}
	h := NewPromotionHandler(svc)

	code, _ := doSupplierJSON(t, http.MethodPost, "/supplier/promotions/toggle", map[string]any{
		"id": 8,
	}, func(r *gin.Engine) {
		r.POST("/supplier/promotions/toggle", withSupplier(10), h.Toggle)
	})
	if code != http.StatusOK {
		t.Fatalf("status = %d", code)
	}
	if svc.lastTargetID != 8 || svc.lastSupplierID != 10 {
		t.Errorf("toggle args = %d/%d", svc.lastTargetID, svc.lastSupplierID)
	}
}

func TestPromotionDelete_HappyPath(t *testing.T) {
	svc := &mockPromotionService{}
	h := NewPromotionHandler(svc)

	code, _ := doSupplierJSON(t, http.MethodPost, "/supplier/promotions/delete", map[string]any{
		"id": 8,
	}, func(r *gin.Engine) {
		r.POST("/supplier/promotions/delete", withSupplier(10), h.Delete)
	})
	if code != http.StatusOK {
		t.Fatalf("status = %d", code)
	}
	if svc.lastTargetID != 8 {
		t.Errorf("delete target = %d", svc.lastTargetID)
	}
}

func TestPromotionToggle_NotFound_404(t *testing.T) {
	svc := &mockPromotionService{
		toggleFunc: func(c *gin.Context, supplierID, promotionID int64) error {
			return supplierSvc.ErrPromotionNotFound
		},
	}
	h := NewPromotionHandler(svc)

	code, body := doSupplierJSON(t, http.MethodPost, "/supplier/promotions/toggle", map[string]any{
		"id": 999,
	}, func(r *gin.Engine) {
		r.POST("/supplier/promotions/toggle", withSupplier(10), h.Toggle)
	})
	if code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", code)
	}
	if body["code"] != "promotion.not_found" {
		t.Errorf("code = %v", body["code"])
	}
}
