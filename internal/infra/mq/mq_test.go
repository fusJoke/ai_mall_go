package mq

import (
	"context"
	"errors"
	"testing"

	"ai-go-mall/internal/infra/config"
	"ai-go-mall/internal/infra/mq/driver"
)

// ============================================================
// sentinel errors / re-export
// ============================================================

// TestSentinels_NonNilAndDistinct 验证三个 sentinel 均非空且互异。
//
// ErrPublishFailed / ErrSubscribeClosed 是 re-export（必须 === driver.*）；
// ErrInvalid 是 mq 包本地定义（driver 模块未配，spec 兼容层）。
func TestSentinels_NonNilAndDistinct(t *testing.T) {
	// Re-export 部分必须指向 driver 同源。
	if ErrPublishFailed != driver.ErrPublishFailed {
		t.Errorf("ErrPublishFailed re-export broken: %v vs %v", ErrPublishFailed, driver.ErrPublishFailed)
	}
	if ErrSubscribeClosed != driver.ErrSubscribeClosed {
		t.Errorf("ErrSubscribeClosed re-export broken: %v vs %v", ErrSubscribeClosed, driver.ErrSubscribeClosed)
	}
	if ErrInvalid == nil {
		t.Error("ErrInvalid must be non-nil")
	}

	// 三个 sentinel 字符串互异。
	seen := map[string]bool{
		ErrPublishFailed.Error():   true,
		ErrSubscribeClosed.Error(): true,
		ErrInvalid.Error():         true,
	}
	if len(seen) != 3 {
		t.Errorf("sentinels collide: %+v", seen)
	}
}

// TestMessage_TypeAlias 验证 Message 与 driver.Message 是同一类型（type alias 契约）。
//
// 代码注释说"alerting on alias breaking"：若 driver.Message 字段被改，
// 这里断言会失败，提醒 review 时同步两边。
func TestMessage_TypeAlias(t *testing.T) {
	var m Message
	var dm driver.Message
	// 编译期类型相同 → 赋值兼容。
	m = dm
	dm = m
	// 字段集相同（透传）。
	if m.ID != dm.ID || m.Topic != dm.Topic || string(m.Payload) != string(dm.Payload) {
		t.Errorf("Message and driver.Message diverge: %+v vs %+v", m, dm)
	}
}

// ============================================================
// Manager wrapper
// ============================================================

// TestGet_NilBeforeInit 验证未 Init 时 Get() 返回 nil。
func TestGet_NilBeforeInit(t *testing.T) {
	Reset() // 兜底
	if g := Get(); g != nil {
		t.Errorf("Get() before Init = %v, want nil", g)
	}
}

// TestReset_ClearsMgr 验证 Reset 后 Get 返回 nil（幂等）。
func TestReset_ClearsMgr(t *testing.T) {
	mgr = &Manager{}
	Reset()
	if Get() != nil {
		t.Errorf("Get() after Reset = %v, want nil", Get())
	}
}

// TestInit_UnknownDriver 验证未知 driver 名 → Init 返回 error，不 panic。
//
// 通过注入 config（unknown driver）+ Reset+Init 走完整路径。
func TestInit_UnknownDriver(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Init panicked on unknown driver: %v", r)
		}
	}()

	Reset()
	config.SetForTest(&config.Config{
		MQ: config.MQConfig{Driver: "no-such-mq-driver"},
	})
	defer config.Reset()

	if err := Init(); err == nil {
		t.Errorf("Init with unknown driver = nil err, want error")
	}
}

// TestManager_EmptyTopicRejected 验证 Manager.Publish / Subscribe 收到空 topic
// 直接返回 ErrInvalid，不调用底层 driver（参数校验在 driver 之前）。
//
// 用空 Manager（driver=nil）跑：若校验放在 driver 之后会 nil pointer panic。
func TestManager_EmptyTopicRejected(t *testing.T) {
	m := &Manager{} // driver=nil
	ctx := context.Background()

	if err := m.Publish(ctx, "", []byte("x")); !errors.Is(err, ErrInvalid) {
		t.Errorf("Publish empty topic = %v, want ErrInvalid", err)
	}
	if err := m.Subscribe(ctx, "", "g", func(_ context.Context, _ Message) error { return nil }); !errors.Is(err, ErrInvalid) {
		t.Errorf("Subscribe empty topic = %v, want ErrInvalid", err)
	}
}

// TestManager_Subscribe_NilHandlerRejected 验证 Subscribe 收到 nil handler → ErrInvalid。
func TestManager_Subscribe_NilHandlerRejected(t *testing.T) {
	m := &Manager{}
	if err := m.Subscribe(context.Background(), "t", "g", nil); !errors.Is(err, ErrInvalid) {
		t.Errorf("Subscribe nil handler = %v, want ErrInvalid", err)
	}
}

// TestManager_Subscribe_EmptyGroupRejected 验证 Subscribe 收到空 group → ErrInvalid。
func TestManager_Subscribe_EmptyGroupRejected(t *testing.T) {
	m := &Manager{}
	if err := m.Subscribe(context.Background(), "t", "", func(_ context.Context, _ Message) error { return nil }); !errors.Is(err, ErrInvalid) {
		t.Errorf("Subscribe empty group = %v, want ErrInvalid", err)
	}
}

// ============================================================
// Init mock driver（lifecycle test）
// ============================================================

// TestInit_MockDriver 验证 Init 装配 mock（driver=mock）成功，Get() 返回非空 Manager。
//
// 这是 mock driver 集成测试：用真实 Init + Reset 路径，避免单测绕开 singleton。
func TestInit_MockDriver(t *testing.T) {
	Reset()
	config.SetForTest(&config.Config{
		MQ: config.MQConfig{Driver: "mock"},
	})
	defer config.Reset()

	if err := Init(); err != nil {
		t.Fatalf("Init mock = %v", err)
	}
	if Get() == nil {
		t.Errorf("Get() after Init = nil, want non-nil")
	}
	// 二次 Init 是 no-op（spec）。
	if err := Init(); err != nil {
		t.Errorf("second Init = %v, want nil", err)
	}

	Reset()
}
