package user

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"ai-go-mall/internal/model/mall"
	"ai-go-mall/internal/repository"
	userSvc "ai-go-mall/internal/service/user"
)

// =============================================================================
// helpers
// =============================================================================

// doBlindboxGET 构造 GET 请求跑 BlindBoxHandler 的指定路由。
func doBlindboxGET(t *testing.T, path string, register func(r *gin.Engine)) (int, map[string]any) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	register(r)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	return w.Code, resp
}

// mockBlindBoxService 实现 userSvc.BlindBoxService。
type mockBlindBoxService struct {
	listFunc   func(c *gin.Context, opts repository.ListOptions) ([]mall.MallBlindBox, int64, error)
	detailFunc func(c *gin.Context, id int64) (*userSvc.BlindBoxDetail, error)

	listCalls   int
	detailCalls int
	lastDetail  int64
}

func (m *mockBlindBoxService) List(c *gin.Context, opts repository.ListOptions) ([]mall.MallBlindBox, int64, error) {
	m.listCalls++
	if m.listFunc != nil {
		return m.listFunc(c, opts)
	}
	return []mall.MallBlindBox{}, 0, nil
}

func (m *mockBlindBoxService) Detail(c *gin.Context, id int64) (*userSvc.BlindBoxDetail, error) {
	m.detailCalls++
	m.lastDetail = id
	if m.detailFunc != nil {
		return m.detailFunc(c, id)
	}
	return &userSvc.BlindBoxDetail{BlindBox: mall.MallBlindBox{ID: id}}, nil
}

var _ userSvc.BlindBoxService = (*mockBlindBoxService)(nil)

// =============================================================================
// List tests
// =============================================================================

func TestBlindBoxList_HappyPath(t *testing.T) {
	svc := &mockBlindBoxService{
		listFunc: func(c *gin.Context, opts repository.ListOptions) ([]mall.MallBlindBox, int64, error) {
			if opts.Page != 2 || opts.PageSize != 5 {
				t.Errorf("opts = %+v, want page=2 page_size=5", opts)
			}
			return []mall.MallBlindBox{
				{ID: 1, Name: "NBA 盲盒", Price: 99},
				{ID: 2, Name: "足球盲盒", Price: 59.5},
			}, 2, nil
		},
	}
	h := NewBlindBoxHandler(svc)

	code, resp := doBlindboxGET(t, "/user/blindbox/list?page=2&page_size=5", func(r *gin.Engine) {
		r.GET("/user/blindbox/list", h.List)
	})

	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200", code)
	}
	if svc.listCalls != 1 {
		t.Errorf("List called %d times, want 1", svc.listCalls)
	}
	items, ok := resp["items"].([]any)
	if !ok || len(items) != 2 {
		t.Fatalf("items missing or wrong: %v", resp["items"])
	}
	first := items[0].(map[string]any)
	if first["id"] != float64(1) || first["name"] != "NBA 盲盒" {
		t.Errorf("first item = %v, want id=1 name=NBA 盲盒", first)
	}
	if resp["total"] != float64(2) || resp["page"] != float64(2) || resp["page_size"] != float64(5) {
		t.Errorf("paging meta = %v/%v/%v, want 2/2/5", resp["total"], resp["page"], resp["page_size"])
	}
}

func TestBlindBoxList_DefaultPaging(t *testing.T) {
	var got repository.ListOptions
	svc := &mockBlindBoxService{
		listFunc: func(c *gin.Context, opts repository.ListOptions) ([]mall.MallBlindBox, int64, error) {
			got = opts
			return []mall.MallBlindBox{}, 0, nil
		},
	}
	h := NewBlindBoxHandler(svc)

	code, _ := doBlindboxGET(t, "/user/blindbox/list", func(r *gin.Engine) {
		r.GET("/user/blindbox/list", h.List)
	})

	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200", code)
	}
	if got.Page != 1 || got.PageSize != 20 {
		t.Errorf("opts = %+v, want page=1 page_size=20", got)
	}
}

func TestBlindBoxList_ServiceError_500(t *testing.T) {
	svc := &mockBlindBoxService{
		listFunc: func(c *gin.Context, opts repository.ListOptions) ([]mall.MallBlindBox, int64, error) {
			return nil, 0, errors.New("db: connection refused")
		},
	}
	h := NewBlindBoxHandler(svc)

	code, _ := doBlindboxGET(t, "/user/blindbox/list", func(r *gin.Engine) {
		r.GET("/user/blindbox/list", h.List)
	})

	if code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", code)
	}
}

// =============================================================================
// Detail tests
// =============================================================================

func TestBlindBoxDetail_HappyPath(t *testing.T) {
	svc := &mockBlindBoxService{
		detailFunc: func(c *gin.Context, id int64) (*userSvc.BlindBoxDetail, error) {
			if id != 7 {
				t.Errorf("id = %d, want 7", id)
			}
			return &userSvc.BlindBoxDetail{
				BlindBox:       mall.MallBlindBox{ID: 7, Name: "NBA 盲盒", Price: 99},
				EffectivePrice: 79,
				PoolTotalStock: 100,
				CachedAt:       time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC),
			}, nil
		},
	}
	h := NewBlindBoxHandler(svc)

	code, resp := doBlindboxGET(t, "/user/blindbox/detail?id=7", func(r *gin.Engine) {
		r.GET("/user/blindbox/detail", h.Detail)
	})

	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200", code)
	}
	if svc.lastDetail != 7 {
		t.Errorf("Detail id = %d, want 7", svc.lastDetail)
	}
	if resp["effective_price"] != float64(79) {
		t.Errorf("effective_price = %v, want 79", resp["effective_price"])
	}
	bb, ok := resp["blind_box"].(map[string]any)
	if !ok || bb["id"] != float64(7) {
		t.Errorf("blind_box missing or wrong: %v", resp["blind_box"])
	}
}

func TestBlindBoxDetail_InvalidID_400(t *testing.T) {
	cases := []string{"", "abc", "0", "-3"}
	for _, raw := range cases {
		svc := &mockBlindBoxService{}
		h := NewBlindBoxHandler(svc)
		path := "/user/blindbox/detail"
		if raw != "" {
			path += "?id=" + raw
		}
		code, _ := doBlindboxGET(t, path, func(r *gin.Engine) {
			r.GET("/user/blindbox/detail", h.Detail)
		})
		if code != http.StatusBadRequest {
			t.Errorf("id=%q: status = %d, want 400", raw, code)
		}
		if svc.detailCalls != 0 {
			t.Errorf("id=%q: Detail should NOT be called", raw)
		}
	}
}

func TestBlindBoxDetail_NotFound_404(t *testing.T) {
	svc := &mockBlindBoxService{
		detailFunc: func(c *gin.Context, id int64) (*userSvc.BlindBoxDetail, error) {
			return nil, userSvc.ErrBlindBoxNotFound
		},
	}
	h := NewBlindBoxHandler(svc)

	code, body := doBlindboxGET(t, "/user/blindbox/detail?id=99999", func(r *gin.Engine) {
		r.GET("/user/blindbox/detail", h.Detail)
	})

	if code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", code)
	}
	if body["code"] != "blindbox.not_found" {
		t.Errorf("code = %v, want blindbox.not_found", body["code"])
	}
}

func TestBlindBoxDetail_ServiceError_500(t *testing.T) {
	svc := &mockBlindBoxService{
		detailFunc: func(c *gin.Context, id int64) (*userSvc.BlindBoxDetail, error) {
			return nil, errors.New("db: connection refused")
		},
	}
	h := NewBlindBoxHandler(svc)

	code, _ := doBlindboxGET(t, "/user/blindbox/detail?id=1", func(r *gin.Engine) {
		r.GET("/user/blindbox/detail", h.Detail)
	})

	if code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", code)
	}
}

// TestBlindBoxDetail_SupplierFinancialFieldsStripped 回归（final review
// Important #6）：C 端详情是公开接口，供应商的待结算余额 / 累计销售额 /
// 手续费率属商业敏感数据，绝不能出现在响应里。
func TestBlindBoxDetail_SupplierFinancialFieldsStripped(t *testing.T) {
	balance := 88888.5
	sales := 999999.0
	rate := 0.08
	svc := &mockBlindBoxService{
		detailFunc: func(c *gin.Context, id int64) (*userSvc.BlindBoxDetail, error) {
			return &userSvc.BlindBoxDetail{
				BlindBox: mall.MallBlindBox{ID: 7, Name: "NBA 盲盒"},
				Supplier: mall.MallSupplier{
					ID:             10,
					Name:           "Panini 旗舰店",
					Balance:        balance,
					TotalSales:     sales,
					CommissionRate: &rate,
				},
				EffectivePrice: 99,
			}, nil
		},
	}
	h := NewBlindBoxHandler(svc)

	code, resp := doBlindboxGET(t, "/user/blindbox/detail?id=7", func(r *gin.Engine) {
		r.GET("/user/blindbox/detail", h.Detail)
	})

	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200", code)
	}
	supplier, ok := resp["supplier"].(map[string]any)
	if !ok {
		t.Fatalf("supplier missing: %v", resp["supplier"])
	}
	for _, leak := range []string{"balance", "total_sales", "commission_rate"} {
		if _, has := supplier[leak]; has {
			t.Errorf("supplier.%s leaked in public detail response", leak)
		}
	}
	if supplier["name"] != "Panini 旗舰店" || supplier["id"] != float64(10) {
		t.Errorf("supplier public view = %v", supplier)
	}
}
