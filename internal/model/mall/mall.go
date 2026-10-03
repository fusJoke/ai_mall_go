// Package mall 收容所有 mall（球星卡盲盒商城）业务的 GORM 模型。
//
// 包内拆分规则（一个 entity 一个文件）：
//   - card.go              MallCard
//   - blind_box.go         MallBlindBox + 状态枚举 MallBlindBoxStatus
//   - card_pool.go         MallCardPool / MallCardPoolItem + 稀有度枚举 MallCardRarity
//   - promotion.go         MallPromotion
//   - draw_order.go        MallDrawOrder / MallDrawOrderItem + 状态/来源枚举
//   - seckill.go           MallSeckillActivity / MallStockDeductionLog
//   - settlement.go        MallSettlement / MallSettlementItem / MallPlatformLedger + 结算状态枚举
//   - supplier.go          MallSupplier / MallSupplierUser
//   - mall.go              init() 把上述模型统一注册到 internal/model.Register 注册表
//
// 命名/字段规则：与 internal/model/admin.go 一致 —— comment 优先、业务字段在前、
// UpdatedAt → CreatedAt → DeletedAt 收尾；不嵌入 gorm.Model（字段无注释、顺序不可控）。
//
// 注册：init() 调用 internal/model.Register —— 数据库侧 AutoMigrate / 启动期一致性
// 检查统一从该注册表拿；不要在 init() 内直接调 AutoMigrate。
package mall

import (
	"ai-go-mall/internal/model"
)

// init 在包加载时把 mall 业务模型注册到 internal/model 注册表。
//
// 注意：不要在这里直接调 AutoMigrate，迁移逻辑统一由 database.Init() /
// migrate.Up() 在 schema-迁移链路里执行。
func init() {
	model.Register(
		MallCard{},
		MallBlindBox{},
		MallCardPool{},
		MallCardPoolItem{},
		MallPromotion{},
		MallDrawOrder{},
		MallDrawOrderItem{},
		MallSupplier{},
		MallSupplierUser{},
		MallSettlement{},
		MallSettlementItem{},
		MallPlatformLedger{},
		MallSeckillActivity{},
		MallStockDeductionLog{},
	)
}
