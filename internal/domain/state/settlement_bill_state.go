package state

import "ai-go-mall/internal/model/mall"

// BillStates 是批次账单所有可能状态的"常量"集合（openspec: add-batch-settlement）。
//
// 用法：state.Transition(state.BillStates.Generated, state.BillStates.Confirmed, ctx)。
//
// 5 个状态来源：mall.MallSettlementBillStatus 枚举值（model/mall/settlement_bill.go）。
var BillStates = struct {
	// Generated 售罄自动生成·待供应商确认（GenerateForBox 事务完成后状态）。
	Generated BillState

	// Confirmed 供应商已确认·待打款（admin 在此态可 MarkPaid）。
	Confirmed BillState

	// Rejected 供应商驳回（附原因；admin Regenerate 后回 generated）。
	Rejected BillState

	// Failed 打款失败（MarkPaid 资金错误兜底；修复后重试回 confirmed）。
	Failed BillState

	// Paid 已打款（终态 — MarkPaid 完成；与商品 lifecycle=settled 同事务）。
	Paid BillState
}{
	Generated: BillState{name: string(mall.BillStatusGenerated)},
	Confirmed: BillState{name: string(mall.BillStatusConfirmed)},
	Rejected:  BillState{name: string(mall.BillStatusRejected)},
	Failed:    BillState{name: string(mall.BillStatusFailed)},
	Paid:      BillState{name: string(mall.BillStatusPaid)},
}

// BillState 批次账单的单一状态（stateless，按 name 区分）。
type BillState struct {
	name string
}

// Name 返回状态字符串名（与 mall.MallSettlementBillStatus 一致）。
func (s BillState) Name() string { return s.name }

// CanTransitionTo 合法转换表（openspec: add-batch-settlement 状态机）：
//
//	generated → confirmed ✓（供应商确认）
//	generated → rejected  ✓（供应商驳回，必填原因）
//	rejected  → generated ✓（admin 重新核算，version+1）
//	confirmed → paid      ✓（admin MarkPaid）
//	confirmed → failed    ✓（打款失败兜底）
//	failed    → confirmed ✓（修复重试）
//	paid      → ✗         （终态）
//
// 其他一律拒绝：跨类型（SettlementState/DrawOrderState ↔ BillState）、自迁、
// 跳跃（generated → paid 直接跳过 confirmed —— 账单必须经供应商确认才可打款，
// 该规则让"未确认就打款"的异常路径在状态机层 fail-fast）、逆向（paid → confirmed）。
func (s BillState) CanTransitionTo(next State) bool {
	ns, ok := next.(BillState)
	if !ok {
		// 跨类型：周期结算单/抽卡订单 ↔ 批次账单不应混用同一状态机。
		return false
	}
	if s.name == ns.name {
		// 自迁：无意义且掩盖"忘记更新"类 bug，统一拒绝。
		return false
	}
	switch s.name {
	case string(mall.BillStatusGenerated):
		return ns.name == string(mall.BillStatusConfirmed) ||
			ns.name == string(mall.BillStatusRejected)
	case string(mall.BillStatusRejected):
		return ns.name == string(mall.BillStatusGenerated)
	case string(mall.BillStatusConfirmed):
		return ns.name == string(mall.BillStatusPaid) ||
			ns.name == string(mall.BillStatusFailed)
	case string(mall.BillStatusFailed):
		return ns.name == string(mall.BillStatusConfirmed)
	}
	// paid 是终态；上面 switch 没命中即拒绝。
	return false
}

// OnEnter 默认无副作用（MVP）；如需 audit log / MQ 通知供应商，覆写此方法。
func (s BillState) OnEnter(_ *Context) error { return nil }
