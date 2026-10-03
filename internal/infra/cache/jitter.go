// Package cache — jitter.go 提供 TTL 抖动 helper（cache stampede 防护）。
//
// 背景（D22 / Phase 18）：
//   - 缓存雪崩：大量 key 同时到期 → 同一时刻大量请求回源 DB → DB 压力尖刺。
//   - 抖动用例：cache.Set(key, val, ttl) 传入的 ttl 不能是「业务配置精确值」，
//     需要在 base TTL 附近 ±10% 随机化，让一批 key 的到期时间分散开。
//
// 设计要点：
//   - 纯函数：JitterTTL(base) → time.Duration，无包级状态，便于测试。
//   - base <= 0 直接返回原值（与 cache.Set 的「ttl <= 0 不过期」语义对齐，
//     业务侧用 if ttl > 0 { ttl = cache.JitterTTL(ttl) } 判断一次即可）。
//   - 抖动幅度 j = ±10%：与社区惯例（go-cache / aws-sdk-go-v2 缓动量）一致；
//     大幅抖动（如 ±50%）会让缓存的有效时长出现明显偏差，业务侧调试时
//     「为什么这条 key 5 秒就失效」的困惑会变多。10% 是甜点位。
//   - 随机源：math/rand + 全局 mutex 锁；不引入新依赖、不要求业务侧注入 rand。
//     TTL 抖动不是密码学敏感场景（不需要不可预测），全局锁开销可忽略
//     （Set 是低频写、不是热路径）。如需后续替换为 crypto/rand 或 per-goroutine rand，
//     改本文件即可，调用方签名不变。
package cache

import (
	"math/rand"
	"time"
)

// jitterFraction TTL 抖动幅度（±10%）。
//
// 设为包级 const 让测试可断言「实际抖动不会超出 ±10%」。
const jitterFraction = 0.10

// JitterTTL 把 base TTL 在 ±jitterFraction 范围内随机化。
//
// base <= 0 返回原值（与 cache.Set 接受 ttl=0 不过期一致）。
// base > 0 返回 [base*(1-jitterFraction), base*(1+jitterFraction)] 区间内
// 的随机 time.Duration。
//
// 用法（业务侧）：
//
//	ttl := cfg.Cache.TTL.BlindBoxDetail.L2
//	if ttl > 0 {
//	    ttl = cache.JitterTTL(ttl)
//	}
//	cache.Get().Set(ctx, key, val, ttl)
//
// 调用频次低（JitterTTL 自身有 mutex 但仅 Set 路径触发），
// 如确为热点再换 per-goroutine rand。
func JitterTTL(base time.Duration) time.Duration {
	if base <= 0 {
		return base
	}
	// math/rand.Float64 在 [0.0, 1.0) 均匀分布。
	// 偏移到 [-jitterFraction, +jitterFraction) 后乘 base 即得抖动量。
	delta := (rand.Float64()*2 - 1) * jitterFraction
	return time.Duration(float64(base) * (1 + delta))
}