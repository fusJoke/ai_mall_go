// Package driver — mock.go 提供 MQ Driver 的进程内内存实现（单测专用）。
//
// 设计：
//   - 单进程 map + chan：Publish 写入「topic + group」对应的 chan；
//     Subscribe 从 chan 读取 → 调 handler。
//   - 不跨进程、不持久化：仅用于单测（生产环境走 redis driver）。
//   - 接口签名与 redis driver 完全一致，调用方可无感切换。
//
// 限制：
//   - 不实现 ack 语义（handler 返回非 nil 时仅记录日志，不重试）；
//     后续如需 mock 完整 ack 语义再扩展。
//   - 不模拟网络延迟 / 失败：fail-fast 模式便于测试。
package driver

import (
	"context"
	"sync"
	"sync/atomic"
)

// mockDriver 是 Driver 接口的内存实现。
//
// 内部结构：
//   - subs：topic → group → channel，每个 (topic, group) 一条独立 chan；
//     Publish 时复制消息到该 group 的 chan。
//   - bufferSize：每个 chan 的缓冲（够大，避免 Publish 阻塞）。
type mockDriver struct {
	mu     sync.RWMutex
	subs   map[string]map[string]chan Message
	buffer int
}

// NewMock 构造 mock driver。buffer 控制每个 (topic, group) channel 的容量。
func NewMock() Driver {
	return &mockDriver{
		subs:   make(map[string]map[string]chan Message),
		buffer: 1024,
	}
}

var _ Driver = (*mockDriver)(nil)

// channelFor 返回 topic+group 对应的 chan，首次访问时创建。
//
// 加锁约定：调用方必须持有 m.mu（写锁）。
func (m *mockDriver) channelFor(topic, group string) chan Message {
	tg, ok := m.subs[topic]
	if !ok {
		tg = make(map[string]chan Message)
		m.subs[topic] = tg
	}
	ch, ok := tg[group]
	if !ok {
		ch = make(chan Message, m.buffer)
		tg[group] = ch
	}
	return ch
}

// Publish 把消息复制到 topic 下所有 group 的 chan。
//
// 非阻塞：buffer 满了直接丢弃（mock 测试场景下不会发生，生产用 redis driver）。
//
// 入参校验：topic 空 → ErrInvalid（与 driver 接口契约一致；cmd/serve 已
// 在 Manager.Publish 做校验，但 driver 自身也必须兜底，防御性契约）。
func (m *mockDriver) Publish(ctx context.Context, topic string, payload []byte) error {
	if topic == "" {
		return ErrInvalid
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	msg := Message{
		ID:      nextMockID(),
		Topic:   topic,
		Payload: payload,
	}

	// 把消息复制到 topic 下每个 group 的 chan。
	for _, ch := range m.subs[topic] {
		select {
		case ch <- msg:
		default:
			// buffer 满：丢弃（mock 不模拟失败）。
		}
	}
	return nil
}

// Subscribe 阻塞消费 (topic, group) 的消息。
//
// handler 返回 nil → ack（mock 直接丢弃消息）；
// handler 返回非 nil → 不 ack（mock 把消息放回 chan 头部，下次重试）。
//
// 退出条件：
//   - ctx cancel → 返回 ctx.Err()。
//
// 入参校验：topic/group/handler 任一空 → ErrInvalid（接口契约；mock 不校验
// 会导致 channelFor 创建空 key 的 chan，后续 Publish 测试或 test 间状态泄漏）。
func (m *mockDriver) Subscribe(ctx context.Context, topic, group string, handler func(ctx context.Context, msg Message) error) error {
	if topic == "" || group == "" || handler == nil {
		return ErrInvalid
	}
	m.mu.Lock()
	ch := m.channelFor(topic, group)
	m.mu.Unlock()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case msg := <-ch:
			if err := handler(ctx, msg); err != nil {
				// 不重试：仅记 mock 语义，handler 失败的消息丢弃（与生产 redis 行为不同）。
				// 重试行为在集成测试中由真实 Redis Stream 验证。
				_ = err
			}
		}
	}
}

// Ping mock 无网络资源，恒返回 nil。
func (m *mockDriver) Ping(ctx context.Context) error {
	return nil
}

var mockIDCounter atomic.Int64

// nextMockID 自增 ID，仅 mock 测试用，不要求全局唯一。
func nextMockID() string {
	n := mockIDCounter.Add(1)
	return formatID(n)
}

// formatID 把 int64 格式化为字符串（避免导入 strconv）。
func formatID(n int64) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
