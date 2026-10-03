package mall

import (
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

// MallSeckillActivity 秒杀活动主体。
//
// 关键设计：
//   - redis_initialized：Redis 库存是否已初始化（创建活动与 SET 库存同事务原子化，
//     防 "Redis 已 init 但 DB 行不存在" 或 "DB 行存在但 Redis 没 init" 的脏数据）。
//   - total_stock ≤ mall_card_pool_items 总 stock：service 层创建时校验（D15 决策点 2），
//     保证 Redis DECR 成功后卡池库存也够用。
//   - status 与 mall_blind_boxes.status 解耦：秒杀被禁用不影响盲盒本身在售。
//   - 索引 (status, start_at, end_at)：服务"查当前生效秒杀活动"列表查询。
type MallSeckillActivity struct {
	// ID 秒杀活动主键。
	ID int64 `json:"id" gorm:"comment:ID;primaryKey;autoIncrement"`

	// SupplierID 所属供应商 ID。
	SupplierID int64 `json:"supplier_id" gorm:"comment:所属供应商ID;not null;index:idx_mall_seckill_supplier"`

	// BlindBoxID 关联盲盒 ID。
	BlindBoxID int64 `json:"blind_box_id" gorm:"comment:盲盒ID;not null;index:idx_mall_seckill_blind_box"`

	// SeckillPrice 秒杀价（必须 < blind_box.price，由 service 层校验）。
	SeckillPrice float64 `json:"seckill_price" gorm:"comment:秒杀价(元);type:decimal(10,2);not null;default:0"`

	// TotalStock 秒杀总名额（用于初始化 Redis 库存 seckill:stock:{id}）。
	TotalStock int `json:"total_stock" gorm:"comment:秒杀总名额;not null;default:0"`

	// PerUserLimit 每用户限购数量（Redis key seckill:user_bought:{sid}:{uid}）。
	PerUserLimit int `json:"per_user_limit" gorm:"comment:每用户限购数量;not null;default:1"`

	// StartAt 活动开始时间。
	StartAt time.Time `json:"start_at" gorm:"comment:活动开始时间;not null;index:idx_mall_seckill_status_window,priority:2"`

	// EndAt 活动结束时间。
	EndAt time.Time `json:"end_at" gorm:"comment:活动结束时间;not null;index:idx_mall_seckill_status_window,priority:3"`

	// Status 状态：StatusActive=启用 / StatusDisabled=禁用。
	Status MallBlindBoxStatus `json:"status" gorm:"comment:状态(active=启用,disabled=禁用);size:16;default:'active';not null;index:idx_mall_seckill_status_window,priority:1"`

	// RedisInitialized Redis 库存是否已初始化（防重复 init）。
	RedisInitialized bool `json:"redis_initialized" gorm:"comment:Redis库存是否已初始化(0=未初始化,1=已初始化);not null;default:0"`

	// UpdatedAt 更新时间（GORM 自动维护）。
	UpdatedAt time.Time `json:"updated_at" gorm:"comment:更新时间;not null;autoUpdateTime"`

	// CreatedAt 创建时间（GORM 自动维护）。
	CreatedAt time.Time `json:"created_at" gorm:"comment:创建时间;not null;autoCreateTime"`

	// DeletedAt 软删除标记（GORM 自动维护；Valid=true 即已删除）。
	DeletedAt gorm.DeletedAt `json:"deleted_at" gorm:"comment:删除时间;index:idx_mall_seckill_deleted_at"`
}

// TableName 让 GORM 把 "mall_seckill_activities" 交给 namer 处理。
func (MallSeckillActivity) TableName(namer schema.Namer) string {
	return namer.TableName("mall_seckill_activities")
}

// MallStockDeductionLog 秒杀路径下的库存扣减审计记录。
//
// 关键设计：
//   - synced_to_pool_at NULL = 未对账；cmd/stock-sync 异步对账仅写该字段，
//     实际 mall_card_pool_items.stock 在秒杀事务内已扣过（D15 步骤 4.b）。
//     此表是"审计/对账"用途，不是库存主表。
//   - 索引 (synced_to_pool_at, deducted_at)：cmd/stock-sync 扫"待对账"批查询。
//   - 索引 (seckill_id)：按活动聚合扣减明细。
//   - snapshot_name / snapshot_image：与 draw_order_items 一致的快照语义，
//     抽中后卡牌改名不影响订单显示。
type MallStockDeductionLog struct {
	// ID 扣减记录主键。
	ID int64 `json:"id" gorm:"comment:ID;primaryKey;autoIncrement"`

	// SeckillID 秒杀活动 ID。
	SeckillID int64 `json:"seckill_id" gorm:"comment:秒杀活动ID;not null;index:idx_mall_stock_log_seckill"`

	// UserID 用户 ID。
	UserID int64 `json:"user_id" gorm:"comment:用户ID;not null;index:idx_mall_stock_log_user"`

	// BlindBoxID 盲盒 ID。
	BlindBoxID int64 `json:"blind_box_id" gorm:"comment:盲盒ID;not null"`

	// CardID 抽中卡牌 ID。
	CardID int64 `json:"card_id" gorm:"comment:卡牌ID;not null"`

	// Rarity 抽中稀有度：RaritySSR / RaritySR / RarityR / RarityN。
	Rarity MallCardRarity `json:"rarity" gorm:"comment:抽中等级(SSR/SR/R/N);size:8;not null;default:'N'"`

	// SnapshotName 卡名快照（防止后续 mall_cards.name 改名）。
	SnapshotName string `json:"snapshot_name" gorm:"comment:卡名快照;size:100;not null;default:''"`

	// SnapshotImage 卡图快照。
	SnapshotImage string `json:"snapshot_image" gorm:"comment:卡图快照;size:255;not null;default:''"`

	// DeductedAt 扣减时间（秒杀事务提交时刻）。
	DeductedAt time.Time `json:"deducted_at" gorm:"comment:扣减时间;not null;index:idx_mall_stock_log_synced_deducted,priority:2"`

	// SyncedToPoolAt 异步对账完成时间（NULL = 未对账）。
	SyncedToPoolAt *time.Time `json:"synced_to_pool_at" gorm:"comment:异步对账完成时间;index:idx_mall_stock_log_synced_deducted,priority:1"`

	// CreatedAt 创建时间（GORM 自动维护）。
	CreatedAt time.Time `json:"created_at" gorm:"comment:创建时间;not null;autoCreateTime"`
}

// TableName 让 GORM 把 "mall_stock_deduction_log" 交给 namer 处理。
func (MallStockDeductionLog) TableName(namer schema.Namer) string {
	return namer.TableName("mall_stock_deduction_log")
}