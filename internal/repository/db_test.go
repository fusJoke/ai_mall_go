package repository

import (
	"errors"
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

// repository 包根级测试：锁定 RunInTx 的事务边界语义。
//
// 重点回归 OCR 审查 P1-5 的前置结论 —— GORM 的 DB.Begin() 不支持嵌套
// （已处于事务时 ConnPool 是 *sql.Tx，既不满足 TxBeginner 也不满足 ConnPoolBeginner，
// 会返回 ErrInvalidTransaction），因此 RunInTx 必须在嵌套时改用 SAVEPOINT。
// 没有这条，service 层就无法把多个「自带事务」的 repo 方法组合成一个原子单元。

// newTxProbeContext 用 sqlmock 造一个请求作用域 DB 的 gin.Context。
func newTxProbeContext(t *testing.T) (*gin.Context, sqlmock.Sqlmock) {
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
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
	c.Set(database.CtxKey, gdb)
	return c, mock
}

// TestRunInTx_OpensRealTransactionWhenNotNested 基线：非嵌套时走 BEGIN/COMMIT。
func TestRunInTx_OpensRealTransactionWhenNotNested(t *testing.T) {
	c, mock := newTxProbeContext(t)

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("SELECT 1")).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	if err := RunInTx(c, func(txCtx *gin.Context) error {
		return DB(txCtx).Exec("SELECT 1").Error
	}); err != nil {
		t.Fatalf("RunInTx: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}

// TestRunInTx_NestedUsesSavepoint 关键回归：嵌套时必须开 SAVEPOINT，
// **不得**再发一次 BEGIN（那会以 ErrInvalidTransaction 失败）。
func TestRunInTx_NestedUsesSavepoint(t *testing.T) {
	c, mock := newTxProbeContext(t)

	mock.ExpectBegin() // 只有外层有 BEGIN
	mock.ExpectExec(`SAVEPOINT sp_rit_\d+`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta("SELECT 1")).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit() // 只有外层有 COMMIT

	err := RunInTx(c, func(outer *gin.Context) error {
		return RunInTx(outer, func(inner *gin.Context) error {
			return DB(inner).Exec("SELECT 1").Error
		})
	})
	if err != nil {
		t.Fatalf("嵌套 RunInTx = %v, want nil（旧实现这里是 tx begin: invalid transaction）", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}

// TestRunInTx_NestedRollsBackToSavepoint 验证嵌套块失败只回滚到 SAVEPOINT，
// 外层事务仍能正常提交 —— 这正是「把多个 repo 方法组合成一个原子单元」的关键性质。
func TestRunInTx_NestedRollsBackToSavepoint(t *testing.T) {
	c, mock := newTxProbeContext(t)

	wantErr := errors.New("inner failed")
	mock.ExpectBegin()
	mock.ExpectExec(`SAVEPOINT sp_rit_\d+`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta("SELECT bad")).WillReturnError(wantErr)
	mock.ExpectExec(`ROLLBACK TO SAVEPOINT sp_rit_\d+`).WillReturnResult(sqlmock.NewResult(0, 1))
	// 外层继续做别的事并提交（嵌套块的回滚不影响外层）
	mock.ExpectExec(regexp.QuoteMeta("SELECT 2")).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	err := RunInTx(c, func(outer *gin.Context) error {
		if innerErr := RunInTx(outer, func(inner *gin.Context) error {
			return DB(inner).Exec("SELECT bad").Error
		}); innerErr == nil {
			t.Errorf("嵌套 RunInTx = nil, want %v", wantErr)
		}
		return DB(outer).Exec("SELECT 2").Error
	})
	if err != nil {
		t.Fatalf("外层 RunInTx = %v, want nil", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}

// TestRunInTx_SavepointNamesAreUnique 确认同一事务内多次嵌套不会重名 ——
// MySQL 方言把 SAVEPOINT 名**内联**进 SQL（非绑定参数），重名会互相覆盖，
// 使先前的回滚点失效。
func TestRunInTx_SavepointNamesAreUnique(t *testing.T) {
	c, mock := newTxProbeContext(t)

	seen := make(map[string]bool)
	mock.ExpectBegin()
	mock.ExpectExec(`SAVEPOINT sp_rit_\d+`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`SAVEPOINT sp_rit_\d+`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	err := RunInTx(c, func(outer *gin.Context) error {
		if err := RunInTx(outer, func(inner *gin.Context) error {
			seen["1"] = true
			return nil
		}); err != nil {
			return err
		}
		return RunInTx(outer, func(inner *gin.Context) error {
			seen["2"] = true
			return nil
		})
	})
	if err != nil {
		t.Fatalf("RunInTx: %v", err)
	}
	if len(seen) != 2 {
		t.Errorf("嵌套块执行次数 = %d, want 2", len(seen))
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}
