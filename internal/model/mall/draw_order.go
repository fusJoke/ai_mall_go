package mall

import (
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

// MallDrawOrderStatus 抽卡订单状态枚举。
// 状态机规则由 internal/domain/state 状态机约束（Phase 14）：
//
//	pending → paid → drawn
//	       ↘ failed（任意阶段兜底）
type MallDrawOrderStatus string

// 订单状态枚举值。
const (
	OrderStatusPending MallDrawOrderStatus = "pending" // 待支付（创建后未支付）
	OrderStatusPaid    MallDrawOrderStatus = "paid"    // 已支付（抽卡前）
	OrderStatusDrawn   MallDrawOrderStatus = "drawn"   // 已抽中（终态）
	OrderStatusFailed  MallDrawOrderStatus = "failed"  // 失败（终态）
)

// MallDrawOrderSource 订单来源枚举。
type MallDrawOrderSource string

// 订单来源枚举值。
const (
	SourceNormal  MallDrawOrderSource = "normal"  // 普通抽卡
	SourceSeckill MallDrawOrderSource = "seckill" // 秒杀抽卡
)

// MallDrawOrder 抽卡订单（单次抽卡即一单，items 多张卡）。
//
// 关键设计：
//   - order_no UK：业务唯一键，支付回调 / 幂等依赖它。
//   - supplier_id 冗余到订单：避免 join blind_box 推断，
//     结算单生成 + 列表筛选场景的热点路径。
//   - source 区分 normal / seckill：秒杀订单走不同事务路径（D15），
//     前端 UI 也按 source 区分提示。
type MallDrawOrder struct {
	// ID 订单主键。
	ID int64 `json:"id" gorm:"comment:ID;primaryKey;autoIncrement"`

	// OrderNo 业务订单号，全局唯一。
	OrderNo string `json:"order_no" gorm:"comment:订单号;size:32;uniqueIndex;not null"`

	// UserID 用户 ID。
	UserID int64 `json:"user_id" gorm:"comment:用户ID;not null;index:idx_mall_draw_orders_user_created,priority:1"`

	// BlindBoxID 盲盒 ID。
	BlindBoxID int64 `json:"blind_box_id" gorm:"comment:盲盒ID;not null;index"`

	// SupplierID 供应商 ID（冗余）。
	SupplierID int64 `json:"supplier_id" gorm:"comment:供应商ID;not null;index:idx_mall_draw_orders_supplier_status_created,priority:1"`

	// Price 实付金额（元，decimal(10,2)）。
	Price float64 `json:"price" gorm:"comment:实付金额(元);type:decimal(10,2);not null;default:0"`

	// Status 订单状态。
	Status MallDrawOrderStatus `json:"status" gorm:"comment:状态(pending/paid/drawn/failed);size:16;default:'pending';not null;index:idx_mall_draw_orders_supplier_status_created,priority:2"`

	// Source 订单来源：normal / seckill。
	Source MallDrawOrderSource `json:"source" gorm:"comment:来源(normal=普通,seckill=秒杀);size:16;default:'normal';not null"`

	// UpdatedAt 更新时间（GORM 自动维护）。
	UpdatedAt time.Time `json:"updated_at" gorm:"comment:更新时间;not null;autoUpdateTime"`

	// CreatedAt 创建时间（GORM 自动维护）。
	CreatedAt time.Time `json:"created_at" gorm:"comment:创建时间;not null;autoCreateTime;index:idx_mall_draw_orders_user_created,priority:2;index:idx_mall_draw_orders_supplier_status_created,priority:3"`

	// DeletedAt 软删除标记（GORM 自动维护；Valid=true 即已删除）。
	DeletedAt gorm.DeletedAt `json:"deleted_at" gorm:"comment:删除时间;index"`
}

// TableName 让 GORM 把 "mall_draw_orders" 交给 namer 处理。
func (MallDrawOrder) TableName(namer schema.Namer) string {
	return namer.TableName("mall_draw_orders")
}

// MallDrawOrderItem 抽卡订单明细（抽中的卡牌 + 快照）。
//
// 关键设计：
//   - snapshot_name / snapshot_image：抽中瞬间保存卡牌名称 / 图片，
//     后续卡牌改名 / 改图不影响历史订单展示。
//   - rarity 也是快照，避免日后卡牌在池里升降级影响订单回放。
type MallDrawOrderItem struct {
	// ID 明细主键。
	ID int64 `json:"id" gorm:"comment:ID;primaryKey;autoIncrement"`

	// OrderID 所属订单 ID。
	OrderID int64 `json:"order_id" gorm:"comment:所属订单ID;not null;index:idx_mall_draw_order_items_order"`

	// CardID 卡牌 ID（FK mall_cards.id，仅用于关联查询；展示走 snapshot_*）。
	CardID int64 `json:"card_id" gorm:"comment:卡牌ID;not null;index"`

	// Rarity 稀有度快照（SSR/SR/R/N）。
	Rarity MallCardRarity `json:"rarity" gorm:"comment:稀有度(快照);size:8;not null"`

	// SnapshotName 卡名快照。
	SnapshotName string `json:"snapshot_name" gorm:"comment:卡名(快照);size:100;not null"`

	// SnapshotImage 卡图快照；nil 表示抽中时无图。
	SnapshotImage string `json:"snapshot_image" gorm:"comment:卡图(快照);size:255"`

	// UpdatedAt 更新时间（GORM 自动维护）。
	UpdatedAt time.Time `json:"updated_at" gorm:"comment:更新时间;not null;autoUpdateTime"`

	// CreatedAt 创建时间（GORM 自动维护）。
	CreatedAt time.Time `json:"created_at" gorm:"comment:创建时间;not null;autoCreateTime"`

	// DeletedAt 软删除标记（GORM 自动维护；Valid=true 即已删除）。
	DeletedAt gorm.DeletedAt `json:"deleted_at" gorm:"comment:删除时间;index"`
}

// TableName 让 GORM 把 "mall_draw_order_items" 交给 namer 处理。
func (MallDrawOrderItem) TableName(namer schema.Namer) string {
	return namer.TableName("mall_draw_order_items")
}
