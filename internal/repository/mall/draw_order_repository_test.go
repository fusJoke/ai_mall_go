package mall

import (
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"

	"ai-go-mall/internal/model/mall"
	"ai-go-mall/internal/repository"
)

// =============================================================================
// DrawOrderRepository
// =============================================================================

// TestDrawOrderRepository_GetByID_Found 验证主键命中。
func TestDrawOrderRepository_GetByID_Found(t *testing.T) {
	c, mock := newMockContext(t)

	rows := sqlmock.NewRows([]string{"id", "order_no"}).AddRow(int64(7), "O20261001")
	mock.ExpectQuery(regexp.QuoteMeta("SELECT")).
		WillReturnRows(rows)

	got, err := NewDrawOrderRepository().GetByID(c, 7)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got == nil || got.OrderNo != "O20261001" {
		t.Errorf("GetByID = %+v", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}

// TestDrawOrderRepository_GetByOrderNo 验证 UK 查唯一订单。
func TestDrawOrderRepository_GetByOrderNo(t *testing.T) {
	c, mock := newMockContext(t)

	rows := sqlmock.NewRows([]string{"id", "order_no"}).AddRow(int64(7), "O20261001")
	mock.ExpectQuery(regexp.QuoteMeta("SELECT")).
		WithArgs("O20261001", sqlmock.AnyArg()).
		WillReturnRows(rows)

	got, err := NewDrawOrderRepository().GetByOrderNo(c, "O20261001")
	if err != nil {
		t.Fatalf("GetByOrderNo: %v", err)
	}
	if got == nil || got.ID != 7 {
		t.Errorf("GetByOrderNo = %+v", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}

// TestDrawOrderRepository_CreateOrder_Nil 验证 nil order 短路。
func TestDrawOrderRepository_CreateOrder_Nil(t *testing.T) {
	c, mock := newMockContext(t)

	if err := NewDrawOrderRepository().CreateOrder(c, nil); err == nil {
		t.Errorf("CreateOrder(nil) = nil, want err")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v (expected zero SQL calls)", err)
	}
}

// TestDrawOrderRepository_CreateOrder 验证 INSERT。
func TestDrawOrderRepository_CreateOrder(t *testing.T) {
	c, mock := newMockContext(t)

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO")).
		WillReturnResult(sqlmock.NewResult(7, 1))
	mock.ExpectCommit()

	order := &mall.MallDrawOrder{OrderNo: "O20261001", UserID: 3, BlindBoxID: 7, Price: 50.0}
	if err := NewDrawOrderRepository().CreateOrder(c, order); err != nil {
		t.Fatalf("CreateOrder: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}

// TestDrawOrderRepository_ListByUser 验证分页查（按 created_at DESC）。
func TestDrawOrderRepository_ListByUser(t *testing.T) {
	c, mock := newMockContext(t)

	mock.ExpectQuery(`(?i)SELECT count`).
		WithArgs(int64(3)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(2)))
	rows := sqlmock.NewRows([]string{"id", "user_id"}).
		AddRow(int64(1), int64(3)).
		AddRow(int64(2), int64(3))
	mock.ExpectQuery(`(?i)SELECT`).
		WithArgs(int64(3), sqlmock.AnyArg()).
		WillReturnRows(rows)

	items, total, err := NewDrawOrderRepository().ListByUser(c, 3, repository.ListOptions{Page: 1, PageSize: 20})
	if err != nil {
		t.Fatalf("ListByUser: %v", err)
	}
	if total != 2 || len(items) != 2 {
		t.Errorf("total=%d items=%d, want 2/2", total, len(items))
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}

// TestDrawOrderRepository_UpdateStatus 验证仅写 status 列。
func TestDrawOrderRepository_UpdateStatus(t *testing.T) {
	c, mock := newMockContext(t)

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("UPDATE")).
		WithArgs("paid", sqlmock.AnyArg(), int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	if err := NewDrawOrderRepository().UpdateStatus(c, 7, mall.OrderStatusPaid); err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}

// TestDrawOrderRepository_ListItemsByOrderID 验证按 order_id 查明细。
func TestDrawOrderRepository_ListItemsByOrderID(t *testing.T) {
	c, mock := newMockContext(t)

	rows := sqlmock.NewRows([]string{"id", "order_id"}).
		AddRow(int64(1), int64(7)).
		AddRow(int64(2), int64(7))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT")).
		WithArgs(int64(7)).
		WillReturnRows(rows)

	items, err := NewDrawOrderRepository().ListItemsByOrderID(c, 7)
	if err != nil {
		t.Fatalf("ListItemsByOrderID: %v", err)
	}
	if len(items) != 2 {
		t.Errorf("len(items) = %d, want 2", len(items))
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}

// TestDrawOrderRepository_CreateItems_Empty 验证空 items 短路。
func TestDrawOrderRepository_CreateItems_Empty(t *testing.T) {
	c, mock := newMockContext(t)

	if err := NewDrawOrderRepository().CreateItems(c, nil); err != nil {
		t.Errorf("CreateItems(nil) = %v, want nil", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v (expected zero SQL calls)", err)
	}
}

// TestDrawOrderRepository_ListSettledOrdersBySupplierInRange 验证按 supplier_id + 时间窗 + status 查 drawn 订单。
func TestDrawOrderRepository_ListSettledOrdersBySupplierInRange(t *testing.T) {
	c, mock := newMockContext(t)

	startAt := time.Date(2026, 9, 1, 0, 0, 0, 0, time.Local)
	endAt := time.Date(2026, 10, 1, 0, 0, 0, 0, time.Local)
	rows := sqlmock.NewRows([]string{"id", "supplier_id", "status"}).
		AddRow(int64(1), int64(3), "drawn").
		AddRow(int64(2), int64(3), "drawn")
	mock.ExpectQuery(regexp.QuoteMeta("SELECT")).
		WithArgs(int64(3), "drawn", startAt, endAt).
		WillReturnRows(rows)

	orders, err := NewDrawOrderRepository().ListSettledOrdersBySupplierInRange(c, 3, startAt, endAt)
	if err != nil {
		t.Fatalf("ListSettledOrdersBySupplierInRange: %v", err)
	}
	if len(orders) != 2 {
		t.Errorf("len(orders) = %d, want 2", len(orders))
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}