// Package driver 收容 cache 各种后端实现（redis / memory / multi ...）。
//
// 每个驱动一个文件，文件名即驱动名（如 redis.go / memory.go / multi.go）。
// Driver 接口在 cache.go 定义，driver 包依赖 cache 包的 sentinel errors
// （如 ErrCacheMiss），形成 cache → driver 单向依赖。
//
// 设计要点（与 internal/infra/token/driver 对齐）：
//   - Driver 是接口，对外只暴露接口；具体实现不导出结构体，只导出构造函数。
//   - Ping 由 driver 实现：在 Init 阶段确认后端可达，失败返回 error
//     （main 调 log.Fatal 退出）。
//   - 所有方法的 ctx 由调用方传入，driver 内部用 ctx 控制超时 / 取消。
package driver

import (
	"context"
	"errors"
	"time"

	"ai-go-mall/internal/infra/config"
)

// ErrCacheMiss key 不存在（业务"未命中"，区别于 driver 故障）。
//
// 业务方通过 errors.Is(err, ErrCacheMiss) 区分；
// 所有 driver 必须返回此 sentinel 以保证上层兼容性。
var ErrCacheMiss = errors.New("cache: miss")

// Driver 是 cache 后端的统一接口。
//
// 当前 6 个方法覆盖 MVP 业务用例；后续如需新增（如 MGet / MSet），
// 扩展接口即可，但保留现有方法签名以保证已落地的调用方不破坏。
type Driver interface {
	// Get 按 key 读 value；不存在返回 ErrCacheMiss。
	Get(ctx context.Context, key string) (string, error)

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

	// Ping 健康检查；Init 阶段调用，失败 fail fast。
	Ping(ctx context.Context) error
}

// RedisConfig 是构造 Redis 驱动所需的最小配置子集。
// 引用 config.RedisConfig 类型避免 driver 包反向依赖 config 内部细节。
type RedisConfig = config.RedisConfig

// NewRedis 是 Redis 驱动的工厂函数。
// 由 cache.NewDriver("redis", cfg) 调用；不直接由业务代码使用。
func NewRedis(cfg RedisConfig) (Driver, error) {
	return newRedisDriver(cfg)
}
