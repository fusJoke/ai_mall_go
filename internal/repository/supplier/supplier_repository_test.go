package supplier

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	"ai-go-mall/internal/infra/database"
	"ai-go-mall/internal/model/mall"
	"ai-go-mall/internal/repository"
)

// newMockContext 构造一个 gin.Context，把 sqlmock 包装成的 *gorm.DB
// 注入到 database.CtxKey 下，使 repository.DB(c) 能拿到它。
func newMockContext(t *testing.T) (*gin.Context, sqlmock.Sqlmock) {
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
	c.Set(database.CtxKey, gdb)
	return c, mock
}

func newTestSupplierRepo() *gormSupplierRepository {
	return &gormSupplierRepository{
		CRUDRepository: repository.NewBaseRepository[mall.MallSupplier](),
	}
}

func newTestSupplierUserRepo() *gormSupplierUserRepository {
	return &gormSupplierUserRepository{
		CRUDRepository: repository.NewBaseRepository[mall.MallSupplierUser](),
	}
}

// =============================================================================
// SupplierRepository.GetByID
// =============================================================================

// TestSupplierRepository_GetByID_Found 验证主键命中。
func TestSupplierRepository_GetByID_Found(t *testing.T) {
	c, mock := newMockContext(t)

	rows := sqlmock.NewRows([]string{"id", "name"}).AddRow(int64(7), "Acme")
	mock.ExpectQuery(regexp.QuoteMeta("SELECT")).
		WillReturnRows(rows)

	got, err := newTestSupplierRepo().GetByID(c, 7)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got == nil || got.ID != 7 || got.Name != "Acme" {
		t.Errorf("GetByID = %+v, want ID=7 Name=Acme", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}

// TestSupplierRepository_GetByID_NotFound 验证未命中透传 gorm.ErrRecordNotFound。
func TestSupplierRepository_GetByID_NotFound(t *testing.T) {
	c, mock := newMockContext(t)

	mock.ExpectQuery(regexp.QuoteMeta("SELECT")).
		WillReturnError(gorm.ErrRecordNotFound)

	_, err := newTestSupplierRepo().GetByID(c, 999)
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Errorf("err = %v, want gorm.ErrRecordNotFound", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}

// =============================================================================
// SupplierRepository.ListByIDs
// =============================================================================

// TestSupplierRepository_ListByIDs_Empty 验证空 ids 短路返回空切片。
func TestSupplierRepository_ListByIDs_Empty(t *testing.T) {
	c, mock := newMockContext(t)

	items, err := newTestSupplierRepo().ListByIDs(c, nil)
	if err != nil {
		t.Errorf("ListByIDs(nil) = %v, want nil", err)
	}
	if len(items) != 0 {
		t.Errorf("len(items) = %d, want 0", len(items))
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v (expected zero SQL calls)", err)
	}
}

// TestSupplierRepository_ListByIDs_Found 验证 IN 单条 SQL 命中。
func TestSupplierRepository_ListByIDs_Found(t *testing.T) {
	c, mock := newMockContext(t)

	rows := sqlmock.NewRows([]string{"id", "name"}).
		AddRow(int64(1), "A").
		AddRow(int64(2), "B")
	mock.ExpectQuery(regexp.QuoteMeta("SELECT")).
		WillReturnRows(rows)

	items, err := newTestSupplierRepo().ListByIDs(c, []int64{1, 2})
	if err != nil {
		t.Fatalf("ListByIDs: %v", err)
	}
	if len(items) != 2 {
		t.Errorf("len(items) = %d, want 2", len(items))
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}

// =============================================================================
// SupplierRepository.ListWithFilter
// =============================================================================

// TestSupplierRepository_ListWithFilter_StatusOnly 验证仅按 status 过滤。
func TestSupplierRepository_ListWithFilter_StatusOnly(t *testing.T) {
	c, mock := newMockContext(t)

	mock.ExpectQuery(`(?i)SELECT count`).
		WithArgs("active").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(5)))
	rows := sqlmock.NewRows([]string{"id", "name", "status"}).
		AddRow(int64(1), "A", "active").
		AddRow(int64(2), "B", "active")
	mock.ExpectQuery(`(?i)SELECT`).
		WithArgs("active", sqlmock.AnyArg()).
		WillReturnRows(rows)

	items, total, err := newTestSupplierRepo().ListWithFilter(c, SupplierListOptions{
		Page: 1, PageSize: 20, StatusFilter: "active",
	})
	if err != nil {
		t.Fatalf("ListWithFilter: %v", err)
	}
	if total != 5 {
		t.Errorf("total = %d, want 5", total)
	}
	if len(items) != 2 {
		t.Errorf("len(items) = %d, want 2", len(items))
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}

// TestSupplierRepository_ListWithFilter_KeywordOnly 验证仅按 name 关键字过滤。
func TestSupplierRepository_ListWithFilter_KeywordOnly(t *testing.T) {
	c, mock := newMockContext(t)

	mock.ExpectQuery(`(?i)SELECT count`).
		WithArgs("%lakers%").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(1)))
	rows := sqlmock.NewRows([]string{"id", "name"}).
		AddRow(int64(3), "Lakers Cards")
	mock.ExpectQuery(`(?i)SELECT`).
		WithArgs("%lakers%", sqlmock.AnyArg()).
		WillReturnRows(rows)

	items, _, err := newTestSupplierRepo().ListWithFilter(c, SupplierListOptions{
		Page: 1, PageSize: 20, NameKeyword: "lakers",
	})
	if err != nil {
		t.Fatalf("ListWithFilter: %v", err)
	}
	if len(items) != 1 {
		t.Errorf("len(items) = %d, want 1", len(items))
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}

// TestSupplierRepository_ListWithFilter_NoFilter 验证无过滤时不拼 WHERE。
func TestSupplierRepository_ListWithFilter_NoFilter(t *testing.T) {
	c, mock := newMockContext(t)

	mock.ExpectQuery(`(?i)SELECT count`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(0)))
	// total=0 → 跳过 Find，只 ExpectQuery 一条

	_, total, err := newTestSupplierRepo().ListWithFilter(c, SupplierListOptions{Page: 1, PageSize: 20})
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

// =============================================================================
// ToggleStatus / ToggleFeatured
// =============================================================================

// TestSupplierRepository_ToggleStatus 验证仅写 status 列。
func TestSupplierRepository_ToggleStatus(t *testing.T) {
	c, mock := newMockContext(t)

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("UPDATE")).
		WithArgs("disabled", sqlmock.AnyArg(), int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	if err := newTestSupplierRepo().ToggleStatus(c, 7, mall.StatusDisabled); err != nil {
		t.Fatalf("ToggleStatus: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}

// TestSupplierRepository_ToggleFeatured 验证仅写 is_featured 列。
func TestSupplierRepository_ToggleFeatured(t *testing.T) {
	c, mock := newMockContext(t)

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("UPDATE")).
		WithArgs(true, sqlmock.AnyArg(), int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	if err := newTestSupplierRepo().ToggleFeatured(c, 7, true); err != nil {
		t.Fatalf("ToggleFeatured: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}

// =============================================================================
// UpdateBalanceAndSales
// =============================================================================

// TestSupplierRepository_UpdateBalanceAndSales_Success 验证影响 1 行。
func TestSupplierRepository_UpdateBalanceAndSales_Success(t *testing.T) {
	c, mock := newMockContext(t)

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("UPDATE")).
		WithArgs(50.0, 50.0, sqlmock.AnyArg(), int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	if err := newTestSupplierRepo().UpdateBalanceAndSales(c, 7, 50.0); err != nil {
		t.Fatalf("UpdateBalanceAndSales: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}

// TestSupplierRepository_UpdateBalanceAndSales_NotFound 验证影响 0 行返回 ErrSupplierNotFound。
func TestSupplierRepository_UpdateBalanceAndSales_NotFound(t *testing.T) {
	c, mock := newMockContext(t)

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("UPDATE")).
		WithArgs(50.0, 50.0, sqlmock.AnyArg(), int64(999)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()

	err := newTestSupplierRepo().UpdateBalanceAndSales(c, 999, 50.0)
	if !errors.Is(err, ErrSupplierNotFound) {
		t.Errorf("err = %v, want ErrSupplierNotFound", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}

// TestSupplierRepository_UpdateBalanceAndSales_NonPositive 验证 amount<=0 短路。
func TestSupplierRepository_UpdateBalanceAndSales_NonPositive(t *testing.T) {
	c, mock := newMockContext(t)

	if err := newTestSupplierRepo().UpdateBalanceAndSales(c, 7, 0); err == nil {
		t.Errorf("UpdateBalanceAndSales(0) = nil, want err")
	}
	if err := newTestSupplierRepo().UpdateBalanceAndSales(c, 7, -1); err == nil {
		t.Errorf("UpdateBalanceAndSales(-1) = nil, want err")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v (expected zero SQL calls)", err)
	}
}

// =============================================================================
// SupplierUserRepository
// =============================================================================

// TestSupplierUserRepository_GetByUsername_Found 命中。
func TestSupplierUserRepository_GetByUsername_Found(t *testing.T) {
	c, mock := newMockContext(t)

	rows := sqlmock.NewRows([]string{"id", "username"}).AddRow(int64(7), "supplier1")
	mock.ExpectQuery(regexp.QuoteMeta("SELECT")).
		WillReturnRows(rows)

	got, err := newTestSupplierUserRepo().GetByUsername(c, "supplier1")
	if err != nil {
		t.Fatalf("GetByUsername: %v", err)
	}
	if got == nil || got.Username != "supplier1" {
		t.Errorf("GetByUsername = %+v, want username=supplier1", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}

// TestSupplierUserRepository_GetByUsername_Empty 验证空 username 短路。
func TestSupplierUserRepository_GetByUsername_Empty(t *testing.T) {
	c, mock := newMockContext(t)

	if _, err := newTestSupplierUserRepo().GetByUsername(c, ""); err == nil {
		t.Errorf("GetByUsername('') = nil, want err")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v (expected zero SQL calls)", err)
	}
}

// TestSupplierUserRepository_GetBySupplierID 验证按 supplier_id 查。
func TestSupplierUserRepository_GetBySupplierID(t *testing.T) {
	c, mock := newMockContext(t)

	rows := sqlmock.NewRows([]string{"id", "supplier_id"}).AddRow(int64(7), int64(3))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT")).
		WillReturnRows(rows)

	got, err := newTestSupplierUserRepo().GetBySupplierID(c, 3)
	if err != nil {
		t.Fatalf("GetBySupplierID: %v", err)
	}
	if got == nil || got.SupplierID != 3 {
		t.Errorf("GetBySupplierID = %+v, want SupplierID=3", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}

// TestSupplierUserRepository_UpdateLoginSuccess 验证登录成功只写两列
// （last_login_at / last_login_ip，不含 password，也**不含 login_failure**）。
//
// 为什么必须显式断言 SQL 全文：mall_supplier_users 没有 login_failure 列
// （spec「mall_supplier_users field schema」与迁移 000007 均无此列），而从
// internal/repository/user 复制过来的实现曾把 login_failure=0 一起写进去 ——
// sqlmock 不知道真实 schema，只有断言 SQL 全文才能拦住它（真实库里是
// Error 1054 Unknown column 'login_failure'，7.5 供应商登录验证即由此暴露）。
func TestSupplierUserRepository_UpdateLoginSuccess(t *testing.T) {
	c, mock := newMockContext(t)

	at := time.Date(2026, 10, 1, 12, 0, 0, 0, time.Local)
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(
		"UPDATE `mall_supplier_users` SET `last_login_at`=?,`last_login_ip`=?,`updated_at`=? WHERE id = ? AND `mall_supplier_users`.`deleted_at` IS NULL")).
		WithArgs(at, "10.0.0.1", sqlmock.AnyArg(), int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	if err := newTestSupplierUserRepo().UpdateLoginSuccess(c, 7, "10.0.0.1", at); err != nil {
		t.Fatalf("UpdateLoginSuccess: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}

// TestSupplierUserRepository_UpdateStatus 验证仅写 status 列。
func TestSupplierUserRepository_UpdateStatus(t *testing.T) {
	c, mock := newMockContext(t)

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("UPDATE")).
		WithArgs(int8(0), sqlmock.AnyArg(), int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	if err := newTestSupplierUserRepo().UpdateStatus(c, 7, 0); err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}