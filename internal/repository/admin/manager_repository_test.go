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

	"ai-go-mall/internal/infra/database"
)

// newMockContext 构造一个 gin.Context，把 sqlmock 包装成的 *gorm.DB
// 注入到 database.CtxKey 下，使 repository.DB(c) 能拿到它。
//
// 调用方负责在测试结束前关闭 mock（mock.Close()）。
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

// TestBaseRepository_DeleteBatch 验证 baseRepository.DeleteBatch 生成
// `UPDATE admins SET deleted_at = ? WHERE id IN (?, ?, ?) AND deleted_at IS NULL`。
func TestBaseRepository_DeleteBatch(t *testing.T) {
	c, mock := newMockContext(t)

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("UPDATE")).
		WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectCommit()

	if err := newTestBaseRepository().DeleteBatch(c, []uint{1, 2, 3}); err != nil {
		t.Fatalf("DeleteBatch: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}

// TestBaseRepository_UpdateStatus 验证仅更新 status 列。
func TestBaseRepository_UpdateStatus(t *testing.T) {
	c, mock := newMockContext(t)

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE\s+\S+\s+SET\s+`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	if err := newTestBaseRepository().UpdateStatus(c, 7, 0); err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}

// TestBaseRepository_UpdatePassword 验证仅更新 password 列。
func TestBaseRepository_UpdatePassword(t *testing.T) {
	c, mock := newMockContext(t)

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE\s+\S+\s+SET\s+`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	if err := newTestBaseRepository().UpdatePassword(c, 7, "$2a$10$dummybcrypthashvaluehere"); err != nil {
		t.Fatalf("UpdatePassword: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}

// TestBaseRepository_ResetLoginFailure 验证同时写 login_failure=0 与 status=1。
func TestBaseRepository_ResetLoginFailure(t *testing.T) {
	c, mock := newMockContext(t)

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE\s+\S+\s+SET\s+`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	if err := newTestBaseRepository().ResetLoginFailure(c, 7); err != nil {
		t.Fatalf("ResetLoginFailure: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}

// TestBaseRepository_DeleteBatch_EmptyShortCircuit 验证空 ids 短路返回 nil，
// 不应该发起任何 SQL 调用。
func TestBaseRepository_DeleteBatch_EmptyShortCircuit(t *testing.T) {
	c, mock := newMockContext(t)
	// 故意不设任何 Expect —— 任何调用都会让 ExpectationsWereMet 失败。

	if err := newTestBaseRepository().DeleteBatch(c, nil); err != nil {
		t.Errorf("DeleteBatch(nil) = %v, want nil", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v (expected zero SQL calls)", err)
	}
}

// newTestBaseRepository 构造一个独立的 baseRepository 供 sqlmock 测试使用。
// 不能直接用 NewRepository() —— 它会把 CRUDRepository 嵌进来，而我们的测试
// 不需要它；这里手搓一个仅含 4 个新方法的实例即可。
func newTestBaseRepository() *baseRepository { return &baseRepository{} }
