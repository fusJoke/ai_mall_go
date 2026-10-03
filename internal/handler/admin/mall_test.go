package admin

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"ai-go-mall/internal/model/mall"
	"ai-go-mall/internal/repository"
	supplierRepo "ai-go-mall/internal/repository/supplier"
	adminSvc "ai-go-mall/internal/service/admin"
)

// =============================================================================
// mocks
// =============================================================================

// mockMallSupplierService 实现 adminSvc.SupplierService。
type mockMallSupplierService struct {
	listFunc   func(c *gin.Context, opts supplierRepo.SupplierListOptions) ([]mall.MallSupplier, int64, error)
	toggleFunc func(c *gin.Context, supplierID int64) error

	lastOpts     supplierRepo.SupplierListOptions
	lastTargetID int64
}

func (m *mockMallSupplierService) List(c *gin.Context, opts supplierRepo.SupplierListOptions) ([]mall.MallSupplier, int64, error) {
	m.lastOpts = opts
	if m.listFunc != nil {
		return m.listFunc(c, opts)
	}
	return []mall.MallSupplier{}, 0, nil
}

func (m *mockMallSupplierService) ToggleStatus(c *gin.Context, supplierID int64) error {
	m.lastTargetID = supplierID
	if m.toggleFunc != nil {
		return m.toggleFunc(c, supplierID)
	}
	return nil
}

func (m *mockMallSupplierService) ToggleFeatured(c *gin.Context, supplierID int64) error {
	m.lastTargetID = supplierID
	return nil
}

var _ adminSvc.SupplierService = (*mockMallSupplierService)(nil)

// mockMallBlindBoxService 实现 adminSvc.BlindBoxService。
type mockMallBlindBoxService struct {
	listFunc     func(c *gin.Context, opts repository.ListOptions) ([]mall.MallBlindBox, int64, error)
	lastTargetID int64
}

func (m *mockMallBlindBoxService) List(c *gin.Context, opts repository.ListOptions) ([]mall.MallBlindBox, int64, error) {
	if m.listFunc != nil {
		return m.listFunc(c, opts)
	}
	return []mall.MallBlindBox{}, 0, nil
}

func (m *mockMallBlindBoxService) ToggleStatus(c *gin.Context, blindBoxID int64) error {
	m.lastTargetID = blindBoxID
	return nil
}

func (m *mockMallBlindBoxService) ToggleOnSale(c *gin.Context, blindBoxID int64) error {
	m.lastTargetID = blindBoxID
	return nil
}

func (m *mockMallBlindBoxService) ToggleFeatured(c *gin.Context, blindBoxID int64) error {
	m.lastTargetID = blindBoxID
	return nil
}

var _ adminSvc.BlindBoxService = (*mockMallBlindBoxService)(nil)

// =============================================================================
// helpers
// =============================================================================

func doMallAdminJSON(t *testing.T, method, path string, body any, register func(r *gin.Engine)) (int, map[string]any) {
	t.Helper()
	var req *http.Request
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		req = httptest.NewRequest(method, path, bytes.NewReader(b))
		req.Header.Set("Content-Type", "application/json")
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	w := httptest.NewRecorder()

	gin.SetMode(gin.TestMode)
	r := gin.New()
	register(r)
	r.ServeHTTP(w, req)

	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	return w.Code, resp
}

// =============================================================================
// MallSupplierHandler
// =============================================================================

func TestMallSupplierList_HappyPath(t *testing.T) {
	svc := &mockMallSupplierService{
		listFunc: func(c *gin.Context, opts supplierRepo.SupplierListOptions) ([]mall.MallSupplier, int64, error) {
			return []mall.MallSupplier{{ID: 1, Name: "Panini"}}, 1, nil
		},
	}
	h := NewMallSupplierHandler(svc)

	code, resp := doMallAdminJSON(t, http.MethodGet, "/admin/supplier/list?page=1&page_size=10&status=active&name=pan", nil, func(r *gin.Engine) {
		r.GET("/admin/supplier/list", h.List)
	})

	if code != http.StatusOK {
		t.Fatalf("status = %d", code)
	}
	if svc.lastOpts.StatusFilter != "active" || svc.lastOpts.NameKeyword != "pan" {
		t.Errorf("filters = %+v", svc.lastOpts)
	}
	if svc.lastOpts.IsFeaturedFilter != nil {
		t.Errorf("is_featured filter should be nil when absent")
	}
	if resp["total"] != float64(1) {
		t.Errorf("total = %v", resp["total"])
	}
}

func TestMallSupplierList_FeaturedFilter(t *testing.T) {
	svc := &mockMallSupplierService{}
	h := NewMallSupplierHandler(svc)

	_, _ = doMallAdminJSON(t, http.MethodGet, "/admin/supplier/list?is_featured=true", nil, func(r *gin.Engine) {
		r.GET("/admin/supplier/list", h.List)
	})
	if svc.lastOpts.IsFeaturedFilter == nil || !*svc.lastOpts.IsFeaturedFilter {
		t.Errorf("is_featured filter = %v, want true", svc.lastOpts.IsFeaturedFilter)
	}

	_, _ = doMallAdminJSON(t, http.MethodGet, "/admin/supplier/list?is_featured=false", nil, func(r *gin.Engine) {
		r.GET("/admin/supplier/list", h.List)
	})
	if svc.lastOpts.IsFeaturedFilter == nil || *svc.lastOpts.IsFeaturedFilter {
		t.Errorf("is_featured filter = %v, want false", svc.lastOpts.IsFeaturedFilter)
	}
}

func TestMallSupplierToggleStatus_HappyPath(t *testing.T) {
	svc := &mockMallSupplierService{}
	h := NewMallSupplierHandler(svc)

	code, body := doMallAdminJSON(t, http.MethodPost, "/admin/supplier/toggle-status", map[string]any{"id": 5}, func(r *gin.Engine) {
		r.POST("/admin/supplier/toggle-status", h.ToggleStatus)
	})
	if code != http.StatusOK {
		t.Fatalf("status = %d; body=%v", code, body)
	}
	if svc.lastTargetID != 5 {
		t.Errorf("target = %d", svc.lastTargetID)
	}
	if body["code"] != "admin.supplier.toggle_status.ok" {
		t.Errorf("code = %v", body["code"])
	}
}

func TestMallSupplierToggleStatus_NotFound_404(t *testing.T) {
	svc := &mockMallSupplierService{
		toggleFunc: func(c *gin.Context, supplierID int64) error {
			return supplierRepo.ErrSupplierNotFound
		},
	}
	h := NewMallSupplierHandler(svc)

	code, body := doMallAdminJSON(t, http.MethodPost, "/admin/supplier/toggle-status", map[string]any{"id": 5}, func(r *gin.Engine) {
		r.POST("/admin/supplier/toggle-status", h.ToggleStatus)
	})
	if code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", code)
	}
	if body["code"] != "admin.supplier.toggle_status.not_found" {
		t.Errorf("code = %v", body["code"])
	}
}

func TestMallSupplierToggleStatus_MissingID_400(t *testing.T) {
	svc := &mockMallSupplierService{}
	h := NewMallSupplierHandler(svc)

	code, _ := doMallAdminJSON(t, http.MethodPost, "/admin/supplier/toggle-status", map[string]any{}, func(r *gin.Engine) {
		r.POST("/admin/supplier/toggle-status", h.ToggleStatus)
	})
	if code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", code)
	}
}

func TestMallSupplierToggleFeatured_HappyPath(t *testing.T) {
	svc := &mockMallSupplierService{}
	h := NewMallSupplierHandler(svc)

	code, body := doMallAdminJSON(t, http.MethodPost, "/admin/supplier/toggle-featured", map[string]any{"id": 5}, func(r *gin.Engine) {
		r.POST("/admin/supplier/toggle-featured", h.ToggleFeatured)
	})
	if code != http.StatusOK {
		t.Fatalf("status = %d", code)
	}
	if body["code"] != "admin.supplier.toggle_featured.ok" {
		t.Errorf("code = %v", body["code"])
	}
}

// =============================================================================
// MallBlindBoxHandler
// =============================================================================

func TestMallBlindBoxList_HappyPath(t *testing.T) {
	svc := &mockMallBlindBoxService{
		listFunc: func(c *gin.Context, opts repository.ListOptions) ([]mall.MallBlindBox, int64, error) {
			return []mall.MallBlindBox{{ID: 1, Name: "NBA"}}, 1, nil
		},
	}
	h := NewMallBlindBoxHandler(svc)

	code, resp := doMallAdminJSON(t, http.MethodGet, "/admin/blindbox/list", nil, func(r *gin.Engine) {
		r.GET("/admin/blindbox/list", h.List)
	})
	if code != http.StatusOK {
		t.Fatalf("status = %d", code)
	}
	if resp["total"] != float64(1) {
		t.Errorf("total = %v", resp["total"])
	}
}

func TestMallBlindBoxToggle_HappyPath(t *testing.T) {
	svc := &mockMallBlindBoxService{}
	h := NewMallBlindBoxHandler(svc)

	for _, route := range []struct {
		path    string
		handler gin.HandlerFunc
		code    string
	}{
		{"/admin/blindbox/toggle-status", h.ToggleStatus, "admin.blindbox.toggle_status.ok"},
		{"/admin/blindbox/toggle-onsale", h.ToggleOnSale, "admin.blindbox.toggle_onsale.ok"},
		{"/admin/blindbox/toggle-featured", h.ToggleFeatured, "admin.blindbox.toggle_featured.ok"},
	} {
		code, body := doMallAdminJSON(t, http.MethodPost, route.path, map[string]any{"id": 9}, func(r *gin.Engine) {
			r.POST(route.path, route.handler)
		})
		if code != http.StatusOK {
			t.Errorf("%s: status = %d", route.path, code)
		}
		if body["code"] != route.code {
			t.Errorf("%s: code = %v, want %v", route.path, body["code"], route.code)
		}
		if svc.lastTargetID != 9 {
			t.Errorf("%s: target = %d", route.path, svc.lastTargetID)
		}
	}
}

func TestMallBlindBoxToggle_NotFound_404(t *testing.T) {
	// ToggleStatus 的 404 路径用独立 stub。
	svc2 := &errMallBlindBoxService{}
	h := NewMallBlindBoxHandler(svc2)

	code, body := doMallAdminJSON(t, http.MethodPost, "/admin/blindbox/toggle-status", map[string]any{"id": 9}, func(r *gin.Engine) {
		r.POST("/admin/blindbox/toggle-status", h.ToggleStatus)
	})
	if code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", code)
	}
	if body["code"] != "admin.blindbox.toggle_status.not_found" {
		t.Errorf("code = %v", body["code"])
	}
}

// errMallBlindBoxService 全部返回 ErrBlindBoxNotFound 的 stub。
type errMallBlindBoxService struct {
	mockMallBlindBoxService
}

func (e *errMallBlindBoxService) ToggleStatus(c *gin.Context, blindBoxID int64) error {
	return adminSvc.ErrBlindBoxNotFound
}

// =============================================================================
// MallPromotionHandler（admin 只读活动列表）
// =============================================================================

// mockMallPromotionService 实现 adminSvc.PromotionService。
type mockMallPromotionService struct {
	listFunc func(c *gin.Context, opts repository.ListOptions) ([]mall.MallPromotion, int64, error)

	lastOpts repository.ListOptions
}

func (m *mockMallPromotionService) List(c *gin.Context, opts repository.ListOptions) ([]mall.MallPromotion, int64, error) {
	m.lastOpts = opts
	if m.listFunc != nil {
		return m.listFunc(c, opts)
	}
	return []mall.MallPromotion{}, 0, nil
}

var _ adminSvc.PromotionService = (*mockMallPromotionService)(nil)

func TestMallPromotionList_HappyPath(t *testing.T) {
	svc := &mockMallPromotionService{
		listFunc: func(c *gin.Context, opts repository.ListOptions) ([]mall.MallPromotion, int64, error) {
			return []mall.MallPromotion{{ID: 1, BlindBoxID: 7, PromoPrice: 79}}, 1, nil
		},
	}
	h := NewMallPromotionHandler(svc)

	code, resp := doMallAdminJSON(t, http.MethodGet, "/admin/promotion/list?page=2&page_size=5", nil, func(r *gin.Engine) {
		r.GET("/admin/promotion/list", h.List)
	})
	if code != http.StatusOK {
		t.Fatalf("status = %d", code)
	}
	if svc.lastOpts.Page != 2 || svc.lastOpts.PageSize != 5 {
		t.Errorf("opts = %+v", svc.lastOpts)
	}
	if resp["total"] != float64(1) {
		t.Errorf("total = %v", resp["total"])
	}
}

func TestMallPromotionList_ServiceError_500(t *testing.T) {
	svc := &mockMallPromotionService{
		listFunc: func(c *gin.Context, opts repository.ListOptions) ([]mall.MallPromotion, int64, error) {
			return nil, 0, errors.New("db: down")
		},
	}
	h := NewMallPromotionHandler(svc)

	code, _ := doMallAdminJSON(t, http.MethodGet, "/admin/promotion/list", nil, func(r *gin.Engine) {
		r.GET("/admin/promotion/list", h.List)
	})
	if code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", code)
	}
}
