package supplier

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
	supplierSvc "ai-go-mall/internal/service/supplier"
)

// mockProductService 实现 supplierSvc.ProductService。
type mockProductService struct {
	listFunc   func(c *gin.Context, supplierID int64, opts repository.ListOptions) ([]mall.MallBlindBox, int64, error)
	createFunc func(c *gin.Context, supplierID int64, b *mall.MallBlindBox, items []mall.MallCardPoolItem) (*mall.MallBlindBox, error)
	updateFunc func(c *gin.Context, supplierID, blindBoxID int64, patch supplierSvc.ProductPatch) error
	toggleFunc func(c *gin.Context, supplierID, blindBoxID int64, onSale bool) error

	lastSupplierID int64
	lastBlindBox   *mall.MallBlindBox
	lastItems      []mall.MallCardPoolItem
	lastToggleID   int64
	lastToggleVal  bool
	lastUpdateID   int64
	lastPatch      supplierSvc.ProductPatch
}

func (m *mockProductService) List(c *gin.Context, supplierID int64, opts repository.ListOptions) ([]mall.MallBlindBox, int64, error) {
	if m.listFunc != nil {
		return m.listFunc(c, supplierID, opts)
	}
	return []mall.MallBlindBox{}, 0, nil
}

func (m *mockProductService) Create(c *gin.Context, supplierID int64, b *mall.MallBlindBox, items []mall.MallCardPoolItem) (*mall.MallBlindBox, error) {
	m.lastSupplierID = supplierID
	m.lastBlindBox = b
	m.lastItems = items
	if m.createFunc != nil {
		return m.createFunc(c, supplierID, b, items)
	}
	b.ID = 77
	return b, nil
}

func (m *mockProductService) Update(c *gin.Context, supplierID, blindBoxID int64, patch supplierSvc.ProductPatch) error {
	m.lastSupplierID = supplierID
	m.lastUpdateID = blindBoxID
	m.lastPatch = patch
	if m.updateFunc != nil {
		return m.updateFunc(c, supplierID, blindBoxID, patch)
	}
	return nil
}

func (m *mockProductService) ToggleOnSale(c *gin.Context, supplierID, blindBoxID int64, onSale bool) error {
	m.lastSupplierID = supplierID
	m.lastToggleID = blindBoxID
	m.lastToggleVal = onSale
	if m.toggleFunc != nil {
		return m.toggleFunc(c, supplierID, blindBoxID, onSale)
	}
	return nil
}

var _ supplierSvc.ProductService = (*mockProductService)(nil)

// withSupplier 模拟 SupplierAuth 写 context。
func withSupplier(supplierID int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		if supplierID > 0 {
			c.Set("supplier.current", &mall.MallSupplierUser{ID: 3, SupplierID: supplierID, Username: "panini_admin"})
		}
		c.Next()
	}
}

// doSupplierJSON 在新 engine 上注册路由并执行 POST/GET。
func doSupplierJSON(t *testing.T, method, path string, body any, register func(r *gin.Engine)) (int, map[string]any) {
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

func TestProductCreate_HappyPath(t *testing.T) {
	svc := &mockProductService{
		createFunc: func(c *gin.Context, supplierID int64, b *mall.MallBlindBox, items []mall.MallCardPoolItem) (*mall.MallBlindBox, error) {
			if supplierID != 10 {
				t.Errorf("supplierID = %d, want 10", supplierID)
			}
			if b.Name != "NBA 盲盒" || b.Price != 99 {
				t.Errorf("blindbox = %+v", b)
			}
			if len(items) != 2 || items[0].CardID != 5 || items[0].Rarity != mall.RaritySSR || items[0].Weight != 300 || items[0].Stock != 50 {
				t.Errorf("items = %+v", items)
			}
			b.ID = 77
			return b, nil
		},
	}
	h := NewProductHandler(svc)

	code, resp := doSupplierJSON(t, http.MethodPost, "/supplier/products/create", map[string]any{
		"name":  "NBA 盲盒",
		"cover": "https://cdn/cover.jpg",
		"price": 99,
		"items": []map[string]any{
			{"card_id": 5, "rarity": "SSR", "weight": 300, "stock": 50},
			{"card_id": 6, "rarity": "N", "weight": 9700, "stock": 100},
		},
	}, func(r *gin.Engine) {
		r.POST("/supplier/products/create", withSupplier(10), h.Create)
	})

	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%v", code, resp)
	}
	if resp["id"] != float64(77) {
		t.Errorf("id = %v", resp["id"])
	}
}

func TestProductCreate_MissingName_400(t *testing.T) {
	svc := &mockProductService{}
	h := NewProductHandler(svc)

	code, _ := doSupplierJSON(t, http.MethodPost, "/supplier/products/create", map[string]any{
		"price": 99,
		"items": []map[string]any{{"card_id": 1, "weight": 10000, "stock": 1}},
	}, func(r *gin.Engine) {
		r.POST("/supplier/products/create", withSupplier(10), h.Create)
	})
	if code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", code)
	}
	if svc.lastSupplierID != 0 {
		t.Errorf("Create should NOT be called")
	}
}

func TestProductCreate_EmptyItems_400(t *testing.T) {
	svc := &mockProductService{}
	h := NewProductHandler(svc)

	code, _ := doSupplierJSON(t, http.MethodPost, "/supplier/products/create", map[string]any{
		"name":  "X",
		"price": 99,
	}, func(r *gin.Engine) {
		r.POST("/supplier/products/create", withSupplier(10), h.Create)
	})
	if code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", code)
	}
}

func TestProductCreate_NoSupplier_401(t *testing.T) {
	svc := &mockProductService{}
	h := NewProductHandler(svc)

	code, body := doSupplierJSON(t, http.MethodPost, "/supplier/products/create", map[string]any{
		"name":  "X",
		"price": 99,
		"items": []map[string]any{{"card_id": 1, "weight": 10000, "stock": 1}},
	}, func(r *gin.Engine) {
		r.POST("/supplier/products/create", withSupplier(0), h.Create)
	})
	if code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", code)
	}
	if body["code"] != "product.unauthorized" {
		t.Errorf("code = %v", body["code"])
	}
}

func TestProductCreate_ServiceErrors(t *testing.T) {
	cases := []struct {
		name     string
		err      error
		wantCode int
		wantStr  string
	}{
		{"weight_sum", supplierSvc.ErrInvalidWeightSum, http.StatusUnprocessableEntity, "product.invalid_weight_sum"},
		{"empty_pool", supplierSvc.ErrEmptyPool, http.StatusUnprocessableEntity, "product.empty_pool"},
		{"forbidden", supplierSvc.ErrBlindBoxForbidden, http.StatusForbidden, "product.forbidden"},
		{"internal", errors.New("db: down"), http.StatusInternalServerError, "product.internal"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := &mockProductService{
				createFunc: func(c *gin.Context, supplierID int64, b *mall.MallBlindBox, items []mall.MallCardPoolItem) (*mall.MallBlindBox, error) {
					return nil, tc.err
				},
			}
			h := NewProductHandler(svc)
			code, body := doSupplierJSON(t, http.MethodPost, "/supplier/products/create", map[string]any{
				"name":  "X",
				"price": 99,
				"items": []map[string]any{{"card_id": 1, "weight": 10000, "stock": 1}},
			}, func(r *gin.Engine) {
				r.POST("/supplier/products/create", withSupplier(10), h.Create)
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

func TestProductList_HappyPath(t *testing.T) {
	svc := &mockProductService{
		listFunc: func(c *gin.Context, supplierID int64, opts repository.ListOptions) ([]mall.MallBlindBox, int64, error) {
			if supplierID != 10 {
				t.Errorf("supplierID = %d, want 10", supplierID)
			}
			return []mall.MallBlindBox{{ID: 1, Name: "A"}}, 1, nil
		},
	}
	h := NewProductHandler(svc)

	code, resp := doSupplierJSON(t, http.MethodGet, "/supplier/products/list", nil, func(r *gin.Engine) {
		r.GET("/supplier/products/list", withSupplier(10), h.List)
	})
	if code != http.StatusOK {
		t.Fatalf("status = %d", code)
	}
	if resp["total"] != float64(1) {
		t.Errorf("total = %v", resp["total"])
	}
}

func TestProductEdit_HappyPath(t *testing.T) {
	svc := &mockProductService{
		updateFunc: func(c *gin.Context, supplierID, blindBoxID int64, patch supplierSvc.ProductPatch) error {
			if supplierID != 10 || blindBoxID != 7 {
				t.Errorf("supplierID=%d blindBoxID=%d, want 10/7", supplierID, blindBoxID)
			}
			if patch.Name == nil || *patch.Name != "新名字" {
				t.Errorf("patch.Name = %v, want 新名字", patch.Name)
			}
			if patch.Price == nil || *patch.Price != 88 {
				t.Errorf("patch.Price = %v, want 88", patch.Price)
			}
			// 未传字段必须保持 nil（= 不改），而不是零值。
			if patch.Cover != nil || patch.Description != nil {
				t.Errorf("未传字段应为 nil: cover=%v description=%v", patch.Cover, patch.Description)
			}
			return nil
		},
	}
	h := NewProductHandler(svc)

	code, _ := doSupplierJSON(t, http.MethodPost, "/supplier/products/edit", map[string]any{
		"id": 7, "name": "新名字", "price": 88,
	}, func(r *gin.Engine) {
		r.POST("/supplier/products/edit", withSupplier(10), h.Edit)
	})
	if code != http.StatusOK {
		t.Fatalf("status = %d", code)
	}
}

func TestProductEdit_NotFound_404(t *testing.T) {
	svc := &mockProductService{
		updateFunc: func(c *gin.Context, supplierID, blindBoxID int64, patch supplierSvc.ProductPatch) error {
			return supplierSvc.ErrBlindBoxNotFound
		},
	}
	h := NewProductHandler(svc)

	code, body := doSupplierJSON(t, http.MethodPost, "/supplier/products/edit", map[string]any{
		"id": 999, "name": "X",
	}, func(r *gin.Engine) {
		r.POST("/supplier/products/edit", withSupplier(10), h.Edit)
	})
	if code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", code)
	}
	if body["code"] != "product.not_found" {
		t.Errorf("code = %v", body["code"])
	}
}

func TestProductEdit_MissingID_400(t *testing.T) {
	svc := &mockProductService{}
	h := NewProductHandler(svc)

	code, _ := doSupplierJSON(t, http.MethodPost, "/supplier/products/edit", map[string]any{
		"name": "no id",
	}, func(r *gin.Engine) {
		r.POST("/supplier/products/edit", withSupplier(10), h.Edit)
	})
	if code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", code)
	}
}

func TestProductToggleOnSale_HappyPath(t *testing.T) {
	svc := &mockProductService{}
	h := NewProductHandler(svc)

	code, _ := doSupplierJSON(t, http.MethodPost, "/supplier/products/toggle-onsale", map[string]any{
		"id": 7, "on_sale": true,
	}, func(r *gin.Engine) {
		r.POST("/supplier/products/toggle-onsale", withSupplier(10), h.ToggleOnSale)
	})
	if code != http.StatusOK {
		t.Fatalf("status = %d", code)
	}
	if svc.lastToggleID != 7 || !svc.lastToggleVal || svc.lastSupplierID != 10 {
		t.Errorf("toggle args = %d/%v/%d", svc.lastToggleID, svc.lastToggleVal, svc.lastSupplierID)
	}
}
