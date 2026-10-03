// Package cache 提供缓存的统一抽象与多驱动实现。
//
// 设计（mall MVP 设计 D5 / D16 / Phase 10）：
//   - Cache 接口：Get / Set / Del / SetNX / Incr / Decr 六类操作覆盖业务 90% 用例。
//   - 实现层：单层 (Manager = 单 Driver) 或 多层 (MultiLevelCache = L1 + L2)，
//     由 config.Cache.Driver 决定，Cache 接口保持稳定。
//   - 驱动放 driver/ 子目录（命名与 internal/infra/token/driver/ 对齐）。
//   - Init() 读 config.Get().Cache，按 Driver 字段实例化对应实现。
//
// 后续扩展（不在本期）：
//   - Phase 16 Hotspot 缓存 + Phase 17 防穿透：基于 Cache 接口再加包装层。
//
// 启动流程：cmd/serve/main.go 在 database.Init() 之后调 cache.Init()，
// 内部 Ping 各层（multi 模式只 Ping L2），失败 log.Fatal（spec fail fast）。
package cache

import (
	"context"
	"errors"
	"fmt"
	"time"

	"ai-go-mall/internal/infra/cache/driver"
	"ai-go-mall/internal/infra/config"
)

// Sentinel errors：调用方通过 errors.Is 区分业务分支。
//
// 当前由 redis 驱动实现 driver.ErrCacheMiss（key 不存在）；
// 后续 driver 切换需保留同名 sentinel 以保证调用方 errors.Is 兼容性。
var (
	// ErrCacheMiss key 不存在（业务上"未命中"，区别于 driver 故障）。
	ErrCacheMiss = driver.ErrCacheMiss

	// ErrCacheInvalid 入参非法（空 key / nil 对象等）。
	ErrCacheInvalid = errors.New("cache: invalid input")
)

// Cache 是缓存的对外门面接口（业务代码通过 Get() 间接持有实现）。
//
// 当前 6 类操作覆盖业务 90% 用例：
//   - Get：读单 key；不存在返回 ErrCacheMiss。
//   - Set：写单 key + TTL；ttl=0 表示用 Redis SET 默认（不过期，需显式 Del）。
//   - Del：删单 key；不存在 no-op。
//   - SetNX：原子"不存在才设" + TTL；返回 true=成功设置，false=key 已存在。
//   - Incr：原子 INCR +1，返回增量后的值（秒杀限购计数器）。
//   - Decr：原子 DECR -1，返回减量后的值，可能为负（秒杀名额预扣）。
//
// 为什么不暴露 MGet / MSet：MVP 业务场景固定 3 类缓存（详情 / feed / 限流），
// 暂无批量需求；后续如需批量再加方法，不破坏现有签名。
type Cache interface {
	// Get 按 key 读 value；三态返回（D5.1 防穿透，任务 17.1）：
	//   - (val, true, nil)           命中真实数据；
	//   - ("", false, nil)           命中 notFound 占位（key 确认不存在，不打 DB）；
	//   - ("", false, ErrCacheMiss)  miss（调用方应回源 loader）。
	// 基础设施故障返回 (""，false, 非 ErrCacheMiss 的 error)。
	Get(ctx context.Context, key string) (value string, found bool, err error)

	// Set 写 key = value + TTL。
	// ttl <= 0 时表示不设 TTL（Redis 默认不过期，需靠 Del 失效）。
	Set(ctx context.Context, key string, value string, ttl time.Duration) error

	// Del 删单 key；不存在 no-op。
	Del(ctx context.Context, key string) error

	// SetNX 原子"不存在才设"；返回 (true, nil) 表示成功设置，(false, nil) 表示 key 已存在。
	SetNX(ctx context.Context, key string, value string, ttl time.Duration) (bool, error)

	// Incr 原子 INCR +1，返回增量后的 int64。
	// Redis INCR 对不存在的 key 视为 0 + 1 = 1，与业务期望一致。
	// 典型场景：秒杀限购计数器（seckill:user_bought:{sid}:{uid}）。
	Incr(ctx context.Context, key string) (int64, error)

	// Decr 原子 DECR -1，返回减量后的 int64。
	// 返回值可能为负（如秒杀名额减到 0 后再减）—— 调用方按业务语义判断是否回滚。
	// 典型场景：秒杀名额预扣（seckill:stock:{sid}）。
	Decr(ctx context.Context, key string) (int64, error)
}

// Manager 是单层 Cache 实现：包一个 Driver（当前默认 Redis）。
//
// 实现 Cache 接口的所有方法，转发给底层 driver。
// 适用于 driver="redis" 配置；driver="multi" 时使用 MultiLevelCache 替代。
type Manager struct {
	driver driver.Driver
}

// mgr 是 Init 缓存的 Cache 单例。Init 未调用或失败时为 nil。
//
// 类型为 Cache 接口：可持有 *Manager（单层）或 *MultiLevelCache（多层），
// 由 Init 根据 config.Cache.Driver 决定实例化哪一种。
var mgr Cache

// Init 读取 config.Get().Cache，按 Driver 字段实例化对应 Cache 实现。
//
// driver 取值：
//   - "redis"（默认）：仅 Redis 单层（向后兼容旧部署）
//   - "multi"：L1 内存 + L2 Redis 双层（推荐配置）
//
// 同一进程重复调用是 no-op；如需强制重载，调用 Reset 后再调用 Init。
//
// 注意：依赖 config.Get()，必须先调 config.Init()。
// Init 内部 Ping 各层（multi 仅 Ping L2，连接失败直接返回 error（spec fail fast）。
func Init() error {
	if mgr != nil {
		return nil
	}

	cfg := config.Get().Cache
	var impl Cache
	var err error

	switch cfg.Driver {
	case "redis", "":
		impl, err = newRedisOnlyCache(cfg.Redis)
	case "multi":
		impl, err = newMultiLevelCache(cfg)
	default:
		return fmt.Errorf("cache: unknown driver %q", cfg.Driver)
	}
	if err != nil {
		return err
	}

	mgr = impl
	return nil
}

// newRedisOnlyCache 构造单层 Redis Cache（Manager）。
func newRedisOnlyCache(cfg config.RedisConfig) (*Manager, error) {
	d, err := driver.NewRedis(cfg)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := d.Ping(ctx); err != nil {
		return nil, fmt.Errorf("cache: ping redis: %w", err)
	}
	return &Manager{driver: d}, nil
}

// newMultiLevelCache 构造双层 Cache（MultiLevelCache = L1 内存 + L2 Redis）。
//
// L1 内存不需 Ping；L2 Redis 必须 Ping 一次确认可达。
func newMultiLevelCache(cfg config.CacheConfig) (*MultiLevelCache, error) {
	l1, err := driver.NewMemory(driver.MemoryConfig{
		DefaultTTL:      cfg.Memory.DefaultTTL,
		CleanupInterval: cfg.Memory.CleanupInterval,
		MaxSize:         cfg.Memory.MaxSize,
	})
	if err != nil {
		return nil, fmt.Errorf("cache: init L1 memory: %w", err)
	}
	l2, err := driver.NewRedis(cfg.Redis)
	if err != nil {
		return nil, fmt.Errorf("cache: init L2 redis: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := l2.Ping(ctx); err != nil {
		return nil, fmt.Errorf("cache: ping L2 redis: %w", err)
	}
	return NewMultiLevelCache(l1, l2)
}

// Get 返回已初始化的 Cache 接口。Init 未调用过或失败时返回 nil。
//
// 返回类型为 Cache 接口（不暴露具体实现），业务代码不感知单层 / 多层差异。
// 内部实现通过类型断言可还原为 *Manager 或 *MultiLevelCache（仅供包内使用）。
func Get() Cache {
	return mgr
}

// Reset 清空 Manager 缓存。专供测试使用。
func Reset() {
	mgr = nil
}

// --- Manager 转发方法（参数校验后转 driver） ---

// Get 按 key 读 value（三态语义见 Cache 接口注释）。
//
// 命中的值若是 notFound 占位（{"_notFound":true}），翻译为 (""，false, nil)
// —— 调用方据此直接返回「不存在」，不回源 DB。
func (m *Manager) Get(ctx context.Context, key string) (string, bool, error) {
	if key == "" {
		return "", false, ErrCacheInvalid
	}
	v, err := m.driver.Get(ctx, key)
	if err != nil {
		return "", false, err
	}
	if IsNotFoundValue(v) {
		return "", false, nil
	}
	return v, true, nil
}

// Set 写 key = value + TTL。ttl <= 0 表示不设 TTL。
func (m *Manager) Set(ctx context.Context, key, value string, ttl time.Duration) error {
	if key == "" {
		return ErrCacheInvalid
	}
	return m.driver.Set(ctx, key, value, ttl)
}

// Del 删单 key；不存在 no-op。
func (m *Manager) Del(ctx context.Context, key string) error {
	if key == "" {
		return ErrCacheInvalid
	}
	return m.driver.Del(ctx, key)
}

// SetNX 原子"不存在才设"；返回 (true, nil) 表示成功设置，(false, nil) 表示 key 已存在。
func (m *Manager) SetNX(ctx context.Context, key, value string, ttl time.Duration) (bool, error) {
	if key == "" {
		return false, ErrCacheInvalid
	}
	return m.driver.SetNX(ctx, key, value, ttl)
}

// Incr 原子 INCR +1，返回增量后的值。
func (m *Manager) Incr(ctx context.Context, key string) (int64, error) {
	if key == "" {
		return 0, ErrCacheInvalid
	}
	return m.driver.Incr(ctx, key)
}

// Decr 原子 DECR -1，返回减量后的值（可能为负）。
func (m *Manager) Decr(ctx context.Context, key string) (int64, error) {
	if key == "" {
		return 0, ErrCacheInvalid
	}
	return m.driver.Decr(ctx, key)
}

// compile-time 断言：Manager 必须实现 Cache 接口。
var _ Cache = (*Manager)(nil)
