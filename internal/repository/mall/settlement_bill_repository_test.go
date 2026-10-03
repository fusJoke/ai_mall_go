package mall

import (
	"errors"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/go-sql-driver/mysql"
	"gorm.io/gorm"

	"ai-go-mall/internal/model/mall"
	"ai-go-mall/internal/repository"
)

// 注意：newMockContext 用 &gorm.Config{} 打开（SkipDefaultTransaction=false），
// 因此所有 chain 写操作（Create / Updates / Update）都会被 GORM 自动包上
// BEGIN ... COMMIT。mock 序列须按此预期；原生 Exec 不包。真实生产路径中
// CAS 操作若运行在 txCtx 上（调用方显式 RunInTx），GORM 检测到已有事务会跳过
// 自动包装，单条 UPDATE 语义不变。

// =============================================================================
// SettlementBillRepository — CreateWithItems
// =============================================================================

// TestSettlementBillRepository_CreateWithItems_WithItems 验证 INSERT bill + INSERT items。
//
// 无外层事务时两次 Create 各自包默认事务（Begin/Exec/Commit ×2）；
// 真实调用路径在 draw.go 事务尾部（txCtx），GORM 复用同一事务不再 Begin。
func TestSettlementBillRepository_CreateWithItems_WithItems(t *testing.T) {
	c, mock := newMockContext(t)

	// 第 1 个自动事务：INSERT bill
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO")).
		WillReturnResult(sqlmock.NewResult(7, 1))
	mock.ExpectCommit()
	// 第 2 个自动事务：INSERT items
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO")).
		WillReturnResult(sqlmock.NewResult(1, 2))
	mock.ExpectCommit()

	bill := &mall.MallSettlementBill{
		BlindBoxID:       9,
		SupplierID:       3,
		TotalAmount:      300,
		CommissionRate:   0.1,
		CommissionAmount: 30,
		PayoutAmount:     270,
		Status:           mall.BillStatusGenerated,
	}
	items := []mall.MallSettlementBillItem{
		{DrawOrderID: 101, Amount: 100, CommissionAmount: 10, PayoutAmount: 90},
		{DrawOrderID: 102, Amount: 200, CommissionAmount: 20, PayoutAmount: 180},
	}
	if err := NewSettlementBillRepository().CreateWithItems(c, bill, items); err != nil {
		t.Fatalf("CreateWithItems: %v", err)
	}
	if bill.ID != 7 {
		t.Errorf("bill.ID = %d, want 7", bill.ID)
	}
	for i, it := range items {
		if it.BillID != 7 {
			t.Errorf("items[%d].BillID = %d, want 7", i, it.BillID)
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}

// TestSettlementBillRepository_CreateWithItems_Nil 验证 nil 短路。
func TestSettlementBillRepository_CreateWithItems_Nil(t *testing.T) {
	c, mock := newMockContext(t)

	if err := NewSettlementBillRepository().CreateWithItems(c, nil, nil); err == nil {
		t.Errorf("CreateWithItems(nil) = nil, want err")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v (expected zero SQL calls)", err)
	}
}

// TestSettlementBillRepository_CreateWithItems_Conflict 验证 UK 冲突 → ErrBillConflict。
func TestSettlementBillRepository_CreateWithItems_Conflict(t *testing.T) {
	c, mock := newMockContext(t)

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO")).
		WillReturnError(&mysql.MySQLError{Number: 1062, Message: "Duplicate entry"})
	mock.ExpectRollback()

	bill := &mall.MallSettlementBill{BlindBoxID: 9, SupplierID: 3}
	err := NewSettlementBillRepository().CreateWithItems(c, bill, nil)
	if !errors.Is(err, ErrBillConflict) {
		t.Errorf("CreateWithItems conflict = %v, want ErrBillConflict", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}

// =============================================================================
// SettlementBillRepository — GetByIDWithItems / GetByBoxID
// =============================================================================

// TestSettlementBillRepository_GetByIDWithItems 验证主键 + items 双查。
func TestSettlementBillRepository_GetByIDWithItems(t *testing.T) {
	c, mock := newMockContext(t)

	// 1. SELECT bill（First 带 LIMIT 参数，宽匹配）
	mock.ExpectQuery(regexp.QuoteMeta("SELECT")).
		WillReturnRows(sqlmock.NewRows([]string{"id", "blind_box_id", "supplier_id", "status"}).
			AddRow(int64(7), int64(9), int64(3), "generated"))
	// 2. SELECT items（Find 无 LIMIT，单参数）
	mock.ExpectQuery(regexp.QuoteMeta("SELECT")).
		WithArgs(int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "bill_id", "draw_order_id"}).
			AddRow(int64(1), int64(7), int64(101)).
			AddRow(int64(2), int64(7), int64(102)))

	bill, items, err := NewSettlementBillRepository().GetByIDWithItems(c, 7)
	if err != nil {
		t.Fatalf("GetByIDWithItems: %v", err)
	}
	if bill == nil || bill.BlindBoxID != 9 {
		t.Errorf("bill = %+v", bill)
	}
	if len(items) != 2 {
		t.Errorf("len(items) = %d, want 2", len(items))
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}

// TestSettlementBillRepository_GetByIDWithItems_NotFound 验证不存在 → ErrBillNotFound。
func TestSettlementBillRepository_GetByIDWithItems_NotFound(t *testing.T) {
	c, mock := newMockContext(t)

	mock.ExpectQuery(regexp.QuoteMeta("SELECT")).
		WillReturnError(gorm.ErrRecordNotFound)

	_, _, err := NewSettlementBillRepository().GetByIDWithItems(c, 999)
	if !errors.Is(err, ErrBillNotFound) {
		t.Errorf("GetByIDWithItems not found = %v, want ErrBillNotFound", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}

// TestSettlementBillRepository_GetByBoxID_Found 验证按商品查账单。
//
// First 渲染 ... ORDER BY id LIMIT ? → 参数 (blind_box_id, limit)。
func TestSettlementBillRepository_GetByBoxID_Found(t *testing.T) {
	c, mock := newMockContext(t)

	mock.ExpectQuery(regexp.QuoteMeta("SELECT")).
		WithArgs(int64(9), int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "blind_box_id", "status"}).
			AddRow(int64(7), int64(9), "generated"))

	bill, err := NewSettlementBillRepository().GetByBoxID(c, 9)
	if err != nil {
		t.Fatalf("GetByBoxID: %v", err)
	}
	if bill == nil || bill.ID != 7 {
		t.Errorf("bill = %+v", bill)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}

// TestSettlementBillRepository_GetByBoxID_NotFound 验证不存在 → ErrBillNotFound。
func TestSettlementBillRepository_GetByBoxID_NotFound(t *testing.T) {
	c, mock := newMockContext(t)

	mock.ExpectQuery(regexp.QuoteMeta("SELECT")).
		WithArgs(int64(9), int64(1)).
		WillReturnError(gorm.ErrRecordNotFound)

	_, err := NewSettlementBillRepository().GetByBoxID(c, 9)
	if !errors.Is(err, ErrBillNotFound) {
		t.Errorf("GetByBoxID not found = %v, want ErrBillNotFound", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}

// =============================================================================
// SettlementBillRepository — ListBySupplier / ListAdmin
// =============================================================================

// TestSettlementBillRepository_ListBySupplier 验证分页查（status 空不筛）。
func TestSettlementBillRepository_ListBySupplier(t *testing.T) {
	c, mock := newMockContext(t)

	mock.ExpectQuery(`(?i)SELECT count`).
		WithArgs(int64(3)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(2)))
	mock.ExpectQuery(`(?i)SELECT`).
		WithArgs(int64(3), sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id", "supplier_id"}).
			AddRow(int64(2), int64(3)).
			AddRow(int64(1), int64(3)))

	items, total, err := NewSettlementBillRepository().ListBySupplier(c, 3, "", repository.ListOptions{Page: 1, PageSize: 20})
	if err != nil {
		t.Fatalf("ListBySupplier: %v", err)
	}
	if total != 2 || len(items) != 2 {
		t.Errorf("total=%d items=%d, want 2/2", total, len(items))
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}

// TestSettlementBillRepository_ListBySupplier_WithStatus 验证 status 筛选参数。
func TestSettlementBillRepository_ListBySupplier_WithStatus(t *testing.T) {
	c, mock := newMockContext(t)

	mock.ExpectQuery(`(?i)SELECT count`).
		WithArgs(int64(3), "generated").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(1)))
	mock.ExpectQuery(`(?i)SELECT`).
		WithArgs(int64(3), "generated", sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id", "supplier_id", "status"}).
			AddRow(int64(2), int64(3), "generated"))

	items, total, err := NewSettlementBillRepository().ListBySupplier(c, 3, mall.BillStatusGenerated, repository.ListOptions{Page: 1, PageSize: 20})
	if err != nil {
		t.Fatalf("ListBySupplier: %v", err)
	}
	if total != 1 || len(items) != 1 {
		t.Errorf("total=%d items=%d, want 1/1", total, len(items))
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}

// TestSettlementBillRepository_ListBySupplier_Empty 验证 total=0 短路（不发 Find）。
func TestSettlementBillRepository_ListBySupplier_Empty(t *testing.T) {
	c, mock := newMockContext(t)

	mock.ExpectQuery(`(?i)SELECT count`).
		WithArgs(int64(3)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(0)))

	items, total, err := NewSettlementBillRepository().ListBySupplier(c, 3, "", repository.ListOptions{Page: 1, PageSize: 20})
	if err != nil {
		t.Fatalf("ListBySupplier: %v", err)
	}
	if total != 0 || len(items) != 0 {
		t.Errorf("total=%d items=%d, want 0/0", total, len(items))
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}

// TestSettlementBillRepository_ListAdmin_WithFilters 验证 admin 双筛选。
func TestSettlementBillRepository_ListAdmin_WithFilters(t *testing.T) {
	c, mock := newMockContext(t)

	mock.ExpectQuery(`(?i)SELECT count`).
		WithArgs(int64(5), "confirmed").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(1)))
	mock.ExpectQuery(`(?i)SELECT`).
		WithArgs(int64(5), "confirmed", sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id", "supplier_id", "status"}).
			AddRow(int64(7), int64(5), "confirmed"))

	items, total, err := NewSettlementBillRepository().ListAdmin(c, mall.BillStatusConfirmed, 5, repository.ListOptions{Page: 1, PageSize: 20})
	if err != nil {
		t.Fatalf("ListAdmin: %v", err)
	}
	if total != 1 || len(items) != 1 {
		t.Errorf("total=%d items=%d, want 1/1", total, len(items))
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}

// =============================================================================
// SettlementBillRepository — UpdateStatus
// =============================================================================

// TestSettlementBillRepository_UpdateStatus_Happy 验证 CAS WHERE status=from。
//
// GORM 渲染：SET status=?, updated_at=? WHERE id=? AND status=?（map 字母序）。
func TestSettlementBillRepository_UpdateStatus_Happy(t *testing.T) {
	c, mock := newMockContext(t)

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("UPDATE")).
		WithArgs("confirmed", sqlmock.AnyArg(), int64(5), "generated").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	rows, err := NewSettlementBillRepository().UpdateStatus(c, 5, mall.BillStatusGenerated, mall.BillStatusConfirmed, nil)
	if err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}
	if rows != 1 {
		t.Errorf("rows = %d, want 1", rows)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}

// TestSettlementBillRepository_UpdateStatus_ZeroRows 验证 CAS 未命中 → rows=0。
func TestSettlementBillRepository_UpdateStatus_ZeroRows(t *testing.T) {
	c, mock := newMockContext(t)

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("UPDATE")).
		WithArgs("confirmed", sqlmock.AnyArg(), int64(5), "generated").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()

	rows, err := NewSettlementBillRepository().UpdateStatus(c, 5, mall.BillStatusGenerated, mall.BillStatusConfirmed, nil)
	if err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}
	if rows != 0 {
		t.Errorf("rows = %d, want 0 (CAS miss)", rows)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}

// TestSettlementBillRepository_UpdateStatus_WithExtra 验证 extra 追加列（Reject 场景）。
//
// map 字母序：reject_reason < status → SET reject_reason=?, status=?, updated_at=?。
func TestSettlementBillRepository_UpdateStatus_WithExtra(t *testing.T) {
	c, mock := newMockContext(t)

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("UPDATE")).
		WithArgs("金额有误", "rejected", sqlmock.AnyArg(), int64(5), "generated").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	rows, err := NewSettlementBillRepository().UpdateStatus(c, 5, mall.BillStatusGenerated, mall.BillStatusRejected,
		map[string]any{"reject_reason": "金额有误"})
	if err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}
	if rows != 1 {
		t.Errorf("rows = %d, want 1", rows)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}

// =============================================================================
// SettlementBillRepository — Regenerate
// =============================================================================

// TestSettlementBillRepository_Regenerate_Happy 验证重核 UPDATE（宽匹配）。
func TestSettlementBillRepository_Regenerate_Happy(t *testing.T) {
	c, mock := newMockContext(t)

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("UPDATE")).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	rows, err := NewSettlementBillRepository().Regenerate(c, 5, BillRegenAmounts{
		TotalAmount:      10,
		CommissionRate:   0.1,
		CommissionAmount: 1,
		PayoutAmount:     9,
	}, "金额核算修正")
	if err != nil {
		t.Fatalf("Regenerate: %v", err)
	}
	if rows != 1 {
		t.Errorf("rows = %d, want 1", rows)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}

// TestSettlementBillRepository_Regenerate_CasMiss 验证非 rejected 态 → rows=0。
func TestSettlementBillRepository_Regenerate_CasMiss(t *testing.T) {
	c, mock := newMockContext(t)

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("UPDATE")).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()

	rows, err := NewSettlementBillRepository().Regenerate(c, 5, BillRegenAmounts{}, "")
	if err != nil {
		t.Fatalf("Regenerate: %v", err)
	}
	if rows != 0 {
		t.Errorf("rows = %d, want 0 (CAS miss)", rows)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}

// TestSettlementBillRepository_Regenerate_InvalidAmounts 验证负数金额短路。
func TestSettlementBillRepository_Regenerate_InvalidAmounts(t *testing.T) {
	c, mock := newMockContext(t)

	_, err := NewSettlementBillRepository().Regenerate(c, 5, BillRegenAmounts{TotalAmount: -1}, "")
	if err == nil {
		t.Errorf("Regenerate(negative) = nil, want err")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v (expected zero SQL calls)", err)
	}
}

// =============================================================================
// SettlementBillRepository — MarkPaid
// =============================================================================

// TestSettlementBillRepository_MarkPaid_Happy 验证 CAS confirmed → paid。
//
// GORM 渲染：SET paid_at=?, paid_by=?, status=?, updated_at=? WHERE id=? AND status=?。
func TestSettlementBillRepository_MarkPaid_Happy(t *testing.T) {
	c, mock := newMockContext(t)

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("UPDATE")).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), "paid", sqlmock.AnyArg(), int64(5), "confirmed").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	rows, err := NewSettlementBillRepository().MarkPaid(c, 5, 1)
	if err != nil {
		t.Fatalf("MarkPaid: %v", err)
	}
	if rows != 1 {
		t.Errorf("rows = %d, want 1", rows)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}

// TestSettlementBillRepository_MarkPaid_ZeroAdmin 验证 adminID 缺失短路。
func TestSettlementBillRepository_MarkPaid_ZeroAdmin(t *testing.T) {
	c, mock := newMockContext(t)

	if _, err := NewSettlementBillRepository().MarkPaid(c, 5, 0); err == nil {
		t.Errorf("MarkPaid(adminID=0) = nil, want err")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v (expected zero SQL calls)", err)
	}
}

// =============================================================================
// SettlementBillRepository — DeductSupplierBalance
// =============================================================================

// TestSettlementBillRepository_DeductSupplierBalance_Happy 验证条件扣减。
//
// GORM 渲染：SET balance=?, updated_at=? WHERE id=? AND balance >= ?。
func TestSettlementBillRepository_DeductSupplierBalance_Happy(t *testing.T) {
	c, mock := newMockContext(t)

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("UPDATE")).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), int64(3), 270.0).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	rows, err := NewSettlementBillRepository().DeductSupplierBalance(c, 3, 270)
	if err != nil {
		t.Fatalf("DeductSupplierBalance: %v", err)
	}
	if rows != 1 {
		t.Errorf("rows = %d, want 1", rows)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}

// TestSettlementBillRepository_DeductSupplierBalance_Insufficient 验证余额不足 → rows=0。
func TestSettlementBillRepository_DeductSupplierBalance_Insufficient(t *testing.T) {
	c, mock := newMockContext(t)

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("UPDATE")).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), int64(3), 99999.0).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()

	rows, err := NewSettlementBillRepository().DeductSupplierBalance(c, 3, 99999)
	if err != nil {
		t.Fatalf("DeductSupplierBalance: %v", err)
	}
	if rows != 0 {
		t.Errorf("rows = %d, want 0 (insufficient)", rows)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}

// TestSettlementBillRepository_DeductSupplierBalance_Negative 验证负数金额短路。
func TestSettlementBillRepository_DeductSupplierBalance_Negative(t *testing.T) {
	c, mock := newMockContext(t)

	if _, err := NewSettlementBillRepository().DeductSupplierBalance(c, 3, -1); err == nil {
		t.Errorf("DeductSupplierBalance(-1) = nil, want err")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v (expected zero SQL calls)", err)
	}
}

// =============================================================================
// SettlementBillRepository — SetBoxLifecycle
// =============================================================================

// TestSettlementBillRepository_SetBoxLifecycle_Happy 验证生命周期 CAS。
//
// GORM 渲染：SET lifecycle=?, updated_at=? WHERE id=? AND lifecycle=?。
func TestSettlementBillRepository_SetBoxLifecycle_Happy(t *testing.T) {
	c, mock := newMockContext(t)

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("UPDATE")).
		WithArgs("settled", sqlmock.AnyArg(), int64(9), "sold_out").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	rows, err := NewSettlementBillRepository().SetBoxLifecycle(c, 9, mall.LifecycleSoldOut, mall.LifecycleSettled)
	if err != nil {
		t.Fatalf("SetBoxLifecycle: %v", err)
	}
	if rows != 1 {
		t.Errorf("rows = %d, want 1", rows)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}

// TestSettlementBillRepository_SetBoxLifecycle_CasMiss 验证生命周期不匹配 → rows=0。
func TestSettlementBillRepository_SetBoxLifecycle_CasMiss(t *testing.T) {
	c, mock := newMockContext(t)

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("UPDATE")).
		WithArgs("settled", sqlmock.AnyArg(), int64(9), "sold_out").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()

	rows, err := NewSettlementBillRepository().SetBoxLifecycle(c, 9, mall.LifecycleSoldOut, mall.LifecycleSettled)
	if err != nil {
		t.Fatalf("SetBoxLifecycle: %v", err)
	}
	if rows != 0 {
		t.Errorf("rows = %d, want 0 (CAS miss)", rows)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}
