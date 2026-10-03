package state

import "ai-go-mall/internal/model/mall"

// DrawOrderStates 是抽卡订单所有可能状态的"常量"集合。
//
// 用法：state.Transition(state.DrawOrderStates.Pending, state.DrawOrderStates.Paid, ctx)。
//
// 4 个状态来源：mall.MallDrawOrderStatus 枚举值（model/mall/draw_order.go）。
var DrawOrderStates = struct {
	// Pending 待支付（订单创建后未抽卡）。
	Pending DrawOrderState

	// Paid 已支付（余额已扣、抽卡前）。
	Paid DrawOrderState

	// Drawn 已抽中（终态 — 抽卡事务完成）。
	Drawn DrawOrderState

	// Failed 失败（终态 — 任意阶段兜底）。
	Failed DrawOrderState
}{
	Pending: DrawOrderState{name: string(mall.OrderStatusPending)},
	Paid:    DrawOrderState{name: string(mall.OrderStatusPaid)},
	Drawn:   DrawOrderState{name: string(mall.OrderStatusDrawn)},
	Failed:  DrawOrderState{name: string(mall.OrderStatusFailed)},
}

// DrawOrderState 抽卡订单的单一状态。
//
// 内部只有 name 字段，状态本身无状态（stateless）；所有实例按 name 区分。
type DrawOrderState struct {
	name string
}

// Name 返回状态字符串名（与 mall.MallDrawOrderStatus 一致）。
func (s DrawOrderState) Name() string { return s.name }

// CanTransitionTo 合法转换表（spec Requirement "Draw order state machine"）：
//
//	pending → paid    ✓
//	pending → failed  ✓
//	paid    → drawn   ✓
//	paid    → failed  ✓
//	drawn   → ✗       （终态）
//	failed  → ✗       （终态）
//
// 其他一律拒绝：跨类型（DrawOrderState → 非 DrawOrderState）、自迁（s → s）、
// 跳跃（pending → drawn）、逆向（drawn → paid）。
func (s DrawOrderState) CanTransitionTo(next State) bool {
	ns, ok := next.(DrawOrderState)
	if !ok {
		// 跨类型：抽卡订单 ↔ 结算单不应混用同一状态机。
		return false
	}
	if s.name == ns.name {
		// 自迁：无意义且掩盖"忘记更新"类 bug，统一拒绝。
		return false
	}
	switch s.name {
	case string(mall.OrderStatusPending):
		return ns.name == string(mall.OrderStatusPaid) ||
			ns.name == string(mall.OrderStatusFailed)
	case string(mall.OrderStatusPaid):
		return ns.name == string(mall.OrderStatusDrawn) ||
			ns.name == string(mall.OrderStatusFailed)
	}
	// drawn / failed 都是终态；上面 switch 没命中即拒绝。
	return false
}

// OnEnter 默认无副作用（MVP）；如需 audit log / MQ，覆写此方法。
func (s DrawOrderState) OnEnter(_ *Context) error { return nil }
