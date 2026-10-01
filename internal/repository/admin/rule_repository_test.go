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
	"ai-go-mall/internal/model"
	"ai-go-mall/internal/repository"
)

// newRuleMockContext 构造一个 gin.Context，把 sqlmock 包装成的 *gorm.DB
// 注入到 database.CtxKey 下，使 repository.DB(c) 能拿到它。
//
// 与 manager_repository_test.go 的 newMockContext 同实现；这里独立一份避免
// 跨测试文件改动牵连。
func newRuleMockContext(t *testing.T) (*gin.Context, sqlmock.Sqlmock) {
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

// newTestRuleRepository 构造一个走 request-scoped DB 的 gormRuleRepository。
//
// 写方法（Create / Update / Delete / DeleteBatch / UpdateStatus / GetPID /
// HasChildren）走 repository.DB(c)，所以这里 r.db 可以是 nil —— 但保留
// 字段以便测试未来若加 ctx-free 路径不破坏构造器。
func newTestRuleRepository() *gormRuleRepository {
	return &gormRuleRepository{}
}

// TestRepository_Create 验证 Create 生成 INSERT SQL。
func TestRepository_Create(t *testing.T) {
	c, mock := newRuleMockContext(t)

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO")).
		WillReturnResult(sqlmock.NewResult(7, 1))
	mock.ExpectCommit()

	rule := &model.AdminRule{Type: model.RuleTypeMenu, Title: "用户管理", Name: "user"}
	if err := newTestRuleRepository().Create(c, rule); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}

// TestRepository_Update 验证 Update 生成 UPDATE SQL。
func TestRepository_Update(t *testing.T) {
	c, mock := newRuleMockContext(t)

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE\s+\S+\s+SET\s+`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	rule := &model.AdminRule{ID: 7, Title: "X"}
	if err := newTestRuleRepository().Update(c, rule); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}

// TestRepository_Delete 验证 Delete 生成软删 UPDATE SQL。
func TestRepository_Delete(t *testing.T) {
	c, mock := newRuleMockContext(t)

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("UPDATE")).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	if err := newTestRuleRepository().Delete(c, 7); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}

// TestRepository_DeleteBatch 验证 DeleteBatch 生成 IN 子句软删 SQL。
func TestRepository_DeleteBatch(t *testing.T) {
	c, mock := newRuleMockContext(t)

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("UPDATE")).
		WillReturnResult(sqlmock.NewResult(0, 3))
	mock.ExpectCommit()

	if err := newTestRuleRepository().DeleteBatch(c, []uint{1, 2, 3}); err != nil {
		t.Fatalf("DeleteBatch: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}

// TestRepository_DeleteBatch_EmptyShortCircuit 验证空 ids 短路返回 nil，
// 不应该发起任何 SQL 调用。
func TestRepository_DeleteBatch_EmptyShortCircuit(t *testing.T) {
	c, mock := newRuleMockContext(t)
	// 故意不设任何 Expect —— 任何调用都会让 ExpectationsWereMet 失败。

	if err := newTestRuleRepository().DeleteBatch(c, nil); err != nil {
		t.Errorf("DeleteBatch(nil) = %v, want nil", err)
	}
	if err := newTestRuleRepository().DeleteBatch(c, []uint{}); err != nil {
		t.Errorf("DeleteBatch([]) = %v, want nil", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v (expected zero SQL calls)", err)
	}
}

// TestRepository_UpdateStatus 验证仅更新 status 列。
func TestRepository_UpdateStatus(t *testing.T) {
	c, mock := newRuleMockContext(t)

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE\s+\S+\s+SET\s+`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	if err := newTestRuleRepository().UpdateStatus(c, 7, 0); err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}

// TestRepository_GetPID 验证 GetPID 生成 Pluck pid SQL。
func TestRepository_GetPID(t *testing.T) {
	c, mock := newRuleMockContext(t)

	rows := sqlmock.NewRows([]string{"pid"}).AddRow(uint(2))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT")).
		WillReturnRows(rows)

	got, err := newTestRuleRepository().GetPID(c, 7)
	if err != nil {
		t.Fatalf("GetPID: %v", err)
	}
	if got != 2 {
		t.Errorf("GetPID = %d, want 2", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}

// TestRepository_HasChildren_True 验证 HasChildren 命中子节点时返回 true。
func TestRepository_HasChildren_True(t *testing.T) {
	c, mock := newRuleMockContext(t)

	rows := sqlmock.NewRows([]string{"id"}).AddRow(uint(8))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT")).
		WillReturnRows(rows)

	ok, err := newTestRuleRepository().HasChildren(c, 7)
	if err != nil {
		t.Fatalf("HasChildren: %v", err)
	}
	if !ok {
		t.Errorf("HasChildren = false, want true")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}

// TestRepository_HasChildren_False 验证 HasChildren 无子节点时返回 false。
func TestRepository_HasChildren_False(t *testing.T) {
	c, mock := newRuleMockContext(t)

	rows := sqlmock.NewRows([]string{"id"})
	mock.ExpectQuery(regexp.QuoteMeta("SELECT")).
		WillReturnRows(rows)

	ok, err := newTestRuleRepository().HasChildren(c, 7)
	if err != nil {
		t.Fatalf("HasChildren: %v", err)
	}
	if ok {
		t.Errorf("HasChildren = true, want false")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}

// TestRepository_GetByID 验证 GetByID 生成主键查询 SQL。
func TestRepository_GetByID(t *testing.T) {
	c, mock := newRuleMockContext(t)

	rows := sqlmock.NewRows([]string{"id", "pid", "type", "title", "name"}).
		AddRow(uint(7), uint(0), "menu", "X", "x")
	mock.ExpectQuery(regexp.QuoteMeta("SELECT")).
		WillReturnRows(rows)

	got, err := newTestRuleRepository().GetByID(c, 7)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got == nil || got.ID != 7 {
		t.Errorf("GetByID = %+v, want ID=7", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}

// TestRepository_List 验证 List 生成 COUNT + 分页两条 SQL。
func TestRepository_List(t *testing.T) {
	c, mock := newRuleMockContext(t)

	mock.ExpectQuery(`(?i)SELECT count`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(42)))
	rows := sqlmock.NewRows([]string{"id", "pid", "type", "title", "name"}).
		AddRow(uint(1), uint(0), "menu", "A", "a").
		AddRow(uint(2), uint(0), "menu", "B", "b")
	mock.ExpectQuery(regexp.QuoteMeta("SELECT")).
		WillReturnRows(rows)

	items, total, err := newTestRuleRepository().List(c, repository.ListOptions{Page: 1, PageSize: 20})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if total != 42 {
		t.Errorf("total = %d, want 42", total)
	}
	if len(items) != 2 {
		t.Errorf("len(items) = %d, want 2", len(items))
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}
