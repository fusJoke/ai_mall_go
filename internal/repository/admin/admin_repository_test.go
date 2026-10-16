package admin

import (
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

// admin_repository_test.go 覆盖 baseRepository 上 Login 专用的定向列更新方法
// （IncrementLoginFailure / UpdateLoginSuccess）。除断言 SQL 形态外，两个核心回归点：
//
//  1. **SET 子句不含 password 列**（P0 fix-admin-login-password-rehash）：
//     Login 落库若携带模型里哈希形态的 Password 走整行更新，会被 hashPasswordIfNeeded
//     二次哈希，导致正确密码永久失效。
//  2. **失败计数必须是原子自增**（OCR 审查 P1-1）：SQL 必须是
//     `login_failure = login_failure + <n>` 而不是 `login_failure = ?`。
//     后者意味着 service 在 Go 层算好绝对值再覆写 —— 并发错密码时读-改-写互相覆盖，
//     阈值永远打不到，管理员账号可被并发暴力破解。

// TestBaseRepository_IncrementLoginFailure 验证未触锁路径：
// 自增 login_failure 并在同事务内回读新值；SET 子句只有 login_failure 一列。
func TestBaseRepository_IncrementLoginFailure(t *testing.T) {
	c, mock := newMockContext(t)

	mock.ExpectBegin()
	// 全串匹配：SET 里业务列只能出现 login_failure（updated_at 为 GORM 自动维护，
	// 用 AnyArg 匹配），多出 password / status 等其他列即失败。
	mock.ExpectExec(regexp.QuoteMeta("UPDATE `admins` SET `login_failure`=login_failure + 1,`updated_at`=? WHERE id = ? AND `admins`.`deleted_at` IS NULL")).
		WithArgs(sqlmock.AnyArg(), uint(42)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT `login_failure` FROM `admins` WHERE id = ? AND `admins`.`deleted_at` IS NULL")).
		WithArgs(uint(42)).
		WillReturnRows(sqlmock.NewRows([]string{"login_failure"}).AddRow(1))
	mock.ExpectCommit()

	got, err := newTestBaseRepository().IncrementLoginFailure(c, 42)
	if err != nil {
		t.Fatalf("IncrementLoginFailure: %v", err)
	}
	if got != 1 {
		t.Errorf("IncrementLoginFailure = %d, want 1（回读的新值）", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}

// TestBaseRepository_IncrementLoginFailure_UsesAtomicIncrement 关键回归：
// 锁住「原子自增」语义，防止未来被改回「service 算好绝对值再覆写」的旧形态。
// 关键断言：UPDATE SQL 必须出现 `` `login_failure`=login_failure + 1 `` 这个完整片段。
func TestBaseRepository_IncrementLoginFailure_UsesAtomicIncrement(t *testing.T) {
	c, mock := newMockContext(t)

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("UPDATE `admins` SET `login_failure`=login_failure + 1")).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT `login_failure` FROM `admins`")).
		WillReturnRows(sqlmock.NewRows([]string{"login_failure"}).AddRow(3))
	mock.ExpectCommit()

	got, err := newTestBaseRepository().IncrementLoginFailure(c, 42)
	if err != nil {
		t.Fatalf("IncrementLoginFailure: %v", err)
	}
	if got != 3 {
		t.Errorf("IncrementLoginFailure = %d, want 3", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}

// TestBaseRepository_IncrementLoginFailure_NotFound 验证影响 0 行时返回 ErrAdminNotFound，
// 且事务回滚（不是提交）—— 与 user 侧 ErrUserNotFound 同语义。
func TestBaseRepository_IncrementLoginFailure_NotFound(t *testing.T) {
	c, mock := newMockContext(t)

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("UPDATE `admins` SET `login_failure`=login_failure + 1")).
		WithArgs(sqlmock.AnyArg(), uint(999)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectRollback()

	got, err := newTestBaseRepository().IncrementLoginFailure(c, 999)
	if !errors.Is(err, ErrAdminNotFound) {
		t.Errorf("err = %v, want ErrAdminNotFound", err)
	}
	if got != 0 {
		t.Errorf("IncrementLoginFailure = %d, want 0 on not-found", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}

// TestBaseRepository_UpdateLoginSuccess 验证登录成功记账一次写三列且不含 password。
func TestBaseRepository_UpdateLoginSuccess(t *testing.T) {
	c, mock := newMockContext(t)

	at := time.Date(2026, 10, 1, 12, 0, 0, 0, time.Local)
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("UPDATE `admins` SET `last_login_at`=?,`last_login_ip`=?,`login_failure`=?,`updated_at`=? WHERE id = ? AND `admins`.`deleted_at` IS NULL")).
		WithArgs(at, "192.0.2.1", 0, sqlmock.AnyArg(), uint(42)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	if err := newTestBaseRepository().UpdateLoginSuccess(c, 42, "192.0.2.1", at); err != nil {
		t.Fatalf("UpdateLoginSuccess: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}
