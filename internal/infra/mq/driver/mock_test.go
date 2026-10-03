package driver

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ============================================================
// 构造 & 基本接口
// ============================================================

// TestNewMock_ReturnsNonNil 验证 NewMock 返回非 nil 实现，且接口驱动 driver 是 mock 类型。
func TestNewMock_ReturnsNonNil(t *testing.T) {
	d := NewMock()
	if d == nil {
		t.Fatal("NewMock returned nil")
	}
	if _, ok := d.(*mockDriver); !ok {
		t.Errorf("NewMock type = %T, want *mockDriver", d)
	}
}

// TestPing_ReturnsNil mock 无网络资源，Ping 永远成功。
func TestPing_ReturnsNil(t *testing.T) {
	d := NewMock()
	if err := d.Ping(context.Background()); err != nil {
		t.Errorf("Ping = %v, want nil", err)
	}
}

// ============================================================
// Publish / Subscribe 端到端
// ============================================================

// TestPublishSubscribe_BasicRoundTrip 验证 Publish 一条消息后 Subscribe 能收到。
//
// 流程：
//  1. Subscribe 注册 handler（goroutine 内跑，handler 收到 → 记到 received chan）；
//  2. Publish 一条 payload；
//  3. 等 handler 收到 + 检查 ID / Topic / Payload 完整。
func TestPublishSubscribe_BasicRoundTrip(t *testing.T) {
	d := NewMock()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	received := make(chan Message, 1)
	go func() {
		_ = d.Subscribe(ctx, "topic-a", "g1", func(_ context.Context, msg Message) error {
			received <- msg
			return nil
		})
	}()

	// 给 Subscribe 时间建好 (topic, group) → chan。
	time.Sleep(20 * time.Millisecond)

	if err := d.Publish(ctx, "topic-a", []byte("hello")); err != nil {
		t.Fatalf("Publish = %v", err)
	}

	select {
	case msg := <-received:
		if msg.Topic != "topic-a" {
			t.Errorf("msg.Topic = %q, want topic-a", msg.Topic)
		}
		if string(msg.Payload) != "hello" {
			t.Errorf("msg.Payload = %q, want hello", msg.Payload)
		}
		if msg.ID == "" {
			t.Errorf("msg.ID is empty")
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("did not receive published message in 500ms")
	}
}

// TestPublish_EmptyTopic_ErrInvalid 验证 Publish 空 topic → ErrInvalid。
func TestPublish_EmptyTopic_ErrInvalid(t *testing.T) {
	d := NewMock()
	if err := d.Publish(context.Background(), "", []byte("x")); !errors.Is(err, ErrInvalid) {
		t.Errorf("Publish empty topic = %v, want ErrInvalid", err)
	}
}

// TestSubscribe_EmptyArgs_ErrInvalid 验证 Subscribe 收到空 topic/group/handler 都返 ErrInvalid。
func TestSubscribe_EmptyArgs_ErrInvalid(t *testing.T) {
	d := NewMock()
	ctx := context.Background()

	if err := d.Subscribe(ctx, "", "g", func(_ context.Context, _ Message) error { return nil }); !errors.Is(err, ErrInvalid) {
		t.Errorf("Subscribe empty topic = %v, want ErrInvalid", err)
	}
	if err := d.Subscribe(ctx, "t", "", func(_ context.Context, _ Message) error { return nil }); !errors.Is(err, ErrInvalid) {
		t.Errorf("Subscribe empty group = %v, want ErrInvalid", err)
	}
	if err := d.Subscribe(ctx, "t", "g", nil); !errors.Is(err, ErrInvalid) {
		t.Errorf("Subscribe nil handler = %v, want ErrInvalid", err)
	}
}

// TestSubscribe_MultipleMessagesInOrder 验证 Subscribe 按 Publish 顺序处理多条消息。
//
// mock 用 chan 串行传递，handler 串行执行，所以顺序 = 入队顺序。
func TestSubscribe_MultipleMessagesInOrder(t *testing.T) {
	d := NewMock()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var received []string
	var mu sync.Mutex
	done := make(chan struct{})

	go func() {
		_ = d.Subscribe(ctx, "topic-multi", "g", func(_ context.Context, msg Message) error {
			mu.Lock()
			received = append(received, string(msg.Payload))
			n := len(received)
			mu.Unlock()
			if n == 3 {
				close(done)
			}
			return nil
		})
	}()

	time.Sleep(20 * time.Millisecond)

	for i := 1; i <= 3; i++ {
		if err := d.Publish(ctx, "topic-multi", []byte{byte('0' + i)}); err != nil {
			t.Fatalf("Publish %d = %v", i, err)
		}
	}

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("did not receive 3 messages in 500ms")
	}

	mu.Lock()
	defer mu.Unlock()
	want := []string{"1", "2", "3"}
	if len(received) != len(want) {
		t.Fatalf("received %d messages, want %d", len(received), len(want))
	}
	for i := range want {
		if received[i] != want[i] {
			t.Errorf("received[%d] = %q, want %q", i, received[i], want[i])
		}
	}
}

// TestSubscribe_HandlerError_NoRetry 验证 handler 返回 error 时，mock 不重试（按 mock 文档）。
//
// 这里"不重试"不是说消息丢，而是按 mock 语义说明，handler 失败的消息丢弃，
// 后续重试行为由集成测试用真实 Redis Stream 验证（spec）。
func TestSubscribe_HandlerError_NoRetry(t *testing.T) {
	d := NewMock()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var callCount atomic.Int32
	handlerErr := errors.New("handler boom")

	done := make(chan struct{})
	go func() {
		_ = d.Subscribe(ctx, "topic-err", "g", func(_ context.Context, _ Message) error {
			callCount.Add(1)
			return handlerErr
		})
		close(done) // Subscribe 立刻"挂住"在 select；handler 失败 → continue 循环
	}()

	time.Sleep(20 * time.Millisecond)

	// Publish 一条消息 → handler 应被调 1 次。
	if err := d.Publish(ctx, "topic-err", []byte("x")); err != nil {
		t.Fatalf("Publish = %v", err)
	}

	// 等一会确认 handler 不会被再次触发。
	time.Sleep(100 * time.Millisecond)

	if n := callCount.Load(); n != 1 {
		t.Errorf("handler call count = %d, want 1 (mock 不应重试)", n)
	}
}

// TestSubscribe_MultipleGroupsReceiveSameMessage 验证同一 topic 下多个 group 各自收到 Publish 的消息。
//
// 这是 Redis Stream 消费组隔离的语义镜像：每个 (topic, group) 独立 chan，
// Publish 复制消息到所有 group 的 chan。
func TestSubscribe_MultipleGroupsReceiveSameMessage(t *testing.T) {
	d := NewMock()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	receivedA := make(chan Message, 1)
	receivedB := make(chan Message, 1)

	go func() {
		_ = d.Subscribe(ctx, "topic-shared", "group-a", func(_ context.Context, msg Message) error {
			receivedA <- msg
			return nil
		})
	}()
	go func() {
		_ = d.Subscribe(ctx, "topic-shared", "group-b", func(_ context.Context, msg Message) error {
			receivedB <- msg
			return nil
		})
	}()

	time.Sleep(20 * time.Millisecond)

	if err := d.Publish(ctx, "topic-shared", []byte("shared")); err != nil {
		t.Fatalf("Publish = %v", err)
	}

	for i, c := range []chan Message{receivedA, receivedB} {
		select {
		case msg := <-c:
			if string(msg.Payload) != "shared" {
				t.Errorf("group %d payload = %q, want shared", i, msg.Payload)
			}
		case <-time.After(500 * time.Millisecond):
			t.Fatalf("group %d did not receive message", i)
		}
	}
}

// TestSubscribe_CtxCancel 验证 Subscribe 在 ctx cancel 时返回 ctx.Err()。
func TestSubscribe_CtxCancel(t *testing.T) {
	d := NewMock()
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		done <- d.Subscribe(ctx, "topic-cancel", "g", func(_ context.Context, _ Message) error {
			return nil
		})
	}()

	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("Subscribe return err = %v, want context.Canceled", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Subscribe did not return after ctx cancel")
	}
}
