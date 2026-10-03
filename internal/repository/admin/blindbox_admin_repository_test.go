package admin

import (
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"

	"ai-go-mall/internal/model/mall"
)

// =============================================================================
// AdminBlindBoxRepository
// =============================================================================

// TestAdminBlindBoxRepository_ListWithFilter_SupplierOnly 验证按 supplier_id 过滤。
func TestAdminBlindBoxRepository_ListWithFilter_SupplierOnly(t *testing.T) {
	_, mock, db := newAdminMockContext(t)

	mock.ExpectQuery(`(?i)SELECT count`).
		WithArgs(int64(3)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(1)))
	rows := sqlmock.NewRows([]string{"id", "supplier_id"}).AddRow(int64(1), int64(3))
	mock.ExpectQuery(`(?i)SELECT`).
		WithArgs(int64(3), sqlmock.AnyArg()).
		WillReturnRows(rows)

	items, total, err := NewAdminBlindBoxRepository(db).ListWithFilter(nil, AdminBlindBoxListOptions{
		Page: 1, PageSize: 20, SupplierID: 3,
	})
	if err != nil {
		t.Fatalf("ListWithFilter: %v", err)
	}
	if total != 1 || len(items) != 1 {
		t.Errorf("total=%d items=%d, want 1/1", total, len(items))
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}

// TestAdminBlindBoxRepository_ListWithFilter_StatusOnly 验证 status 过滤。
func TestAdminBlindBoxRepository_ListWithFilter_StatusOnly(t *testing.T) {
	_, mock, db := newAdminMockContext(t)

	mock.ExpectQuery(`(?i)SELECT count`).
		WithArgs("disabled").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(1)))
	rows := sqlmock.NewRows([]string{"id", "status"}).AddRow(int64(1), "disabled")
	mock.ExpectQuery(`(?i)SELECT`).
		WithArgs("disabled", sqlmock.AnyArg()).
		WillReturnRows(rows)

	items, total, err := NewAdminBlindBoxRepository(db).ListWithFilter(nil, AdminBlindBoxListOptions{
		Page: 1, PageSize: 20, StatusFilter: "disabled",
	})
	if err != nil {
		t.Fatalf("ListWithFilter: %v", err)
	}
	if total != 1 || len(items) != 1 {
		t.Errorf("total=%d items=%d, want 1/1", total, len(items))
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}

// TestAdminBlindBoxRepository_ListWithFilter_AllFilters 验证全过滤项。
func TestAdminBlindBoxRepository_ListWithFilter_AllFilters(t *testing.T) {
	_, mock, db := newAdminMockContext(t)

	featured := true
	mock.ExpectQuery(`(?i)SELECT count`).
		WithArgs(int64(3), "active", true).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(1)))
	rows := sqlmock.NewRows([]string{"id", "supplier_id"}).AddRow(int64(1), int64(3))
	mock.ExpectQuery(`(?i)SELECT`).
		WithArgs(int64(3), "active", true, sqlmock.AnyArg()).
		WillReturnRows(rows)

	items, total, err := NewAdminBlindBoxRepository(db).ListWithFilter(nil, AdminBlindBoxListOptions{
		Page: 1, PageSize: 20,
		SupplierID:       3,
		StatusFilter:     "active",
		IsFeaturedFilter: &featured,
	})
	if err != nil {
		t.Fatalf("ListWithFilter: %v", err)
	}
	if total != 1 || len(items) != 1 {
		t.Errorf("total=%d items=%d, want 1/1", total, len(items))
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}

// TestAdminBlindBoxRepository_ListWithFilter_NoFilter 验证无过滤。
func TestAdminBlindBoxRepository_ListWithFilter_NoFilter(t *testing.T) {
	_, mock, db := newAdminMockContext(t)

	mock.ExpectQuery(`(?i)SELECT count`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(0)))

	_, total, err := NewAdminBlindBoxRepository(db).ListWithFilter(nil, AdminBlindBoxListOptions{Page: 1, PageSize: 20})
	if err != nil {
		t.Fatalf("ListWithFilter: %v", err)
	}
	if total != 0 {
		t.Errorf("total = %d, want 0", total)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}

// TestAdminBlindBoxRepository_ToggleStatus 验证仅写 status 列。
func TestAdminBlindBoxRepository_ToggleStatus(t *testing.T) {
	_, mock, db := newAdminMockContext(t)

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("UPDATE")).
		WithArgs("disabled", sqlmock.AnyArg(), int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	if err := NewAdminBlindBoxRepository(db).ToggleStatus(nil, 7, mall.StatusDisabled); err != nil {
		t.Fatalf("ToggleStatus: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}

// TestAdminBlindBoxRepository_ToggleOnSale 验证仅写 on_sale 列。
func TestAdminBlindBoxRepository_ToggleOnSale(t *testing.T) {
	_, mock, db := newAdminMockContext(t)

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("UPDATE")).
		WithArgs(false, sqlmock.AnyArg(), int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	if err := NewAdminBlindBoxRepository(db).ToggleOnSale(nil, 7, false); err != nil {
		t.Fatalf("ToggleOnSale: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}

// TestAdminBlindBoxRepository_ToggleFeatured 验证仅写 is_featured 列。
func TestAdminBlindBoxRepository_ToggleFeatured(t *testing.T) {
	_, mock, db := newAdminMockContext(t)

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("UPDATE")).
		WithArgs(true, sqlmock.AnyArg(), int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	if err := NewAdminBlindBoxRepository(db).ToggleFeatured(nil, 7, true); err != nil {
		t.Fatalf("ToggleFeatured: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}