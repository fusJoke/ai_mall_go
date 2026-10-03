package mall

import (
	"errors"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"

	"ai-go-mall/internal/model/mall"
)

// =============================================================================
// CardPoolRepository
// =============================================================================

// TestCardPoolRepository_GetPoolByBlindBoxID 验证按 blind_box_id 查 1:1 卡池。
func TestCardPoolRepository_GetPoolByBlindBoxID(t *testing.T) {
	c, mock := newMockContext(t)

	rows := sqlmock.NewRows([]string{"id", "blind_box_id"}).AddRow(int64(11), int64(7))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT")).
		WillReturnRows(rows)

	got, err := NewCardPoolRepository().GetPoolByBlindBoxID(c, 7)
	if err != nil {
		t.Fatalf("GetPoolByBlindBoxID: %v", err)
	}
	if got == nil || got.BlindBoxID != 7 {
		t.Errorf("GetPoolByBlindBoxID = %+v, want BlindBoxID=7", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}

// TestCardPoolRepository_CreatePool_Nil 验证 nil pool 短路。
func TestCardPoolRepository_CreatePool_Nil(t *testing.T) {
	c, mock := newMockContext(t)

	if err := NewCardPoolRepository().CreatePool(c, nil); err == nil {
		t.Errorf("CreatePool(nil) = nil, want err")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v (expected zero SQL calls)", err)
	}
}

// TestCardPoolRepository_CreatePool 验证 INSERT。
func TestCardPoolRepository_CreatePool(t *testing.T) {
	c, mock := newMockContext(t)

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO")).
		WillReturnResult(sqlmock.NewResult(11, 1))
	mock.ExpectCommit()

	pool := &mall.MallCardPool{BlindBoxID: 7}
	if err := NewCardPoolRepository().CreatePool(c, pool); err != nil {
		t.Fatalf("CreatePool: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}

// TestCardPoolRepository_ListItemsByPoolID 验证按 pool_id 查 items。
func TestCardPoolRepository_ListItemsByPoolID(t *testing.T) {
	c, mock := newMockContext(t)

	rows := sqlmock.NewRows([]string{"id", "pool_id", "rarity"}).
		AddRow(int64(1), int64(11), "SSR").
		AddRow(int64(2), int64(11), "R")
	mock.ExpectQuery(regexp.QuoteMeta("SELECT")).
		WithArgs(int64(11)).
		WillReturnRows(rows)

	items, err := NewCardPoolRepository().ListItemsByPoolID(c, 11)
	if err != nil {
		t.Fatalf("ListItemsByPoolID: %v", err)
	}
	if len(items) != 2 {
		t.Errorf("len(items) = %d, want 2", len(items))
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}

// TestCardPoolRepository_ListItemsByPoolIDs_Empty 验证空 pool ids 短路。
func TestCardPoolRepository_ListItemsByPoolIDs_Empty(t *testing.T) {
	c, mock := newMockContext(t)

	items, err := NewCardPoolRepository().ListItemsByPoolIDs(c, nil)
	if err != nil || len(items) != 0 {
		t.Errorf("ListItemsByPoolIDs(nil) = %v, items=%+v", err, items)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v (expected zero SQL calls)", err)
	}
}

// TestCardPoolRepository_CreateItems_Empty 验证空 items 短路。
func TestCardPoolRepository_CreateItems_Empty(t *testing.T) {
	c, mock := newMockContext(t)

	if err := NewCardPoolRepository().CreateItems(c, nil); err != nil {
		t.Errorf("CreateItems(nil) = %v, want nil", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v (expected zero SQL calls)", err)
	}
}

// TestCardPoolRepository_DecrementStock_Success 验证 stock-1 影响 1 行。
func TestCardPoolRepository_DecrementStock_Success(t *testing.T) {
	c, mock := newMockContext(t)

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("UPDATE")).
		WithArgs(sqlmock.AnyArg(), int64(99)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	if err := NewCardPoolRepository().DecrementStock(c, 99); err != nil {
		t.Fatalf("DecrementStock: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}

// TestCardPoolRepository_DecrementStock_Empty 验证影响 0 行返回 ErrStockEmpty。
func TestCardPoolRepository_DecrementStock_Empty(t *testing.T) {
	c, mock := newMockContext(t)

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("UPDATE")).
		WithArgs(sqlmock.AnyArg(), int64(99)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()

	err := NewCardPoolRepository().DecrementStock(c, 99)
	if !errors.Is(err, ErrStockEmpty) {
		t.Errorf("err = %v, want ErrStockEmpty", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}

// TestCardPoolRepository_IncrementStock_Success 验证 stock+delta 影响 1 行。
func TestCardPoolRepository_IncrementStock_Success(t *testing.T) {
	c, mock := newMockContext(t)

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("UPDATE")).
		WithArgs(10, sqlmock.AnyArg(), int64(99)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	if err := NewCardPoolRepository().IncrementStock(c, 99, 10); err != nil {
		t.Fatalf("IncrementStock: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}

// TestCardPoolRepository_IncrementStock_NotPositive 验证 delta<=0 短路。
func TestCardPoolRepository_IncrementStock_NotPositive(t *testing.T) {
	c, mock := newMockContext(t)

	if err := NewCardPoolRepository().IncrementStock(c, 99, 0); err == nil {
		t.Errorf("IncrementStock(0) = nil, want err")
	}
	if err := NewCardPoolRepository().IncrementStock(c, 99, -1); err == nil {
		t.Errorf("IncrementStock(-1) = nil, want err")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v (expected zero SQL calls)", err)
	}
}

// TestCardPoolRepository_SumWeightByPoolID 验证 SUM 聚合。
func TestCardPoolRepository_SumWeightByPoolID(t *testing.T) {
	c, mock := newMockContext(t)

	rows := sqlmock.NewRows([]string{"sum"}).AddRow(int64(10000))
	mock.ExpectQuery(`(?i)SELECT`).
		WithArgs(int64(11)).
		WillReturnRows(rows)

	got, err := NewCardPoolRepository().SumWeightByPoolID(c, 11)
	if err != nil {
		t.Fatalf("SumWeightByPoolID: %v", err)
	}
	if got != 10000 {
		t.Errorf("SumWeightByPoolID = %d, want 10000", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}

// TestCardPoolRepository_SubtotalStockByPoolID 验证 SUM 聚合。
func TestCardPoolRepository_SubtotalStockByPoolID(t *testing.T) {
	c, mock := newMockContext(t)

	rows := sqlmock.NewRows([]string{"sum"}).AddRow(int64(50))
	mock.ExpectQuery(`(?i)SELECT`).
		WithArgs(int64(11)).
		WillReturnRows(rows)

	got, err := NewCardPoolRepository().SubtotalStockByPoolID(c, 11)
	if err != nil {
		t.Fatalf("SubtotalStockByPoolID: %v", err)
	}
	if got != 50 {
		t.Errorf("SubtotalStockByPoolID = %d, want 50", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}