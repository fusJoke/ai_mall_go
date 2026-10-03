package supplier

import (
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"ai-go-mall/internal/domain/state"
	"ai-go-mall/internal/model/mall"
	"ai-go-mall/internal/repository"
	mallRepo "ai-go-mall/internal/repository/mall"
)

// =============================================================================
// helpers / fakes
// =============================================================================

func newBillTestCtx() *gin.Context {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", "/", nil)
	return c
}

// fakeBillRepo 嵌入 nil 接口 + 按需 override（未 mock 方法被调 → panic = 测试红）。
type fakeBillRepo struct {
	mallRepo.SettlementBillRepository

	getByIDFunc           func(c *gin.Context, id int64) (*mall.MallSettlementBill, error)
	getByIDWithItemsFunc  func(c *gin.Context, id int64) (*mall.MallSettlementBill, []mall.MallSettlementBillItem, error)
	listBySupplierFunc    func(c *gin.Context, supplierID int64, status mall.MallSettlementBillStatus, opts repository.ListOptions) ([]mall.MallSettlementBill, int64, error)
	updateStatusFunc      func(c *gin.Context, id int64, from, to mall.MallSettlementBillStatus, extra map[string]any) (int64, error)

	listCalls              int
	lastListSupplierID     int64
	lastListStatus         mall.MallSettlementBillStatus
	updateCalls            int
	lastUpdateID           int64
	lastUpdateFrom, lastTo mall.MallSettlementBillStatus
	lastUpdateExtra        map[string]any
}

func (f *fakeBillRepo) GetByID(c *gin.Context, id int64) (*mall.MallSettlementBill, error) {
	if f.getByIDFunc != nil {
		return f.getByIDFunc(c, id)
	}
	return nil, gorm.ErrRecordNotFound
}

func (f *fakeBillRepo) GetByIDWithItems(c *gin.Context, id int64) (*mall.MallSettlementBill, []mall.MallSettlementBillItem, error) {
	if f.getByIDWithItemsFunc != nil {
		return f.getByIDWithItemsFunc(c, id)
	}
	return nil, nil, mallRepo.ErrBillNotFound
}

func (f *fakeBillRepo) ListBySupplier(c *gin.Context, supplierID int64, status mall.MallSettlementBillStatus, opts repository.ListOptions) ([]mall.MallSettlementBill, int64, error) {
	f.listCalls++
	f.lastListSupplierID = supplierID
	f.lastListStatus = status
	if f.listBySupplierFunc != nil {
		return f.listBySupplierFunc(c, supplierID, status, opts)
	}
	return nil, 0, nil
}

func (f *fakeBillRepo) UpdateStatus(c *gin.Context, id int64, from, to mall.MallSettlementBillStatus, extra map[string]any) (int64, error) {
	f.updateCalls++
	f.lastUpdateID = id
	f.lastUpdateFrom = from
	f.lastTo = to
	f.lastUpdateExtra = extra
	if f.updateStatusFunc != nil {
		return f.updateStatusFunc(c, id, from, to, extra)
	}
	return 1, nil
}

// generatedBill 返回 generated 态账单 fixture（supplier 3）。
func generatedBill() *mall.MallSettlementBill {
	return &mall.MallSettlementBill{ID: 77, BlindBoxID: 9, SupplierID: 3, Status: mall.BillStatusGenerated}
}

func newSupBillSvc(b *fakeBillRepo) BillService {
	return NewBillService(BillServiceDeps{BillRepo: b})
}

// =============================================================================
// List
// =============================================================================

// TestSupBillList_Delegates 验证 supplierID / status 透传。
func TestSupBillList_Delegates(t *testing.T) {
	b := &fakeBillRepo{}
	got, total, err := newSupBillSvc(b).List(newBillTestCtx(), 3, mall.BillStatusGenerated, repository.ListOptions{Page: 1, PageSize: 20})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if b.listCalls != 1 || b.lastListSupplierID != 3 || b.lastListStatus != mall.BillStatusGenerated {
		t.Errorf("list args = calls:%d supplier:%d status:%s, want 1/3/generated", b.listCalls, b.lastListSupplierID, b.lastListStatus)
	}
	if got != nil || total != 0 {
		t.Errorf("result = %v/%d", got, total)
	}
}

// TestSupBillList_InvalidSupplier supplierID<=0 短路。
func TestSupBillList_InvalidSupplier(t *testing.T) {
	b := &fakeBillRepo{}
	if _, _, err := newSupBillSvc(b).List(newBillTestCtx(), 0, "", repository.ListOptions{}); !errors.Is(err, ErrBillNotFound) {
		t.Errorf("List(supplierID=0) = %v, want ErrBillNotFound", err)
	}
	if b.listCalls != 0 {
		t.Errorf("listCalls = %d, want 0", b.listCalls)
	}
}

// =============================================================================
// Detail
// =============================================================================

// TestSupBillDetail_Happy 自家账单 → 账单 + 明细。
func TestSupBillDetail_Happy(t *testing.T) {
	bill := generatedBill()
	b := &fakeBillRepo{getByIDWithItemsFunc: func(c *gin.Context, id int64) (*mall.MallSettlementBill, []mall.MallSettlementBillItem, error) {
		return bill, []mall.MallSettlementBillItem{{ID: 1, BillID: 77, DrawOrderID: 101}}, nil
	}}

	gotBill, gotItems, err := newSupBillSvc(b).Detail(newBillTestCtx(), 3, 77)
	if err != nil {
		t.Fatalf("Detail: %v", err)
	}
	if gotBill != bill || len(gotItems) != 1 {
		t.Errorf("detail = %+v / %d items", gotBill, len(gotItems))
	}
}

// TestSupBillDetail_NotFound 账单不存在 → 404。
func TestSupBillDetail_NotFound(t *testing.T) {
	b := &fakeBillRepo{} // GetByIDWithItems → ErrBillNotFound
	if _, _, err := newSupBillSvc(b).Detail(newBillTestCtx(), 3, 999); !errors.Is(err, ErrBillNotFound) {
		t.Errorf("Detail(not found) = %v, want ErrBillNotFound", err)
	}
}

// TestSupBillDetail_Forbidden 他人账单 → 403（B 端越权可识别，不模糊成 404）。
func TestSupBillDetail_Forbidden(t *testing.T) {
	b := &fakeBillRepo{getByIDWithItemsFunc: func(c *gin.Context, id int64) (*mall.MallSettlementBill, []mall.MallSettlementBillItem, error) {
		return generatedBill(), nil, nil // bill.SupplierID=3
	}}
	if _, _, err := newSupBillSvc(b).Detail(newBillTestCtx(), 4, 77); !errors.Is(err, ErrBillForbidden) {
		t.Errorf("Detail(other's bill) = %v, want ErrBillForbidden", err)
	}
}

// =============================================================================
// Confirm
// =============================================================================

// TestSupBillConfirm_Happy generated → confirmed CAS。
func TestSupBillConfirm_Happy(t *testing.T) {
	b := &fakeBillRepo{getByIDFunc: func(c *gin.Context, id int64) (*mall.MallSettlementBill, error) {
		return generatedBill(), nil
	}}

	if err := newSupBillSvc(b).Confirm(newBillTestCtx(), 3, 77); err != nil {
		t.Fatalf("Confirm: %v", err)
	}
	if b.updateCalls != 1 || b.lastUpdateID != 77 ||
		b.lastUpdateFrom != mall.BillStatusGenerated || b.lastTo != mall.BillStatusConfirmed {
		t.Errorf("update = calls:%d id:%d %s->%s, want 1/77/generated->confirmed",
			b.updateCalls, b.lastUpdateID, b.lastUpdateFrom, b.lastTo)
	}
	if b.lastUpdateExtra != nil {
		t.Errorf("extra = %v, want nil (pure status CAS)", b.lastUpdateExtra)
	}
}

// TestSupBillConfirm_WrongState 非 generated 态 → 状态机拒绝。
func TestSupBillConfirm_WrongState(t *testing.T) {
	bill := generatedBill()
	bill.Status = mall.BillStatusConfirmed
	b := &fakeBillRepo{getByIDFunc: func(c *gin.Context, id int64) (*mall.MallSettlementBill, error) {
		return bill, nil
	}}

	err := newSupBillSvc(b).Confirm(newBillTestCtx(), 3, 77)
	if !errors.Is(err, state.ErrInvalidStateTransition) {
		t.Errorf("Confirm(confirmed bill) = %v, want ErrInvalidStateTransition", err)
	}
	if b.updateCalls != 0 {
		t.Errorf("updateCalls = %d, want 0 (state check must short-circuit)", b.updateCalls)
	}
}

// TestSupBillConfirm_Forbidden 他人账单 → 403。
func TestSupBillConfirm_Forbidden(t *testing.T) {
	b := &fakeBillRepo{getByIDFunc: func(c *gin.Context, id int64) (*mall.MallSettlementBill, error) {
		return generatedBill(), nil // supplier 3
	}}
	if err := newSupBillSvc(b).Confirm(newBillTestCtx(), 4, 77); !errors.Is(err, ErrBillForbidden) {
		t.Errorf("Confirm(other's bill) = %v, want ErrBillForbidden", err)
	}
	if b.updateCalls != 0 {
		t.Errorf("updateCalls = %d, want 0 (ownership check must short-circuit)", b.updateCalls)
	}
}

// TestSupBillConfirm_CASMisss 并发抢先 → ErrBillStatusConflict。
func TestSupBillConfirm_CASMisss(t *testing.T) {
	b := &fakeBillRepo{
		getByIDFunc: func(c *gin.Context, id int64) (*mall.MallSettlementBill, error) {
			return generatedBill(), nil
		},
		updateStatusFunc: func(c *gin.Context, id int64, from, to mall.MallSettlementBillStatus, extra map[string]any) (int64, error) {
			return 0, nil // CAS 未命中
		},
	}
	if err := newSupBillSvc(b).Confirm(newBillTestCtx(), 3, 77); !errors.Is(err, ErrBillStatusConflict) {
		t.Errorf("Confirm CAS miss = %v, want ErrBillStatusConflict", err)
	}
}

// =============================================================================
// Reject
// =============================================================================

// TestSupBillReject_Happy generated → rejected + reject_reason 落库。
func TestSupBillReject_Happy(t *testing.T) {
	b := &fakeBillRepo{getByIDFunc: func(c *gin.Context, id int64) (*mall.MallSettlementBill, error) {
		return generatedBill(), nil
	}}

	if err := newSupBillSvc(b).Reject(newBillTestCtx(), 3, 77, "  金额核算有误  "); err != nil {
		t.Fatalf("Reject: %v", err)
	}
	if b.lastTo != mall.BillStatusRejected {
		t.Errorf("to = %s, want rejected", b.lastTo)
	}
	if v, ok := b.lastUpdateExtra["reject_reason"]; !ok || v != "金额核算有误" {
		t.Errorf("extra[reject_reason] = %v (ok=%v), want trimmed reason", v, ok)
	}
}

// TestSupBillReject_EmptyReason 原因必填（纯空白 → 422）。
func TestSupBillReject_EmptyReason(t *testing.T) {
	b := &fakeBillRepo{}
	for _, reason := range []string{"", "   "} {
		err := newSupBillSvc(b).Reject(newBillTestCtx(), 3, 77, reason)
		if !errors.Is(err, ErrRejectReasonRequired) {
			t.Errorf("Reject(reason=%q) = %v, want ErrRejectReasonRequired", reason, err)
		}
	}
	if b.updateCalls != 0 {
		t.Errorf("updateCalls = %d, want 0", b.updateCalls)
	}
}

// TestSupBillReject_WrongState 非 generated 态 → 状态机拒绝（rejected/confirmed/paid 不可驳回）。
func TestSupBillReject_WrongState(t *testing.T) {
	for _, st := range []mall.MallSettlementBillStatus{mall.BillStatusConfirmed, mall.BillStatusRejected, mall.BillStatusPaid} {
		bill := generatedBill()
		bill.Status = st
		b := &fakeBillRepo{getByIDFunc: func(c *gin.Context, id int64) (*mall.MallSettlementBill, error) {
			return bill, nil
		}}
		if err := newSupBillSvc(b).Reject(newBillTestCtx(), 3, 77, "x"); !errors.Is(err, state.ErrInvalidStateTransition) {
			t.Errorf("Reject(%s bill) = %v, want ErrInvalidStateTransition", st, err)
		}
	}
}

// TestSupBillReject_CASMisss 并发抢先 → ErrBillStatusConflict。
func TestSupBillReject_CASMisss(t *testing.T) {
	b := &fakeBillRepo{
		getByIDFunc: func(c *gin.Context, id int64) (*mall.MallSettlementBill, error) {
			return generatedBill(), nil
		},
		updateStatusFunc: func(c *gin.Context, id int64, from, to mall.MallSettlementBillStatus, extra map[string]any) (int64, error) {
			return 0, nil
		},
	}
	if err := newSupBillSvc(b).Reject(newBillTestCtx(), 3, 77, "x"); !errors.Is(err, ErrBillStatusConflict) {
		t.Errorf("Reject CAS miss = %v, want ErrBillStatusConflict", err)
	}
}
