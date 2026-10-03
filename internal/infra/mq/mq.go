// Package mq 提供消息队列的统一抽象与多驱动实现。
//
// 设计（mall MVP 设计 D17）：
//   - MQ 接口：Publish / Subscribe 两个操作覆盖业务 90% 用例。
//   - driver 放 driver/ 子目录（命名与 internal/infra/cache/driver 对齐），
//     每个驱动一个文件，文件名即驱动名（redis.go / mock.go）。
//   - Manager 持有 Driver，对外暴露统一 API（业务代码不感知 driver 切换）。
//   - Init() 读 config.Get().MQ，按 Driver 字段实例化对应驱动。
//
// 业务动机（秒杀场景）：
//   - 抽卡事务 COMMIT 后异步 Publish 库存扣减消息（stock.deduction.sync）；
//   - 消费者 cmd/stock-sync 处理消息，UPDATE synced_to_pool_at；
//   - MQ 失败不阻塞业务主路径（事务已落库，下次 cron 兜底）。
//
// 后续扩展（不在本期）：
//   - Phase D17 后续 Kafka / RabbitMQ driver 不动业务代码（仅新加 driver + config.mq.driver 切换）。
//   - 死信队列（dead letter）：handler 失败 N 次后入 DLQ（Phase 后续）。
//
// 启动流程：cmd/serve/main.go 在 cache.Init() 之后调 mq.Init()，
// 内部 Ping broker，失败 log.Fatal（spec fail fast）。
package mq

import (
	"context"
	"errors"
	"time"

	"ai-go-mall/internal/infra/config"
	"ai-go-mall/internal/infra/mq/driver"
)

// Sentinel errors：调用方通过 errors.Is 区分业务分支。
//
// 所有 driver 必须返回 driver.ErrPublishFailed / driver.ErrSubscribeClosed
// 等 sentinel 以保证上层 errors.Is 兼容性。
var (
	// ErrPublishFailed 消息发布失败（broker 不可达 / 写入失败）。
	//
	// 业务侧处理：warn + 后续 cron 兜底（不视为业务失败）。
	ErrPublishFailed = driver.ErrPublishFailed

	// ErrSubscribeClosed Subscribe 循环已关闭（ctx cancel 或 driver 内部退出）。
	//
	// 业务侧处理：常驻进程优雅退出；非预期关闭 → log.Fatal 重启。
	ErrSubscribeClosed = driver.ErrSubscribeClosed

	// ErrInvalid 入参非法（空 topic / nil payload / nil handler）。
	ErrInvalid = errors.New("mq: invalid input")
)

// Message 是 MQ 消息的最小结构（payload + 元信息）。
//
// 字段语义：
//   - ID：broker 分配的消息 ID（Redis Stream 是 stream-id）；
//     Subscribe 时由 driver 填充；Publish 时 driver 忽略。
//   - Topic：消息所属 topic（与 Subscribe / Publish 入参一致；driver 冗余填一份便于 logging）。
//   - Payload：消息正文（业务 JSON）；由 handler 自行反序列化。
type Message = driver.Message

// Handler 是 Subscribe 注册的回调函数。
//
// 返回值语义：
//   - nil → driver ack（消息处理成功，从 pending list 移除）；
//   - 非 nil → driver 不 ack（保留在 pending list，下次重试）。
//
// 为什么不用 panic 表达失败：handler panic 应在外层 recover（cmd/stock-sync 的 defer recover）。
// 这里只通过返回值传递业务级失败语义。
type Handler func(ctx context.Context, msg Message) error

// MQ 是消息队列的对外门面接口（业务代码通过 Manager 间接持有 Driver）。
//
// 当前 2 类操作覆盖业务用例：
//   - Publish：发布消息到指定 topic；
//   - Subscribe：阻塞消费 topic，handler 返回 nil → ack。
//
// 为什么仅 2 个方法：MVP 业务场景固定（秒杀异步对账），暂无 ack-with-id /
// batch-publish 等需求；后续如需再加方法，不破坏现有签名。
type MQ interface {
	// Publish 同步发送消息到 topic。
	//
	// 返回 err 仅表示消息未到达 broker，不代表业务失败：
	//   - 业务侧在事务 COMMIT 后调用 Publish；
	//   - Publish 失败仅 warn + 后续 cron 兜底，不影响业务主路径。
	Publish(ctx context.Context, topic string, payload []byte) error

	// Subscribe 阻塞消费 topic 的消息。
	//
	// handler 返回 nil → ack（消息成功处理）；
	// handler 返回非 nil → 不 ack（保留 pending，driver 内部按 strategy 重试）。
	//
	// 返回 err 仅表示 Subscribe 循环非预期退出（ctx cancel / driver 内部异常）；
	// ctx cancel 时 driver 应返回 ctx.Err()，调用方通过 errors.Is(err, context.Canceled) 区分。
	Subscribe(ctx context.Context, topic, consumerGroup string, handler Handler) error
}

// Manager 是 MQ 业务的对外门面，持有 Driver 提供基础 Publish/Subscribe。
type Manager struct {
	driver driver.Driver
}

// mgr 是 Init 缓存的 Manager 单例。Init 未调用或失败时为 nil。
var mgr *Manager

// Init 读取 config.Get().MQ，按 Driver 字段实例化对应 driver，
// 包装成 Manager 缓存到包级变量。
//
// 同一进程重复调用是 no-op；如需强制重载，调用 Reset 后再调用 Init。
//
// 注意：依赖 config.Get()，必须先调 config.Init()。
// Init 内部 Ping broker 一次，连接失败直接返回 error（spec fail fast）。
func Init() error {
	if mgr != nil {
		return nil
	}

	cfg := config.Get().MQ
	d, err := newDriver(cfg.Driver, cfg)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := d.Ping(ctx); err != nil {
		return err
	}

	mgr = &Manager{driver: d}
	return nil
}

// Get 返回已初始化的 *Manager。Init 未调用过或失败时返回 nil。
func Get() *Manager {
	return mgr
}

// Reset 清空 Manager 缓存。专供测试使用。
func Reset() {
	mgr = nil
}

// --- Manager 转发方法（参数校验后转 driver） ---

// Publish 同步发送消息到 topic。
//
// topic 为空 → ErrInvalid；payload 为 nil 也允许（部分场景发空 marker）。
func (m *Manager) Publish(ctx context.Context, topic string, payload []byte) error {
	if topic == "" {
		return ErrInvalid
	}
	return m.driver.Publish(ctx, topic, payload)
}

// Subscribe 阻塞消费 topic。
//
// 任意入参空 → ErrInvalid。consumerGroup 用于 Redis Stream 消费组隔离。
func (m *Manager) Subscribe(ctx context.Context, topic, consumerGroup string, handler Handler) error {
	if topic == "" || consumerGroup == "" || handler == nil {
		return ErrInvalid
	}
	// Handler 是命名类型，与 driver.Subscribe 期望的函数类型不直接兼容；
	// 包一层 inline 函数转换（零成本）。
	return m.driver.Subscribe(ctx, topic, consumerGroup, func(ctx context.Context, msg driver.Message) error {
		return handler(ctx, msg)
	})
}

// --- 内部 helper ---

// newDriver 是 driver 工厂：根据 driver 名字返回对应实例。
//
// 后续新增 driver（kafka / rabbitmq）只需在此追加 case。
func newDriver(name string, cfg config.MQConfig) (driver.Driver, error) {
	switch name {
	case "redis", "":
		return driver.NewRedisWithGroup(cfg.Redis, cfg.ConsumerGroup, cfg.BlockTimeout)
	case "mock":
		return driver.NewMock(), nil
	default:
		return nil, errors.New("mq: unknown driver " + name)
	}
}
