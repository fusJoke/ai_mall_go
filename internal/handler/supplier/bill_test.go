package supplier

import (
	"errors"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"

	"ai-go-mall/internal/domain/state"
	"ai-go-mall/internal/model/mall"
	"ai-go-mall/internal/repository"
	supplierSvc "ai-go-mall/internal/service/supplier"
)

// mockBillService 实现 supplierSvc.BillService（按需 override，默认零值返回）。
type mockBillService struct {
	listFunc    func(c *gin.Context, supplierID int64, status mall.MallSettlementBillStatus, opts repository.ListOptions) ([]mall.MallSettlementBill, int64, error)
	detailFunc  func(c *gin.Context, supplierID, billID int64) (*mall.MallSettlementBill, []mall.MallSettlementBillItem, error)
	confirmFunc func(c *gin.Context, supplierID, billID int64) error
	rejectFunc  func(c *gin.Context, supplierID, billID int64, reason string) error

	lastSupplierID   int64
	lastListStatus   mall.MallSettlementBillStatus
	lastListOpts     repository.ListOptions
	lastDetailID     int64
	lastConfirmID    int64
	lastRejectID     int64
	lastRejectReason string
}

func (m *mockBillService) List(c *gin.Context, supplierID int64, status mall.MallSettlementBillStatus, opts repository.ListOptions) ([]mall.MallSettlementBill, int64, error) {
	m.lastSupplierID = supplierID
	m.lastListStatus = status
	m.lastListOpts = opts
	if m.listFunc != nil {
		return m.listFunc(c, supplierID, status, opts)
	}
	return []mall.MallSettlementBill{{ID: 1, SupplierID: supplierID}}, 1, nil
}

func (m *mockBillService) Detail(c *gin.Context, supplierID, billID int64) (*mall.MallSettlementBill, []mall.MallSettlementBillItem, error) {
	m.lastSupplierID = supplierID
	m.lastDetailID = billID
	if m.detailFunc != nil {
		return m.detailFunc(c, supplierID, billID)
	}
	return &mall.MallSettlementBill{ID: billID, SupplierID: supplierID}, nil, nil
}

func (m *mockBillService) Confirm(c *gin.Context, supplierID, billID int64) error {
	m.lastSupplierID = supplierID
	m.lastConfirmID = billID
	if m.confirmFunc != nil {
		return m.confirmFunc(c, supplierID, billID)
	}
	return nil
}

func (m *mockBillService) Reject(c *gin.Context, supplierID, billID int64, reason string) error {
	m.lastSupplierID = supplierID
	m.lastRejectID = billID
	m.lastRejectReason = reason
	if m.rejectFunc != nil {
		return m.rejectFunc(c, supplierID, billID, reason)
	}
	return nil
}

var _ supplierSvc.BillService = (*mockBillService)(nil)

func TestBillList_HappyPath(t *testing.T) {
	svc := &mockBillService{
		listFunc: func(c *gin.Context, supplierID int64, status mall.MallSettlementBillStatus, opts repository.ListOptions) ([]mall.MallSettlementBill, int64, error) {
			if supplierID != 10 {
				t.Errorf("supplierID = %d, want 10", supplierID)
			}
			return []mall.MallSettlementBill{{ID: 1, SupplierID: supplierID}}, 1, nil
		},
	}
	h := NewBillHandler(svc)

	code, resp := doSupplierJSON(t, http.MethodGet, "/supplier/bills/list?status=confirmed&page=2&page_size=5", nil, func(r *gin.Engine) {
		r.GET("/supplier/bills/list", withSupplier(10), h.List)
	})
	if code != http.StatusOK {
		t.Fatalf("status = %d; body=%v", code, resp)
	}
	if svc.lastListStatus != mall.BillStatusConfirmed {
		t.Errorf("status = %q, want confirmed", svc.lastListStatus)
	}
	if svc.lastListOpts.Page != 2 || svc.lastListOpts.PageSize != 5 {
		t.Errorf("opts = %+v, want page=2 page_size=5", svc.lastListOpts)
	}
	if resp["total"] != float64(1) {
		t.Errorf("total = %v", resp["total"])
	}
}

func TestBillList_Defaults(t *testing.T) {
	svc := &mockBillService{}
	h := NewBillHandler(svc)

	code, _ := doSupplierJSON(t, http.MethodGet, "/supplier/bills/list", nil, func(r *gin.Engine) {
		r.GET("/supplier/bills/list", withSupplier(10), h.List)
	})
	if code != http.StatusOK {
		t.Fatalf("status = %d", code)
	}
	// status 缺省 = 全部 tab（不筛）。
	if svc.lastListStatus != "" {
		t.Errorf("status = %q, want empty", svc.lastListStatus)
	}
	if svc.lastListOpts.Page != 1 || svc.lastListOpts.PageSize != 20 {
		t.Errorf("opts = %+v, want page=1 page_size=20", svc.lastListOpts)
	}
}

func TestBillList_InvalidStatus_400(t *testing.T) {
	svc := &mockBillService{}
	h := NewBillHandler(svc)

	code, body := doSupplierJSON(t, http.MethodGet, "/supplier/bills/list?status=bogus", nil, func(r *gin.Engine) {
		r.GET("/supplier/bills/list", withSupplier(10), h.List)
	})
	if code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", code)
	}
	if body["code"] != "bill.invalid_input" {
		t.Errorf("code = %v", body["code"])
	}
}

func TestBillList_NoSupplier_401(t *testing.T) {
	svc := &mockBillService{}
	h := NewBillHandler(svc)

	code, body := doSupplierJSON(t, http.MethodGet, "/supplier/bills/list", nil, func(r *gin.Engine) {
		r.GET("/supplier/bills/list", withSupplier(0), h.List)
	})
	if code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", code)
	}
	if body["code"] != "bill.unauthorized" {
		t.Errorf("code = %v", body["code"])
	}
}

func TestBillDetail_HappyPath(t *testing.T) {
	svc := &mockBillService{
		detailFunc: func(c *gin.Context, supplierID, billID int64) (*mall.MallSettlementBill, []mall.MallSettlementBillItem, error) {
			if supplierID != 10 || billID != 77 {
				t.Errorf("args = %d/%d, want 10/77", supplierID, billID)
			}
			bill := &mall.MallSettlementBill{ID: 77, SupplierID: supplierID}
			items := []mall.MallSettlementBillItem{{DrawOrderID: 9, PayoutAmount: 90}}
			return bill, items, nil
		},
	}
	h := NewBillHandler(svc)

	code, resp := doSupplierJSON(t, http.MethodGet, "/supplier/bills/detail?id=77", nil, func(r *gin.Engine) {
		r.GET("/supplier/bills/detail", withSupplier(10), h.Detail)
	})
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
	h := NewBillHandler(svc)

	code, body := doSupplierJSON(t, http.MethodGet, "/supplier/bills/detail", nil, func(r *gin.Engine) {
		r.GET("/supplier/bills/detail", withSupplier(10), h.Detail)
	})
	if code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", code)
	}
	if body["code"] != "bill.invalid_input" {
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
		{"not_found", supplierSvc.ErrBillNotFound, http.StatusNotFound, "bill.not_found"},
		{"forbidden", supplierSvc.ErrBillForbidden, http.StatusForbidden, "bill.forbidden"},
		{"internal", errors.New("db: down"), http.StatusInternalServerError, "bill.internal"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := &mockBillService{
				detailFunc: func(c *gin.Context, supplierID, billID int64) (*mall.MallSettlementBill, []mall.MallSettlementBillItem, error) {
					return nil, nil, tc.err
				},
			}
			h := NewBillHandler(svc)
			code, body := doSupplierJSON(t, http.MethodGet, "/supplier/bills/detail?id=77", nil, func(r *gin.Engine) {
				r.GET("/supplier/bills/detail", withSupplier(10), h.Detail)
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

func TestBillConfirm_HappyPath(t *testing.T) {
	svc := &mockBillService{}
	h := NewBillHandler(svc)

	code, body := doSupplierJSON(t, http.MethodPost, "/supplier/bills/confirm", map[string]any{"id": 77}, func(r *gin.Engine) {
		r.POST("/supplier/bills/confirm", withSupplier(10), h.Confirm)
	})
	if code != http.StatusOK {
		t.Fatalf("status = %d; body=%v", code, body)
	}
	if svc.lastConfirmID != 77 || svc.lastSupplierID != 10 {
		t.Errorf("confirm args = %d/%d", svc.lastSupplierID, svc.lastConfirmID)
	}
}

func TestBillConfirm_MissingID_400(t *testing.T) {
	svc := &mockBillService{}
	h := NewBillHandler(svc)

	code, _ := doSupplierJSON(t, http.MethodPost, "/supplier/bills/confirm", map[string]any{}, func(r *gin.Engine) {
		r.POST("/supplier/bills/confirm", withSupplier(10), h.Confirm)
	})
	if code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", code)
	}
	if svc.lastConfirmID != 0 {
		t.Errorf("Confirm should NOT be called")
	}
}

func TestBillConfirm_NoSupplier_401(t *testing.T) {
	svc := &mockBillService{}
	h := NewBillHandler(svc)

	code, body := doSupplierJSON(t, http.MethodPost, "/supplier/bills/confirm", map[string]any{"id": 77}, func(r *gin.Engine) {
		r.POST("/supplier/bills/confirm", withSupplier(0), h.Confirm)
	})
	if code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", code)
	}
	if body["code"] != "bill.unauthorized" {
		t.Errorf("code = %v", body["code"])
	}
}

func TestBillConfirm_ServiceErrors(t *testing.T) {
	cases := []struct {
		name     string
		err      error
		wantCode int
		wantStr  string
	}{
		{"not_found", supplierSvc.ErrBillNotFound, http.StatusNotFound, "bill.not_found"},
		{"forbidden", supplierSvc.ErrBillForbidden, http.StatusForbidden, "bill.forbidden"},
		{"invalid_transition", state.ErrInvalidStateTransition, http.StatusConflict, "bill.invalid_transition"},
		{"status_conflict", supplierSvc.ErrBillStatusConflict, http.StatusConflict, "bill.status_conflict"},
		{"internal", errors.New("db: down"), http.StatusInternalServerError, "bill.internal"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := &mockBillService{
				confirmFunc: func(c *gin.Context, supplierID, billID int64) error {
					return tc.err
				},
			}
			h := NewBillHandler(svc)
			code, body := doSupplierJSON(t, http.MethodPost, "/supplier/bills/confirm", map[string]any{"id": 77}, func(r *gin.Engine) {
				r.POST("/supplier/bills/confirm", withSupplier(10), h.Confirm)
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

func TestBillReject_HappyPath(t *testing.T) {
	svc := &mockBillService{}
	h := NewBillHandler(svc)

	code, _ := doSupplierJSON(t, http.MethodPost, "/supplier/bills/reject", map[string]any{
		"id": 77, "reason": "  金额核算有误  ",
	}, func(r *gin.Engine) {
		r.POST("/supplier/bills/reject", withSupplier(10), h.Reject)
	})
	if code != http.StatusOK {
		t.Fatalf("status = %d", code)
	}
	// handler 透传原文，trim 是 service 层职责（单一校验点）。
	if svc.lastRejectID != 77 || svc.lastRejectReason != "  金额核算有误  " {
		t.Errorf("reject args = %d/%q", svc.lastRejectID, svc.lastRejectReason)
	}
}

func TestBillReject_ReasonRequired_422(t *testing.T) {
	svc := &mockBillService{
		rejectFunc: func(c *gin.Context, supplierID, billID int64, reason string) error {
			return supplierSvc.ErrRejectReasonRequired
		},
	}
	h := NewBillHandler(svc)

	// 空白 reason 透传给 service，由 service 校验 → 422。
	code, body := doSupplierJSON(t, http.MethodPost, "/supplier/bills/reject", map[string]any{
		"id": 77, "reason": "   ",
	}, func(r *gin.Engine) {
		r.POST("/supplier/bills/reject", withSupplier(10), h.Reject)
	})
	if code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", code)
	}
	if body["code"] != "bill.reject_reason_required" {
		t.Errorf("code = %v", body["code"])
	}
}

func TestBillReject_ServiceErrors(t *testing.T) {
	cases := []struct {
		name     string
		err      error
		wantCode int
		wantStr  string
	}{
		{"not_found", supplierSvc.ErrBillNotFound, http.StatusNotFound, "bill.not_found"},
		{"forbidden", supplierSvc.ErrBillForbidden, http.StatusForbidden, "bill.forbidden"},
		{"invalid_transition", state.ErrInvalidStateTransition, http.StatusConflict, "bill.invalid_transition"},
		{"status_conflict", supplierSvc.ErrBillStatusConflict, http.StatusConflict, "bill.status_conflict"},
		{"internal", errors.New("db: down"), http.StatusInternalServerError, "bill.internal"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := &mockBillService{
				rejectFunc: func(c *gin.Context, supplierID, billID int64, reason string) error {
					return tc.err
				},
			}
			h := NewBillHandler(svc)
			code, body := doSupplierJSON(t, http.MethodPost, "/supplier/bills/reject", map[string]any{
				"id": 77, "reason": "金额有误",
			}, func(r *gin.Engine) {
				r.POST("/supplier/bills/reject", withSupplier(10), h.Reject)
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
