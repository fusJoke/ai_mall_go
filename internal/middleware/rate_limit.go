// Package middleware — rate_limit.go 实现「每分钟 N 次」固定窗口限流。
//
// 选型说明：
//   - 算法：Redis SetNX 固定窗口桶（每分钟 n 个 slot，每个 slot 是独立 key）。
//   - 理由：MVP cache.Cache 仅暴露 Get/Set/Del/SetNX，未引入 INCR；用 SetNX
//     替代"INCR + EXPIRE"思路，把"窗口内第几次"映射到 slot 下标 0..n-1。
//     SetNX 命中即视为"占用了一个 slot"，n 个 slot 全被占 → 拒绝。
//   - 复杂度：单请求最坏 O(n) 次 Redis 调用（n ≤ 10 时可接受；n 较大时建议
//     后续切换到 INCR + EXPIRE 或令牌桶 / 滑动窗口 Lua 脚本）。
//
// 关键约定：
//   - 限流粒度：每分钟 = 60s。窗口边界用 now.Unix() / 60 取整。
//   - TTL：70s（比 60s 多 10s 容差）—— 避免分钟切换时跨窗口的边界效应导致
//     "本应允许的请求被前 1 分钟遗留 key 误判为占满"。
//   - fail-open：限流服务异常（cache 未初始化 / SetNX err）→ 直接放行，
//     不阻塞业务。这是有意识的策略，不是 bug：限流是辅助防护，业务可用性优先。
//   - uidExtractor 返回 ""：跳过限流（让上游 auth 中间件自己决断 401）。
//
// 与社区惯例对齐：
//   - "rate:<key>:<minute>:<slot>" key 形态参考 github.com/gin-contrib/* 限流中间件风格；
//   - 429 + {code, message} 响应格式与本仓库 admin / user handler 保持一致。
package middleware

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"ai-go-mall/internal/infra/cache"
)

// rateLimitPrefix 是 RateLimitPerMinute 内部生成的 Redis key 的固定前缀。
//
// 固定字符串而非参数注入，理由：
//   - 避免调用方误用空前缀污染其它限流桶 / 业务 key 命名空间；
//   - 调用方想分桶，通过 uidExtractor 返回值带命名空间即可
//     （如 "draw:42"、"login:42"），无需关心前缀。
const rateLimitPrefix = "rate:rl:"

// rateLimitTTL 是每个 slot 的 Redis TTL（70s）。比 60s 多 10s 容差，
// 避免分钟切换跨窗口的边界效应。
const rateLimitTTL = 70 * time.Second

// rateLimitStatusCode 是命中限流时的 HTTP 状态码。
const rateLimitStatusCode = http.StatusTooManyRequests

// rateLimiter 是 RateLimitPerMinute 实际依赖的最小接口（仅 SetNX）。
//
// 抽出来仅为测试 mock —— 让 RateLimitPerMinute 单测不依赖真实 Redis。
// 生产实现通过 rateLimitCacheAdapter 包一层 cache.Cache。
type rateLimiter interface {
	SetNX(ctx context.Context, key, value string, ttl time.Duration) (bool, error)
}

// rateLimitCacheAdapter 把 cache.Cache 适配成 rateLimiter。
//
// 限流必须全集群共享原子 SetNX，因此无论 Cache 是单层 (Manager)
// 还是多层 (MultiLevelCache)，SetNX 都会穿透到 L2 Redis（D5 / D16 设计）。
type rateLimitCacheAdapter struct{ c cache.Cache }

func (a *rateLimitCacheAdapter) SetNX(ctx context.Context, key, value string, ttl time.Duration) (bool, error) {
	return a.c.SetNX(ctx, key, value, ttl)
}

// getCacheForRateLimit 是测试可替换的函数变量；生产实现返回 cache.Get()，
// nil 表示 cache 未初始化（Init 未调或失败）—— RateLimitPerMinute 走 fail-open。
//
// 用函数变量而非 interface：避免把 cache.Cache 这种重依赖内部的接口暴露给
// middleware 包公共 API；function hook 局限在本包内，副作用最小。
var getCacheForRateLimit = func() rateLimiter {
	c := cache.Get()
	if c == nil {
		return nil
	}
	return &rateLimitCacheAdapter{c: c}
}

// RateLimitPerMinute 返回"每分钟最多 n 次"的 gin 中间件。
//
// 参数：
//   - uidExtractor：从 gin.Context 抽取限流唯一标识（通常是 "业务前缀:user_id"）。
//     返回空串时跳过（不报错），由上游 auth 中间件决定是否 401。
//   - n：每分钟允许的最大次数。n <= 0 时返回 no-op（关闭限流）。
//
// 行为：
//   - 同一 uid 在同一分钟内最多 n 次请求通过；超出 → 429 rate_limited。
//   - cache infra 不可用 / SetNX err → fail-open，放行。
//
// 复杂度：单请求最坏 O(n) 次 Redis 调用（适合 n ≤ 10 的小配额场景，如抽卡 5 笔/分钟）。
func RateLimitPerMinute(uidExtractor func(c *gin.Context) string, n int) gin.HandlerFunc {
	// 参数兜底：n <= 0 直接关掉限流，避免误用。
	if n <= 0 {
		return func(c *gin.Context) {}
	}

	return func(c *gin.Context) {
		uid := uidExtractor(c)
		if uid == "" {
			// 没拿到 uid —— 通常是上游 auth 中间件还没塞 user 进 context。
			// 此处选择放行（让 auth 中间件自己决定 401），不引入新的"无 uid 即拒绝"语义。
			c.Next()
			return
		}

		rl := getCacheForRateLimit()
		if rl == nil {
			// fail-open：限流基础设施未初始化时不阻塞业务。
			c.Next()
			return
		}

		// 当前分钟窗口号（unix 时间除 60 取整）。
		minute := time.Now().Unix() / 60

		// 顺序尝试占用 slot 0..n-1；第一个 SetNX 成功的 slot 即视为
		// "本分钟剩余配额减一"，允许通过；全部 n 个都被占 → 拒绝。
		//
		// 注：早期退出（找到第一个空 slot 后 break）保证单次成功请求只产生
		// ≤ n 次 Redis 调用；失败请求固定 n 次。
		allowed := false
		for i := 0; i < n; i++ {
			key := fmt.Sprintf("%s%s:%d:%d", rateLimitPrefix, uid, minute, i)
			ok, err := rl.SetNX(c.Request.Context(), key, "1", rateLimitTTL)
			if err != nil {
				// fail-open：单次 SetNX 错误不阻塞业务。
				c.Next()
				return
			}
			if ok {
				allowed = true
				break
			}
		}

		if !allowed {
			abortRateLimited(c)
			return
		}
		c.Next()
	}
}

// abortRateLimited 收敛 429 响应形状（与 abort401 风格一致）。
func abortRateLimited(c *gin.Context) {
	c.AbortWithStatusJSON(rateLimitStatusCode, gin.H{
		"code":    "rate_limited",
		"message": "too many requests",
	})
}

// 编译期断言：cache.Manager 必须实现 SetNX，间接证明 rateLimitCacheAdapter 可用。
// 若 SetNX 签名漂移，此断言失败并在 CI 第一时间暴露。
var _ rateLimiter = (*rateLimitCacheAdapter)(nil)