package mall

import (
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/gorm"

	"ai-go-mall/internal/model/mall"
	"ai-go-mall/internal/repository"
)

// =============================================================================
// PromotionRepository
// =============================================================================

// TestPromotionRepository_GetByID_Found 验证主键命中。
func TestPromotionRepository_GetByID_Found(t *testing.T) {
	c, mock := newMockContext(t)

	rows := sqlmock.NewRows([]string{"id", "blind_box_id"}).AddRow(int64(7), int64(3))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT")).
		WillReturnRows(rows)

	got, err := NewPromotionRepository().GetByID(c, 7)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got == nil || got.BlindBoxID != 3 {
		t.Errorf("GetByID = %+v, want BlindBoxID=3", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}

// TestPromotionRepository_FindActiveByBlindBoxID_NotFound 验证 NotFound 转 (nil, nil)。
func TestPromotionRepository_FindActiveByBlindBoxID_NotFound(t *testing.T) {
	c, mock := newMockContext(t)

	mock.ExpectQuery(regexp.QuoteMeta("SELECT")).
		WillReturnError(gorm.ErrRecordNotFound)

	got, err := NewPromotionRepository().FindActiveByBlindBoxID(c, 7, time.Now())
	if err != nil {
		t.Fatalf("err = %v, want nil on NotFound", err)
	}
	if got != nil {
		t.Errorf("got = %+v, want nil", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}

// TestPromotionRepository_FindActiveByBlindBoxID_Found 验证活动命中。
func TestPromotionRepository_FindActiveByBlindBoxID_Found(t *testing.T) {
	c, mock := newMockContext(t)

	now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.Local)
	rows := sqlmock.NewRows([]string{"id", "blind_box_id", "promo_price"}).
		AddRow(int64(7), int64(3), 80.0)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT")).
		WithArgs(int64(3), mall.StatusActive, now, now, sqlmock.AnyArg()).
		WillReturnRows(rows)

	got, err := NewPromotionRepository().FindActiveByBlindBoxID(c, 3, now)
	if err != nil {
		t.Fatalf("FindActiveByBlindBoxID: %v", err)
	}
	if got == nil || got.PromoPrice != 80.0 {
		t.Errorf("got = %+v, want PromoPrice=80", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}

// TestPromotionRepository_ListBySupplier 验证分页查。
func TestPromotionRepository_ListBySupplier(t *testing.T) {
	c, mock := newMockContext(t)

	mock.ExpectQuery(`(?i)SELECT count`).
		WithArgs(int64(3)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(1)))
	rows := sqlmock.NewRows([]string{"id", "supplier_id"}).AddRow(int64(7), int64(3))
	mock.ExpectQuery(`(?i)SELECT`).
		WithArgs(int64(3), sqlmock.AnyArg()).
		WillReturnRows(rows)

	items, total, err := NewPromotionRepository().ListBySupplier(c, 3, repository.ListOptions{Page: 1, PageSize: 20})
	if err != nil {
		t.Fatalf("ListBySupplier: %v", err)
	}
	if total != 1 || len(items) != 1 {
		t.Errorf("total=%d items=%d, want 1/1", total, len(items))
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}

// TestPromotionRepository_ListTimeOverlap 验证时间区间重叠 SQL。
func TestPromotionRepository_ListTimeOverlap(t *testing.T) {
	c, mock := newMockContext(t)

	startAt := time.Date(2026, 10, 1, 0, 0, 0, 0, time.Local)
	endAt := time.Date(2026, 10, 8, 0, 0, 0, 0, time.Local)
	rows := sqlmock.NewRows([]string{"id", "start_at", "end_at"}).
		AddRow(int64(7), startAt, endAt)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT")).
		WithArgs(int64(3), endAt, startAt).
		WillReturnRows(rows)

	overlaps, err := NewPromotionRepository().ListTimeOverlap(c, 3, startAt, endAt)
	if err != nil {
		t.Fatalf("ListTimeOverlap: %v", err)
	}
	if len(overlaps) != 1 {
		t.Errorf("len(overlaps) = %d, want 1", len(overlaps))
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}

// TestPromotionRepository_ToggleStatus 验证仅写 status 列。
func TestPromotionRepository_ToggleStatus(t *testing.T) {
	c, mock := newMockContext(t)

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("UPDATE")).
		WithArgs("disabled", sqlmock.AnyArg(), int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	if err := NewPromotionRepository().ToggleStatus(c, 7, mall.StatusDisabled); err != nil {
		t.Fatalf("ToggleStatus: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}

// 避免 unused errors 引用。
var _ = errors.New