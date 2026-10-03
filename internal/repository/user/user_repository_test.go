package user

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
	"ai-go-mall/internal/model"
	"ai-go-mall/internal/repository"
)

// newMockContext 构造一个 gin.Context，把 sqlmock 包装成的 *gorm.DB
// 注入到 database.CtxKey 下，使 repository.DB(c) 能拿到它。
//
// 与 admin 包 newMockContext 同实现；本包独立一份避免跨测试文件改动牵连。
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

// newTestRepository 构造一个走 request-scoped DB 的 gormUserRepository。
func newTestRepository() *gormUserRepository {
	return &gormUserRepository{
		CRUDRepository: repository.NewBaseRepository[model.User](),
	}
}

// =============================================================================
// GetByUsername
// =============================================================================

// TestUserRepository_GetByUsername_Empty 验证空用户名短路返回 error，不查 DB。
func TestUserRepository_GetByUsername_Empty(t *testing.T) {
	c, mock := newMockContext(t)
	// 故意不设 Expect —— 任何调用都会失败。

	if _, err := newTestRepository().GetByUsername(c, ""); err == nil {
		t.Errorf("GetByUsername('') = nil, want err")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v (expected zero SQL calls)", err)
	}
}

// TestUserRepository_GetByUsername_Found 验证命中时返回行。
func TestUserRepository_GetByUsername_Found(t *testing.T) {
	c, mock := newMockContext(t)

	rows := sqlmock.NewRows([]string{"id", "username"}).
		AddRow(int64(7), "alice")
	mock.ExpectQuery(regexp.QuoteMeta("SELECT")).
		WillReturnRows(rows)

	got, err := newTestRepository().GetByUsername(c, "alice")
	if err != nil {
		t.Fatalf("GetByUsername: %v", err)
	}
	if got == nil || got.ID != 7 || got.Username != "alice" {
		t.Errorf("GetByUsername = %+v, want ID=7 username=alice", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}

// TestUserRepository_GetByUsername_NotFound 验证未命中时透传 gorm.ErrRecordNotFound。
func TestUserRepository_GetByUsername_NotFound(t *testing.T) {
	c, mock := newMockContext(t)

	mock.ExpectQuery(regexp.QuoteMeta("SELECT")).
		WillReturnError(gorm.ErrRecordNotFound)

	_, err := newTestRepository().GetByUsername(c, "ghost")
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Errorf("err = %v, want gorm.ErrRecordNotFound", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}

// =============================================================================
// DecrementBalance
// =============================================================================

// TestUserRepository_DecrementBalance_Success 验证影响 1 行时返回 nil。
func TestUserRepository_DecrementBalance_Success(t *testing.T) {
	c, mock := newMockContext(t)

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("UPDATE")).
		WithArgs(50.0, sqlmock.AnyArg(), int64(7), 50.0).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	if err := newTestRepository().DecrementBalance(c, 7, 50.0); err != nil {
		t.Fatalf("DecrementBalance: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}

// TestUserRepository_DecrementBalance_Insufficient 验证影响 0 行时返回 ErrInsufficientBalance。
func TestUserRepository_DecrementBalance_Insufficient(t *testing.T) {
	c, mock := newMockContext(t)

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("UPDATE")).
		WithArgs(1000.0, sqlmock.AnyArg(), int64(7), 1000.0).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()

	err := newTestRepository().DecrementBalance(c, 7, 1000.0)
	if !errors.Is(err, ErrInsufficientBalance) {
		t.Errorf("err = %v, want ErrInsufficientBalance", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}

// TestUserRepository_DecrementBalance_NonPositive 验证 amount<=0 短路。
func TestUserRepository_DecrementBalance_NonPositive(t *testing.T) {
	c, mock := newMockContext(t)

	if err := newTestRepository().DecrementBalance(c, 7, 0); err == nil {
		t.Errorf("DecrementBalance(0) = nil, want err")
	}
	if err := newTestRepository().DecrementBalance(c, 7, -1); err == nil {
		t.Errorf("DecrementBalance(-1) = nil, want err")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v (expected zero SQL calls)", err)
	}
}

// =============================================================================
// IncrementBalance
// =============================================================================

// TestUserRepository_IncrementBalance_Success 验证影响 1 行时返回 nil。
func TestUserRepository_IncrementBalance_Success(t *testing.T) {
	c, mock := newMockContext(t)

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("UPDATE")).
		WithArgs(20.0, sqlmock.AnyArg(), int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	if err := newTestRepository().IncrementBalance(c, 7, 20.0); err != nil {
		t.Fatalf("IncrementBalance: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}

// TestUserRepository_IncrementBalance_NotFound 验证 id 不存在时返回 ErrUserNotFound。
func TestUserRepository_IncrementBalance_NotFound(t *testing.T) {
	c, mock := newMockContext(t)

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("UPDATE")).
		WithArgs(20.0, sqlmock.AnyArg(), int64(999)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()

	err := newTestRepository().IncrementBalance(c, 999, 20.0)
	if !errors.Is(err, ErrUserNotFound) {
		t.Errorf("err = %v, want ErrUserNotFound", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}

// TestUserRepository_IncrementBalance_NonPositive 验证 amount<=0 短路。
func TestUserRepository_IncrementBalance_NonPositive(t *testing.T) {
	c, mock := newMockContext(t)

	if err := newTestRepository().IncrementBalance(c, 7, 0); err == nil {
		t.Errorf("IncrementBalance(0) = nil, want err")
	}
	if err := newTestRepository().IncrementBalance(c, 7, -10); err == nil {
		t.Errorf("IncrementBalance(-10) = nil, want err")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v (expected zero SQL calls)", err)
	}
}

// =============================================================================
// Login / Status updates
// =============================================================================

// TestUserRepository_UpdateLoginSuccess 验证登录成功写三列（不含 password）。
func TestUserRepository_UpdateLoginSuccess(t *testing.T) {
	c, mock := newMockContext(t)

	at := time.Date(2026, 10, 1, 12, 0, 0, 0, time.Local)
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("UPDATE")).
		WithArgs(at, "10.0.0.1", 0, sqlmock.AnyArg(), int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	if err := newTestRepository().UpdateLoginSuccess(c, 7, "10.0.0.1", at); err != nil {
		t.Fatalf("UpdateLoginSuccess: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}

// TestUserRepository_IncrementLoginFailure_Success 验证 H1 修复：
// 仓储层走「原子自增」SQL（`login_failure = login_failure + ?`）而不是 service
// 传入的值；并通过同事务回读返回新值。
//
// 实际 GORM 生成的 SQL 形态（注意 `+1` 是 gorm.Expr 内联字面量、无占位符）：
//
//	UPDATE `users` SET `login_failure`=login_failure + 1, `updated_at`=? WHERE id = ? AND ...
//
// 因此 UPDATE 只有两个占位符：`updated_at` + `id`。
func TestUserRepository_IncrementLoginFailure_Success(t *testing.T) {
	c, mock := newMockContext(t)

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("UPDATE")).
		WithArgs(sqlmock.AnyArg(), int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	// 回读新值 —— 仅 SELECT login_failure 列
	mock.ExpectQuery(regexp.QuoteMeta("SELECT")).
		WithArgs(int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"login_failure"}).AddRow(6))
	mock.ExpectCommit()

	got, err := newTestRepository().IncrementLoginFailure(c, 7)
	if err != nil {
		t.Fatalf("IncrementLoginFailure: %v", err)
	}
	if got != 6 {
		t.Errorf("IncrementLoginFailure returned newFailure = %d, want 6", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}

// TestUserRepository_IncrementLoginFailure_NotFound 验证影响 0 行（用户不存在）
// 时返回 ErrUserNotFound。事务在 closure 返回 error 时由 GORM 回滚，不是提交。
func TestUserRepository_IncrementLoginFailure_NotFound(t *testing.T) {
	c, mock := newMockContext(t)

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("UPDATE")).
		WithArgs(sqlmock.AnyArg(), int64(999)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectRollback()

	got, err := newTestRepository().IncrementLoginFailure(c, 999)
	if !errors.Is(err, ErrUserNotFound) {
		t.Errorf("err = %v, want ErrUserNotFound", err)
	}
	if got != 0 {
		t.Errorf("newFailure = %d, want 0 on not-found", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}

// TestUserRepository_IncrementLoginFailure_UsesAtomicIncrement 关键回归：
// SQL 必须是 `login_failure = login_failure + <num>`（原子自增），
// 而**不**是 `login_failure = ?`（service 计算后覆写 —— 旧实现）。
//
// GORM 实际生成形如：
//
//	UPDATE `users` SET `login_failure`=login_failure + 1, `updated_at`=? WHERE ...
//
// 用 QueryMatcherRegexp 锁住「原子」语义不被未来 PR 改回覆写。
// 关键断言：UPDATE SQL 必须出现 `` `login_failure`=login_failure + 1 `` 这个完整片段。
func TestUserRepository_IncrementLoginFailure_UsesAtomicIncrement(t *testing.T) {
	c, mock := newMockContext(t)

	mock.ExpectBegin()
	// QuoteMeta 后整字面匹配原子自增的核心子句
	mock.ExpectExec(regexp.QuoteMeta("UPDATE `users` SET `login_failure`=login_failure + 1")).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT")).
		WillReturnRows(sqlmock.NewRows([]string{"login_failure"}).AddRow(1))
	mock.ExpectCommit()

	if _, err := newTestRepository().IncrementLoginFailure(c, 42); err != nil {
		t.Fatalf("IncrementLoginFailure: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}

// TestUserRepository_UpdateStatus 验证仅写 status 列。
func TestUserRepository_UpdateStatus(t *testing.T) {
	c, mock := newMockContext(t)

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("UPDATE")).
		WithArgs(int8(0), sqlmock.AnyArg(), int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	if err := newTestRepository().UpdateStatus(c, 7, 0); err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}