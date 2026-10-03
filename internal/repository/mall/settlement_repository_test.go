package mall

import (
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/go-sql-driver/mysql"
	"gorm.io/gorm"

	"ai-go-mall/internal/model/mall"
	"ai-go-mall/internal/repository"
)

// =============================================================================
// SettlementRepository — GetByID
// =============================================================================

// TestSettlementRepository_GetByID_Found 验证主键命中。
func TestSettlementRepository_GetByID_Found(t *testing.T) {
	c, mock := newMockContext(t)

	rows := sqlmock.NewRows([]string{"id", "supplier_id"}).
		AddRow(int64(7), int64(3))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT")).
		WillReturnRows(rows)

	got, err := NewSettlementRepository().GetByID(c, 7)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got == nil || got.SupplierID != 3 {
		t.Errorf("GetByID = %+v", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}

// =============================================================================
// SettlementRepository — GetByIDWithItems
// =============================================================================

// TestSettlementRepository_GetByIDWithItems 验证主键 + items 双查。
func TestSettlementRepository_GetByIDWithItems(t *testing.T) {
	c, mock := newMockContext(t)

	// 1. SELECT settlement
	rows := sqlmock.NewRows([]string{"id", "supplier_id"}).
		AddRow(int64(7), int64(3))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT")).
		WillReturnRows(rows)
	// 2. SELECT items
	itemsRows := sqlmock.NewRows([]string{"id", "settlement_id", "draw_order_id"}).
		AddRow(int64(1), int64(7), int64(101)).
		AddRow(int64(2), int64(7), int64(102))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT")).
		WithArgs(int64(7)).
		WillReturnRows(itemsRows)

	s, items, err := NewSettlementRepository().GetByIDWithItems(c, 7)
	if err != nil {
		t.Fatalf("GetByIDWithItems: %v", err)
	}
	if s == nil || s.ID != 7 {
		t.Errorf("settlement = %+v", s)
	}
	if len(items) != 2 {
		t.Errorf("len(items) = %d, want 2", len(items))
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}

// =============================================================================
// SettlementRepository — ListBySupplier
// =============================================================================

// TestSettlementRepository_ListBySupplier 验证分页查。
func TestSettlementRepository_ListBySupplier(t *testing.T) {
	c, mock := newMockContext(t)

	mock.ExpectQuery(`(?i)SELECT count`).
		WithArgs(int64(3)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(2)))
	rows := sqlmock.NewRows([]string{"id", "supplier_id"}).
		AddRow(int64(1), int64(3)).
		AddRow(int64(2), int64(3))
	mock.ExpectQuery(`(?i)SELECT`).
		WithArgs(int64(3), sqlmock.AnyArg()).
		WillReturnRows(rows)

	items, total, err := NewSettlementRepository().ListBySupplier(c, 3, repository.ListOptions{Page: 1, PageSize: 20})
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

// =============================================================================
// SettlementRepository — ListItemsBySettlementID
// =============================================================================

// TestSettlementRepository_ListItemsBySettlementID 拉某 settlement 明细。
func TestSettlementRepository_ListItemsBySettlementID(t *testing.T) {
	c, mock := newMockContext(t)

	rows := sqlmock.NewRows([]string{"id", "settlement_id"}).
		AddRow(int64(1), int64(7)).
		AddRow(int64(2), int64(7))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT")).
		WithArgs(int64(7)).
		WillReturnRows(rows)

	items, err := NewSettlementRepository().ListItemsBySettlementID(c, 7)
	if err != nil {
		t.Fatalf("ListItemsBySettlementID: %v", err)
	}
	if len(items) != 2 {
		t.Errorf("len(items) = %d, want 2", len(items))
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}

// =============================================================================
// SettlementRepository — ListEligibleOrders
// =============================================================================

// TestSettlementRepository_ListEligibleOrders 验证 NOT EXISTS 子查询。
func TestSettlementRepository_ListEligibleOrders(t *testing.T) {
	c, mock := newMockContext(t)

	start := time.Date(2026, 9, 1, 0, 0, 0, 0, time.Local)
	end := time.Date(2026, 10, 1, 0, 0, 0, 0, time.Local)
	rows := sqlmock.NewRows([]string{"id", "supplier_id", "status"}).
		AddRow(int64(101), int64(3), "drawn").
		AddRow(int64(102), int64(3), "drawn")
	mock.ExpectQuery(`(?i)SELECT`).
		WithArgs(int64(3), "drawn", start, end).
		WillReturnRows(rows)

	orders, err := NewSettlementRepository().ListEligibleOrders(c, 3, start, end)
	if err != nil {
		t.Fatalf("ListEligibleOrders: %v", err)
	}
	if len(orders) != 2 {
		t.Errorf("len(orders) = %d, want 2", len(orders))
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}

// =============================================================================
// SettlementRepository — Generate
// =============================================================================

// TestSettlementRepository_Generate_Nil 验证 nil 短路。
func TestSettlementRepository_Generate_Nil(t *testing.T) {
	c, mock := newMockContext(t)

	if err := NewSettlementRepository().Generate(c, nil, nil); err == nil {
		t.Errorf("Generate(nil) = nil, want err")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v (expected zero SQL calls)", err)
	}
}

// TestSettlementRepository_Generate_EmptyItems 验证 items=[] 时仅写 settlement。
func TestSettlementRepository_Generate_EmptyItems(t *testing.T) {
	c, mock := newMockContext(t)

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO")).
		WillReturnResult(sqlmock.NewResult(7, 1))
	mock.ExpectCommit()

	s := &mall.MallSettlement{
		SupplierID:   3,
		PeriodStart:  time.Date(2026, 9, 1, 0, 0, 0, 0, time.Local),
		PeriodEnd:    time.Date(2026, 10, 1, 0, 0, 0, 0, time.Local),
		TotalAmount:  1000.0,
		Status:       mall.SettlementStatusPending,
	}
	if err := NewSettlementRepository().Generate(c, s, nil); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if s.ID != 7 {
		t.Errorf("s.ID = %d, want 7", s.ID)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}

// TestSettlementRepository_Generate_WithItems 验证单事务 INSERT settlement + INSERT items。
func TestSettlementRepository_Generate_WithItems(t *testing.T) {
	c, mock := newMockContext(t)

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO")).
		WillReturnResult(sqlmock.NewResult(7, 1))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO")).
		WillReturnResult(sqlmock.NewResult(1, 2))
	mock.ExpectCommit()

	s := &mall.MallSettlement{
		SupplierID:  3,
		PeriodStart: time.Date(2026, 9, 1, 0, 0, 0, 0, time.Local),
		PeriodEnd:   time.Date(2026, 10, 1, 0, 0, 0, 0, time.Local),
		Status:      mall.SettlementStatusPending,
	}
	items := []mall.MallSettlementItem{
		{DrawOrderID: 101, Amount: 100},
		{DrawOrderID: 102, Amount: 200},
	}
	if err := NewSettlementRepository().Generate(c, s, items); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	// items 应回填 SettlementID。
	for i, it := range items {
		if it.SettlementID != 7 {
			t.Errorf("items[%d].SettlementID = %d, want 7", i, it.SettlementID)
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}

// TestSettlementRepository_Generate_Conflict 验证 UK 冲突 → ErrSettlementConflict。
func TestSettlementRepository_Generate_Conflict(t *testing.T) {
	c, mock := newMockContext(t)

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO")).
		WillReturnError(&mysql.MySQLError{Number: 1062, Message: "Duplicate entry"})
	mock.ExpectRollback()

	s := &mall.MallSettlement{SupplierID: 3, Status: mall.SettlementStatusPending}
	err := NewSettlementRepository().Generate(c, s, nil)
	if !errors.Is(err, ErrSettlementConflict) {
		t.Errorf("Generate conflict = %v, want ErrSettlementConflict", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}

// =============================================================================
// SettlementRepository — MarkPaid
// =============================================================================

// TestSettlementRepository_MarkPaid_HappyPath 完整正常路径：pending → UPDATE 成功 → UPDATE supplier 成功。
func TestSettlementRepository_MarkPaid_HappyPath(t *testing.T) {
	c, mock := newMockContext(t)

	// 1. SELECT settlement（status=pending）
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT")).
		WillReturnRows(sqlmock.NewRows([]string{"id", "supplier_id", "status"}).
			AddRow(int64(7), int64(3), "pending"))

	// 2. UPDATE settlement status='paid'
	// GORM 渲染：`SET paid_at=?, paid_by=?, status=?, updated_at=? WHERE id=? AND status IN (?,?)`
	// 参数顺序：paid_at, paid_by, status, updated_at, id, "pending", "processing"
	mock.ExpectExec(regexp.QuoteMeta("UPDATE")).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), "paid", sqlmock.AnyArg(), int64(7), "pending", "processing").
		WillReturnResult(sqlmock.NewResult(0, 1))

	// 3. UPDATE supplier balance WHERE balance >= ?
	// 参数：balance_value, updated_at, id, balance >= value
	mock.ExpectExec(regexp.QuoteMeta("UPDATE")).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), int64(3), 500.0).
		WillReturnResult(sqlmock.NewResult(0, 1))

	mock.ExpectCommit()

	if err := NewSettlementRepository().MarkPaid(c, 7, 1, 500.0); err != nil {
		t.Fatalf("MarkPaid: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}

// TestSettlementRepository_MarkPaid_NotFound 验证 settlement 不存在 → ErrSettlementNotFound。
func TestSettlementRepository_MarkPaid_NotFound(t *testing.T) {
	c, mock := newMockContext(t)

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT")).
		WillReturnError(gorm.ErrRecordNotFound)
	mock.ExpectRollback()

	if err := NewSettlementRepository().MarkPaid(c, 999, 1, 100); !errors.Is(err, ErrSettlementNotFound) {
		t.Errorf("MarkPaid not found = %v, want ErrSettlementNotFound", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}

// TestSettlementRepository_MarkPaid_AlreadyPaid 验证二次 mark_paid → ErrSettlementAlreadyPaid。
func TestSettlementRepository_MarkPaid_AlreadyPaid(t *testing.T) {
	c, mock := newMockContext(t)

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT")).
		WillReturnRows(sqlmock.NewRows([]string{"id", "supplier_id", "status"}).
			AddRow(int64(7), int64(3), "paid"))
	mock.ExpectRollback()

	if err := NewSettlementRepository().MarkPaid(c, 7, 1, 100); !errors.Is(err, ErrSettlementAlreadyPaid) {
		t.Errorf("MarkPaid already paid = %v, want ErrSettlementAlreadyPaid", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}

// TestSettlementRepository_MarkPaid_InsufficientBalance 验证余额不够 → ErrInsufficientSupplierBalance。
func TestSettlementRepository_MarkPaid_InsufficientBalance(t *testing.T) {
	c, mock := newMockContext(t)

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT")).
		WillReturnRows(sqlmock.NewRows([]string{"id", "supplier_id", "status"}).
			AddRow(int64(7), int64(3), "processing"))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE")).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), "paid", sqlmock.AnyArg(), int64(7), "pending", "processing").
		WillReturnResult(sqlmock.NewResult(0, 1))
	// UPDATE supplier 条件不满足 → 0 行
	// 参数：balance_value, updated_at, id, balance >= value
	mock.ExpectExec(regexp.QuoteMeta("UPDATE")).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), int64(3), 99999.0).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectRollback()

	if err := NewSettlementRepository().MarkPaid(c, 7, 1, 99999); !errors.Is(err, ErrInsufficientSupplierBalance) {
		t.Errorf("MarkPaid insufficient = %v, want ErrInsufficientSupplierBalance", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}

// TestSettlementRepository_MarkPaid_ZeroPayout 验证 payoutAmount=0 时跳过 supplier UPDATE。
func TestSettlementRepository_MarkPaid_ZeroPayout(t *testing.T) {
	c, mock := newMockContext(t)

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT")).
		WillReturnRows(sqlmock.NewRows([]string{"id", "supplier_id", "status"}).
			AddRow(int64(7), int64(3), "pending"))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE")).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), "paid", sqlmock.AnyArg(), int64(7), "pending", "processing").
		WillReturnResult(sqlmock.NewResult(0, 1))
	// 注意：payoutAmount=0 时不应触发 supplier UPDATE
	mock.ExpectCommit()

	if err := NewSettlementRepository().MarkPaid(c, 7, 1, 0); err != nil {
		t.Fatalf("MarkPaid: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}

// =============================================================================
// SettlementRepository — IncrementLedger
// =============================================================================

// TestSettlementRepository_IncrementLedger 验证 ON DUPLICATE KEY UPDATE 累加 SQL。
func TestSettlementRepository_IncrementLedger(t *testing.T) {
	c, mock := newMockContext(t)

	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO")).
		WithArgs("total_revenue", 100.0).
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := NewSettlementRepository().IncrementLedger(c, "total_revenue", 100.0); err != nil {
		t.Fatalf("IncrementLedger: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}

// TestSettlementRepository_IncrementLedger_EmptyKey 验证空 counter_key 短路。
func TestSettlementRepository_IncrementLedger_EmptyKey(t *testing.T) {
	c, mock := newMockContext(t)

	if err := NewSettlementRepository().IncrementLedger(c, "", 1); err == nil {
		t.Errorf("IncrementLedger('') = nil, want err")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v (expected zero SQL calls)", err)
	}
}

// =============================================================================
// isDuplicateKeyErr
// =============================================================================

// TestIsDuplicateKeyErr 验证 1062 错误识别。
func TestIsDuplicateKeyErr(t *testing.T) {
	if isDuplicateKeyErr(nil) {
		t.Errorf("nil err should not match")
	}
	if isDuplicateKeyErr(errors.New("plain")) {
		t.Errorf("plain err should not match")
	}
	if !isDuplicateKeyErr(&mysql.MySQLError{Number: 1062, Message: "Duplicate entry"}) {
		t.Errorf("1062 should match")
	}
	if isDuplicateKeyErr(&mysql.MySQLError{Number: 1064, Message: "Syntax error"}) {
		t.Errorf("1064 should not match")
	}
}