package mall

import (
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

// MallBlindBoxStatus 盲盒状态枚举。
//   - StatusActive：供应商正常营业
//   - StatusDisabled：admin 强制下架（含供应商被禁用的间接下架）
type MallBlindBoxStatus string

// 盲盒状态枚举值。
const (
	StatusActive   MallBlindBoxStatus = "active"   // 启用
	StatusDisabled MallBlindBoxStatus = "disabled" // 禁用
)

// MallBlindBox 盲盒 SKU（supplier_id NOT NULL，纯多供应商模式）。
//
// 关键设计：
//   - supplier_id NOT NULL：纯多供应商模式，无平台自营（设计 D2 关键约束）。
//   - status 与 on_sale 是两个独立字段：status=disabled 是 admin 强制屏蔽，
//     on_sale=false 是供应商自己下架，两者解耦（设计 D9）。
//   - price 用 decimal(10,2)：单盲盒售价上限 99,999,999.99 元，远超业务范围。
//   - is_featured=1 的盲盒会被 cmd/es-sync 推到 ES 首页 feed。
//
// 索引说明：
//   - idx_mall_blind_boxes_supplier_status_onsale：供应商视角筛「在售」。
//   - idx_mall_blind_boxes_featured_status_onsale：前端首页 feed。
type MallBlindBox struct {
	// ID 盲盒主键。
	ID int64 `json:"id" gorm:"comment:ID;primaryKey;autoIncrement"`

	// SupplierID 所属供应商 ID（NOT NULL，纯多供应商模式）。
	SupplierID int64 `json:"supplier_id" gorm:"comment:所属供应商ID;not null;index:idx_mall_blind_boxes_supplier_status_onsale,priority:1"`

	// Name 盲盒名称，必填。
	Name string `json:"name" gorm:"comment:盲盒名称;size:100;not null"`

	// Cover 封面图 URL；nil 表示未上传。
	Cover string `json:"cover" gorm:"comment:封面图URL;size:255"`

	// Price 原价（活动期外的售价），decimal(10,2)。
	Price float64 `json:"price" gorm:"comment:原价(元);type:decimal(10,2);not null;default:0"`

	// Status 状态：StatusActive=启用 / StatusDisabled=禁用。
	Status MallBlindBoxStatus `json:"status" gorm:"comment:状态(active=启用,disabled=禁用);size:16;default:'active';not null;index:idx_mall_blind_boxes_supplier_status_onsale,priority:3;index:idx_mall_blind_boxes_featured_status_onsale,priority:3"`

	// OnSale 是否在售：true=在售 / false=下架。
	OnSale bool `json:"on_sale" gorm:"comment:是否在售(1在售,0下架);default:true;not null;index:idx_mall_blind_boxes_supplier_status_onsale,priority:2;index:idx_mall_blind_boxes_featured_status_onsale,priority:2"`

	// IsFeatured 是否推荐：true=推荐 / false=普通。
	// 推荐 + 启用 + 在售 三条件 AND 才会进 ES 首页 feed。
	IsFeatured bool `json:"is_featured" gorm:"comment:是否推荐(1推荐,0普通);default:false;not null;index:idx_mall_blind_boxes_featured_status_onsale,priority:1"`

	// Description 盲盒描述。
	Description string `json:"description" gorm:"comment:盲盒描述;size:500"`

	// UpdatedAt 更新时间（GORM 自动维护）。
	UpdatedAt time.Time `json:"updated_at" gorm:"comment:更新时间;not null;autoUpdateTime"`

	// CreatedAt 创建时间（GORM 自动维护）。
	CreatedAt time.Time `json:"created_at" gorm:"comment:创建时间;not null;autoCreateTime"`

	// DeletedAt 软删除标记（GORM 自动维护；Valid=true 即已删除）。
	DeletedAt gorm.DeletedAt `json:"deleted_at" gorm:"comment:删除时间;index"`
}

// TableName 让 GORM 把 "mall_blind_boxes" 交给 namer 处理。
func (MallBlindBox) TableName(namer schema.Namer) string {
	return namer.TableName("mall_blind_boxes")
}
