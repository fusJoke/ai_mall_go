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
// SeckillRepository - Activity 部分
// =============================================================================

// TestSeckillRepository_Create 验证 INSERT 后 ID 回填。
func TestSeckillRepository_Create(t *testing.T) {
	c, mock := newMockContext(t)

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("INSERT")).
		WillReturnResult(sqlmock.NewResult(42, 1))
	mock.ExpectCommit()

	got, err := NewSeckillRepository().Create(c, &mall.MallSeckillActivity{
		SupplierID:   1,
		BlindBoxID:    2,
		SeckillPrice:  79.0,
		TotalStock:    100,
		PerUserLimit:  1,
		StartAt:       time.Now(),
		EndAt:         time.Now().Add(24 * time.Hour),
		Status:        mall.StatusActive,
		RedisInitialized: false,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if got != 42 {
		t.Errorf("Create returned id=%d, want 42", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}

// TestSeckillRepository_GetByID_Found 验证主键命中。
func TestSeckillRepository_GetByID_Found(t *testing.T) {
	c, mock := newMockContext(t)

	rows := sqlmock.NewRows([]string{"id", "supplier_id", "total_stock"}).
		AddRow(int64(7), int64(3), 100)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT")).
		WillReturnRows(rows)

	got, err := NewSeckillRepository().GetByID(c, 7)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got == nil || got.TotalStock != 100 {
		t.Errorf("got = %+v, want TotalStock=100", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}

// TestSeckillRepository_GetByID_NotFound 验证主键未命中。
func TestSeckillRepository_GetByID_NotFound(t *testing.T) {
	c, mock := newMockContext(t)

	mock.ExpectQuery(regexp.QuoteMeta("SELECT")).
		WillReturnError(gorm.ErrRecordNotFound)

	got, err := NewSeckillRepository().GetByID(c, 999)
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("err = %v, want ErrRecordNotFound", err)
	}
	if got != nil {
		t.Errorf("got = %+v, want nil", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}

// TestSeckillRepository_Update 验证整行更新走 UPDATE。
func TestSeckillRepository_Update(t *testing.T) {
	c, mock := newMockContext(t)

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("UPDATE")).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	if err := NewSeckillRepository().Update(c, &mall.MallSeckillActivity{
		ID:           42,
		SupplierID:   1,
		BlindBoxID:    2,
		SeckillPrice:  69.0,
		TotalStock:    100,
		PerUserLimit:  2,
		Status:        mall.StatusActive,
	}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}

// TestSeckillRepository_MarkRedisInitialized 验证仅写 redis_initialized=true。
func TestSeckillRepository_MarkRedisInitialized(t *testing.T) {
	c, mock := newMockContext(t)

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("UPDATE")).
		WithArgs(true, sqlmock.AnyArg(), int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	if err := NewSeckillRepository().MarkRedisInitialized(c, 7); err != nil {
		t.Fatalf("MarkRedisInitialized: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}

// TestSeckillRepository_ListActive_Empty 验证 0 行时直接返回空切片。
func TestSeckillRepository_ListActive_Empty(t *testing.T) {
	c, mock := newMockContext(t)

	mock.ExpectQuery(`(?i)SELECT count`).
		WithArgs(mall.StatusActive, sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(0)))

	now := time.Now()
	items, total, err := NewSeckillRepository().ListActive(c, now, repository.ListOptions{Page: 1, PageSize: 20})
	if err != nil {
		t.Fatalf("ListActive: %v", err)
	}
	if total != 0 || len(items) != 0 {
		t.Errorf("total=%d items=%d, want 0/0", total, len(items))
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}

// TestSeckillRepository_ListActive_HasRows 验证分页查带 ORDER BY。
func TestSeckillRepository_ListActive_HasRows(t *testing.T) {
	c, mock := newMockContext(t)

	mock.ExpectQuery(`(?i)SELECT count`).
		WithArgs(mall.StatusActive, sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(1)))
	rows := sqlmock.NewRows([]string{"id", "total_stock"}).AddRow(int64(7), 100)
	mock.ExpectQuery(`(?i)SELECT`).
		WithArgs(mall.StatusActive, sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnRows(rows)

	now := time.Now()
	items, total, err := NewSeckillRepository().ListActive(c, now, repository.ListOptions{Page: 1, PageSize: 20})
	if err != nil {
		t.Fatalf("ListActive: %v", err)
	}
	if total != 1 || len(items) != 1 {
		t.Errorf("total=%d items=%d, want 1/1", total, len(items))
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}

// TestSeckillRepository_ListBySupplier 验证供应商维度分页。
func TestSeckillRepository_ListBySupplier(t *testing.T) {
	c, mock := newMockContext(t)

	mock.ExpectQuery(`(?i)SELECT count`).
		WithArgs(int64(3)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(1)))
	rows := sqlmock.NewRows([]string{"id", "supplier_id"}).AddRow(int64(7), int64(3))
	mock.ExpectQuery(`(?i)SELECT`).
		WithArgs(int64(3), sqlmock.AnyArg()).
		WillReturnRows(rows)

	items, total, err := NewSeckillRepository().ListBySupplier(c, 3, repository.ListOptions{Page: 1, PageSize: 20})
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

// TestSeckillRepository_List 验证 admin 视角全量列表（无 supplier_id 过滤）。
func TestSeckillRepository_List(t *testing.T) {
	c, mock := newMockContext(t)

	mock.ExpectQuery(`(?i)SELECT count`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(2)))
	rows := sqlmock.NewRows([]string{"id", "supplier_id"}).
		AddRow(int64(7), int64(3)).
		AddRow(int64(8), int64(4))
	mock.ExpectQuery(`(?i)SELECT`).
		WillReturnRows(rows)

	items, total, err := NewSeckillRepository().List(c, repository.ListOptions{Page: 1, PageSize: 20})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if total != 2 || len(items) != 2 {
		t.Errorf("total=%d items=%d, want 2/2", total, len(items))
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}

// TestSeckillRepository_ToggleStatus 验证仅写 status 列。
func TestSeckillRepository_ToggleStatus(t *testing.T) {
	c, mock := newMockContext(t)

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("UPDATE")).
		WithArgs("disabled", sqlmock.AnyArg(), int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	if err := NewSeckillRepository().ToggleStatus(c, 7, mall.StatusDisabled); err != nil {
		t.Fatalf("ToggleStatus: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}

// TestSeckillRepository_SumPoolStock 验证 SUM(stock) 查询。
func TestSeckillRepository_SumPoolStock(t *testing.T) {
	c, mock := newMockContext(t)

	mock.ExpectQuery(`(?i)SELECT`).
		WithArgs(int64(2)).
		WillReturnRows(sqlmock.NewRows([]string{"sum"}).AddRow(500))

	got, err := NewSeckillRepository().SumPoolStock(c, 2)
	if err != nil {
		t.Fatalf("SumPoolStock: %v", err)
	}
	if got != 500 {
		t.Errorf("got = %d, want 500", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}

// =============================================================================
// SeckillRepository - DeductionLog 部分
// =============================================================================

// TestSeckillRepository_InsertDeductionLog 验证 INSERT 走 status code 200。
func TestSeckillRepository_InsertDeductionLog(t *testing.T) {
	c, mock := newMockContext(t)

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("INSERT")).
		WillReturnResult(sqlmock.NewResult(99, 1))
	mock.ExpectCommit()

	if err := NewSeckillRepository().InsertDeductionLog(c, &mall.MallStockDeductionLog{
		SeckillID:   1,
		UserID:      2,
		BlindBoxID:   3,
		CardID:      4,
		Rarity:      mall.RaritySSR,
		DeductedAt: time.Now(),
	}); err != nil {
		t.Fatalf("InsertDeductionLog: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}

// TestSeckillRepository_MarkDeductionLogSynced 验证影响行数返回。
func TestSeckillRepository_MarkDeductionLogSynced(t *testing.T) {
	c, mock := newMockContext(t)

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("UPDATE")).
		WithArgs(sqlmock.AnyArg(), int64(99)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	rows, err := NewSeckillRepository().MarkDeductionLogSynced(c, 99, time.Now())
	if err != nil {
		t.Fatalf("MarkDeductionLogSynced: %v", err)
	}
	if rows != 1 {
		t.Errorf("rows = %d, want 1", rows)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}

// TestSeckillRepository_ListUnsyncedLogs 验证扫描待对账记录。
func TestSeckillRepository_ListUnsyncedLogs(t *testing.T) {
	c, mock := newMockContext(t)

	rows := sqlmock.NewRows([]string{"id", "seckill_id"}).
		AddRow(int64(1), int64(11)).
		AddRow(int64(2), int64(11))
	mock.ExpectQuery(`(?i)SELECT`).
		WillReturnRows(rows)

	got, err := NewSeckillRepository().ListUnsyncedLogs(c, 100)
	if err != nil {
		t.Fatalf("ListUnsyncedLogs: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("len(got) = %d, want 2", len(got))
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}

// 避免 unused errors 引用。
var _ = errors.New
// TestSeckillRepository_CountDeductionLogsBySeckillIDs 验证按活动聚合扣减数
// （C 端剩余名额 DB 兜底的取数口径）。
func TestSeckillRepository_CountDeductionLogsBySeckillIDs(t *testing.T) {
	c, mock := newMockContext(t)

	rows := sqlmock.NewRows([]string{"seckill_id", "cnt"}).
		AddRow(int64(11), int64(37)).
		AddRow(int64(12), int64(3))
	mock.ExpectQuery(`(?i)SELECT`).
		WillReturnRows(rows)

	got, err := NewSeckillRepository().CountDeductionLogsBySeckillIDs(c, []int64{11, 12, 13})
	if err != nil {
		t.Fatalf("CountDeductionLogsBySeckillIDs: %v", err)
	}
	if got[11] != 37 || got[12] != 3 {
		t.Errorf("got = %v, want 11→37, 12→3", got)
	}
	if _, ok := got[13]; ok {
		t.Errorf("id 13 should be absent (caller treats as 0), got %v", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}

// TestSeckillRepository_CountDeductionLogs_EmptyIDs 验证空入参短路（不打 SQL）。
func TestSeckillRepository_CountDeductionLogs_EmptyIDs(t *testing.T) {
	c, _ := newMockContext(t)

	got, err := NewSeckillRepository().CountDeductionLogsBySeckillIDs(c, nil)
	if err != nil || len(got) != 0 {
		t.Errorf("got = %v, err = %v; want empty map, nil", got, err)
	}
}
