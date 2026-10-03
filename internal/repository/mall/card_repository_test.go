package mall

import (
	"errors"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/gorm"

	"ai-go-mall/internal/model/mall"
	"ai-go-mall/internal/repository"
)

// =============================================================================
// CardRepository
// =============================================================================

// TestCardRepository_GetByID_Found 验证主键命中。
func TestCardRepository_GetByID_Found(t *testing.T) {
	c, mock := newMockContext(t)

	rows := sqlmock.NewRows([]string{"id", "name"}).AddRow(int64(7), "Curry")
	mock.ExpectQuery(regexp.QuoteMeta("SELECT")).
		WillReturnRows(rows)

	got, err := NewCardRepository().GetByID(c, 7)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got == nil || got.ID != 7 || got.Name != "Curry" {
		t.Errorf("GetByID = %+v, want ID=7 Name=Curry", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}

// TestCardRepository_GetByID_NotFound 验证未命中透传 gorm.ErrRecordNotFound。
func TestCardRepository_GetByID_NotFound(t *testing.T) {
	c, mock := newMockContext(t)

	mock.ExpectQuery(regexp.QuoteMeta("SELECT")).
		WillReturnError(gorm.ErrRecordNotFound)

	_, err := NewCardRepository().GetByID(c, 999)
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Errorf("err = %v, want gorm.ErrRecordNotFound", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}

// TestCardRepository_ListByIDs_Empty 验证空 ids 短路返回空切片。
func TestCardRepository_ListByIDs_Empty(t *testing.T) {
	c, mock := newMockContext(t)

	items, err := NewCardRepository().ListByIDs(c, nil)
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

// TestCardRepository_ListByIDs_Found 验证 IN 单条 SQL。
func TestCardRepository_ListByIDs_Found(t *testing.T) {
	c, mock := newMockContext(t)

	rows := sqlmock.NewRows([]string{"id", "name"}).
		AddRow(int64(1), "A").
		AddRow(int64(2), "B")
	mock.ExpectQuery(regexp.QuoteMeta("SELECT")).
		WillReturnRows(rows)

	items, err := NewCardRepository().ListByIDs(c, []int64{1, 2})
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

// TestCardRepository_ListByTeam 验证按 team 批量取 + limit。
func TestCardRepository_ListByTeam(t *testing.T) {
	c, mock := newMockContext(t)

	rows := sqlmock.NewRows([]string{"id", "team"}).AddRow(int64(1), "LAL")
	mock.ExpectQuery(regexp.QuoteMeta("SELECT")).
		WithArgs("LAL", sqlmock.AnyArg()).
		WillReturnRows(rows)

	items, err := NewCardRepository().ListByTeam(c, "LAL", 10)
	if err != nil {
		t.Fatalf("ListByTeam: %v", err)
	}
	if len(items) != 1 {
		t.Errorf("len(items) = %d, want 1", len(items))
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}

// 避免 unused mall/repository 引用编译警告。
var _ = mall.MallCard{}
var _ = repository.ListOptions{}