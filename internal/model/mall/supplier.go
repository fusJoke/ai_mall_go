package mall

import (
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

// MallSupplier 供应商主体。
//
// 关键设计：
//   - status 用 varchar('active' / 'disabled')：业务状态比通用启停更明确，
//     对应设计 D9「供应商状态与盲盒状态解耦」 —— 供应商被禁用不影响
//     盲盒本身的 status / on_sale 字段，仅影响前端列表过滤。
//   - balance = ∑ 抽卡收入 - ∑ 已打款，单一数字表达「待结算金额」
//     （设计 D13 关键决策）。
//   - commission_rate 可空（指针类型）：NULL 时使用 config.mall.default_commission_rate。
type MallSupplier struct {
	// ID 供应商主键。
	ID int64 `json:"id" gorm:"comment:ID;primaryKey;autoIncrement"`

	// Name 供应商名称，必填。
	Name string `json:"name" gorm:"comment:供应商名称;size:100;not null"`

	// Logo LOGO URL；nil 表示未上传。
	Logo string `json:"logo" gorm:"comment:LOGO URL;size:255"`

	// Bio 简介。
	Bio string `json:"bio" gorm:"comment:简介;size:500"`

	// Status 状态：StatusActive=启用 / StatusDisabled=禁用。
	Status MallBlindBoxStatus `json:"status" gorm:"comment:状态(active=启用,disabled=禁用);size:16;default:'active';not null;index:idx_mall_suppliers_status_featured,priority:1"`

	// IsFeatured 是否推荐：true=推荐 / false=普通。
	IsFeatured bool `json:"is_featured" gorm:"comment:是否推荐(1推荐,0普通);default:false;not null;index:idx_mall_suppliers_status_featured,priority:2"`

	// ContactPhone 联系电话；nil 表示未填写。
	ContactPhone string `json:"contact_phone" gorm:"comment:联系电话;size:20"`

	// Balance 待结算余额（元）。
	Balance float64 `json:"balance" gorm:"comment:待结算余额(元);type:decimal(12,2);not null;default:0"`

	// TotalSales 累计销售额（元）。
	TotalSales float64 `json:"total_sales" gorm:"comment:累计销售额(元);type:decimal(14,2);not null;default:0"`

	// CommissionRate 手续费率（4 位小数）；nil 表示走 config 默认值。
	CommissionRate *float64 `json:"commission_rate" gorm:"comment:手续费率(NULL=走config默认);type:decimal(5,4)"`

	// UpdatedAt 更新时间（GORM 自动维护）。
	UpdatedAt time.Time `json:"updated_at" gorm:"comment:更新时间;not null;autoUpdateTime"`

	// CreatedAt 创建时间（GORM 自动维护）。
	CreatedAt time.Time `json:"created_at" gorm:"comment:创建时间;not null;autoCreateTime"`

	// DeletedAt 软删除标记（GORM 自动维护；Valid=true 即已删除）。
	DeletedAt gorm.DeletedAt `json:"deleted_at" gorm:"comment:删除时间;index"`
}

// TableName 让 GORM 把 "mall_suppliers" 交给 namer 处理。
func (MallSupplier) TableName(namer schema.Namer) string {
	return namer.TableName("mall_suppliers")
}

// MallSupplierUser 供应商登录账号（1:1 绑定 MallSupplier）。
//
// 关键设计：
//   - supplier_id UK：每供应商唯一登录账号，避免"多账号共享一家供应商"的权限歧义。
//   - username UK：登录用，全局唯一。
//   - status 与 MallSupplier.status 解耦：账号被禁用不影响供应商主体。
type MallSupplierUser struct {
	// ID 账号主键。
	ID int64 `json:"id" gorm:"comment:ID;primaryKey;autoIncrement"`

	// SupplierID 所属供应商 ID（UK，1:1）。
	SupplierID int64 `json:"supplier_id" gorm:"comment:所属供应商ID;uniqueIndex;not null"`

	// Username 登录用户名，全局唯一。
	Username string `json:"username" gorm:"comment:登录用户名;size:64;uniqueIndex;not null"`

	// Password 哈希后的密码（bcrypt / argon2 等，长度预留 255）。
	Password string `json:"-" gorm:"comment:密码(已哈希);size:255;not null"`

	// Status 账号状态：1=启用 / 0=禁用。
	Status int8 `json:"status" gorm:"comment:状态(1启用,0禁用);default:1;not null"`

	// LastLoginAt 最后登录时间；nil 表示从未登录。
	LastLoginAt *time.Time `json:"last_login_at" gorm:"comment:最后登录时间"`

	// LastLoginIp 最后登录 IP（兼容 IPv6，最长 45 字符）。
	LastLoginIp string `json:"last_login_ip" gorm:"comment:最后登录IP;size:45"`

	// UpdatedAt 更新时间（GORM 自动维护）。
	UpdatedAt time.Time `json:"updated_at" gorm:"comment:更新时间;not null;autoUpdateTime"`

	// CreatedAt 创建时间（GORM 自动维护）。
	CreatedAt time.Time `json:"created_at" gorm:"comment:创建时间;not null;autoCreateTime"`

	// DeletedAt 软删除标记（GORM 自动维护；Valid=true 即已删除）。
	DeletedAt gorm.DeletedAt `json:"deleted_at" gorm:"comment:删除时间;index"`
}

// TableName 让 GORM 把 "mall_supplier_users" 交给 namer 处理。
func (MallSupplierUser) TableName(namer schema.Namer) string {
	return namer.TableName("mall_supplier_users")
}
