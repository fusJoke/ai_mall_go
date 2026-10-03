// Package preheat 提供活动创建后的缓存预热函数（D22）。
//
// 业务动机：
//   - 限时特价 / 秒杀活动开始瞬间会有大量请求涌入；
//   - 如果此时缓存是冷的，每个请求触发 cold start → DB 单点打挂；
//   - 解决方案：活动创建时预先触发相关缓存加载，让活动开始时缓存已是热的。
//
// 调用模式：
//   - 异步触发：上游 Create 在事务 COMMIT 后用 `go preheat.PromotionPreheat(p, deps)` 调用；
//   - 失败不影响主业务：内部 log.Printf 记录 warn，调用方拿不到 error；
//   - 单活动范围内预热：本实现只预热「该活动关联的盲盒详情」，不预热所有用户的 feed
//     （设计文档 D22 边界说明里明确指出"不预热所有用户的 feed"成本不可控）。
//
// 脱离请求生命周期：
//   - 调用方需传入 context.Background() 或经 context.WithoutCancel 派生的 ctx，
//     不能用 c.Request.Context()（请求结束会取消，goroutine 里的 DB 操作直接报 canceled）。
package preheat

import (
	"context"
	"log"
	"strconv"
	"time"

	"ai-go-mall/internal/infra/cache/hotspot"
	"ai-go-mall/internal/model/mall"
)

// BlindBoxLoader 加载盲盒详情的闭包（参数：blindbox_id）。
//
// 返回值可以是任意 JSON-marshalable 类型——热点缓存会 json 序列化后写 Redis。
// 调用方一般传 `func(ctx) (*BlindBoxDetail, error)` 之类的 service loader。
type BlindBoxLoader func(ctx context.Context, blindBoxID int64) (any, error)

// Deps 预热所需依赖（每个预热调用都需要一个完整 Deps，避免包级全局状态）。
type Deps struct {
	// Hotspot 热点缓存写入端；nil 时预热整体跳过（单测 / 缺依赖）。
	Hotspot hotspot.HotspotCache

	// BlindBoxLoader 盲盒详情加载闭包；nil 时同 Hotspot 处理。
	BlindBoxLoader BlindBoxLoader

	// StaleAfter 热点逻辑过期时长。设计 D22 给 30s，传 0 回落 staleAfterDefault。
	StaleAfter time.Duration
}

// staleAfterDefault 兜底逻辑过期时长（D22 设计：30s）。
const staleAfterDefault = 30 * time.Second

// PromotionPreheat 预热限时特价活动关联的盲盒详情。
//
// 入参 promo 是 Create 后的 *mall.MallPromotion（含 BlindBoxID / SupplierID）。
// 失败仅 log 记录，不返回 error —— 设计明确「预热失败 → 走原有 daily cold-start 兜底」。
//
// 典型用法（supplier/promotion.go 的 Create 末尾）：
//
//	if promo != nil {
//	    go preheat.PromotionPreheat(context.Background(), promo, deps)
//	}
func PromotionPreheat(ctx context.Context, promo *mall.MallPromotion, deps Deps) {
	if promo == nil || promo.BlindBoxID <= 0 {
		return
	}
	preheatBlindBox(ctx, promo.BlindBoxID, deps)
}

// SeckillPreheat 预热秒杀活动关联的盲盒详情。
//
// 典型用法（supplier/seckill.go 的 Create 末尾，Redis 库存 init 完成后）：
//
//	if sec != nil {
//	    go preheat.SeckillPreheat(context.Background(), sec, deps)
//	}
func SeckillPreheat(ctx context.Context, sec *mall.MallSeckillActivity, deps Deps) {
	if sec == nil || sec.BlindBoxID <= 0 {
		return
	}
	preheatBlindBox(ctx, sec.BlindBoxID, deps)
}

// preheatBlindBox 调一次 HotspotCache.Get 触发 loader（loader 走 DB 然后写热点），
// loader 返回的内容立即被序列化进 Redis，下一次同 key 的 Get 直接命中。
//
// 为什么不调 IsInplaceHotspot 自己手工 Set：
//   - HotspotCache 自身负责「逻辑过期 + 异步刷新」等复杂策略，
//     手写 Set 会绕过这套机制，让下一次 Get 误判数据已陈旧；
//   - 走 Get + 临时 loader：等价于「模拟一次冷读」，loader 跑完 Redis 里就有数据了。
func preheatBlindBox(ctx context.Context, blindBoxID int64, deps Deps) {
	if deps.Hotspot == nil || deps.BlindBoxLoader == nil {
		return
	}
	stale := deps.StaleAfter
	if stale <= 0 {
		stale = staleAfterDefault
	}
	key := "hotspot:blindbox:" + strconv.FormatInt(blindBoxID, 10)

	loader := deps.BlindBoxLoader
	_, err := deps.Hotspot.Get(ctx, key, func(ctx context.Context, k string) (any, error) {
		return loader(ctx, blindBoxID)
	}, stale)
	if err != nil {
		log.Printf("preheat: blindbox %d failed: %v", blindBoxID, err)
	}
}