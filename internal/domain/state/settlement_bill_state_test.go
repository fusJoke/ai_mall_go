package state

import (
	"testing"

	"ai-go-mall/internal/model/mall"
)

// =============================================================================
// BillState — valid transitions（合法路径全覆盖）
// =============================================================================

func TestBillState_ValidTransitions(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		from BillState
		to   BillState
	}{
		{"generated->confirmed", BillStates.Generated, BillStates.Confirmed},
		{"generated->rejected", BillStates.Generated, BillStates.Rejected},
		{"rejected->generated", BillStates.Rejected, BillStates.Generated},
		{"confirmed->paid", BillStates.Confirmed, BillStates.Paid},
		{"confirmed->failed", BillStates.Confirmed, BillStates.Failed},
		{"failed->confirmed", BillStates.Failed, BillStates.Confirmed},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if !c.from.CanTransitionTo(c.to) {
				t.Fatalf("%s -> %s should be legal", c.from.Name(), c.to.Name())
			}
		})
	}
}

// TestBillState_IllegalTransitions 自迁 / 跳跃 / 逆向 / 终态全拒绝。
func TestBillState_IllegalTransitions(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		from BillState
		to   BillState
	}{
		// 自迁：每个状态到自己。
		{"self generated", BillStates.Generated, BillStates.Generated},
		{"self confirmed", BillStates.Confirmed, BillStates.Confirmed},
		{"self rejected", BillStates.Rejected, BillStates.Rejected},
		{"self failed", BillStates.Failed, BillStates.Failed},
		{"self paid", BillStates.Paid, BillStates.Paid},

		// 跳跃：generated 必须先经 confirmed 才能到 paid（未确认就打款 fail-fast）。
		{"generated->paid skip confirm", BillStates.Generated, BillStates.Paid},
		{"generated->failed skip confirm", BillStates.Generated, BillStates.Failed},
		{"rejected->confirmed skip regen", BillStates.Rejected, BillStates.Confirmed},
		{"rejected->paid", BillStates.Rejected, BillStates.Paid},

		// 逆向：paid 终态；confirmed 不能回 generated（重核走 rejected 中转）。
		{"paid->confirmed", BillStates.Paid, BillStates.Confirmed},
		{"paid->generated", BillStates.Paid, BillStates.Generated},
		{"confirmed->generated", BillStates.Confirmed, BillStates.Generated},
		{"failed->generated", BillStates.Failed, BillStates.Generated},
		{"failed->rejected", BillStates.Failed, BillStates.Rejected},
		{"rejected->failed", BillStates.Rejected, BillStates.Failed},

		// 终态：paid 不可再转换（由上面逆向用例覆盖，此处自迁已断）。
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if c.from.CanTransitionTo(c.to) {
				t.Fatalf("%s -> %s should be illegal", c.from.Name(), c.to.Name())
			}
		})
	}
}

// TestBillState_CrossTypeRejected 跨类型状态机互不兼容：
// 周期结算单状态 / 抽卡订单状态都不能喂给 BillState。
func TestBillState_CrossTypeRejected(t *testing.T) {
	t.Parallel()

	if BillStates.Generated.CanTransitionTo(SettlementStates.Pending) {
		t.Fatal("BillState -> SettlementState should be illegal (cross type)")
	}
	if SettlementStates.Pending.CanTransitionTo(BillStates.Generated) {
		t.Fatal("SettlementState -> BillState should be illegal (cross type)")
	}
}

// TestBillState_TransitionEndToEnd 走 state.Transition 全链路：
// 合法迁移返回下一态；非法迁移返回 ErrInvalidStateTransition。
func TestBillState_TransitionEndToEnd(t *testing.T) {
	t.Parallel()

	next, err := Transition(BillStates.Generated, BillStates.Confirmed, &Context{})
	if err != nil {
		t.Fatalf("generated -> confirmed: unexpected err: %v", err)
	}
	if next.Name() != string(mall.BillStatusConfirmed) {
		t.Fatalf("next.Name() = %q, want %q", next.Name(), mall.BillStatusConfirmed)
	}

	if _, err := Transition(BillStates.Generated, BillStates.Paid, &Context{}); err == nil {
		t.Fatal("generated -> paid should return ErrInvalidStateTransition")
	}
}
