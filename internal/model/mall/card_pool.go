package mall

import (
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

// MallCardPool 卡池（1:1 绑定盲盒）。
//
// 关键设计：
//   - blind_box_id UK：1:1 关系，每个盲盒只能有一个卡池。
//   - 单卡池存在而非「多池切换」：MVP 简化，运营调整卡池 = 调 pool_items
//     stock 与 weight，不切池。
type MallCardPool struct {
	// ID 卡池主键。
	ID int64 `json:"id" gorm:"comment:ID;primaryKey;autoIncrement"`

	// BlindBoxID 所属盲盒 ID（UK，1:1 关系）。
	BlindBoxID int64 `json:"blind_box_id" gorm:"comment:所属盲盒ID;uniqueIndex;not null"`

	// UpdatedAt 更新时间（GORM 自动维护）。
	UpdatedAt time.Time `json:"updated_at" gorm:"comment:更新时间;not null;autoUpdateTime"`

	// CreatedAt 创建时间（GORM 自动维护）。
	CreatedAt time.Time `json:"created_at" gorm:"comment:创建时间;not null;autoCreateTime"`

	// DeletedAt 软删除标记（GORM 自动维护；Valid=true 即已删除）。
	DeletedAt gorm.DeletedAt `json:"deleted_at" gorm:"comment:删除时间;index"`
}

// TableName 让 GORM 把 "mall_card_pools" 交给 namer 处理。
func (MallCardPool) TableName(namer schema.Namer) string {
	return namer.TableName("mall_card_pools")
}

// MallCardRarity 卡牌稀有度枚举。
type MallCardRarity string

// 稀有度枚举值。
const (
	RaritySSR MallCardRarity = "SSR" // 至稀有
	RaritySR  MallCardRarity = "SR"  // 超稀有
	RarityR   MallCardRarity = "R"   // 稀有
	RarityN   MallCardRarity = "N"   // 普通
)

// MallCardPoolItem 卡池内卡条目（rarity / weight / stock）。
//
// 关键设计：
//   - weight INT 同池加和固定 10000（设计 D2 关键约束），
//     整数避免浮点累计误差。
//   - stock 用条件更新 `SET stock=stock-1 WHERE id=? AND stock>0`，
//     抽卡事务内保证不超卖。
//   - 索引 (pool_id, stock)：抽卡算法 D3 步骤 1 SELECT FOR UPDATE + stock>0 走此索引。
type MallCardPoolItem struct {
	// ID 卡池条目主键。
	ID int64 `json:"id" gorm:"comment:ID;primaryKey;autoIncrement"`

	// PoolID 所属卡池 ID。
	PoolID int64 `json:"pool_id" gorm:"comment:所属卡池ID;not null;index:idx_mall_card_pool_items_pool_stock,priority:1"`

	// CardID 卡牌 ID（FK mall_cards.id）。
	CardID int64 `json:"card_id" gorm:"comment:卡牌ID;not null;index"`

	// Rarity 稀有度：RaritySSR / RaritySR / RarityR / RarityN。
	Rarity MallCardRarity `json:"rarity" gorm:"comment:稀有度(SSR/SR/R/N);size:8;not null"`

	// Weight 权重（同池加和=10000）。
	Weight int `json:"weight" gorm:"comment:权重(同池加和=10000);default:0;not null"`

	// Stock 剩余库存。
	Stock int `json:"stock" gorm:"comment:剩余库存;default:0;not null;index:idx_mall_card_pool_items_pool_stock,priority:2"`

	// UpdatedAt 更新时间（GORM 自动维护）。
	UpdatedAt time.Time `json:"updated_at" gorm:"comment:更新时间;not null;autoUpdateTime"`

	// CreatedAt 创建时间（GORM 自动维护）。
	CreatedAt time.Time `json:"created_at" gorm:"comment:创建时间;not null;autoCreateTime"`

	// DeletedAt 软删除标记（GORM 自动维护；Valid=true 即已删除）。
	DeletedAt gorm.DeletedAt `json:"deleted_at" gorm:"comment:删除时间;index"`
}

// TableName 让 GORM 把 "mall_card_pool_items" 交给 namer 处理。
func (MallCardPoolItem) TableName(namer schema.Namer) string {
	return namer.TableName("mall_card_pool_items")
}
