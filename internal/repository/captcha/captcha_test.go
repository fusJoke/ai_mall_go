package captcha

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	"ai-go-mall/internal/model"
)

// ============================================================
// helpers
// ============================================================

// newMockRepo 构造 Repository + sqlmock，配对的 sqlmock.MatchExpectationsInOrder。
func newMockRepo(t *testing.T) (Repository, sqlmock.Sqlmock, func()) {
	t.Helper()
	mockDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	// sqlmock 不直接支持 GORM Dialector —— 用 gorm.io/driver/mysql 包装。
	// mysql.New 需要 *sql.DB，这里用 sqlmock 的连接。
	gdb, err := gorm.Open(mysql.New(mysql.Config{
		Conn: mockDB,
		SkipInitializeWithVersion: true,
	}), &gorm.Config{})
	if err != nil {
		t.Fatalf("gorm.Open: %v", err)
	}

	repo := newWithDB(func() *gorm.DB { return gdb })
	return repo, mock, func() { _ = mockDB.Close() }
}

// ============================================================
// Create
// ============================================================

func TestCaptchaRepository_Create(t *testing.T) {
	repo, mock, cleanup := newMockRepo(t)
	defer cleanup()

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO")).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	code := "encrypted-answer"
	cpt := &model.Captcha{Key: "k1", Code: &code, ExpiresAt: time.Now().Add(time.Minute)}
	if err := repo.Create(context.Background(), cpt); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}

func TestCaptchaRepository_Create_Nil(t *testing.T) {
	repo, mock, cleanup := newMockRepo(t)
	defer cleanup()

	if err := repo.Create(context.Background(), nil); err == nil {
		t.Errorf("Create(nil) = nil, want err")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v (expected zero SQL calls)", err)
	}
}

// ============================================================
// GetByKey
// ============================================================

func TestCaptchaRepository_GetByKey_Found(t *testing.T) {
	repo, mock, cleanup := newMockRepo(t)
	defer cleanup()

	rows := sqlmock.NewRows([]string{"key", "code", "expires_at"}).
		AddRow("k1", "encrypted-answer", time.Now().Add(time.Minute))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT")).
		WithArgs("k1", sqlmock.AnyArg()). // GORM First 加 LIMIT 1 占位
		WillReturnRows(rows)

	got, err := repo.GetByKey(context.Background(), "k1")
	if err != nil {
		t.Fatalf("GetByKey: %v", err)
	}
	if got == nil || got.Key != "k1" || got.Code == nil || *got.Code != "encrypted-answer" {
		t.Errorf("GetByKey = %+v", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}

func TestCaptchaRepository_GetByKey_NotFound(t *testing.T) {
	repo, mock, cleanup := newMockRepo(t)
	defer cleanup()

	mock.ExpectQuery(regexp.QuoteMeta("SELECT")).
		WithArgs("missing", sqlmock.AnyArg()). // GORM First 加 LIMIT 1 占位
		WillReturnError(gorm.ErrRecordNotFound)

	_, err := repo.GetByKey(context.Background(), "missing")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("GetByKey missing = %v, want ErrNotFound", err)
	}
}

func TestCaptchaRepository_GetByKey_Empty(t *testing.T) {
	repo, _, cleanup := newMockRepo(t)
	defer cleanup()

	if _, err := repo.GetByKey(context.Background(), ""); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetByKey empty = %v, want ErrNotFound", err)
	}
}

// ============================================================
// DeleteByKey
// ============================================================

func TestCaptchaRepository_DeleteByKey(t *testing.T) {
	repo, mock, cleanup := newMockRepo(t)
	defer cleanup()

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("DELETE")).
		WithArgs("k1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	if err := repo.DeleteByKey(context.Background(), "k1"); err != nil {
		t.Fatalf("DeleteByKey: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}

func TestCaptchaRepository_DeleteByKey_Empty(t *testing.T) {
	repo, mock, cleanup := newMockRepo(t)
	defer cleanup()

	if err := repo.DeleteByKey(context.Background(), ""); err != nil {
		t.Errorf("DeleteByKey empty = %v, want nil", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v (expected zero SQL calls)", err)
	}
}

// ============================================================
// DeleteExpired
// ============================================================

func TestCaptchaRepository_DeleteExpired(t *testing.T) {
	repo, mock, cleanup := newMockRepo(t)
	defer cleanup()

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("DELETE")).
		WithArgs(sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 5))
	mock.ExpectCommit()

	deleted, err := repo.DeleteExpired(context.Background(), time.Now())
	if err != nil {
		t.Fatalf("DeleteExpired: %v", err)
	}
	if deleted != 5 {
		t.Errorf("deleted = %d, want 5", deleted)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}