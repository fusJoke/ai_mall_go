package admin

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	"ai-go-mall/internal/model/mall"
)

// newAdminMockContext 构造一个 gin.Context，把 sqlmock 包装成的 *gorm.DB
// 同时注入到 database.CtxKey 与返回值中（admin 包 repo 直接吃 db 字段）。
func newAdminMockContext(t *testing.T) (*gin.Context, sqlmock.Sqlmock, *gorm.DB) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })

	gdb, err := gorm.Open(mysql.New(mysql.Config{
		Conn:                      sqlDB,
		SkipInitializeWithVersion: true,
	}), &gorm.Config{})
	if err != nil {
		t.Fatalf("gorm.Open: %v", err)
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	return c, mock, gdb
}

// =============================================================================
// AdminSupplierRepository
// =============================================================================

// TestAdminSupplierRepository_ListWithFilter_StatusOnly 验证仅 status 过滤。
func TestAdminSupplierRepository_ListWithFilter_StatusOnly(t *testing.T) {
	_, mock, db := newAdminMockContext(t)

	mock.ExpectQuery(`(?i)SELECT count`).
		WithArgs("disabled").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(2)))
	rows := sqlmock.NewRows([]string{"id", "status"}).
		AddRow(int64(1), "disabled").
		AddRow(int64(2), "disabled")
	mock.ExpectQuery(`(?i)SELECT`).
		WithArgs("disabled", sqlmock.AnyArg()).
		WillReturnRows(rows)

	items, total, err := NewAdminSupplierRepository(db).ListWithFilter(nil, AdminSupplierListOptions{
		Page: 1, PageSize: 20, StatusFilter: "disabled",
	})
	if err != nil {
		t.Fatalf("ListWithFilter: %v", err)
	}
	if total != 2 || len(items) != 2 {
		t.Errorf("total=%d items=%d, want 2/2", total, len(items))
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}

// TestAdminSupplierRepository_ListWithFilter_AllFilters 验证全过滤项。
func TestAdminSupplierRepository_ListWithFilter_AllFilters(t *testing.T) {
	_, mock, db := newAdminMockContext(t)

	featured := true
	mock.ExpectQuery(`(?i)SELECT count`).
		WithArgs("active", featured, "%lakers%").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(1)))
	rows := sqlmock.NewRows([]string{"id", "name"}).AddRow(int64(1), "Lakers Cards")
	mock.ExpectQuery(`(?i)SELECT`).
		WithArgs("active", featured, "%lakers%", sqlmock.AnyArg()).
		WillReturnRows(rows)

	items, total, err := NewAdminSupplierRepository(db).ListWithFilter(nil, AdminSupplierListOptions{
		Page: 1, PageSize: 20,
		StatusFilter:     "active",
		IsFeaturedFilter: &featured,
		NameKeyword:      "lakers",
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

// TestAdminSupplierRepository_ListWithFilter_NoFilter 验证无过滤走纯 Count。
func TestAdminSupplierRepository_ListWithFilter_NoFilter(t *testing.T) {
	_, mock, db := newAdminMockContext(t)

	mock.ExpectQuery(`(?i)SELECT count`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(0)))
	// total=0 跳过 Find，只 ExpectQuery 一次

	_, total, err := NewAdminSupplierRepository(db).ListWithFilter(nil, AdminSupplierListOptions{Page: 1, PageSize: 20})
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

// TestAdminSupplierRepository_ToggleStatus 验证仅写 status 列。
func TestAdminSupplierRepository_ToggleStatus(t *testing.T) {
	_, mock, db := newAdminMockContext(t)

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("UPDATE")).
		WithArgs("disabled", sqlmock.AnyArg(), int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	if err := NewAdminSupplierRepository(db).ToggleStatus(nil, 7, mall.StatusDisabled); err != nil {
		t.Fatalf("ToggleStatus: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}

// TestAdminSupplierRepository_ToggleFeatured 验证仅写 is_featured 列。
func TestAdminSupplierRepository_ToggleFeatured(t *testing.T) {
	_, mock, db := newAdminMockContext(t)

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("UPDATE")).
		WithArgs(true, sqlmock.AnyArg(), int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	if err := NewAdminSupplierRepository(db).ToggleFeatured(nil, 7, true); err != nil {
		t.Fatalf("ToggleFeatured: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}