package mall

import (
	"errors"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"ai-go-mall/internal/model/mall"
	"ai-go-mall/internal/repository"
)

// SeckillRepository 定义 mall_seckill_activities 与 mall_stock_deduction_log 表的数据访问接口。
//
// 设计原则：
//   - Create 同事务返回新 ID，由服务层在同事务外 SET Redis 库存保证原子；
//     mall_seckill_activities.redis_initialized 标志 Redis 库存是否已 init。
//   - ListActive 走 (status, start_at, end_at) 索引（D15 决策点 1 的列表查询）。
//   - SumPoolStock 服务层校验"按需设置 total_stock ≤ 卡池库存"（D15 决策点 2）。
type SeckillRepository interface {
	// Activity 部分。

	// Create INSERT 一条秒杀活动，返回新行 ID。
	Create(c *gin.Context, s *mall.MallSeckillActivity) (int64, error)

	// GetByID 按主键查单行（含软删除过滤）。
	GetByID(c *gin.Context, id int64) (*mall.MallSeckillActivity, error)

	// Update 整行更新（业务字段全量覆盖）。
	Update(c *gin.Context, s *mall.MallSeckillActivity) error

	// MarkRedisInitialized 标记 Redis 库存已初始化（创建活动后由 service 同步）。
	MarkRedisInitialized(c *gin.Context, id int64) error

	// ListActive 拉当前生效的秒杀活动列表（status=active AND now IN [start_at, end_at)）。
	ListActive(c *gin.Context, now time.Time, opts repository.ListOptions) ([]mall.MallSeckillActivity, int64, error)

	// ListBySupplier 拉某供应商的全部秒杀活动（供应商秒杀管理页）。
	ListBySupplier(c *gin.Context, supplierID int64, opts repository.ListOptions) ([]mall.MallSeckillActivity, int64, error)

	// List 拉全量秒杀活动（admin 视角管理列表；按 start_at DESC）。
	List(c *gin.Context, opts repository.ListOptions) ([]mall.MallSeckillActivity, int64, error)

	// ToggleStatus 仅写 Status 列（admin / supplier 启停）。
	ToggleStatus(c *gin.Context, id int64, status mall.MallBlindBoxStatus) error

	// SumPoolStock 统计某盲盒卡池总剩余库存（service 层校验秒杀 total_stock）。
	SumPoolStock(c *gin.Context, blindBoxID int64) (int, error)

	// DeductionLog 部分。

	// InsertDeductionLog INSERT 一条扣减记录（秒杀事务内调用）。
	InsertDeductionLog(c *gin.Context, log *mall.MallStockDeductionLog) error

	// CountDeductionLogsBySeckillIDs 按 seckill_id 聚合扣减记录数
	// （C 端浏览的剩余名额 DB 兜底：total_stock − 已扣数；走 idx_mall_stock_log_seckill）。
	CountDeductionLogsBySeckillIDs(c *gin.Context, seckillIDs []int64) (map[int64]int64, error)

	// MarkDeductionLogSynced 把 synced_to_pool_at 写为 NOW()（cmd/stock-sync 异步对账）。
	//
	// 幂等：若已 synced 则影响行数 = 0；调用方按影响行数判断"是否需要做对账"。
	MarkDeductionLogSynced(c *gin.Context, logID int64, syncedAt time.Time) (int64, error)

	// ListUnsyncedLogs 拉 synced_to_pool_at IS NULL 的记录（cmd/stock-sync cron 兜底）。
	ListUnsyncedLogs(c *gin.Context, limit int) ([]mall.MallStockDeductionLog, error)
}

// gormSeckillRepository 是 SeckillRepository 的 *gorm.DB 实现。
type gormSeckillRepository struct{}

// NewSeckillRepository 返回 SeckillRepository 接口。
func NewSeckillRepository() SeckillRepository {
	return &gormSeckillRepository{}
}

// Create INSERT 一条秒杀活动，返回新行 ID。
func (r *gormSeckillRepository) Create(c *gin.Context, s *mall.MallSeckillActivity) (int64, error) {
	if err := repository.DB(c).Create(s).Error; err != nil {
		return 0, err
	}
	return s.ID, nil
}

// GetByID 按主键查单行。
func (r *gormSeckillRepository) GetByID(c *gin.Context, id int64) (*mall.MallSeckillActivity, error) {
	var s mall.MallSeckillActivity
	if err := repository.DB(c).First(&s, id).Error; err != nil {
		return nil, err
	}
	return &s, nil
}

// Update 整行更新。
func (r *gormSeckillRepository) Update(c *gin.Context, s *mall.MallSeckillActivity) error {
	return repository.DB(c).Save(s).Error
}

// MarkRedisInitialized 标记 Redis 库存已初始化。
func (r *gormSeckillRepository) MarkRedisInitialized(c *gin.Context, id int64) error {
	return repository.DB(c).
		Model(&mall.MallSeckillActivity{}).
		Where("id = ?", id).
		Update("redis_initialized", true).Error
}

// ListActive 拉当前生效的秒杀活动。
func (r *gormSeckillRepository) ListActive(c *gin.Context, now time.Time, opts repository.ListOptions) ([]mall.MallSeckillActivity, int64, error) {
	db := repository.DB(c).Model(&mall.MallSeckillActivity{}).
		Where("status = ? AND start_at <= ? AND end_at > ?", mall.StatusActive, now, now)

	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	items := make([]mall.MallSeckillActivity, 0)
	if total == 0 {
		return items, total, nil
	}

	offset := (opts.Page - 1) * opts.PageSize
	if err := db.Offset(offset).Limit(opts.PageSize).
		Order("start_at ASC").
		Find(&items).Error; err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

// ListBySupplier 拉某供应商的全部秒杀活动。
func (r *gormSeckillRepository) ListBySupplier(c *gin.Context, supplierID int64, opts repository.ListOptions) ([]mall.MallSeckillActivity, int64, error) {
	db := repository.DB(c).Model(&mall.MallSeckillActivity{}).Where("supplier_id = ?", supplierID)

	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	items := make([]mall.MallSeckillActivity, 0)
	if total == 0 {
		return items, total, nil
	}

	offset := (opts.Page - 1) * opts.PageSize
	if err := db.Offset(offset).Limit(opts.PageSize).
		Order("start_at DESC").
		Find(&items).Error; err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

// List 拉全量秒杀活动（admin 视角，按 start_at DESC）。
func (r *gormSeckillRepository) List(c *gin.Context, opts repository.ListOptions) ([]mall.MallSeckillActivity, int64, error) {
	db := repository.DB(c).Model(&mall.MallSeckillActivity{})

	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	items := make([]mall.MallSeckillActivity, 0)
	if total == 0 {
		return items, total, nil
	}

	offset := (opts.Page - 1) * opts.PageSize
	if err := db.Offset(offset).Limit(opts.PageSize).
		Order("start_at DESC").
		Find(&items).Error; err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

// ToggleStatus 仅写 Status 列。
func (r *gormSeckillRepository) ToggleStatus(c *gin.Context, id int64, status mall.MallBlindBoxStatus) error {
	return repository.DB(c).
		Model(&mall.MallSeckillActivity{}).
		Where("id = ?", id).
		Update("status", status).Error
}

// SumPoolStock 统计某盲盒卡池总剩余库存。
//
// SQL：`SELECT COALESCE(SUM(stock), 0) FROM mall_card_pool_items WHERE blind_box_id IN
//
//	(SELECT id FROM mall_card_pools WHERE blind_box_id = ?) AND deleted_at IS NULL`
//
// 实际写法走 mall_card_pool_items JOIN mall_card_pools（pool_id 关联 pool，pool.blind_box_id 关联盲盒）。
func (r *gormSeckillRepository) SumPoolStock(c *gin.Context, blindBoxID int64) (int, error) {
	var total int
	err := repository.DB(c).
		Table("mall_card_pool_items AS i").
		Joins("JOIN mall_card_pools AS p ON p.id = i.pool_id").
		Where("p.blind_box_id = ?", blindBoxID).
		Where("i.deleted_at IS NULL AND p.deleted_at IS NULL").
		Select("COALESCE(SUM(i.stock), 0)").
		Scan(&total).Error
	if err != nil {
		return 0, err
	}
	return total, nil
}

// InsertDeductionLog INSERT 一条扣减记录。
func (r *gormSeckillRepository) InsertDeductionLog(c *gin.Context, log *mall.MallStockDeductionLog) error {
	return repository.DB(c).Create(log).Error
}

// CountDeductionLogsBySeckillIDs 按 seckill_id 聚合扣减记录数。
//
// 单条 SQL 批量查（GROUP BY），供 C 端列表页一次兜底整页活动；
// 查不到的 id 不出现在返回 map 里，调用方按 0 处理。
func (r *gormSeckillRepository) CountDeductionLogsBySeckillIDs(c *gin.Context, seckillIDs []int64) (map[int64]int64, error) {
	out := make(map[int64]int64, len(seckillIDs))
	if len(seckillIDs) == 0 {
		return out, nil
	}
	var rows []struct {
		SeckillID int64 `gorm:"column:seckill_id"`
		Cnt       int64 `gorm:"column:cnt"`
	}
	err := repository.DB(c).
		Model(&mall.MallStockDeductionLog{}).
		Select("seckill_id, COUNT(*) AS cnt").
		Where("seckill_id IN ?", seckillIDs).
		Group("seckill_id").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		out[row.SeckillID] = row.Cnt
	}
	return out, nil
}

// MarkDeductionLogSynced 把 synced_to_pool_at 写为 NOW()。
//
// 返回受影响行数（0 表示行不存在或已 synced）。
func (r *gormSeckillRepository) MarkDeductionLogSynced(c *gin.Context, logID int64, syncedAt time.Time) (int64, error) {
	res := repository.DB(c).
		Model(&mall.MallStockDeductionLog{}).
		Where("id = ? AND synced_to_pool_at IS NULL", logID).
		Update("synced_to_pool_at", syncedAt)
	return res.RowsAffected, res.Error
}

// ListUnsyncedLogs 拉 synced_to_pool_at IS NULL 的记录，按 deducted_at 升序。
func (r *gormSeckillRepository) ListUnsyncedLogs(c *gin.Context, limit int) ([]mall.MallStockDeductionLog, error) {
	rows := make([]mall.MallStockDeductionLog, 0)
	err := repository.DB(c).
		Where("synced_to_pool_at IS NULL").
		Order("deducted_at ASC").
		Limit(limit).
		Find(&rows).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	return rows, nil
}

// 编译期断言。
var _ SeckillRepository = (*gormSeckillRepository)(nil)