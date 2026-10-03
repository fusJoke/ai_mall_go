package mall

import (
	"errors"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"ai-go-mall/internal/model/mall"
	"ai-go-mall/internal/repository"
)

// CardPoolRepository 定义 mall_card_pools + mall_card_pool_items 表的数据访问接口。
//
// 设计原则：
//   - GetPoolByBlindBoxID 走 blind_box_id UK（1:1）；
//   - ListItemsByPoolID 走 (pool_id, stock) 索引；
//   - DecrementStock 走条件更新 `SET stock = stock - 1 WHERE id = ? AND stock > 0`，
//     抽卡事务 D3 步骤 1 的原子保证（同 mall_card_pool_items 表的索引走此列）。
type CardPoolRepository interface {
	// GetPoolByBlindBoxID 走 blind_box_id UK 查 1:1 卡池。
	GetPoolByBlindBoxID(c *gin.Context, blindBoxID int64) (*mall.MallCardPool, error)

	// CreatePool 插入新卡池（事务内使用）。entity 为 nil 时返回错误。
	CreatePool(c *gin.Context, pool *mall.MallCardPool) error

	// ListItemsByPoolID 拉牌池所有 items（卡池预览 / 抽卡算法）。
	// 含 stock=0 的条目（管理页 / 概率公示），调用方按需剔除。
	ListItemsByPoolID(c *gin.Context, poolID int64) ([]mall.MallCardPoolItem, error)

	// ListItemsForUpdateByPoolID 事务内专用：拉牌池 items 并加 FOR UPDATE 行锁。
	//
	// 仅在 service.user.Draw 抽卡事务（D3 步骤 1）内调用，repository.DB(c) 应是 tx；
	// 非事务上下文调用无意义（FOR UPDATE 在 autocommit 下退化为普通 SELECT）。
	//
	// 与 ListItemsByPoolID 的差别：本方法自动加 stock > 0 过滤 + FOR UPDATE 锁，
	// 调用方拿到的是「可抽中」的候选 items 集合。
	ListItemsForUpdateByPoolID(c *gin.Context, poolID int64) ([]mall.MallCardPoolItem, error)

	// ListItemsByPoolIDs 按 pool id 集合批量取 items（首页批量预览）。
	ListItemsByPoolIDs(c *gin.Context, poolIDs []int64) ([]mall.MallCardPoolItem, error)

	// CreateItem 插入卡牌条目。
	CreateItem(c *gin.Context, item *mall.MallCardPoolItem) error

	// CreateItems 批量插入卡牌条目。空 items 短路返回 nil。
	CreateItems(c *gin.Context, items []mall.MallCardPoolItem) error

	// DecrementStock 条件扣减 stock（抽卡事务 D3 步骤 4 关键）：
	//   `UPDATE mall_card_pool_items SET stock = stock - 1 WHERE id = ? AND stock > 0`
	// id 不存在 / 已软删 / stock=0 均影响 0 行 → 返回 ErrStockEmpty。
	DecrementStock(c *gin.Context, itemID int64) error

	// IncrementStock 条件增加 stock（admin 手动补货 / 秒杀活动结束回填）。
	IncrementStock(c *gin.Context, itemID int64, delta int) error

	// SumWeightByPoolID 拉某 pool 的 items 总 weight，service 层校验
	// 「weight 加和 = 10000」用。
	SumWeightByPoolID(c *gin.Context, poolID int64) (int64, error)

	// SubtotalStockByPoolID 拉某 pool 的总库存（卡池余量显示）。
	SubtotalStockByPoolID(c *gin.Context, poolID int64) (int64, error)
}

// ErrStockEmpty 库存不足 / item 不存在。DecrementStock 影响 0 行时返回。
var ErrStockEmpty = errors.New("cardpool: stock empty")

// gormCardPoolRepository 是 CardPoolRepository 的 *gorm.DB 实现。
type gormCardPoolRepository struct{}

// NewCardPoolRepository 返回 CardPoolRepository 接口。
func NewCardPoolRepository() CardPoolRepository {
	return &gormCardPoolRepository{}
}

// GetPoolByBlindBoxID 走 blind_box_id UK 查 1:1 卡池。
func (r *gormCardPoolRepository) GetPoolByBlindBoxID(c *gin.Context, blindBoxID int64) (*mall.MallCardPool, error) {
	var pool mall.MallCardPool
	if err := repository.DB(c).
		Where("blind_box_id = ?", blindBoxID).
		First(&pool).Error; err != nil {
		return nil, err
	}
	return &pool, nil
}

// CreatePool 插入新卡池（事务内使用）。
func (r *gormCardPoolRepository) CreatePool(c *gin.Context, pool *mall.MallCardPool) error {
	if pool == nil {
		return errors.New("cardpool: nil pool")
	}
	return repository.DB(c).Create(pool).Error
}

// ListItemsByPoolID 拉牌池所有 items。
func (r *gormCardPoolRepository) ListItemsByPoolID(c *gin.Context, poolID int64) ([]mall.MallCardPoolItem, error) {
	items := make([]mall.MallCardPoolItem, 0)
	err := repository.DB(c).
		Where("pool_id = ?", poolID).
		Order("id ASC").
		Find(&items).Error
	return items, err
}

// ListItemsForUpdateByPoolID 事务内专用：拉牌池 items 并加 FOR UPDATE 行锁。
//
// SQL：`SELECT * FROM mall_card_pool_items WHERE pool_id = ? AND stock > 0 FOR UPDATE`。
// 调用方必须在事务上下文（repository.DB(c) 是 tx）；非事务下行锁退化为普通 SELECT。
func (r *gormCardPoolRepository) ListItemsForUpdateByPoolID(c *gin.Context, poolID int64) ([]mall.MallCardPoolItem, error) {
	items := make([]mall.MallCardPoolItem, 0)
	err := repository.DB(c).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("pool_id = ? AND stock > 0", poolID).
		Find(&items).Error
	return items, err
}

// ListItemsByPoolIDs 按 pool id 集合批量取 items（IN 单条 SQL）。
func (r *gormCardPoolRepository) ListItemsByPoolIDs(c *gin.Context, poolIDs []int64) ([]mall.MallCardPoolItem, error) {
	items := make([]mall.MallCardPoolItem, 0)
	if len(poolIDs) == 0 {
		return items, nil
	}
	err := repository.DB(c).
		Where("pool_id IN ?", poolIDs).
		Find(&items).Error
	return items, err
}

// CreateItem 插入卡牌条目。
func (r *gormCardPoolRepository) CreateItem(c *gin.Context, item *mall.MallCardPoolItem) error {
	if item == nil {
		return errors.New("cardpool: nil item")
	}
	return repository.DB(c).Create(item).Error
}

// CreateItems 批量插入卡牌条目。空 items 短路返回 nil。
func (r *gormCardPoolRepository) CreateItems(c *gin.Context, items []mall.MallCardPoolItem) error {
	if len(items) == 0 {
		return nil
	}
	return repository.DB(c).Create(&items).Error
}

// DecrementStock 条件扣减 stock。
//
// 用 `Updates(map[string]any{"stock": gorm.Expr("stock - 1")})`：DB 自己算，
// 影响行数 = 1 即成功；= 0 时（item 不存在 / 已软删 / stock=0）统一用 ErrStockEmpty 上报。
func (r *gormCardPoolRepository) DecrementStock(c *gin.Context, itemID int64) error {
	res := repository.DB(c).
		Model(&mall.MallCardPoolItem{}).
		Where("id = ? AND stock > 0", itemID).
		Update("stock", gorm.Expr("stock - 1"))
	if err := res.Error; err != nil {
		return err
	}
	if res.RowsAffected == 0 {
		return ErrStockEmpty
	}
	return nil
}

// IncrementStock 条件增加 stock。id 不存在 / 已软删均影响 0 行 → 返回 ErrStockEmpty。
func (r *gormCardPoolRepository) IncrementStock(c *gin.Context, itemID int64, delta int) error {
	if delta <= 0 {
		return errors.New("cardpool: increment delta must be positive")
	}
	res := repository.DB(c).
		Model(&mall.MallCardPoolItem{}).
		Where("id = ?", itemID).
		Update("stock", gorm.Expr("stock + ?", delta))
	if err := res.Error; err != nil {
		return err
	}
	if res.RowsAffected == 0 {
		return ErrStockEmpty
	}
	return nil
}

// SumWeightByPoolID 拉某 pool 的 items 总 weight。
func (r *gormCardPoolRepository) SumWeightByPoolID(c *gin.Context, poolID int64) (int64, error) {
	var total int64
	err := repository.DB(c).
		Model(&mall.MallCardPoolItem{}).
		Where("pool_id = ?", poolID).
		Select("COALESCE(SUM(weight), 0)").
		Scan(&total).Error
	return total, err
}

// SubtotalStockByPoolID 拉某 pool 的总库存。
func (r *gormCardPoolRepository) SubtotalStockByPoolID(c *gin.Context, poolID int64) (int64, error) {
	var total int64
	err := repository.DB(c).
		Model(&mall.MallCardPoolItem{}).
		Where("pool_id = ?", poolID).
		Select("COALESCE(SUM(stock), 0)").
		Scan(&total).Error
	return total, err
}

// 编译期断言。
var _ CardPoolRepository = (*gormCardPoolRepository)(nil)