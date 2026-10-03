// Package cache — multi.go 实现多层缓存（L1 内存 + L2 Redis）的组合。
//
// 设计（D5 / D16）：
//   - L1 进程内内存缓存（TTL 短 5-30s），命中即返回，避免打 Redis。
//   - L2 Redis 集群共享（TTL 长 5min+），跨进程一致。
//   - Get 流程：L1 命中直接返回；L1 miss → 查 L2；L2 命中时回填 L1；双 miss 返回 ErrCacheMiss。
//   - Set 流程：L2 + L1 同时写（write-back 双向）。任一失败不影响主调用，warn log。
//   - Del 流程：L2 + L1 同时删（reverse invalidate 双向）。
//   - SetNX / Incr / Decr：仅走 L2（L1 不支持分布式原子）。
//
// 为什么 L1 失败不阻断主流程：缓存是优化，不是正确性。L1 故障时退化到仅 L2，
// 业务侧只感知到「稍微慢一点」。
//
// 为什么 L2 失败也不阻断 L1 写：L2 写失败通常意味着网络抖动，进程内仍然
// 可服务（虽然下一次进程重启会丢数据，但 TTL 短，影响有限）。
package cache

import (
	"context"
	"time"

	"ai-go-mall/internal/infra/cache/driver"
)

// MultiLevelCache 是 cache.Cache 接口的多层实现：L1 内存 + L2 Redis。
//
// 业务代码通过 cache.Cache 接口持有；不直接依赖本类型。
type MultiLevelCache struct {
	l1 driver.Driver // 进程内内存
	l2 driver.Driver // 分布式 Redis
}

// compile-time 断言：MultiLevelCache 必须实现 Cache 接口。
var _ Cache = (*MultiLevelCache)(nil)

// NewMultiLevelCache 构造多层缓存。
//
// l1/l2 都不能为 nil —— 缺任一层即失去「多层」语义。
// 校验在构造期 fail fast，避免运行时 nil pointer。
func NewMultiLevelCache(l1, l2 driver.Driver) (*MultiLevelCache, error) {
	if l1 == nil {
		return nil, ErrCacheInvalid
	}
	if l2 == nil {
		return nil, ErrCacheInvalid
	}
	return &MultiLevelCache{l1: l1, l2: l2}, nil
}

// --- 接口实现（Cache） ---

// Get 按 L1 → L2 顺序读；命中任一层即返回，并回填上游（三态语义见 Cache 接口）。
//
// L1 命中：直接返回（不再打 L2）。
// L1 miss / L2 命中：返回 L2 value 并回填 L1（让下一次请求走 L1）。
// 双 miss：返回 ErrCacheMiss。
// 命中的值若是 notFound 占位 → (""，false, nil)（D5.1，任务 17.1）。
func (m *MultiLevelCache) Get(ctx context.Context, key string) (string, bool, error) {
	if key == "" {
		return "", false, ErrCacheInvalid
	}
	// L1 命中直接返回。
	if v, err := m.l1.Get(ctx, key); err == nil {
		if IsNotFoundValue(v) {
			return "", false, nil
		}
		return v, true, nil
	}
	// L1 miss → 查 L2。
	v, err := m.l2.Get(ctx, key)
	if err != nil {
		// L2 也 miss 或失败：原样返回（ErrCacheMiss 或包装 error）。
		return "", false, err
	}
	if IsNotFoundValue(v) {
		return "", false, nil
	}
	// L2 命中：回填 L1（失败 warn 即可）。
	if setErr := m.l1.Set(ctx, key, v, 0); setErr != nil {
		// L1 回填失败不阻断主流程，业务下次仍会再走 L2。
		// 此处不返回 error，调用方拿到的是 L2 的 value。
		_ = setErr
	}
	return v, true, nil
}

// Set 同时写 L2 + L1。
//
// 顺序：先 L2 后 L1 —— L2 是权威，失败需让调用方感知（但当前实现不返回 err，
// 仅 warn —— 与 D5「缓存是优化」一致）。
//
// ttl 语义：
//   - ttl > 0 → 两层都用此 TTL；
//   - ttl == 0 → L2 不设过期（Redis 默认），L1 走 L1 driver 的 DefaultTTL；
//   - ttl < 0 → 同 ttl=0。
func (m *MultiLevelCache) Set(ctx context.Context, key, value string, ttl time.Duration) error {
	if key == "" {
		return ErrCacheInvalid
	}
	// L2 优先写：分布式一致性由 L2 兜底。
	if err := m.l2.Set(ctx, key, value, ttl); err != nil {
		return err
	}
	// L1 回填失败不阻断主流程。
	_ = m.l1.Set(ctx, key, value, ttl)
	return nil
}

// Del 同时删 L2 + L1（reverse invalidate 双向）。
//
// L2 失败返回 error（分布式未失效会让其他进程仍读旧数据，需让调用方感知）。
// L1 失败 warn 即可（仅本进程）。
func (m *MultiLevelCache) Del(ctx context.Context, key string) error {
	if key == "" {
		return ErrCacheInvalid
	}
	if err := m.l2.Del(ctx, key); err != nil {
		return err
	}
	_ = m.l1.Del(ctx, key)
	return nil
}

// SetNX 仅走 L2（L1 不支持分布式原子 SetNX）。
//
// 行为：先 L2 SetNX；返回 true（首次设置）时回填 L1。
func (m *MultiLevelCache) SetNX(ctx context.Context, key, value string, ttl time.Duration) (bool, error) {
	if key == "" {
		return false, ErrCacheInvalid
	}
	ok, err := m.l2.SetNX(ctx, key, value, ttl)
	if err != nil {
		return false, err
	}
	if ok {
		// 仅在新设置成功时回填 L1，避免覆盖已存在的 L1 数据。
		_ = m.l1.Set(ctx, key, value, ttl)
	}
	return ok, nil
}

// Incr 仅走 L2（L1 不支持分布式原子 INCR）。
func (m *MultiLevelCache) Incr(ctx context.Context, key string) (int64, error) {
	if key == "" {
		return 0, ErrCacheInvalid
	}
	return m.l2.Incr(ctx, key)
}

// Decr 仅走 L2（L1 不支持分布式原子 DECR）。
func (m *MultiLevelCache) Decr(ctx context.Context, key string) (int64, error) {
	if key == "" {
		return 0, ErrCacheInvalid
	}
	return m.l2.Decr(ctx, key)
}
