package admin

import (
	"errors"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"

	"ai-go-mall/internal/domain/state"
	"ai-go-mall/internal/model/mall"
	"ai-go-mall/internal/repository"
	mallRepo "ai-go-mall/internal/repository/mall"
	mallSvc "ai-go-mall/internal/service/mall"
)

// =============================================================================
// mocks
// =============================================================================

// mockBillService 实现 mallSvc.BillService（admin 视角 4 方法）。
type mockBillService struct {
	listAdminFunc  func(c *gin.Context, status mall.MallSettlementBillStatus, supplierID int64, opts repository.ListOptions) ([]mall.MallSettlementBill, int64, error)
	detailFunc     func(c *gin.Context, billID int64) (*mall.MallSettlementBill, []mall.MallSettlementBillItem, error)
	regenerateFunc func(c *gin.Context, billID int64, amounts mallRepo.BillRegenAmounts, remark string) error
	markPaidFunc   func(c *gin.Context, billID int64, adminID int64) error

	lastStatus      mall.MallSettlementBillStatus
	lastSupplierID  int64
	lastOpts        repository.ListOptions
	lastDetailID    int64
	lastRegenID     int64
	lastAmounts     mallRepo.BillRegenAmounts
	lastRemark      string
	lastMarkID      int64
	lastMarkAdminID int64
}

func (m *mockBillService) ListAdmin(c *gin.Context, status mall.MallSettlementBillStatus, supplierID int64, opts repository.ListOptions) ([]mall.MallSettlementBill, int64, error) {
	m.lastStatus = status
	m.lastSupplierID = supplierID
	m.lastOpts = opts
	if m.listAdminFunc != nil {
		return m.listAdminFunc(c, status, supplierID, opts)
	}
	return []mall.MallSettlementBill{{ID: 1, SupplierID: supplierID}}, 1, nil
}

func (m *mockBillService) Detail(c *gin.Context, billID int64) (*mall.MallSettlementBill, []mall.MallSettlementBillItem, error) {
	m.lastDetailID = billID
	if m.detailFunc != nil {
		return m.detailFunc(c, billID)
	}
	return &mall.MallSettlementBill{ID: billID}, []mall.MallSettlementBillItem{{DrawOrderID: 9}}, nil
}

func (m *mockBillService) Regenerate(c *gin.Context, billID int64, amounts mallRepo.BillRegenAmounts, remark string) error {
	m.lastRegenID = billID
	m.lastAmounts = amounts
	m.lastRemark = remark
	if m.regenerateFunc != nil {
		return m.regenerateFunc(c, billID, amounts, remark)
	}
	return nil
}

func (m *mockBillService) MarkPaid(c *gin.Context, billID int64, adminID int64) error {
	m.lastMarkID = billID
	m.lastMarkAdminID = adminID
	if m.markPaidFunc != nil {
		return m.markPaidFunc(c, billID, adminID)
	}
	return nil
}

var _ mallSvc.BillService = (*mockBillService)(nil)

// GenerateForBox / FinalizeSoldOut 不在 admin 路由使用范围（抽卡事务尾部调用），
// no-op 桩仅为满足接口——被调用即 panic（= 测试红，说明 handler 误用）。

func (m *mockBillService) GenerateForBox(c *gin.Context, boxID int64) (*mall.MallSettlementBill, error) {
	panic("mockBillService: GenerateForBox should not be called in admin handler tests")
}

func (m *mockBillService) FinalizeSoldOut(c *gin.Context, boxID int64) error {
	panic("mockBillService: FinalizeSoldOut should not be called in admin handler tests")
}

// =============================================================================
// List
// =============================================================================

func TestBillList_HappyPath(t *testing.T) {
	svc := &mockBillService{}
	h := NewMallBillHandler(svc)

	code, resp := doSettlementJSON(t, http.MethodGet,
		"/admin/bill/list?status=confirmed&supplier_id=7&page=2&page_size=5", nil, func(r *gin.Engine) {
			r.GET("/admin/bill/list", h.List)
		}, false)

	if code != http.StatusOK {
		t.Fatalf("status = %d; body=%v", code, resp)
	}
	if svc.lastStatus != mall.BillStatusConfirmed {
		t.Errorf("status = %q, want confirmed", svc.lastStatus)
	}
	if svc.lastSupplierID != 7 {
		t.Errorf("supplierID = %d, want 7", svc.lastSupplierID)
	}
	if svc.lastOpts.Page != 2 || svc.lastOpts.PageSize != 5 {
		t.Errorf("opts = %+v", svc.lastOpts)
	}
	if resp["total"] != float64(1) {
		t.Errorf("total = %v", resp["total"])
	}
}

func TestBillList_Defaults(t *testing.T) {
	svc := &mockBillService{}
	h := NewMallBillHandler(svc)

	code, _ := doSettlementJSON(t, http.MethodGet, "/admin/bill/list", nil, func(r *gin.Engine) {
		r.GET("/admin/bill/list", h.List)
	}, false)
	if code != http.StatusOK {
		t.Fatalf("status = %d", code)
	}
	if svc.lastStatus != "" || svc.lastSupplierID != 0 {
		t.Errorf("filters = %q/%d, want empty (不筛)", svc.lastStatus, svc.lastSupplierID)
	}
	if svc.lastOpts.Page != 1 || svc.lastOpts.PageSize != 20 {
		t.Errorf("opts = %+v, want page=1 page_size=20", svc.lastOpts)
	}
}

func TestBillList_InvalidStatus_400(t *testing.T) {
	svc := &mockBillService{}
	h := NewMallBillHandler(svc)

	code, body := doSettlementJSON(t, http.MethodGet, "/admin/bill/list?status=bogus", nil, func(r *gin.Engine) {
		r.GET("/admin/bill/list", h.List)
	}, false)
	if code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", code)
	}
	if body["code"] != "admin.bill.list.invalid_input" {
		t.Errorf("code = %v", body["code"])
	}
}

func TestBillList_ServiceError_500(t *testing.T) {
	svc := &mockBillService{
		listAdminFunc: func(c *gin.Context, status mall.MallSettlementBillStatus, supplierID int64, opts repository.ListOptions) ([]mall.MallSettlementBill, int64, error) {
			return nil, 0, errors.New("db: down")
		},
	}
	h := NewMallBillHandler(svc)

	code, body := doSettlementJSON(t, http.MethodGet, "/admin/bill/list", nil, func(r *gin.Engine) {
		r.GET("/admin/bill/list", h.List)
	}, false)
	if code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", code)
	}
	if body["code"] != "admin.bill.list.internal" {
		t.Errorf("code = %v", body["code"])
	}
}

// =============================================================================
// Detail
// =============================================================================

func TestBillDetail_HappyPath(t *testing.T) {
	svc := &mockBillService{
		detailFunc: func(c *gin.Context, billID int64) (*mall.MallSettlementBill, []mall.MallSettlementBillItem, error) {
			if billID != 77 {
				t.Errorf("id = %d, want 77", billID)
			}
			return &mall.MallSettlementBill{ID: 77}, []mall.MallSettlementBillItem{{DrawOrderID: 9}}, nil
		},
	}
	h := NewMallBillHandler(svc)

	code, resp := doSettlementJSON(t, http.MethodGet, "/admin/bill/detail?id=77", nil, func(r *gin.Engine) {
		r.GET("/admin/bill/detail", h.Detail)
	}, false)
	if code != http.StatusOK {
		t.Fatalf("status = %d; body=%v", code, resp)
	}
	billMap, _ := resp["bill"].(map[string]any)
	if billMap == nil || billMap["id"] != float64(77) {
		t.Errorf("bill = %v", resp["bill"])
	}
	items, _ := resp["items"].([]any)
	if len(items) != 1 {
		t.Errorf("items = %v", resp["items"])
	}
}

func TestBillDetail_MissingID_400(t *testing.T) {
	svc := &mockBillService{}
	h := NewMallBillHandler(svc)

	code, body := doSettlementJSON(t, http.MethodGet, "/admin/bill/detail", nil, func(r *gin.Engine) {
		r.GET("/admin/bill/detail", h.Detail)
	}, false)
	if code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", code)
	}
	if body["code"] != "admin.bill.detail.invalid_input" {
		t.Errorf("code = %v", body["code"])
	}
}

func TestBillDetail_ServiceErrors(t *testing.T) {
	cases := []struct {
		name     string
		err      error
		wantCode int
		wantStr  string
	}{
		{"not_found", mallSvc.ErrBillNotFound, http.StatusNotFound, "admin.bill.detail.not_found"},
		{"internal", errors.New("db: down"), http.StatusInternalServerError, "admin.bill.detail.internal"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := &mockBillService{
				detailFunc: func(c *gin.Context, billID int64) (*mall.MallSettlementBill, []mall.MallSettlementBillItem, error) {
					return nil, nil, tc.err
				},
			}
			h := NewMallBillHandler(svc)
			code, body := doSettlementJSON(t, http.MethodGet, "/admin/bill/detail?id=77", nil, func(r *gin.Engine) {
				r.GET("/admin/bill/detail", h.Detail)
			}, false)
			if code != tc.wantCode {
				t.Errorf("status = %d, want %d", code, tc.wantCode)
			}
			if body["code"] != tc.wantStr {
				t.Errorf("code = %v, want %v", body["code"], tc.wantStr)
			}
		})
	}
}

// =============================================================================
// Regenerate
// =============================================================================

func TestBillRegenerate_HappyPath(t *testing.T) {
	svc := &mockBillService{}
	h := NewMallBillHandler(svc)

	code, body := doSettlementJSON(t, http.MethodPost, "/admin/bill/regenerate", map[string]any{
		"id": 77, "total_amount": 370.0, "commission_rate": 0.1,
		"commission_amount": 37.0, "payout_amount": 333.0, "remark": "金额核算有误已修正",
	}, func(r *gin.Engine) {
		r.POST("/admin/bill/regenerate", h.Regenerate)
	}, false)

	if code != http.StatusOK {
		t.Fatalf("status = %d; body=%v", code, body)
	}
	if svc.lastRegenID != 77 {
		t.Errorf("id = %d, want 77", svc.lastRegenID)
	}
	if svc.lastAmounts.TotalAmount != 370 || svc.lastAmounts.CommissionRate != 0.1 ||
		svc.lastAmounts.CommissionAmount != 37 || svc.lastAmounts.PayoutAmount != 333 {
		t.Errorf("amounts = %+v", svc.lastAmounts)
	}
	if svc.lastRemark != "金额核算有误已修正" {
		t.Errorf("remark = %q", svc.lastRemark)
	}
}

func TestBillRegenerate_MissingID_400(t *testing.T) {
	svc := &mockBillService{}
	h := NewMallBillHandler(svc)

	code, _ := doSettlementJSON(t, http.MethodPost, "/admin/bill/regenerate", map[string]any{
		"total_amount": 370,
	}, func(r *gin.Engine) {
		r.POST("/admin/bill/regenerate", h.Regenerate)
	}, false)
	if code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", code)
	}
	if svc.lastRegenID != 0 {
		t.Errorf("Regenerate should NOT be called")
	}
}

func TestBillRegenerate_ServiceErrors(t *testing.T) {
	cases := []struct {
		name     string
		err      error
		wantCode int
		wantStr  string
	}{
		{"not_found", mallSvc.ErrBillNotFound, http.StatusNotFound, "admin.bill.regenerate.not_found"},
		{"invalid_transition", state.ErrInvalidStateTransition, http.StatusConflict, "admin.bill.regenerate.invalid_transition"},
		{"status_conflict", mallSvc.ErrBillStatusConflict, http.StatusConflict, "admin.bill.regenerate.status_conflict"},
		{"internal", errors.New("db: down"), http.StatusInternalServerError, "admin.bill.regenerate.internal"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := &mockBillService{
				regenerateFunc: func(c *gin.Context, billID int64, amounts mallRepo.BillRegenAmounts, remark string) error {
					return tc.err
				},
			}
			h := NewMallBillHandler(svc)
			code, body := doSettlementJSON(t, http.MethodPost, "/admin/bill/regenerate", map[string]any{
				"id": 77, "total_amount": 1,
			}, func(r *gin.Engine) {
				r.POST("/admin/bill/regenerate", h.Regenerate)
			}, false)
			if code != tc.wantCode {
				t.Errorf("status = %d, want %d", code, tc.wantCode)
			}
			if body["code"] != tc.wantStr {
				t.Errorf("code = %v, want %v", body["code"], tc.wantStr)
			}
		})
	}
}

// =============================================================================
// MarkPaid
// =============================================================================

func TestBillMarkPaid_HappyPath(t *testing.T) {
	svc := &mockBillService{}
	h := NewMallBillHandler(svc)

	code, body := doSettlementJSON(t, http.MethodPost, "/admin/bill/mark-paid", map[string]any{"id": 77},
		func(r *gin.Engine) {
			r.POST("/admin/bill/mark-paid", h.MarkPaid)
		}, true)

	if code != http.StatusOK {
		t.Fatalf("status = %d; body=%v", code, body)
	}
	if svc.lastMarkID != 77 || svc.lastMarkAdminID != 42 {
		t.Errorf("mark args = %d/%d, want 77/42", svc.lastMarkID, svc.lastMarkAdminID)
	}
}

func TestBillMarkPaid_NoAdmin_401(t *testing.T) {
	svc := &mockBillService{}
	h := NewMallBillHandler(svc)

	code, body := doSettlementJSON(t, http.MethodPost, "/admin/bill/mark-paid", map[string]any{"id": 77},
		func(r *gin.Engine) {
			r.POST("/admin/bill/mark-paid", h.MarkPaid)
		}, false)
	if code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", code)
	}
	if body["code"] != "admin.bill.mark_paid.unauthorized" {
		t.Errorf("code = %v", body["code"])
	}
	if svc.lastMarkID != 0 {
		t.Errorf("MarkPaid should NOT be called")
	}
}

func TestBillMarkPaid_MissingID_400(t *testing.T) {
	svc := &mockBillService{}
	h := NewMallBillHandler(svc)

	code, _ := doSettlementJSON(t, http.MethodPost, "/admin/bill/mark-paid", map[string]any{},
		func(r *gin.Engine) {
			r.POST("/admin/bill/mark-paid", h.MarkPaid)
		}, true)
	if code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", code)
	}
	if svc.lastMarkID != 0 {
		t.Errorf("MarkPaid should NOT be called")
	}
}

func TestBillMarkPaid_ServiceErrors(t *testing.T) {
	cases := []struct {
		name     string
		err      error
		wantCode int
		wantStr  string
	}{
		{"not_found", mallSvc.ErrBillNotFound, http.StatusNotFound, "admin.bill.mark_paid.not_found"},
		{"invalid_transition", state.ErrInvalidStateTransition, http.StatusConflict, "admin.bill.mark_paid.invalid_transition"},
		{"status_conflict", mallSvc.ErrBillStatusConflict, http.StatusConflict, "admin.bill.mark_paid.status_conflict"},
		{"box_lifecycle_conflict", mallSvc.ErrBoxLifecycleConflict, http.StatusConflict, "admin.bill.mark_paid.box_lifecycle_conflict"},
		{"insufficient_balance", mallSvc.ErrInsufficientSupplierBalance, http.StatusUnprocessableEntity, "admin.bill.mark_paid.insufficient_balance"},
		{"internal", errors.New("db: down"), http.StatusInternalServerError, "admin.bill.mark_paid.internal"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := &mockBillService{
				markPaidFunc: func(c *gin.Context, billID int64, adminID int64) error {
					return tc.err
				},
			}
			h := NewMallBillHandler(svc)
			code, body := doSettlementJSON(t, http.MethodPost, "/admin/bill/mark-paid", map[string]any{"id": 77},
				func(r *gin.Engine) {
					r.POST("/admin/bill/mark-paid", h.MarkPaid)
				}, true)
			if code != tc.wantCode {
				t.Errorf("status = %d, want %d", code, tc.wantCode)
			}
			if body["code"] != tc.wantStr {
				t.Errorf("code = %v, want %v", body["code"], tc.wantStr)
			}
		})
	}
}
