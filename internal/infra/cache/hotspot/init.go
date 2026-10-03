// init.go — hotspot 的进程级装配（与 cache / mq 同模式）。
//
// Init 依赖 cache.Init()（共享其 Redis 连接池），失败即返回 error 由
// cmd/serve log.Fatalf fail fast。
package hotspot

import (
	"errors"

	"github.com/google/uuid"

	"ai-go-mall/internal/infra/cache"
)

// mgr 是 Init 缓存的 HotspotCache 单例。Init 未调用或失败时为 nil。
var mgr HotspotCache

// Init 从 cache 包取共享的 go-redis 客户端装配 RedisCache。
//
// 同一进程重复调用是 no-op；如需强制重载，先调 Reset。
// cache.Init() 未运行 / driver 非 redis 系 → 返回 error（fail fast）。
func Init() error {
	if mgr != nil {
		return nil
	}
	client, ok := cache.RawRedisClient()
	if !ok {
		return errors.New("hotspot: raw redis client unavailable (cache.Init not called or driver is not redis)")
	}
	// 连接可用性已由 cache.Init 的 Ping 保证，这里不再重复 Ping。
	mgr = NewRedis(client)
	return nil
}

// Get 返回已初始化的 HotspotCache。Init 未调用过或失败时返回 nil
// （业务侧按 nil 跳过热点缓存，走原有 TTL / 直查路径）。
func Get() HotspotCache {
	return mgr
}

// Reset 清空单例。专供测试使用。
func Reset() {
	mgr = nil
}

// newLockToken 生成分布式锁的持有者标识（释放时比对，防误删）。
func newLockToken() string {
	return uuid.NewString()
}
