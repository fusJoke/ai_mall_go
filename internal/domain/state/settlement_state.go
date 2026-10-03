package state

import "ai-go-mall/internal/model/mall"

// SettlementStates 是结算单所有可能状态的"常量"集合。
//
// 用法：state.Transition(state.SettlementStates.Pending, state.SettlementStates.Processing, ctx)。
//
// 4 个状态来源：mall.MallSettlementStatus 枚举值（model/mall/settlement.go）。
var SettlementStates = struct {
	// Pending 待生成（admin 触发 Generate 前；生成后即迁到 processing）。
	Pending SettlementState

	// Processing 已生成待打款（Generate 事务完成后状态；admin 在此态可 MarkPaid）。
	Processing SettlementState

	// Paid 已打款（终态 — MarkPaid 完成后）。
	Paid SettlementState

	// Failed 失败（终态 — 任意阶段兜底）。
	Failed SettlementState
}{
	Pending:    SettlementState{name: string(mall.SettlementStatusPending)},
	Processing: SettlementState{name: string(mall.SettlementStatusProcessing)},
	Paid:       SettlementState{name: string(mall.SettlementStatusPaid)},
	Failed:     SettlementState{name: string(mall.SettlementStatusFailed)},
}

// SettlementState 结算单的单一状态。
//
// 内部只有 name 字段，状态本身无状态（stateless）；所有实例按 name 区分。
type SettlementState struct {
	name string
}

// Name 返回状态字符串名（与 mall.MallSettlementStatus 一致）。
func (s SettlementState) Name() string { return s.name }

// CanTransitionTo 合法转换表（spec Requirement "Settlement state machine"）：
//
//	pending    → processing ✓
//	pending    → failed     ✓
//	processing → paid       ✓
//	processing → failed     ✓
//	paid       → ✗          （终态）
//	failed     → ✗          （终态）
//
// 其他一律拒绝：跨类型（SettlementState → DrawOrderState）、自迁（s → s）、
// 跳跃（pending → paid / pending → paid 直接跳过 processing）、逆向（paid → processing）。
//
// pending → paid 看似"MarkPaid 直接打到 paid"的捷径，但设计上禁止：settlement 必须
// 先经过 Generate 落到 processing 才能被打款。这条规则让"还没生成的结算单被打款"这种
// 异常路径在状态机层就 fail-fast。
func (s SettlementState) CanTransitionTo(next State) bool {
	ns, ok := next.(SettlementState)
	if !ok {
		// 跨类型：抽卡订单 ↔ 结算单不应混用同一状态机。
		return false
	}
	if s.name == ns.name {
		// 自迁：无意义且掩盖"忘记更新"类 bug，统一拒绝。
		return false
	}
	switch s.name {
	case string(mall.SettlementStatusPending):
		return ns.name == string(mall.SettlementStatusProcessing) ||
			ns.name == string(mall.SettlementStatusFailed)
	case string(mall.SettlementStatusProcessing):
		return ns.name == string(mall.SettlementStatusPaid) ||
			ns.name == string(mall.SettlementStatusFailed)
	}
	// paid / failed 都是终态；上面 switch 没命中即拒绝。
	return false
}

// OnEnter 默认无副作用（MVP）；如需 audit log / MQ，覆写此方法。
func (s SettlementState) OnEnter(_ *Context) error { return nil }
