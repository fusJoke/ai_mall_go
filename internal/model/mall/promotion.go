package mall

import (
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

// MallPromotion 限时特价活动。
//
// 关键设计：
//   - (blind_box_id, start_at) UK：防「同盲盒 + 同起始时间」重复插入。
//   - 真正的「同盲盒时间段重叠」防重由 service 层在 Create 时校验
//     （见 internal/service/supplier/promotion.go），UK 只防"完全重复"。
//   - status 与 mall_blind_boxes.status 解耦：活动被禁用不影响盲盒本身。
//   - 索引 (status, end_at)：抽卡事务 D4 用 LEFT JOIN 查"当前生效"活动
//     走此索引，复合索引高选择性。
type MallPromotion struct {
	// ID 活动主键。
	ID int64 `json:"id" gorm:"comment:ID;primaryKey;autoIncrement"`

	// SupplierID 所属供应商 ID。
	SupplierID int64 `json:"supplier_id" gorm:"comment:所属供应商ID;not null;index"`

	// BlindBoxID 关联盲盒 ID。
	BlindBoxID int64 `json:"blind_box_id" gorm:"comment:盲盒ID;not null;uniqueIndex:idx_mall_promotions_bb_start,priority:1"`

	// OriginalPrice 原价（快照，活动期内不随 blind_box.price 变化）。
	OriginalPrice float64 `json:"original_price" gorm:"comment:原价(元);type:decimal(10,2);not null;default:0"`

	// PromoPrice 活动价（必须 < original_price，由 service 层校验）。
	PromoPrice float64 `json:"promo_price" gorm:"comment:活动价(元);type:decimal(10,2);not null;default:0"`

	// StartAt 活动开始时间。
	StartAt time.Time `json:"start_at" gorm:"comment:活动开始时间;not null;uniqueIndex:idx_mall_promotions_bb_start,priority:2"`

	// EndAt 活动结束时间。
	EndAt time.Time `json:"end_at" gorm:"comment:活动结束时间;not null;index:idx_mall_promotions_status_end,priority:2"`

	// Status 状态：StatusActive=启用 / StatusDisabled=禁用。
	Status MallBlindBoxStatus `json:"status" gorm:"comment:状态(active=启用,disabled=禁用);size:16;default:'active';not null;index:idx_mall_promotions_status_end,priority:1"`

	// UpdatedAt 更新时间（GORM 自动维护）。
	UpdatedAt time.Time `json:"updated_at" gorm:"comment:更新时间;not null;autoUpdateTime"`

	// CreatedAt 创建时间（GORM 自动维护）。
	CreatedAt time.Time `json:"created_at" gorm:"comment:创建时间;not null;autoCreateTime"`

	// DeletedAt 软删除标记（GORM 自动维护；Valid=true 即已删除）。
	DeletedAt gorm.DeletedAt `json:"deleted_at" gorm:"comment:删除时间;index"`
}

// TableName 让 GORM 把 "mall_promotions" 交给 namer 处理。
func (MallPromotion) TableName(namer schema.Namer) string {
	return namer.TableName("mall_promotions")
}
