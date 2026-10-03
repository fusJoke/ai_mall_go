// Package driver 收容 mq 各种后端实现（redis / mock ...）。
//
// 每个驱动一个文件，文件名即驱动名（如 redis.go / mock.go）。
// Driver 接口在 driver.go 定义，driver 包依赖 mq 包的 sentinel errors
// （如 ErrPublishFailed / ErrSubscribeClosed），形成 mq → driver 单向依赖。
//
// 设计要点（与 internal/infra/cache/driver 对齐）：
//   - Driver 是接口，对外只暴露接口；具体实现不导出结构体，只导出构造函数。
//   - Ping 由 driver 实现：在 Init 阶段确认后端可达，失败返回 error
//     （main 调 log.Fatal 退出）。
//   - 所有方法的 ctx 由调用方传入，driver 内部用 ctx 控制超时 / 取消。
package driver

import (
	"context"
	"errors"

	"ai-go-mall/internal/infra/config"
)

// Sentinel errors：调用方通过 errors.Is 区分业务分支。
//
// 所有 driver 必须返回这些 sentinel 以保证上层 errors.Is 兼容性。
var (
	// ErrPublishFailed 消息发布失败（broker 不可达 / 写入失败）。
	ErrPublishFailed = errors.New("mq: publish failed")

	// ErrSubscribeClosed Subscribe 循环已关闭（ctx cancel 或 driver 内部退出）。
	//
	// driver 实现应在 ctx.Done() 时立即返回本错误（或 context.Canceled）。
	ErrSubscribeClosed = errors.New("mq: subscribe closed")

	// ErrInvalid 入参非法（空 topic / nil payload / nil handler）。
	ErrInvalid = errors.New("mq: invalid input")
)

// Driver 是 mq 后端的统一接口。
//
// 当前 3 个方法覆盖 MVP 业务用例：
//   - Publish：发布消息；
//   - Subscribe：阻塞消费；
//   - Ping：Init 阶段健康检查。
//
// 后续如需新增（如 AckWithID / BatchPublish），扩展接口即可，
// 但保留现有方法签名以保证已落地的调用方不破坏。
type Driver interface {
	// Publish 同步发送消息到 topic。
	//
	// 返回 err 仅表示消息未到达 broker，不代表业务失败。
	Publish(ctx context.Context, topic string, payload []byte) error

	// Subscribe 阻塞消费 topic。
	//
	// handler 返回 nil → driver ack；非 nil → 不 ack（保留 pending 重试）。
	// 返回 err 表示 Subscribe 循环非预期退出（ctx cancel / driver 异常）。
	Subscribe(ctx context.Context, topic, consumerGroup string, handler func(ctx context.Context, msg Message) error) error

	// Ping 健康检查；Init 阶段调用，失败 fail fast。
	Ping(ctx context.Context) error
}

// Message 是 MQ 消息的最小结构。
//
// 字段语义与 mq.Message 一致；driver 层独立定义避免循环依赖。
type Message struct {
	ID      string
	Topic   string
	Payload []byte
}

// RedisConfig 是构造 Redis 驱动所需的最小配置子集。
//
// 引用 config.RedisConfig 类型避免 driver 包反向依赖 config 内部细节。
type RedisConfig = config.RedisConfig
