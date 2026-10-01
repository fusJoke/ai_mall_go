package admin

import (
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

// admin_repository_test.go 覆盖 baseRepository 上 Login 专用的 2 个定向列更新方法
// （UpdateLoginFailure / UpdateLoginSuccess）。除断言 SQL 形态外，核心回归点是
// **SET 子句不含 password 列**（P0 fix-admin-login-password-rehash）：
// Login 落库若携带模型里哈希形态的 Password 走整行更新，会被 hashPasswordIfNeeded
// 二次哈希，导致正确密码永久失效。

// TestBaseRepository_UpdateLoginFailure 验证未触锁时仅更新 login_failure 列。
func TestBaseRepository_UpdateLoginFailure(t *testing.T) {
	c, mock := newMockContext(t)

	// 全串匹配：SET 里业务列只能出现 login_failure（updated_at 为 GORM 自动维护，
	// 用 AnyArg 匹配），多出 password 等其他列即失败。
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("UPDATE `admins` SET `login_failure`=?,`updated_at`=? WHERE id = ? AND `admins`.`deleted_at` IS NULL")).
		WithArgs(1, sqlmock.AnyArg(), uint(42)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	if err := newTestBaseRepository().UpdateLoginFailure(c, 42, 1, false); err != nil {
		t.Fatalf("UpdateLoginFailure: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}

// TestBaseRepository_UpdateLoginFailure_Locked 验证触锁时一次写 login_failure + status 两列。
func TestBaseRepository_UpdateLoginFailure_Locked(t *testing.T) {
	c, mock := newMockContext(t)

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("UPDATE `admins` SET `login_failure`=?,`status`=?,`updated_at`=? WHERE id = ? AND `admins`.`deleted_at` IS NULL")).
		WithArgs(5, int8(0), sqlmock.AnyArg(), uint(42)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	if err := newTestBaseRepository().UpdateLoginFailure(c, 42, 5, true); err != nil {
		t.Fatalf("UpdateLoginFailure(locked): %v", err)
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
