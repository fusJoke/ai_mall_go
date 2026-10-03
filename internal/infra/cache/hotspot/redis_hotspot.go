// redis_hotspot.go — HotspotCache 的 Redis 实现（D21）。
//
// 结构：
//   - redisPort：本包对底层 Redis 的最小能力面（Get/Set/Del/SetNX/Eval），
//     生产实现包 *redis.Client（共享 cache 的连接池），单测可注入 fake / miniredis。
//   - envelope：持久化值的外层信封 {data, refreshed_at}；data 是业务值的 JSON。
//   - singleflight：冷启动合并，N 个并发 miss 只放 1 个打 loader。
//   - 分布式锁：SET lock:{key} {token} NX EX 5s；释放走 Lua「比对 token 才删」，
//     防止误删后继持锁者的锁（spec Scenario "Lock release safety via Lua script"）。
//
// 刷新 goroutine 的生命周期：脱离请求 ctx（context.WithoutCancel）——请求结束
// 不能打断后台刷新；loader 内部的 DB 访问由调用方负责用脱离请求的 DB 会话
// （见 database 包的 detached context helper）。
package hotspot

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"golang.org/x/sync/singleflight"

	"ai-go-mall/internal/infra/cache"
	"ai-go-mall/internal/infra/cache/driver"
)

// redisPort 是 hotspot 对底层 Redis 的最小能力面。
//
// Get 语义与 cache driver 一致：key 不存在返回 driver.ErrCacheMiss。
type redisPort interface {
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key, value string, ttl time.Duration) error
	Del(ctx context.Context, key string) error
	SetNX(ctx context.Context, key, value string, ttl time.Duration) (bool, error)
	Eval(ctx context.Context, script string, keys []string, args ...any) (any, error)
}

// scriptUnlock 释放分布式锁的 Lua 脚本：只删「自己的锁」。
//
// GET 与传入 token 比对一致才 DEL——持锁者 A 执行超时锁自动过期、B 抢到
// 新锁后，A 迟到的释放不会误删 B 的锁。
const scriptUnlock = `if redis.call("GET", KEYS[1]) == ARGV[1] then return redis.call("DEL", KEYS[1]) else return 0 end`

// envelope 是热点值的外层信封。
type envelope struct {
	// Data 业务值的 JSON（json.RawMessage 避免二次编解码）；
	// NotFound=true 时 Data 是 *cache.NotFoundPayload 的 JSON。
	Data json.RawMessage `json:"data"`

	// RefreshedAt 逻辑时间戳：判断是否触发异步刷新的唯一依据。
	RefreshedAt time.Time `json:"refreshed_at"`

	// NotFound 防穿透标记（D5.1 / 任务 17.3）：loader 返回 ErrNotFound 时置位。
	// 占位与真实数据一样永驻（TTL=0），随 staleAfter 逻辑过期触发重查——
	// 数据后来真的被创建了，刷新即替换为真实值。
	NotFound bool `json:"not_found,omitempty"`
}

// RedisCache 是 HotspotCache 的 Redis 实现。
type RedisCache struct {
	rdb redisPort
	sf  singleflight.Group

	lockTTL time.Duration
	nowFn   func() time.Time // 可注入时钟（单测控制逻辑过期）；nil 走 time.Now

	mu       sync.Mutex
	observer func(key string) // 测试钩子：异步刷新完成后回调；nil = 无
}

// NewRedis 用现成的 go-redis 客户端构造（共享 cache 的连接池）。
func NewRedis(client *redis.Client) *RedisCache {
	return &RedisCache{rdb: &redisClientPort{client: client}, lockTTL: DefaultLockTTL}
}

// newWithPort 供单测注入 fake / miniredis 适配器。
func newWithPort(p redisPort) *RedisCache {
	return &RedisCache{rdb: p, lockTTL: DefaultLockTTL}
}

// SetRefreshObserver 注册异步刷新完成回调（测试用；生产不注册）。
func (h *RedisCache) SetRefreshObserver(fn func(key string)) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.observer = fn
}

// SetNowFn 注入时钟（测试用）。
func (h *RedisCache) SetNowFn(fn func() time.Time) {
	h.nowFn = fn
}

// --- HotspotCache 接口实现 ---

// Get 实现见接口注释。
//
// singleflight 以 key 合并并发请求：同一 key 的 N 个并发 Get 共享一次
// getOnce 的结果（含 error）——冷启动时 DB 只被打 1 次（spec Scenario
// "Cold start loads from DB"）。
func (h *RedisCache) Get(ctx context.Context, key string, loader HotspotLoader, staleAfter time.Duration) (any, error) {
	if key == "" || loader == nil {
		return nil, ErrInvalidInput
	}
	v, err, _ := h.sf.Do(key, func() (any, error) {
		return h.getOnce(ctx, key, loader, staleAfter)
	})
	if err != nil {
		return nil, err
	}
	return v, nil
}

// Invalidate 实现见接口注释（DEL；下次读触发同步冷启动）。
func (h *RedisCache) Invalidate(ctx context.Context, key string) error {
	if key == "" {
		return ErrInvalidInput
	}
	if err := h.rdb.Del(ctx, key); err != nil {
		return fmt.Errorf("hotspot: invalidate %q: %w", key, err)
	}
	return nil
}

// --- 内部路径 ---

// getOnce 是「读一次 + 按需触发刷新」的完整路径（singleflight 合并后单执行）。
func (h *RedisCache) getOnce(ctx context.Context, key string, loader HotspotLoader, staleAfter time.Duration) (any, error) {
	raw, err := h.rdb.Get(ctx, key)
	if errors.Is(err, driver.ErrCacheMiss) {
		// 冷启动：同步 loader（不可避免）+ 写回（永驻）。
		return h.loadAndStore(ctx, key, loader)
	}
	if err != nil {
		return nil, fmt.Errorf("hotspot: get %q: %w", key, err)
	}

	var env envelope
	if err := json.Unmarshal([]byte(raw), &env); err != nil || env.Data == nil {
		// 脏数据（版本升级 / 人工写入）→ 视作 miss，冷启动覆盖。
		log.Printf("hotspot: corrupt envelope for %q (%v), reloading", key, err)
		return h.loadAndStore(ctx, key, loader)
	}

	// 防穿透（D5.1）：占位命中 → 直接「不存在」，不打 DB。
	// 占位同样参与逻辑过期：staleAfter 后的读会触发异步重查，
	// 数据后来被创建时刷新即替换为真实值（不被占位误导）。
	if env.NotFound {
		if staleAfter > 0 && h.now().Sub(env.RefreshedAt) > staleAfter {
			h.tryAsyncRefresh(ctx, key, loader)
		}
		var p cache.NotFoundPayload
		if uerr := json.Unmarshal(env.Data, &p); uerr == nil {
			if _, ok := cache.UnwrapNotFound(&p); ok {
				return nil, cache.ErrNotFound
			}
		}
		return nil, cache.ErrNotFound
	}

	// 逻辑时间到期 → 尝试异步刷新（抢锁，单飞行器）；无论抢没抢到都返回老数据。
	if staleAfter > 0 && h.now().Sub(env.RefreshedAt) > staleAfter {
		h.tryAsyncRefresh(ctx, key, loader)
	}
	return env.Data, nil
} // loadAndStore 执行 loader 并写回热点值；写回失败不阻断（返回 loader 结果）。
// loader 返回 ErrNotFound（D5.1）→ 写 notFound 占位信封并返回原错误：
// 后续相同 key 的读直接得到 ErrNotFound，不打 DB（任务 17.3）。
func (h *RedisCache) loadAndStore(ctx context.Context, key string, loader HotspotLoader) (any, error) {
	data, err := loader(ctx, key)
	if err != nil {
		if cache.IsNotFound(err) {
			if serr := h.storeNotFound(ctx, key); serr != nil {
				log.Printf("hotspot: store notFound placeholder %q failed: %v", key, serr)
			}
		}
		return nil, err
	}
	if serr := h.store(ctx, key, data); serr != nil {
		log.Printf("hotspot: store %q failed (serve loader result anyway): %v", key, serr)
	}
	return data, nil
}

// storeNotFound 写 notFound 占位信封（NotFound 标记 + NotFoundPayload 载荷）。
func (h *RedisCache) storeNotFound(ctx context.Context, key string) error {
	payload, err := json.Marshal(cache.WrapNotFound(nil))
	if err != nil {
		return fmt.Errorf("hotspot: marshal notFound payload for %q: %w", key, err)
	}
	env := envelope{Data: payload, RefreshedAt: h.now(), NotFound: true}
	raw, err := json.Marshal(env)
	if err != nil {
		return fmt.Errorf("hotspot: marshal notFound envelope for %q: %w", key, err)
	}
	return h.rdb.Set(ctx, key, string(raw), 0)
}

// store 序列化 envelope 并写 Redis（ttl=0 永驻；容量管理交给 Invalidate / 未来 LRU）。
func (h *RedisCache) store(ctx context.Context, key string, data any) error {
	dataJSON, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("hotspot: marshal data for %q: %w", key, err)
	}
	payload, err := json.Marshal(envelope{Data: dataJSON, RefreshedAt: h.now()})
	if err != nil {
		return fmt.Errorf("hotspot: marshal envelope for %q: %w", key, err)
	}
	return h.rdb.Set(ctx, key, string(payload), 0)
}

// tryAsyncRefresh 逻辑过期后的刷新入口：抢锁成功才刷新，失败静默（读路径零感知）。
func (h *RedisCache) tryAsyncRefresh(ctx context.Context, key string, loader HotspotLoader) {
	lockKey := "lock:" + key
	token := newLockToken()
	ok, err := h.rdb.SetNX(ctx, lockKey, token, h.lockTTLValue())
	if err != nil {
		log.Printf("hotspot: acquire lock %q failed: %v", lockKey, err)
		return
	}
	if !ok {
		// 别的请求正在刷新 —— 本请求直接返回老数据（spec Scenario
		// "Concurrent requests see stale data without lock contention"）。
		return
	}

	// 脱离请求生命周期：请求结束/客户端断开不能打断后台刷新。
	runCtx := context.WithoutCancel(ctx)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("hotspot: async refresh %q panicked: %v", key, r)
			}
			h.releaseLock(runCtx, lockKey, token)
			h.notifyRefresh(key)
		}()
		data, lerr := loader(runCtx, key)
		if lerr != nil {
			// 刷新失败保留老数据（热点永驻的容错语义）。
			log.Printf("hotspot: async refresh %q failed (keep stale): %v", key, lerr)
			return
		}
		if serr := h.store(runCtx, key, data); serr != nil {
			log.Printf("hotspot: async refresh store %q failed: %v", key, serr)
		}
	}()
}

// releaseLock 用 Lua 只删自己的锁；Eval 失败仅 warn（锁 5s TTL 自动过期兜底）。
func (h *RedisCache) releaseLock(ctx context.Context, lockKey, token string) {
	if _, err := h.rdb.Eval(ctx, scriptUnlock, []string{lockKey}, token); err != nil {
		log.Printf("hotspot: release lock %q failed (expires in %s): %v", lockKey, h.lockTTL, err)
	}
}

func (h *RedisCache) notifyRefresh(key string) {
	h.mu.Lock()
	observer := h.observer
	h.mu.Unlock()
	if observer != nil {
		observer(key)
	}
}

func (h *RedisCache) now() time.Time {
	if h.nowFn != nil {
		return h.nowFn()
	}
	return time.Now()
}

func (h *RedisCache) lockTTLValue() time.Duration {
	if h.lockTTL <= 0 {
		return DefaultLockTTL
	}
	return h.lockTTL
}

// --- redis 客户端适配 ---

// redisClientPort 把 *redis.Client 适配成 redisPort。
type redisClientPort struct {
	client *redis.Client
}

func (p *redisClientPort) Get(ctx context.Context, key string) (string, error) {
	val, err := p.client.Get(ctx, key).Result()
	if errors.Is(err, redis.Nil) {
		return "", driver.ErrCacheMiss
	}
	if err != nil {
		return "", fmt.Errorf("hotspot: redis get: %w", err)
	}
	return val, nil
}

func (p *redisClientPort) Set(ctx context.Context, key, value string, ttl time.Duration) error {
	if err := p.client.Set(ctx, key, value, ttl).Err(); err != nil {
		return fmt.Errorf("hotspot: redis set: %w", err)
	}
	return nil
}

func (p *redisClientPort) Del(ctx context.Context, key string) error {
	if err := p.client.Del(ctx, key).Err(); err != nil {
		return fmt.Errorf("hotspot: redis del: %w", err)
	}
	return nil
}

func (p *redisClientPort) SetNX(ctx context.Context, key, value string, ttl time.Duration) (bool, error) {
	ok, err := p.client.SetNX(ctx, key, value, ttl).Result()
	if err != nil {
		return false, fmt.Errorf("hotspot: redis setnx: %w", err)
	}
	return ok, nil
}

func (p *redisClientPort) Eval(ctx context.Context, script string, keys []string, args ...any) (any, error) {
	res, err := p.client.Eval(ctx, script, keys, args...).Result()
	if err != nil {
		return nil, fmt.Errorf("hotspot: redis eval: %w", err)
	}
	return res, nil
}
