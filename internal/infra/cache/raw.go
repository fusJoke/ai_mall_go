// Package cache — raw.go 暴露底层 go-redis 客户端给需要门面之外能力的高级用法。
//
// 当前消费者：internal/infra/cache/hotspot（D21 热点缓存需要 Lua 脚本释放
// 分布式锁、SETNX 抢锁，这些不在 Cache 门面接口里）。
//
// 为什么走共享客户端而不是各开各的：连接池（默认 50）是进程级稀缺资源；
// hotspot 与普通缓存共享同一个池，避免连接数翻倍。
package cache

import (
	"github.com/redis/go-redis/v9"
)

// rawClientProvider 是持有底层 go-redis 客户端的 Cache 实现的约定接口。
// Manager（单层 redis）与 MultiLevelCache（L2 redis）都实现它。
type rawClientProvider interface {
	RawClient() (*redis.Client, bool)
}

// RawClient 返回底层 go-redis 客户端。
//
// Manager：driver 是 redis 实现时返回；MultiLevelCache：L2 是 redis 时返回；
// 其他 driver（memory / 测试 fake）返回 (nil, false)。
func (m *Manager) RawClient() (*redis.Client, bool) {
	if rp, ok := m.driver.(interface{ RawClient() *redis.Client }); ok {
		return rp.RawClient(), true
	}
	return nil, false
}

// RawClient 见 Manager 同名方法（L2 为 redis driver 时可用）。
func (m *MultiLevelCache) RawClient() (*redis.Client, bool) {
	if rp, ok := m.l2.(interface{ RawClient() *redis.Client }); ok {
		return rp.RawClient(), true
	}
	return nil, false
}

// RawRedisClient 从当前全局 Cache 单例取底层 go-redis 客户端（共享连接池）。
//
// cache.Init() 未调用、driver 非 redis 系（如纯内存测试装配）→ (nil, false)。
// 调用方按 false 降级（如 hotspot.Init 返回错误 fail fast）。
func RawRedisClient() (*redis.Client, bool) {
	c := Get()
	if c == nil {
		return nil, false
	}
	if rp, ok := c.(rawClientProvider); ok {
		return rp.RawClient()
	}
	return nil, false
}
