// Package driver — memory.go 提供进程内 L1 内存缓存实现。
//
// 设计（D5 / D16）：
//   - L1 内存缓存用于热点详情 / feed 的进程内复用，避免每次都打 Redis。
//   - 选型：github.com/patrickmn/go-cache，社区主流的进程内 KV 缓存库，
//     自带 TTL + 后台 CleanupInterval 自动过期。
//   - L1 不参与分布式原子操作（SetNX / Incr / Decr）——
//     这三个方法直接返回 ErrL1NotSupported，由调用方 fallback 到 L2 Redis。
//     原因见 design.md D5：限流必须全集群共享。
//   - 容量上限：当前 go-cache v2.1.0 不提供内置 LRU 字段；
//     MaxSize 仅作配置项保留，不在 v1 启用；后续如需 LRU 切换到 v3 或自行实现。
package driver

import (
	"context"
	"errors"
	"time"

	"github.com/patrickmn/go-cache"
)

// ErrL1NotSupported 表示 L1 内存缓存不支持该操作。
//
// 触发场景：MultiLevelCache 收到 SetNX / Incr / Decr 调用时直接返回本错误，
// 由调用方 fallback 到 L2 Redis（分布式原子）。
//
// 业务代码不应捕获本 error —— 通过 cache.Cache 接口拿到的是 MultiLevelCache，
// 它内部已经处理 fallback，业务层只看到「成功」或 ErrCacheMiss。
var ErrL1NotSupported = errors.New("cache: L1 memory driver does not support this operation")

// MemoryConfig L1 内存缓存的配置。
//
// 所有字段都有合理默认值（见 NewMemory 的赋值），yaml 不写也能跑。
type MemoryConfig struct {
	// DefaultTTL 每条数据默认过期时间。
	// 调用方显式传 ttl=0 时走此默认值。
	DefaultTTL time.Duration

	// CleanupInterval 后台清扫 goroutine 的间隔。
	// 0 → 不启动清扫（依赖访问时的 lazy 过期）；建议非 0。
	CleanupInterval time.Duration

	// MaxSize 容量上限（条目数）。当前版本未启用（go-cache v2.1.0 无内置 LRU），
	// 保留字段供后续 v3 或自定义实现接入。
	MaxSize int
}

// memoryDriver 是 Driver 接口的进程内 L1 实现。
type memoryDriver struct {
	c *cache.Cache
}

// NewMemory 按配置构造 L1 内存驱动。
//
// 配置缺省值：
//   - DefaultTTL=10s（设计推荐值，对应热点详情 L1 TTL）
//   - CleanupInterval=60s
//   - MaxSize=0（未启用）
func NewMemory(cfg MemoryConfig) (Driver, error) {
	if cfg.DefaultTTL <= 0 {
		cfg.DefaultTTL = 10 * time.Second
	}
	if cfg.CleanupInterval < 0 {
		cfg.CleanupInterval = 0
	}
	return &memoryDriver{c: cache.New(cfg.DefaultTTL, cfg.CleanupInterval)}, nil
}

// compile-time 接口断言。
var _ Driver = (*memoryDriver)(nil)

// Get 按 key 读 value；不存在返回 ErrCacheMiss。
func (d *memoryDriver) Get(ctx context.Context, key string) (string, error) {
	v, found := d.c.Get(key)
	if !found {
		return "", ErrCacheMiss
	}
	s, ok := v.(string)
	if !ok {
		// 类型断言失败：理论上不会发生（Set 时就 string 进去），
		// 防御性返回 ErrCacheMiss 避免上层 panic。
		return "", ErrCacheMiss
	}
	return s, nil
}

// Set 写 key = value + TTL。
//
// ttl > 0 → go-cache.Set 带过期；ttl == 0 → 走 DefaultTTL；ttl < 0 → 不过期。
func (d *memoryDriver) Set(ctx context.Context, key, value string, ttl time.Duration) error {
	var effectiveTTL time.Duration
	switch {
	case ttl > 0:
		effectiveTTL = ttl
	case ttl == 0:
		effectiveTTL = cache.DefaultExpiration
	case ttl < 0:
		effectiveTTL = cache.NoExpiration
	}
	d.c.Set(key, value, effectiveTTL)
	return nil
}

// Del 删单 key；不存在 no-op。
func (d *memoryDriver) Del(ctx context.Context, key string) error {
	d.c.Delete(key)
	return nil
}

// SetNX 在 L1 不支持（必须全集群共享）。
//
// 返回 ErrL1NotSupported —— MultiLevelCache 收到后会自动 fallback 到 L2 Redis。
func (d *memoryDriver) SetNX(ctx context.Context, key, value string, ttl time.Duration) (bool, error) {
	return false, ErrL1NotSupported
}

// Incr 在 L1 不支持（必须全集群共享的原子计数）。
func (d *memoryDriver) Incr(ctx context.Context, key string) (int64, error) {
	return 0, ErrL1NotSupported
}

// Decr 在 L1 不支持（必须全集群共享的原子计数）。
func (d *memoryDriver) Decr(ctx context.Context, key string) (int64, error) {
	return 0, ErrL1NotSupported
}

// Ping L1 内存缓存无需网络检查，返回 nil。
//
// Ping 在 Init 阶段调用；L1 不会失败，但保留接口实现以便上层统一处理。
func (d *memoryDriver) Ping(ctx context.Context) error {
	return nil
}

// Close 释放资源。L1 内存驱动无外部资源，但保留接口对称性。
func (d *memoryDriver) Close() error {
	// go-cache 没有 Close；保留方法签名便于未来扩展。
	return nil
}
