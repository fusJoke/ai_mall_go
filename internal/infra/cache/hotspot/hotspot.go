// Package hotspot 提供热点数据缓存：不过期 + 逻辑时间 + 分布式锁 + 异步刷新
// （mall MVP 设计 D21 / spec Requirement "Hotspot cache"）。
//
// 业务场景：盲盒详情、首页 feed 等「高读低写」数据。经典 Cache-Aside 的
// TTL 到期瞬间会产生缓存雪崩（多请求同时 miss 同时打 DB）；本包的方案：
//
//   - 数据永驻 Redis（TTL=0），值内嵌 RefreshedAt 逻辑时间戳；
//   - 读路径永远立即返回当前缓存值（老数据），DB 压力与读延迟解耦；
//   - 逻辑时间到期（now - RefreshedAt > staleAfter）→ 抢分布式锁
//     （SET NX EX），抢到的那个请求 go 异步刷新，其余请求无感知；
//   - 冷启动（完全 miss）→ singleflight 合并并发请求，只放 1 个打 DB；
//   - 主动失效（Invalidate）→ DEL，下次读触发同步冷启动。
//
// 与 MultiLevelCache（L1/L2 TTL 缓存）的关系（D21「写路径」约定）：
// 写操作先 Hotspot.Invalidate 再 MultiLevelCache.Del —— 两级缓存并存，
// Hotspot 服务热点读，TTL 缓存兜底容量（自动过期防 OOM）。
//
// 穿透防护（D5.1 / 任务 17）：本包暂不缓存 NotFound 占位；loader 返回
// ErrNotFound 语义的占位缓存由任务 17 在本包扩展（预留 envelope 结构）。
package hotspot

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// key 与 staleAfter 约定（D21 应用映射表）。
const (
	// KeyPrefixBlindBox 盲盒详情热点 key 前缀：hotspot:blindbox:{id}。
	KeyPrefixBlindBox = "hotspot:blindbox:"

	// KeyHomeFeed 首页 feed 热点 key：hotspot:feed:home。
	KeyHomeFeed = "hotspot:feed:home"

	// BlindBoxStaleAfter 盲盒详情逻辑过期阈值（D21 映射表）。
	BlindBoxStaleAfter = 30 * time.Second

	// HomeFeedStaleAfter 首页 feed 逻辑过期阈值（D21 映射表）。
	HomeFeedStaleAfter = 60 * time.Second

	// DefaultLockTTL 分布式锁自动过期时长（防持锁者 crash 死锁）。
	DefaultLockTTL = 5 * time.Second
)

// BlindBoxKey 构造盲盒详情热点 key。
func BlindBoxKey(blindBoxID int64) string {
	return fmt.Sprintf("%s%d", KeyPrefixBlindBox, blindBoxID)
}

// Sentinel errors。
var (
	// ErrInvalidInput 入参非法（空 key / nil loader）。
	ErrInvalidInput = errors.New("hotspot: invalid input")
)

// HotspotLoader 是缓存未命中 / 需要刷新时的回源函数。
//
// 返回值语义：
//   - 正常数据 → hotspot 序列化后写 Redis（永驻）；
//   - 数据不存在 → 返回 ErrNotFound 语义错误（任务 17 将在此加占位缓存）。
type HotspotLoader func(ctx context.Context, key string) (data any, err error)

// HotspotCache 是热点缓存的对外门面接口（spec 定义的签名）。
type HotspotCache interface {
	// Get 读热点数据；miss 走 loader 冷启动；逻辑过期触发异步刷新。
	//
	// 无论刷新与否都立即返回当前值：
	//   - 冷启动路径返回 loader 的原始返回值（类型保持）；
	//   - 命中路径返回 json.RawMessage（由 hotspot.Get[T] 类型化封装解码）。
	Get(ctx context.Context, key string, loader HotspotLoader, staleAfter time.Duration) (data any, err error)

	// Invalidate 主动失效（DEL）。下次 Get 触发同步冷启动。
	Invalidate(ctx context.Context, key string) error
}

// Load 是 HotspotCache.Get 的类型安全封装（泛型 helper，规避接口方法的
// any 返回值在调用方的类型断言负担）。
//
// 类型恢复策略：
//   - 冷启动路径：HotspotCache.Get 直接透传 loader 的 T → 零成本类型断言；
//   - 命中路径：底层返回 json.RawMessage → 一次性解码到 T。
//
// 用法：
//
//	detail, err := hotspot.Get(ctx, h, hotspot.BlindBoxKey(id),
//	    func(ctx context.Context, key string) (*userSvc.BlindBoxDetail, error) {
//	        return s.loadDetail(ctx, id)
//	    }, hotspot.BlindBoxStaleAfter)
func Load[T any](ctx context.Context, c HotspotCache, key string, loader func(ctx context.Context, key string) (T, error), staleAfter time.Duration) (T, error) {
	var zero T
	v, err := c.Get(ctx, key, func(ctx context.Context, k string) (any, error) {
		return loader(ctx, k)
	}, staleAfter)
	if err != nil {
		return zero, err
	}
	if typed, ok := v.(T); ok {
		return typed, nil
	}
	// 命中路径：底层是 JSON 字节，解码到 T。
	var raw []byte
	switch data := v.(type) {
	case json.RawMessage:
		raw = data
	case []byte:
		raw = data
	default:
		if raw, err = json.Marshal(v); err != nil {
			return zero, fmt.Errorf("hotspot: encode cached value for %q: %w", key, err)
		}
	}
	var out T
	if err := json.Unmarshal(raw, &out); err != nil {
		return zero, fmt.Errorf("hotspot: decode cached value for %q: %w", key, err)
	}
	return out, nil
}
