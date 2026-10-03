package mall

import (
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"

	"ai-go-mall/internal/model/mall"
	"ai-go-mall/internal/repository"
)

// =============================================================================
// BlindBoxRepository
// =============================================================================

// TestBlindBoxRepository_GetByID_Found 验证主键命中。
func TestBlindBoxRepository_GetByID_Found(t *testing.T) {
	c, mock := newMockContext(t)

	rows := sqlmock.NewRows([]string{"id", "name"}).AddRow(int64(7), "Curry Box")
	mock.ExpectQuery(regexp.QuoteMeta("SELECT")).
		WillReturnRows(rows)

	got, err := NewBlindBoxRepository().GetByID(c, 7)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got == nil || got.Name != "Curry Box" {
		t.Errorf("GetByID = %+v", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}

// TestBlindBoxRepository_ListBySupplier 验证分页查（先 Count 再 Find）。
func TestBlindBoxRepository_ListBySupplier(t *testing.T) {
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

	items, total, err := NewBlindBoxRepository().ListBySupplier(c, 3, repository.ListOptions{Page: 1, PageSize: 20})
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

// TestBlindBoxRepository_ListFeaturedOnSale 验证三条件 AND。
func TestBlindBoxRepository_ListFeaturedOnSale(t *testing.T) {
	c, mock := newMockContext(t)

	mock.ExpectQuery(`(?i)SELECT count`).
		WithArgs(true, mall.StatusActive, true).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(1)))
	rows := sqlmock.NewRows([]string{"id", "name"}).
		AddRow(int64(7), "Featured Box")
	mock.ExpectQuery(`(?i)SELECT`).
		WithArgs(true, mall.StatusActive, true, sqlmock.AnyArg()).
		WillReturnRows(rows)

	items, total, err := NewBlindBoxRepository().ListFeaturedOnSale(c, repository.ListOptions{Page: 1, PageSize: 20})
	if err != nil {
		t.Fatalf("ListFeaturedOnSale: %v", err)
	}
	if total != 1 || len(items) != 1 {
		t.Errorf("total=%d items=%d, want 1/1", total, len(items))
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}

// TestBlindBoxRepository_ListActiveOnSaleBySupplierIDs_Empty 验证空 ids 短路。
func TestBlindBoxRepository_ListActiveOnSaleBySupplierIDs_Empty(t *testing.T) {
	c, mock := newMockContext(t)

	items, total, err := NewBlindBoxRepository().ListActiveOnSaleBySupplierIDs(c, nil, repository.ListOptions{Page: 1, PageSize: 20})
	if err != nil {
		t.Errorf("ListActiveOnSaleBySupplierIDs(nil) = %v, want nil", err)
	}
	if total != 0 || len(items) != 0 {
		t.Errorf("total=%d items=%d, want 0/0", total, len(items))
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v (expected zero SQL calls)", err)
	}
}

// TestBlindBoxRepository_ListActiveOnSaleBySupplierIDs_Found 验证 IN + 状态过滤。
func TestBlindBoxRepository_ListActiveOnSaleBySupplierIDs_Found(t *testing.T) {
	c, mock := newMockContext(t)

	mock.ExpectQuery(`(?i)SELECT count`).
		WithArgs(int64(1), int64(2), mall.StatusActive, true).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(1)))
	rows := sqlmock.NewRows([]string{"id", "supplier_id"}).AddRow(int64(1), int64(1))
	mock.ExpectQuery(`(?i)SELECT`).
		WithArgs(int64(1), int64(2), mall.StatusActive, true, sqlmock.AnyArg()).
		WillReturnRows(rows)

	items, total, err := NewBlindBoxRepository().ListActiveOnSaleBySupplierIDs(c, []int64{1, 2}, repository.ListOptions{Page: 1, PageSize: 20})
	if err != nil {
		t.Fatalf("ListActiveOnSaleBySupplierIDs: %v", err)
	}
	if total != 1 || len(items) != 1 {
		t.Errorf("total=%d items=%d, want 1/1", total, len(items))
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}

// TestBlindBoxRepository_ListByIDs_Empty 验证空 ids 短路。
func TestBlindBoxRepository_ListByIDs_Empty(t *testing.T) {
	c, mock := newMockContext(t)

	items, err := NewBlindBoxRepository().ListByIDs(c, nil)
	if err != nil || len(items) != 0 {
		t.Errorf("ListByIDs(nil) = %v, items=%+v, want nil/empty", err, items)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v (expected zero SQL calls)", err)
	}
}

// TestBlindBoxRepository_ToggleStatus 验证仅写 status 列。
func TestBlindBoxRepository_ToggleStatus(t *testing.T) {
	c, mock := newMockContext(t)

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("UPDATE")).
		WithArgs("disabled", sqlmock.AnyArg(), int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	if err := NewBlindBoxRepository().ToggleStatus(c, 7, mall.StatusDisabled); err != nil {
		t.Fatalf("ToggleStatus: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}

// TestBlindBoxRepository_ToggleOnSale 验证仅写 on_sale 列。
func TestBlindBoxRepository_ToggleOnSale(t *testing.T) {
	c, mock := newMockContext(t)

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("UPDATE")).
		WithArgs(false, sqlmock.AnyArg(), int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	if err := NewBlindBoxRepository().ToggleOnSale(c, 7, false); err != nil {
		t.Fatalf("ToggleOnSale: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}

// TestBlindBoxRepository_ToggleFeatured 验证仅写 is_featured 列。
func TestBlindBoxRepository_ToggleFeatured(t *testing.T) {
	c, mock := newMockContext(t)

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("UPDATE")).
		WithArgs(true, sqlmock.AnyArg(), int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	if err := NewBlindBoxRepository().ToggleFeatured(c, 7, true); err != nil {
		t.Fatalf("ToggleFeatured: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}