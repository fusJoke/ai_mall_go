// Package driver — redis.go 提供 MQ Driver 的 Redis Stream 实现。
//
// 设计（mall MVP 设计 D17）：
//   - 用 Redis Stream + 消费组实现「多消费者组隔离 + 失败重试 + ack」语义。
//   - Publish：XADD topic * payload <bytes>
//   - Subscribe：XGROUP CREATE（首次）+ XREADGROUP BLOCK <ms> ... > + XACK
//   - 失败处理：handler 返回 err → 不 ack → 保留在 pending list，下次重试。
//
// 选型：github.com/redis/go-redis/v9（与 cache/driver/redis 共享同一 client 依赖）。
//
// 已知限制（MVP 范围内）：
//   - 失败重试无上限：handler 一直失败则一直 pending（后续 dead letter 队列是 Phase 后续）；
//   - 不处理 XCLAIM（消费者崩溃后由其他消费者接管）—— MVP 单消费者足够。
//   - 不限制 maxlen：stream 会持续增长，由调用方定期 XTRIM。
package driver

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// redisDriver 是 Driver 接口的 Redis Stream 实现。
type redisDriver struct {
	client  *redis.Client
	group   string        // 消费组（来自 cfg.ConsumerGroup，全进程共享）
	timeout time.Duration // XREADGROUP BLOCK 超时
}

// NewRedis 构造 Redis Stream driver。
//
// group 必须非空（XREADGROUP 需要 group）；
// blockTimeout <= 0 时用 5s 默认值。
func NewRedis(cfg RedisConfig) (Driver, error) {
	if cfg.Host == "" {
		return nil, errors.New("mq: redis host is empty")
	}
	// 注意：Driver.NewRedis 当前签名只接收 RedisConfig；
	// consumer_group / block_timeout 走包级字段或扩展签名（MVP 用包级 init 模式）。
	return &redisDriver{
		client: redis.NewClient(&redis.Options{
			Addr:         fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
			Password:     cfg.Password,
			DB:           cfg.DB,
			PoolSize:     cfg.PoolSize,
			DialTimeout:  3 * time.Second,
			ReadTimeout:  5 * time.Second,
			WriteTimeout: 3 * time.Second,
		}),
		timeout: 5 * time.Second,
	}, nil
}

// NewRedisWithGroup 构造带消费组配置的 Redis Stream driver。
//
// 完整签名版：consumerGroup 与 blockTimeout 由调用方提供（来自 config.MQConfig）。
func NewRedisWithGroup(cfg RedisConfig, consumerGroup string, blockTimeout time.Duration) (Driver, error) {
	if cfg.Host == "" {
		return nil, errors.New("mq: redis host is empty")
	}
	if consumerGroup == "" {
		return nil, errors.New("mq: consumer group is empty")
	}
	if blockTimeout <= 0 {
		blockTimeout = 5 * time.Second
	}
	return &redisDriver{
		client: redis.NewClient(&redis.Options{
			Addr:         fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
			Password:     cfg.Password,
			DB:           cfg.DB,
			PoolSize:     cfg.PoolSize,
			DialTimeout:  3 * time.Second,
			ReadTimeout:  5 * time.Second,
			WriteTimeout: 3 * time.Second,
		}),
		group:   consumerGroup,
		timeout: blockTimeout,
	}, nil
}

var _ Driver = (*redisDriver)(nil)

// Publish 用 XADD 把消息写入 topic。
//
// Redis Stream XADD topic * field value [field value ...]
// payload 作为单个 field "payload" 存入；调用方自行 JSON 序列化。
//
// 为什么用 * 作为 ID：让 Redis 自动分配 stream-id（毫秒级 + 序号）。
func (d *redisDriver) Publish(ctx context.Context, topic string, payload []byte) error {
	if topic == "" {
		return ErrInvalid
	}
	if _, err := d.client.XAdd(ctx, &redis.XAddArgs{
		Stream: topic,
		Values: map[string]any{
			"payload": payload,
		},
	}).Result(); err != nil {
		return fmt.Errorf("%w: %v", ErrPublishFailed, err)
	}
	return nil
}

// Subscribe 阻塞消费 topic 的消息，消费组由 cfg.group 决定。
//
// 流程：
//  1. 首次启动时 XGROUP CREATE（BUSYGROUP 错误忽略）；
//  2. 循环 XREADGROUP BLOCK timeout COUNT 1 STREAMS topic >；
//  3. 收到消息 → 调 handler；
//  4. handler nil → XACK；handler err → 不 ack（pending 保留，下次重试）。
//
// 退出条件：ctx cancel → 返回 ctx.Err()。
func (d *redisDriver) Subscribe(ctx context.Context, topic, consumerGroup string, handler func(ctx context.Context, msg Message) error) error {
	if topic == "" || consumerGroup == "" {
		return ErrInvalid
	}
	if handler == nil {
		return ErrInvalid
	}

	// 1) 创建消费组（首次）。BUSYGROUP 表示已存在，忽略。
	//
	// 使用 $ 作为起始 ID：新消费者只接收创建组之后的新消息；
	// 若想从头消费，用 0。
	if err := d.client.XGroupCreateMkStream(ctx, topic, consumerGroup, "$").Err(); err != nil {
		if !isBusyGroupErr(err) {
			return fmt.Errorf("xgroup create: %w", err)
		}
	}

	consumerName := "consumer-" + strconv.FormatInt(time.Now().UnixNano(), 36)

	for {
		// 检查 ctx。
		if err := ctx.Err(); err != nil {
			return err
		}

		// 2) 阻塞读 1 条消息。
		streams, err := d.client.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    consumerGroup,
			Consumer: consumerName,
			Streams:  []string{topic, ">"},
			Count:    1,
			Block:    d.timeout,
		}).Result()
		if err != nil {
			if errors.Is(err, redis.Nil) || isTimeoutErr(err) {
				// BLOCK 超时：无消息，继续下一轮。
				continue
			}
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return err
			}
			return fmt.Errorf("xreadgroup: %w", err)
		}

		for _, stream := range streams {
			for _, xmsg := range stream.Messages {
				payload, _ := xmsg.Values["payload"].(string)
				msg := Message{
					ID:      xmsg.ID,
					Topic:   topic,
					Payload: []byte(payload),
				}

				if herr := handler(ctx, msg); herr != nil {
					// handler 失败：不 ack，保留 pending 等下次重试。
					// MVP 不实现 dead letter / max retry 策略。
					continue
				}

				// handler 成功：ack。
				if err := d.client.XAck(ctx, topic, consumerGroup, xmsg.ID).Err(); err != nil {
					// ack 失败：消息会再次被消费，业务需幂等。
					return fmt.Errorf("xack: %w", err)
				}
			}
		}
	}
}

// Ping 探活。
func (d *redisDriver) Ping(ctx context.Context) error {
	return d.client.Ping(ctx).Err()
}

// Close 释放连接池。
func (d *redisDriver) Close() error {
	return d.client.Close()
}

// isBusyGroupErr 判断 XGROUP CREATE 返回的 BUSYGROUP 错误（group 已存在）。
//
// go-redis 把 Redis 错误包成 redis.Cmdable.Error()，需要匹配 "BUSYGROUP" 子串。
func isBusyGroupErr(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "BUSYGROUP")
}

// isTimeoutErr 判断 XREADGROUP BLOCK 超时。
//
// Redis 返回的 nil result 对应超时；go-redis 在某些版本也返回 wrapped error。
func isTimeoutErr(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "timeout") || strings.Contains(msg, "i/o timeout")
}
