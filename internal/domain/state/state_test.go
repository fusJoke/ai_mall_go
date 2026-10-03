package state

import (
	"errors"
	"testing"

	"ai-go-mall/internal/model/mall"
)

// =============================================================================
// helpers
// =============================================================================

// fakeState 用于测试 OnEnter 副作用路径（具体实现类型不参与 CanTransitionTo 校验）。
//
// fakeState 不在 DrawOrderStates / SettlementStates 集合里，仅出现在 Transition 副作用测试。
type fakeState struct {
	name    string
	onEnter func(*Context) error
}

func (f fakeState) Name() string { return f.name }

func (f fakeState) CanTransitionTo(next State) bool {
	_, ok := next.(fakeState)
	return ok
}

func (f fakeState) OnEnter(ctx *Context) error {
	if f.onEnter != nil {
		return f.onEnter(ctx)
	}
	return nil
}

var _ State = fakeState{}

// =============================================================================
// DrawOrderState — valid transitions
// =============================================================================

func TestDrawOrderState_ValidTransitions(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		from    DrawOrderState
		to      DrawOrderState
		want    bool
		wantTo  string
	}{
		{"pending->paid", DrawOrderStates.Pending, DrawOrderStates.Paid, true, "paid"},
		{"pending->failed", DrawOrderStates.Pending, DrawOrderStates.Failed, true, "failed"},
		{"paid->drawn", DrawOrderStates.Paid, DrawOrderStates.Drawn, true, "drawn"},
		{"paid->failed", DrawOrderStates.Paid, DrawOrderStates.Failed, true, "failed"},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if got := c.from.CanTransitionTo(c.to); got != c.want {
				t.Fatalf("CanTransitionTo(%s -> %s) = %v, want %v", c.from.Name(), c.to.Name(), got, c.want)
			}
			// 同时跑 Transition helper，确保 OnEnter 路径也被覆盖。
			got, err := Transition(c.from, c.to, &Context{})
			if err != nil {
				t.Fatalf("Transition(%s -> %s) unexpected err: %v", c.from.Name(), c.to.Name(), err)
			}
			if got.Name() != c.wantTo {
				t.Fatalf("Transition returned %s, want %s", got.Name(), c.wantTo)
			}
		})
	}
}

// =============================================================================
// DrawOrderState — invalid transitions
// =============================================================================

func TestDrawOrderState_InvalidTransitions(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		from DrawOrderState
		to   DrawOrderState
	}{
		{"pending->drawn（跳跃）", DrawOrderStates.Pending, DrawOrderStates.Drawn},
		{"pending->pending（自迁）", DrawOrderStates.Pending, DrawOrderStates.Pending},
		{"paid->paid（自迁）", DrawOrderStates.Paid, DrawOrderStates.Paid},
		{"paid->pending（逆向）", DrawOrderStates.Paid, DrawOrderStates.Pending},
		{"drawn->pending（终态）", DrawOrderStates.Drawn, DrawOrderStates.Pending},
		{"drawn->paid（终态）", DrawOrderStates.Drawn, DrawOrderStates.Paid},
		{"drawn->failed（终态）", DrawOrderStates.Drawn, DrawOrderStates.Failed},
		{"drawn->drawn（自迁）", DrawOrderStates.Drawn, DrawOrderStates.Drawn},
		{"failed->pending（终态）", DrawOrderStates.Failed, DrawOrderStates.Pending},
		{"failed->paid（终态）", DrawOrderStates.Failed, DrawOrderStates.Paid},
		{"failed->drawn（终态）", DrawOrderStates.Failed, DrawOrderStates.Drawn},
		{"failed->failed（自迁）", DrawOrderStates.Failed, DrawOrderStates.Failed},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if got := c.from.CanTransitionTo(c.to); got {
				t.Fatalf("CanTransitionTo(%s -> %s) = true, want false", c.from.Name(), c.to.Name())
			}
			_, err := Transition(c.from, c.to, &Context{})
			if !errors.Is(err, ErrInvalidStateTransition) {
				t.Fatalf("Transition(%s -> %s) err = %v, want ErrInvalidStateTransition", c.from.Name(), c.to.Name(), err)
			}
		})
	}
}

// TestDrawOrderState_CrossTypeRejected 跨类型转换拒绝（DrawOrderState → SettlementState）。
func TestDrawOrderState_CrossTypeRejected(t *testing.T) {
	t.Parallel()

	if DrawOrderStates.Pending.CanTransitionTo(SettlementStates.Pending) {
		t.Fatal("DrawOrderState.Pending → SettlementState.Pending should be rejected")
	}
	if DrawOrderStates.Paid.CanTransitionTo(SettlementStates.Processing) {
		t.Fatal("DrawOrderState.Paid → SettlementState.Processing should be rejected")
	}
	if _, err := Transition(DrawOrderStates.Pending, SettlementStates.Pending, &Context{}); !errors.Is(err, ErrInvalidStateTransition) {
		t.Fatalf("Transition(DrawOrder.Pending, Settlement.Pending) err = %v, want ErrInvalidStateTransition", err)
	}
}

// =============================================================================
// SettlementState — valid transitions
// =============================================================================

func TestSettlementState_ValidTransitions(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		from SettlementState
		to   SettlementState
	}{
		{"pending->processing", SettlementStates.Pending, SettlementStates.Processing},
		{"pending->failed", SettlementStates.Pending, SettlementStates.Failed},
		{"processing->paid", SettlementStates.Processing, SettlementStates.Paid},
		{"processing->failed", SettlementStates.Processing, SettlementStates.Failed},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if got := c.from.CanTransitionTo(c.to); !got {
				t.Fatalf("CanTransitionTo(%s -> %s) = false, want true", c.from.Name(), c.to.Name())
			}
			got, err := Transition(c.from, c.to, &Context{})
			if err != nil {
				t.Fatalf("Transition(%s -> %s) unexpected err: %v", c.from.Name(), c.to.Name(), err)
			}
			if got.Name() != c.to.Name() {
				t.Fatalf("Transition returned %s, want %s", got.Name(), c.to.Name())
			}
		})
	}
}

// =============================================================================
// SettlementState — invalid transitions
// =============================================================================

func TestSettlementState_InvalidTransitions(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		from SettlementState
		to   SettlementState
	}{
		// pending 不允许直接 → paid（必须先经 processing，规则写在设计 doc §6）。
		{"pending->paid（跳跃）", SettlementStates.Pending, SettlementStates.Paid},
		{"pending->pending（自迁）", SettlementStates.Pending, SettlementStates.Pending},
		{"processing->pending（逆向）", SettlementStates.Processing, SettlementStates.Pending},
		{"processing->processing（自迁）", SettlementStates.Processing, SettlementStates.Processing},
		{"paid->pending（终态）", SettlementStates.Paid, SettlementStates.Pending},
		{"paid->processing（终态）", SettlementStates.Paid, SettlementStates.Processing},
		{"paid->failed（终态）", SettlementStates.Paid, SettlementStates.Failed},
		{"paid->paid（自迁）", SettlementStates.Paid, SettlementStates.Paid},
		{"failed->pending（终态）", SettlementStates.Failed, SettlementStates.Pending},
		{"failed->processing（终态）", SettlementStates.Failed, SettlementStates.Processing},
		{"failed->paid（终态）", SettlementStates.Failed, SettlementStates.Paid},
		{"failed->failed（自迁）", SettlementStates.Failed, SettlementStates.Failed},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if got := c.from.CanTransitionTo(c.to); got {
				t.Fatalf("CanTransitionTo(%s -> %s) = true, want false", c.from.Name(), c.to.Name())
			}
			_, err := Transition(c.from, c.to, &Context{})
			if !errors.Is(err, ErrInvalidStateTransition) {
				t.Fatalf("Transition(%s -> %s) err = %v, want ErrInvalidStateTransition", c.from.Name(), c.to.Name(), err)
			}
		})
	}
}

// TestSettlementState_CrossTypeRejected 跨类型拒绝（SettlementState → DrawOrderState）。
func TestSettlementState_CrossTypeRejected(t *testing.T) {
	t.Parallel()

	if SettlementStates.Pending.CanTransitionTo(DrawOrderStates.Pending) {
		t.Fatal("SettlementState.Pending → DrawOrderState.Pending should be rejected")
	}
	if SettlementStates.Processing.CanTransitionTo(DrawOrderStates.Drawn) {
		t.Fatal("SettlementState.Processing → DrawOrderState.Drawn should be rejected")
	}
	if _, err := Transition(SettlementStates.Pending, DrawOrderStates.Paid, &Context{}); !errors.Is(err, ErrInvalidStateTransition) {
		t.Fatalf("Transition(Settlement.Pending, DrawOrder.Paid) err = %v, want ErrInvalidStateTransition", err)
	}
}

// =============================================================================
// Transition helper — nil / error 路径
// =============================================================================

func TestTransition_NilArgs(t *testing.T) {
	t.Parallel()

	if _, err := Transition(nil, DrawOrderStates.Paid, nil); !errors.Is(err, ErrInvalidStateTransition) {
		t.Fatalf("Transition(nil, Paid) err = %v, want ErrInvalidStateTransition", err)
	}
	if _, err := Transition(DrawOrderStates.Pending, nil, nil); !errors.Is(err, ErrInvalidStateTransition) {
		t.Fatalf("Transition(Pending, nil) err = %v, want ErrInvalidStateTransition", err)
	}
	if _, err := Transition(nil, nil, nil); !errors.Is(err, ErrInvalidStateTransition) {
		t.Fatalf("Transition(nil, nil) err = %v, want ErrInvalidStateTransition", err)
	}
}

// TestTransition_OnEnterPropagates 验证 OnEnter 失败时 Transition 原样返回 err。
//
// 用 fakeState 模拟一个 OnEnter 报错的副作用，确认错误不会被状态机吞掉。
func TestTransition_OnEnterPropagates(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("audit log failed")
	src := fakeState{name: "src"}
	dst := fakeState{
		name: "dst",
		onEnter: func(_ *Context) error {
			return sentinel
		},
	}
	got, err := Transition(src, dst, &Context{Entity: "any"})
	if !errors.Is(err, sentinel) {
		t.Fatalf("Transition err = %v, want sentinel %v", err, sentinel)
	}
	if got != nil {
		t.Fatalf("Transition returned %v on error, want nil", got)
	}
}

// TestTransition_NilContextAllowed 验证 ctx == nil 时仍能跑合法转换（自动用空 Context）。
func TestTransition_NilContextAllowed(t *testing.T) {
	t.Parallel()

	got, err := Transition(DrawOrderStates.Pending, DrawOrderStates.Paid, nil)
	if err != nil {
		t.Fatalf("Transition(Pending, Paid, nil) unexpected err: %v", err)
	}
	if got.Name() != string(mall.OrderStatusPaid) {
		t.Fatalf("Transition returned %s, want paid", got.Name())
	}
}

// TestName_Consistency 验证 Name() 返回的字符串与 mall 包枚举值一字不差。
//
// 这是契约：DB 列值与状态机状态必须一一对应，否则 WHERE status=? 会出现"幽灵状态"。
func TestName_Consistency(t *testing.T) {
	t.Parallel()

	cases := []struct {
		state State
		want  string
	}{
		{DrawOrderStates.Pending, string(mall.OrderStatusPending)},
		{DrawOrderStates.Paid, string(mall.OrderStatusPaid)},
		{DrawOrderStates.Drawn, string(mall.OrderStatusDrawn)},
		{DrawOrderStates.Failed, string(mall.OrderStatusFailed)},
		{SettlementStates.Pending, string(mall.SettlementStatusPending)},
		{SettlementStates.Processing, string(mall.SettlementStatusProcessing)},
		{SettlementStates.Paid, string(mall.SettlementStatusPaid)},
		{SettlementStates.Failed, string(mall.SettlementStatusFailed)},
	}
	for _, c := range cases {
		c := c
		t.Run(c.want, func(t *testing.T) {
			t.Parallel()
			if got := c.state.Name(); got != c.want {
				t.Fatalf("Name() = %q, want %q", got, c.want)
			}
		})
	}
}
