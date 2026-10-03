// Package draw_check — chain_builder.go 装配 DrawChain / SeckillDrawChain。
//
// 组装规则（design D20 §一 节点清单；task 15.8）：
//
//	DrawChain       = [user_active, blindbox_buyable, time_window, balance]
//	SeckillDrawChain = [user_active, blindbox_buyable, time_window, balance]
//
// 两者节点集相同（time_window 在 normal 链路内 fast return nil，
// 保持节点顺序一致便于未来扩展）。user_limit 不在 chain 内——
// 它带 Redis 副作用，由 DrawSeckill 直接调 AcquireUserLimit，
// 出错时由 service 层统一回滚（详见 user_limit_check.go 包注释）。
//
// 节点排序遵循 design D20 §三「便宜在前，副作用在后」原则：
//   - user_active：单行 SELECT users（最便宜）；
//   - blindbox_buyable：SELECT blind_boxes + SELECT suppliers（2 次读）；
//   - time_window：normal 跳过 / seckill 多 1 次 SELECT seckill_activities；
//   - balance：复用 time_window 已读过的行（额外仅 1 次 SELECT users）。
//
// 这样「无副作用的校验」在前，「依赖多表读的余额判定」在后；任一节点失败即终止。
package draw_check

import (
	"ai-go-mall/internal/domain/chain"
	mallRepo "ai-go-mall/internal/repository/mall"
	supplierRepo "ai-go-mall/internal/repository/supplier"
	userRepo "ai-go-mall/internal/repository/user"
)

// CheckDeps 装配 chain 需要的全部依赖。
//
// service 层注入一次，传给两个工厂方法。
type CheckDeps struct {
	Users     userRepo.UserRepository
	BlindBox  mallRepo.BlindBoxRepository
	Promotion mallRepo.PromotionRepository
	Seckill   mallRepo.SeckillRepository
	Supplier  supplierRepo.SupplierRepository
}

// NewDrawCheckChain 装配普通抽卡的责任链。
//
// 节点顺序（按 cheap → expensive 排列）：
//  1. user_active — 校验 mall_users.status = 1；
//  2. blindbox_buyable — 校验盲盒 status=active + on_sale + 供应商启用；
//  3. time_window — normal 链路 fast return nil；
//  4. balance — 校验 user.balance >= actual_price（活动价优先）。
func NewDrawCheckChain(d CheckDeps) *chain.Chain {
	return chain.NewChain(
		NewUserActiveCheck(d.Users),
		NewBlindBoxBuyableCheck(d.BlindBox, d.Supplier),
		NewTimeWindowCheck(d.Seckill),
		NewBalanceCheck(d.Users, d.BlindBox, d.Promotion, d.Seckill),
	)
}

// NewSeckillCheckChain 装配秒杀抽卡的责任链。
//
// 节点顺序同 DrawChain：time_window 在 seckill 链路内真实校验时间窗；
// 其他三个节点语义不变。user_limit 副作用由 DrawSeckill 单独调 AcquireUserLimit。
func NewSeckillCheckChain(d CheckDeps) *chain.Chain {
	return chain.NewChain(
		NewUserActiveCheck(d.Users),
		NewBlindBoxBuyableCheck(d.BlindBox, d.Supplier),
		NewTimeWindowCheck(d.Seckill),
		NewBalanceCheck(d.Users, d.BlindBox, d.Promotion, d.Seckill),
	)
}
