package driver

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// redisDriver 是 cache.Driver 的 Redis 实现。
//
// 选型：github.com/redis/go-redis/v9（社区维护的活跃 Go Redis 客户端，
// 与 redis 6+ 兼容，支持 Cluster / Sentinel / 单实例）。
//
// 设计要点：
//   - 不在构造期 Ping，由 cache.Init() 显式调 Ping 一次做 fail fast，
//     构造期失败直接返回 error 给上层。
//   - 连接池上限来自 cfg.PoolSize（默认 50），与 database.write 风格一致。
//   - TTL=0 → redis.Set 不带 EX（Redis 默认不过期）；TTL<0 → 走 redis.SetNX
//     同语义。业务上"不过期"场景极少，主要用于"热点 key 永驻"，
//     由 Phase 16 Hotspot 缓存路径处理，这里保留 ttl=0 直通。
type redisDriver struct {
	client *redis.Client
}

// newRedisDriver 按 RedisConfig 构建 Redis 客户端 + 连接池。
func newRedisDriver(cfg RedisConfig) (*redisDriver, error) {
	if cfg.Host == "" {
		return nil, errors.New("cache: redis host is empty")
	}

	client := redis.NewClient(&redis.Options{
		Addr:         fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
		Password:     cfg.Password,
		DB:           cfg.DB,
		PoolSize:     cfg.PoolSize,
		DialTimeout:  3 * time.Second,
		ReadTimeout:  2 * time.Second,
		WriteTimeout: 2 * time.Second,
	})
	return &redisDriver{client: client}, nil
}

// Get 按 key 读 value；不存在返回 ErrCacheMiss。
//
// redis.Nil 是 go-redis 的 sentinel，业务方应通过 errors.Is(err, ErrCacheMiss)
// 判定，统一封装一层避免外部依赖 go-redis。
func (d *redisDriver) Get(ctx context.Context, key string) (string, error) {
	val, err := d.client.Get(ctx, key).Result()
	if errors.Is(err, redis.Nil) {
		return "", ErrCacheMiss
	}
	if err != nil {
		return "", fmt.Errorf("cache: redis get: %w", err)
	}
	return val, nil
}

// Set 写 key = value + TTL。
//
// ttl > 0 → SET key value EX ttl（按秒过期）
// ttl == 0 → SET key value（不过期，需 Del 失效）
// ttl < 0 → 视同 ttl = 0（同不过期，避免误用产生无意义的微秒 TTL）
func (d *redisDriver) Set(ctx context.Context, key, value string, ttl time.Duration) error {
	if ttl < 0 {
		ttl = 0
	}
	if err := d.client.Set(ctx, key, value, ttl).Err(); err != nil {
		return fmt.Errorf("cache: redis set: %w", err)
	}
	return nil
}

// Del 删单 key；不存在 no-op（redis Del 对不存在 key 返回 0，无错）。
func (d *redisDriver) Del(ctx context.Context, key string) error {
	if err := d.client.Del(ctx, key).Err(); err != nil {
		return fmt.Errorf("cache: redis del: %w", err)
	}
	return nil
}

// SetNX 原子"不存在才设"。
//
// 返回 (true, nil) 表示成功设置，(false, nil) 表示 key 已存在。
// go-redis 的 SetNX 当 key 已存在时返回 (false, nil)，无错误；与业务期望一致。
func (d *redisDriver) SetNX(ctx context.Context, key, value string, ttl time.Duration) (bool, error) {
	if ttl < 0 {
		ttl = 0
	}
	ok, err := d.client.SetNX(ctx, key, value, ttl).Result()
	if err != nil {
		return false, fmt.Errorf("cache: redis setnx: %w", err)
	}
	return ok, nil
}

// Incr 原子 INCR +1，返回增量后的值。
//
// go-redis 的 Incr 对不存在的 key 视为 0 + 1 = 1，与业务期望一致。
func (d *redisDriver) Incr(ctx context.Context, key string) (int64, error) {
	v, err := d.client.Incr(ctx, key).Result()
	if err != nil {
		return 0, fmt.Errorf("cache: redis incr: %w", err)
	}
	return v, nil
}

// Decr 原子 DECR -1，返回减量后的值（可能为负）。
func (d *redisDriver) Decr(ctx context.Context, key string) (int64, error) {
	v, err := d.client.Decr(ctx, key).Result()
	if err != nil {
		return 0, fmt.Errorf("cache: redis decr: %w", err)
	}
	return v, nil
}

// Ping 健康检查；Init 阶段调用，失败 fail fast。
func (d *redisDriver) Ping(ctx context.Context) error {
	if err := d.client.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("cache: redis ping: %w", err)
	}
	return nil
}

// RawClient 暴露底层 go-redis 客户端。
//
// 供 cache 包的 RawRedisClient() 断言转发，让需要 Lua / SETNX 等门面之外
// 能力的高级用法（如 hotspot 热点缓存）共享同一连接池，而不是各开各的客户端。
func (d *redisDriver) RawClient() *redis.Client {
	return d.client
}

// Close 释放底层连接池；目前未在 cache 层暴露 Reset+Close 路径，
// 后续若需要 runtime 释放连接池，再补 Close 方法并由 cache.Reset 调用。
func (d *redisDriver) Close() error {
	return d.client.Close()
}
