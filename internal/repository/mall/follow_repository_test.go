package mall

import (
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/gorm"

	"ai-go-mall/internal/repository"
)

// =============================================================================
// FollowRepository
// =============================================================================

// TestFollowRepository_Follow_New 验证未关注时插入新行。
func TestFollowRepository_Follow_New(t *testing.T) {
	c, mock := newMockContext(t)

	mock.ExpectQuery(regexp.QuoteMeta("SELECT")).
		WillReturnError(gorm.ErrRecordNotFound)
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("INSERT")).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	if err := NewFollowRepository().Follow(c, 1, 2); err != nil {
		t.Fatalf("Follow: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}

// TestFollowRepository_Follow_AlreadyActive 验证已关注时为 no-op。
func TestFollowRepository_Follow_AlreadyActive(t *testing.T) {
	c, mock := newMockContext(t)

	rows := sqlmock.NewRows([]string{"id", "deleted_at"}).AddRow(int64(7), nil)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT")).
		WillReturnRows(rows)

	if err := NewFollowRepository().Follow(c, 1, 2); err != nil {
		t.Fatalf("Follow: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}

// TestFollowRepository_Follow_ReviveSoftDeleted 验证取关后再次关注走复活路径。
func TestFollowRepository_Follow_ReviveSoftDeleted(t *testing.T) {
	c, mock := newMockContext(t)

	deletedAt := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	rows := sqlmock.NewRows([]string{"id", "deleted_at"}).AddRow(int64(7), deletedAt)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT")).
		WillReturnRows(rows)
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("UPDATE")).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	if err := NewFollowRepository().Follow(c, 1, 2); err != nil {
		t.Fatalf("Follow: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}

// TestFollowRepository_Follow_InvalidIDs 验证参数校验短路返回。
func TestFollowRepository_Follow_InvalidIDs(t *testing.T) {
	c, mock := newMockContext(t)

	if err := NewFollowRepository().Follow(c, 0, 2); err == nil {
		t.Errorf("Follow(0,2) = nil, want error")
	}
	if err := NewFollowRepository().Follow(c, 1, -1); err == nil {
		t.Errorf("Follow(1,-1) = nil, want error")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}

// TestFollowRepository_Unfollow 验证软删 SQL。
func TestFollowRepository_Unfollow(t *testing.T) {
	c, mock := newMockContext(t)

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("UPDATE")).
		WithArgs(sqlmock.AnyArg(), int64(1), int64(2)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	if err := NewFollowRepository().Unfollow(c, 1, 2); err != nil {
		t.Fatalf("Unfollow: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}

// TestFollowRepository_IsFollowing_True 验证 count > 0 时返回 true。
func TestFollowRepository_IsFollowing_True(t *testing.T) {
	c, mock := newMockContext(t)

	mock.ExpectQuery(regexp.QuoteMeta("SELECT count")).
		WithArgs(int64(1), int64(2)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(1)))

	ok, err := NewFollowRepository().IsFollowing(c, 1, 2)
	if err != nil {
		t.Fatalf("IsFollowing: %v", err)
	}
	if !ok {
		t.Errorf("IsFollowing = false, want true")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}

// TestFollowRepository_IsFollowing_False 验证 count == 0 时返回 false。
func TestFollowRepository_IsFollowing_False(t *testing.T) {
	c, mock := newMockContext(t)

	mock.ExpectQuery(regexp.QuoteMeta("SELECT count")).
		WithArgs(int64(1), int64(2)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(0)))

	ok, err := NewFollowRepository().IsFollowing(c, 1, 2)
	if err != nil {
		t.Fatalf("IsFollowing: %v", err)
	}
	if ok {
		t.Errorf("IsFollowing = true, want false")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}

// TestFollowRepository_IsFollowing_InvalidIDs 验证无效 ID 返回 (false, nil) 而不打 DB。
func TestFollowRepository_IsFollowing_InvalidIDs(t *testing.T) {
	c, mock := newMockContext(t)

	ok, err := NewFollowRepository().IsFollowing(c, 0, 2)
	if err != nil || ok {
		t.Errorf("IsFollowing(0,2) = (%v,%v), want (false,nil)", ok, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}

// TestFollowRepository_ListFollowing_Empty 验证 total=0 时不打第二次查询。
func TestFollowRepository_ListFollowing_Empty(t *testing.T) {
	c, mock := newMockContext(t)

	mock.ExpectQuery(`(?i)SELECT count`).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(0)))

	items, total, err := NewFollowRepository().ListFollowing(c, 1, repository.ListOptions{Page: 1, PageSize: 20})
	if err != nil {
		t.Fatalf("ListFollowing: %v", err)
	}
	if total != 0 || len(items) != 0 {
		t.Errorf("total=%d items=%d, want 0/0", total, len(items))
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}

// TestFollowRepository_ListFollowing_Page 验证分页 + JOIN 查 supplier。
func TestFollowRepository_ListFollowing_Page(t *testing.T) {
	c, mock := newMockContext(t)

	mock.ExpectQuery(`(?i)SELECT count`).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(1)))
	rows := sqlmock.NewRows([]string{"id", "name"}).AddRow(int64(2), "supplier-A")
	mock.ExpectQuery(`(?i)SELECT`).
		WithArgs(int64(1), sqlmock.AnyArg()).
		WillReturnRows(rows)

	items, total, err := NewFollowRepository().ListFollowing(c, 1, repository.ListOptions{Page: 1, PageSize: 20})
	if err != nil {
		t.Fatalf("ListFollowing: %v", err)
	}
	if total != 1 || len(items) != 1 {
		t.Errorf("total=%d items=%d, want 1/1", total, len(items))
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}

// TestFollowRepository_ListFollowers_Empty 验证粉丝列表为空场景。
func TestFollowRepository_ListFollowers_Empty(t *testing.T) {
	c, mock := newMockContext(t)

	mock.ExpectQuery(`(?i)SELECT count`).
		WithArgs(int64(2)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(0)))

	items, total, err := NewFollowRepository().ListFollowers(c, 2, repository.ListOptions{Page: 1, PageSize: 20})
	if err != nil {
		t.Fatalf("ListFollowers: %v", err)
	}
	if total != 0 || len(items) != 0 {
		t.Errorf("total=%d items=%d, want 0/0", total, len(items))
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}

// TestFollowRepository_ListFollowers_Page 验证粉丝列表 + Distinct pluck。
func TestFollowRepository_ListFollowers_Page(t *testing.T) {
	c, mock := newMockContext(t)

	mock.ExpectQuery(`(?i)SELECT count`).
		WithArgs(int64(2)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(2)))
	mock.ExpectQuery(`(?i)SELECT DISTINCT`).
		WithArgs(int64(2), sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow(int64(11)).AddRow(int64(22)))

	items, total, err := NewFollowRepository().ListFollowers(c, 2, repository.ListOptions{Page: 1, PageSize: 20})
	if err != nil {
		t.Fatalf("ListFollowers: %v", err)
	}
	if total != 2 || len(items) != 2 {
		t.Errorf("total=%d items=%d, want 2/2", total, len(items))
	}
	if items[0] != 11 || items[1] != 22 {
		t.Errorf("items = %v, want [11 22]", items)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}