package admin

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"ai-go-mall/internal/domain/state"
	"ai-go-mall/internal/model"
	"ai-go-mall/internal/model/mall"
	"ai-go-mall/internal/repository"
	adminSvc "ai-go-mall/internal/service/admin"
)

// =============================================================================
// mocks
// =============================================================================

// mockSettlementService 实现 adminSvc.SettlementService。
//
// 每个 service 方法都暴露 func 字段；test 按需覆盖，避免引入额外 stub。
type mockSettlementService struct {
	listFunc     func(c *gin.Context, opts repository.ListOptions) ([]mall.MallSettlement, int64, error)
	detailFunc   func(c *gin.Context, id int64) (*mall.MallSettlement, []mall.MallSettlementItem, error)
	previewFunc  func(c *gin.Context, supplierID int64, start, end time.Time) (*adminSvc.PreviewSettlement, error)
	generateFunc func(c *gin.Context, supplierID int64, start, end time.Time) (*mall.MallSettlement, error)
	markPaidFunc func(c *gin.Context, settlementID int64, adminID int64) error

	lastOpts        repository.ListOptions
	lastDetailID    int64
	lastSupplierID  int64
	lastPeriodStart time.Time
	lastPeriodEnd   time.Time
	lastSettlement  int64
	lastAdminID     int64
}

func (m *mockSettlementService) List(c *gin.Context, opts repository.ListOptions) ([]mall.MallSettlement, int64, error) {
	m.lastOpts = opts
	if m.listFunc != nil {
		return m.listFunc(c, opts)
	}
	return []mall.MallSettlement{}, 0, nil
}

func (m *mockSettlementService) Detail(c *gin.Context, id int64) (*mall.MallSettlement, []mall.MallSettlementItem, error) {
	m.lastDetailID = id
	if m.detailFunc != nil {
		return m.detailFunc(c, id)
	}
	return &mall.MallSettlement{ID: id}, nil, nil
}

func (m *mockSettlementService) Preview(c *gin.Context, supplierID int64, start, end time.Time) (*adminSvc.PreviewSettlement, error) {
	m.lastSupplierID = supplierID
	m.lastPeriodStart = start
	m.lastPeriodEnd = end
	if m.previewFunc != nil {
		return m.previewFunc(c, supplierID, start, end)
	}
	return &adminSvc.PreviewSettlement{SupplierID: supplierID}, nil
}

func (m *mockSettlementService) Generate(c *gin.Context, supplierID int64, start, end time.Time) (*mall.MallSettlement, error) {
	m.lastSupplierID = supplierID
	m.lastPeriodStart = start
	m.lastPeriodEnd = end
	if m.generateFunc != nil {
		return m.generateFunc(c, supplierID, start, end)
	}
	return &mall.MallSettlement{ID: 99, SupplierID: supplierID}, nil
}

func (m *mockSettlementService) MarkPaid(c *gin.Context, settlementID int64, adminID int64) error {
	m.lastSettlement = settlementID
	m.lastAdminID = adminID
	if m.markPaidFunc != nil {
		return m.markPaidFunc(c, settlementID, adminID)
	}
	return nil
}

var _ adminSvc.SettlementService = (*mockSettlementService)(nil)

// =============================================================================
// helpers
// =============================================================================

// doSettlementJSON 注册路由并执行请求。
//
// 与 doMallAdminJSON 的差异：可注入 admin context（MarkPaid 依赖）。
func doSettlementJSON(t *testing.T, method, path string, body any, register func(r *gin.Engine), injectAdmin bool) (int, map[string]any) {
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
	if injectAdmin {
		// 模拟 AdminAuth 已写入 context —— 必须放真实类型 *model.Admin 才能通过 AdminFromContext 的 cast
		r.Use(func(c *gin.Context) {
			c.Set("admin.current", &model.Admin{ID: 42})
			c.Next()
		})
	}
	register(r)
	r.ServeHTTP(w, req)

	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	return w.Code, resp
}

// =============================================================================
// List
// =============================================================================

func TestSettlementList_HappyPath(t *testing.T) {
	svc := &mockSettlementService{
		listFunc: func(c *gin.Context, opts repository.ListOptions) ([]mall.MallSettlement, int64, error) {
			return []mall.MallSettlement{{ID: 1}, {ID: 2}}, 2, nil
		},
	}
	h := NewMallSettlementHandler(svc)

	code, resp := doSettlementJSON(t, http.MethodGet, "/admin/settlement/list?page=2&page_size=5", nil, func(r *gin.Engine) {
		r.GET("/admin/settlement/list", h.List)
	}, false)

	if code != http.StatusOK {
		t.Fatalf("status = %d", code)
	}
	if svc.lastOpts.Page != 2 || svc.lastOpts.PageSize != 5 {
		t.Errorf("opts = %+v", svc.lastOpts)
	}
	if resp["total"] != float64(2) {
		t.Errorf("total = %v", resp["total"])
	}
}

func TestSettlementList_ServiceError_500(t *testing.T) {
	svc := &mockSettlementService{
		listFunc: func(c *gin.Context, opts repository.ListOptions) ([]mall.MallSettlement, int64, error) {
			return nil, 0, errors.New("db: down")
		},
	}
	h := NewMallSettlementHandler(svc)

	code, body := doSettlementJSON(t, http.MethodGet, "/admin/settlement/list", nil, func(r *gin.Engine) {
		r.GET("/admin/settlement/list", h.List)
	}, false)

	if code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", code)
	}
	if body["code"] != "admin.settlement.list.internal" {
		t.Errorf("code = %v", body["code"])
	}
}

// =============================================================================
// Detail
// =============================================================================

func TestSettlementDetail_HappyPath(t *testing.T) {
	svc := &mockSettlementService{
		detailFunc: func(c *gin.Context, id int64) (*mall.MallSettlement, []mall.MallSettlementItem, error) {
			return &mall.MallSettlement{ID: id, SupplierID: 3},
				[]mall.MallSettlementItem{{ID: 1, SettlementID: id}},
				nil
		},
	}
	h := NewMallSettlementHandler(svc)

	code, resp := doSettlementJSON(t, http.MethodGet, "/admin/settlement/detail?id=7", nil, func(r *gin.Engine) {
		r.GET("/admin/settlement/detail", h.Detail)
	}, false)

	if code != http.StatusOK {
		t.Fatalf("status = %d; body=%v", code, resp)
	}
	if svc.lastDetailID != 7 {
		t.Errorf("detail id = %d, want 7", svc.lastDetailID)
	}
}

func TestSettlementDetail_NotFound_404(t *testing.T) {
	svc := &mockSettlementService{
		detailFunc: func(c *gin.Context, id int64) (*mall.MallSettlement, []mall.MallSettlementItem, error) {
			return nil, nil, adminSvc.ErrSettlementNotFound
		},
	}
	h := NewMallSettlementHandler(svc)

	code, body := doSettlementJSON(t, http.MethodGet, "/admin/settlement/detail?id=9", nil, func(r *gin.Engine) {
		r.GET("/admin/settlement/detail", h.Detail)
	}, false)

	if code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", code)
	}
	if body["code"] != "admin.settlement.detail.not_found" {
		t.Errorf("code = %v", body["code"])
	}
}

func TestSettlementDetail_InvalidID_400(t *testing.T) {
	svc := &mockSettlementService{}
	h := NewMallSettlementHandler(svc)

	code, body := doSettlementJSON(t, http.MethodGet, "/admin/settlement/detail?id=abc", nil, func(r *gin.Engine) {
		r.GET("/admin/settlement/detail", h.Detail)
	}, false)
	if code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 (bad id)", code)
	}
	if body["code"] != "admin.settlement.detail.invalid_input" {
		t.Errorf("code = %v", body["code"])
	}

	code, body2 := doSettlementJSON(t, http.MethodGet, "/admin/settlement/detail?id=0", nil, func(r *gin.Engine) {
		r.GET("/admin/settlement/detail", h.Detail)
	}, false)
	if code != http.StatusBadRequest {
		t.Errorf("id=0: status = %d, want 400", code)
	}
	if body2["code"] != "admin.settlement.detail.invalid_input" {
		t.Errorf("id=0: code = %v", body2["code"])
	}
}

// =============================================================================
// Preview
// =============================================================================

func TestSettlementPreview_HappyPath(t *testing.T) {
	svc := &mockSettlementService{
		previewFunc: func(c *gin.Context, supplierID int64, start, end time.Time) (*adminSvc.PreviewSettlement, error) {
			return &adminSvc.PreviewSettlement{
				SupplierID:       supplierID,
				SupplierName:     "Panini",
				OrderCount:       3,
				TotalAmount:      300,
				CommissionAmount: 30,
				PayoutAmount:     270,
			}, nil
		},
	}
	h := NewMallSettlementHandler(svc)

	body := map[string]any{
		"supplier_id":  3,
		"period_start": "2026-09-01T00:00:00Z",
		"period_end":   "2026-10-01T00:00:00Z",
	}
	code, resp := doSettlementJSON(t, http.MethodPost, "/admin/settlement/preview", body, func(r *gin.Engine) {
		r.POST("/admin/settlement/preview", h.Preview)
	}, false)

	if code != http.StatusOK {
		t.Fatalf("status = %d; body=%v", code, resp)
	}
	if svc.lastSupplierID != 3 {
		t.Errorf("supplier_id = %d, want 3", svc.lastSupplierID)
	}
	if resp["supplier_name"] != "Panini" {
		t.Errorf("supplier_name = %v", resp["supplier_name"])
	}
}

func TestSettlementPreview_InvalidPeriod_422(t *testing.T) {
	svc := &mockSettlementService{
		previewFunc: func(c *gin.Context, supplierID int64, start, end time.Time) (*adminSvc.PreviewSettlement, error) {
			return nil, adminSvc.ErrInvalidPeriod
		},
	}
	h := NewMallSettlementHandler(svc)

	body := map[string]any{
		"supplier_id":  3,
		"period_start": "2026-10-01T00:00:00Z",
		"period_end":   "2026-09-01T00:00:00Z",
	}
	code, body2 := doSettlementJSON(t, http.MethodPost, "/admin/settlement/preview", body, func(r *gin.Engine) {
		r.POST("/admin/settlement/preview", h.Preview)
	}, false)
	if code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want 422", code)
	}
	if body2["code"] != "admin.settlement.preview.invalid_period" {
		t.Errorf("code = %v", body2["code"])
	}
}

func TestSettlementPreview_SupplierNotFound_404(t *testing.T) {
	svc := &mockSettlementService{
		previewFunc: func(c *gin.Context, supplierID int64, start, end time.Time) (*adminSvc.PreviewSettlement, error) {
			return nil, adminSvc.ErrSupplierNotFound
		},
	}
	h := NewMallSettlementHandler(svc)

	body := map[string]any{
		"supplier_id":  3,
		"period_start": "2026-09-01T00:00:00Z",
		"period_end":   "2026-10-01T00:00:00Z",
	}
	code, body2 := doSettlementJSON(t, http.MethodPost, "/admin/settlement/preview", body, func(r *gin.Engine) {
		r.POST("/admin/settlement/preview", h.Preview)
	}, false)
	if code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", code)
	}
	if body2["code"] != "admin.settlement.preview.supplier_not_found" {
		t.Errorf("code = %v", body2["code"])
	}
}

func TestSettlementPreview_InvalidInput_400(t *testing.T) {
	svc := &mockSettlementService{}
	h := NewMallSettlementHandler(svc)

	cases := []struct {
		name string
		body map[string]any
	}{
		{"missing supplier_id", map[string]any{"period_start": "2026-09-01T00:00:00Z", "period_end": "2026-10-01T00:00:00Z"}},
		{"supplier_id=0", map[string]any{"supplier_id": 0, "period_start": "2026-09-01T00:00:00Z", "period_end": "2026-10-01T00:00:00Z"}},
		{"bad period format", map[string]any{"supplier_id": 3, "period_start": "bad", "period_end": "2026-10-01T00:00:00Z"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, body2 := doSettlementJSON(t, http.MethodPost, "/admin/settlement/preview", tc.body, func(r *gin.Engine) {
				r.POST("/admin/settlement/preview", h.Preview)
			}, false)
			if code != http.StatusBadRequest {
				t.Errorf("status = %d, want 400", code)
			}
			if body2["code"] != "admin.settlement.preview.invalid_input" {
				t.Errorf("code = %v", body2["code"])
			}
		})
	}
}

// =============================================================================
// Generate
// =============================================================================

func TestSettlementGenerate_HappyPath(t *testing.T) {
	svc := &mockSettlementService{
		generateFunc: func(c *gin.Context, supplierID int64, start, end time.Time) (*mall.MallSettlement, error) {
			return &mall.MallSettlement{ID: 99, SupplierID: supplierID, Status: "pending"}, nil
		},
	}
	h := NewMallSettlementHandler(svc)

	body := map[string]any{
		"supplier_id":  3,
		"period_start": "2026-09-01T00:00:00Z",
		"period_end":   "2026-10-01T00:00:00Z",
	}
	code, resp := doSettlementJSON(t, http.MethodPost, "/admin/settlement/generate", body, func(r *gin.Engine) {
		r.POST("/admin/settlement/generate", h.Generate)
	}, false)

	if code != http.StatusOK {
		t.Fatalf("status = %d; body=%v", code, resp)
	}
	if resp["code"] != "admin.settlement.generate.ok" {
		t.Errorf("code = %v", resp["code"])
	}
}

func TestSettlementGenerate_Conflict_409(t *testing.T) {
	svc := &mockSettlementService{
		generateFunc: func(c *gin.Context, supplierID int64, start, end time.Time) (*mall.MallSettlement, error) {
			return nil, adminSvc.ErrSettlementConflict
		},
	}
	h := NewMallSettlementHandler(svc)

	body := map[string]any{
		"supplier_id":  3,
		"period_start": "2026-09-01T00:00:00Z",
		"period_end":   "2026-10-01T00:00:00Z",
	}
	code, body2 := doSettlementJSON(t, http.MethodPost, "/admin/settlement/generate", body, func(r *gin.Engine) {
		r.POST("/admin/settlement/generate", h.Generate)
	}, false)
	if code != http.StatusConflict {
		t.Errorf("status = %d, want 409", code)
	}
	if body2["code"] != "admin.settlement.generate.conflict" {
		t.Errorf("code = %v", body2["code"])
	}
}

func TestSettlementGenerate_NoOrders_422(t *testing.T) {
	svc := &mockSettlementService{
		generateFunc: func(c *gin.Context, supplierID int64, start, end time.Time) (*mall.MallSettlement, error) {
			return nil, adminSvc.ErrNoOrdersToSettle
		},
	}
	h := NewMallSettlementHandler(svc)

	body := map[string]any{
		"supplier_id":  3,
		"period_start": "2026-09-01T00:00:00Z",
		"period_end":   "2026-10-01T00:00:00Z",
	}
	code, body2 := doSettlementJSON(t, http.MethodPost, "/admin/settlement/generate", body, func(r *gin.Engine) {
		r.POST("/admin/settlement/generate", h.Generate)
	}, false)
	if code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want 422", code)
	}
	if body2["code"] != "admin.settlement.generate.no_orders" {
		t.Errorf("code = %v", body2["code"])
	}
}

func TestSettlementGenerate_SupplierNotFound_404(t *testing.T) {
	svc := &mockSettlementService{
		generateFunc: func(c *gin.Context, supplierID int64, start, end time.Time) (*mall.MallSettlement, error) {
			return nil, adminSvc.ErrSupplierNotFound
		},
	}
	h := NewMallSettlementHandler(svc)

	body := map[string]any{
		"supplier_id":  3,
		"period_start": "2026-09-01T00:00:00Z",
		"period_end":   "2026-10-01T00:00:00Z",
	}
	code, body2 := doSettlementJSON(t, http.MethodPost, "/admin/settlement/generate", body, func(r *gin.Engine) {
		r.POST("/admin/settlement/generate", h.Generate)
	}, false)
	if code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", code)
	}
	if body2["code"] != "admin.settlement.generate.supplier_not_found" {
		t.Errorf("code = %v", body2["code"])
	}
}

// =============================================================================
// MarkPaid
// =============================================================================

func TestSettlementMarkPaid_HappyPath(t *testing.T) {
	svc := &mockSettlementService{}
	h := NewMallSettlementHandler(svc)

	code, resp := doSettlementJSON(t, http.MethodPost, "/admin/settlement/mark-paid",
		map[string]any{"id": 7}, func(r *gin.Engine) {
			r.POST("/admin/settlement/mark-paid", h.MarkPaid)
		}, true)

	if code != http.StatusOK {
		t.Fatalf("status = %d; body=%v", code, resp)
	}
	if resp["code"] != "admin.settlement.mark_paid.ok" {
		t.Errorf("code = %v", resp["code"])
	}
	if svc.lastSettlement != 7 {
		t.Errorf("settlement = %d, want 7", svc.lastSettlement)
	}
	if svc.lastAdminID != 42 {
		t.Errorf("adminID = %d, want 42", svc.lastAdminID)
	}
}

func TestSettlementMarkPaid_AlreadyPaid_409(t *testing.T) {
	svc := &mockSettlementService{
		markPaidFunc: func(c *gin.Context, settlementID int64, adminID int64) error {
			return adminSvc.ErrSettlementAlreadyPaid
		},
	}
	h := NewMallSettlementHandler(svc)

	code, body := doSettlementJSON(t, http.MethodPost, "/admin/settlement/mark-paid",
		map[string]any{"id": 7}, func(r *gin.Engine) {
			r.POST("/admin/settlement/mark-paid", h.MarkPaid)
		}, true)
	if code != http.StatusConflict {
		t.Errorf("status = %d, want 409", code)
	}
	if body["code"] != "admin.settlement.mark_paid.already_paid" {
		t.Errorf("code = %v", body["code"])
	}
}

// TestSettlementMarkPaid_InvalidStateTransition_409 spec 14.7：
//
//	状态机拒绝非法转换（paid → paid / pending → paid / failed → paid）
//	→ handler 映射 HTTP 409 + code "invalid_state_transition"。
func TestSettlementMarkPaid_InvalidStateTransition_409(t *testing.T) {
	svc := &mockSettlementService{
		markPaidFunc: func(c *gin.Context, settlementID int64, adminID int64) error {
			return state.ErrInvalidStateTransition
		},
	}
	h := NewMallSettlementHandler(svc)

	code, body := doSettlementJSON(t, http.MethodPost, "/admin/settlement/mark-paid",
		map[string]any{"id": 7}, func(r *gin.Engine) {
			r.POST("/admin/settlement/mark-paid", h.MarkPaid)
		}, true)
	if code != http.StatusConflict {
		t.Errorf("status = %d, want 409", code)
	}
	if body["code"] != "admin.settlement.mark_paid.invalid_state_transition" {
		t.Errorf("code = %v", body["code"])
	}
}

func TestSettlementMarkPaid_InsufficientBalance_422(t *testing.T) {
	svc := &mockSettlementService{
		markPaidFunc: func(c *gin.Context, settlementID int64, adminID int64) error {
			return adminSvc.ErrInsufficientSupplierBalance
		},
	}
	h := NewMallSettlementHandler(svc)

	code, body := doSettlementJSON(t, http.MethodPost, "/admin/settlement/mark-paid",
		map[string]any{"id": 7}, func(r *gin.Engine) {
			r.POST("/admin/settlement/mark-paid", h.MarkPaid)
		}, true)
	if code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want 422", code)
	}
	if body["code"] != "admin.settlement.mark_paid.insufficient_balance" {
		t.Errorf("code = %v", body["code"])
	}
}

func TestSettlementMarkPaid_NotFound_404(t *testing.T) {
	svc := &mockSettlementService{
		markPaidFunc: func(c *gin.Context, settlementID int64, adminID int64) error {
			return adminSvc.ErrSettlementNotFound
		},
	}
	h := NewMallSettlementHandler(svc)

	code, body := doSettlementJSON(t, http.MethodPost, "/admin/settlement/mark-paid",
		map[string]any{"id": 7}, func(r *gin.Engine) {
			r.POST("/admin/settlement/mark-paid", h.MarkPaid)
		}, true)
	if code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", code)
	}
	if body["code"] != "admin.settlement.mark_paid.not_found" {
		t.Errorf("code = %v", body["code"])
	}
}

func TestSettlementMarkPaid_InvalidInput_400(t *testing.T) {
	svc := &mockSettlementService{}
	h := NewMallSettlementHandler(svc)

	code, body := doSettlementJSON(t, http.MethodPost, "/admin/settlement/mark-paid",
		map[string]any{}, func(r *gin.Engine) {
			r.POST("/admin/settlement/mark-paid", h.MarkPaid)
		}, true)
	if code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", code)
	}
	if body["code"] != "admin.settlement.mark_paid.invalid_input" {
		t.Errorf("code = %v", body["code"])
	}
}

func TestSettlementMarkPaid_NoAdmin_401(t *testing.T) {
	svc := &mockSettlementService{}
	h := NewMallSettlementHandler(svc)

	code, body := doSettlementJSON(t, http.MethodPost, "/admin/settlement/mark-paid",
		map[string]any{"id": 7}, func(r *gin.Engine) {
			r.POST("/admin/settlement/mark-paid", h.MarkPaid)
		}, false) // 不注入 admin
	if code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", code)
	}
	if body["code"] != "admin.settlement.mark_paid.unauthorized" {
		t.Errorf("code = %v", body["code"])
	}
}