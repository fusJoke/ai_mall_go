package mall

import (
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

// MallCard 卡牌主数据（不可变元数据，库存与稀有度在 pool_items 里）。
//
// 字段命名遵循 GORM 约定（与 admin.go 风格一致）：
//   - ID / CreatedAt / UpdatedAt / DeletedAt 标准三件套
//   - 业务字段在前、时间戳字段在后，UpdatedAt → CreatedAt → DeletedAt 收尾
//
// 关键设计：
//   - mall_cards 是「模板」表，单卡可被多个 pool_items 引用（不同稀有度 / 库存）。
//   - 不存 stock / rarity —— 这些是 pool_items 的属性，
//     设计 D2「卡池内卡」语义。
type MallCard struct {
	// ID 卡牌主键。
	ID int64 `json:"id" gorm:"comment:ID;primaryKey;autoIncrement"`

	// Name 卡名，必填。
	Name string `json:"name" gorm:"comment:卡名;size:100;not null"`

	// Image 卡图 URL；nil 表示未上传。
	Image string `json:"image" gorm:"comment:卡图URL;size:255"`

	// Team 球队标签（如 "LAL" / "BOS"）；nil 表示非球队卡。
	Team string `json:"team" gorm:"comment:球队;size:64;index"`

	// Player 球员姓名；nil 表示非球员卡。
	Player string `json:"player" gorm:"comment:球员;size:64;index"`

	// SerialNo 编号 / 限量编号（如 "001/100"）；nil 表示不限量。
	SerialNo string `json:"serial_no" gorm:"comment:编号/限量编号;size:64"`

	// Description 卡描述，例卡面介绍 / 工艺说明。
	Description string `json:"description" gorm:"comment:卡描述;size:500"`

	// UpdatedAt 更新时间（GORM 自动维护）。
	UpdatedAt time.Time `json:"updated_at" gorm:"comment:更新时间;not null;autoUpdateTime"`

	// CreatedAt 创建时间（GORM 自动维护）。
	CreatedAt time.Time `json:"created_at" gorm:"comment:创建时间;not null;autoCreateTime"`

	// DeletedAt 软删除标记（GORM 自动维护；Valid=true 即已删除）。
	DeletedAt gorm.DeletedAt `json:"deleted_at" gorm:"comment:删除时间;index"`
}

// TableName 实现 schema.TablerWithNamer 接口，让 GORM 把 "mall_cards"
// 交给带前缀配置的 namer 处理，NamingStrategy.TablePrefix 才能真正落到表名上。
func (MallCard) TableName(namer schema.Namer) string {
	return namer.TableName("mall_cards")
}
