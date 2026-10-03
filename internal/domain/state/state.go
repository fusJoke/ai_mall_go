// Package state 是订单 / 结算单的状态机抽象（spec Requirement "Order state machine"，design D19）。
//
// 设计动机（参考 订单状态机设计.md §四 并发保护）：
//
//	状态字段永远 WHERE 当前态=期望态 更新；禁止裸 UPDATE、禁止跨状态跳跃。
//
// 把状态机从"散落在 repo 的 CAS 条件 UPDATE" 抽到独立包：
//   - 业务侧只需 Transition(current, next, ctx) 一次校验；
//   - 非法跳跃在「写库前」就 fail-fast，错误类型单一（ErrInvalidStateTransition）；
//   - 终态、跨类型、自迁等异常一律拒绝，便于 handler 统一映射 HTTP 409。
//
// 不直接写 DB：状态机只校验合法性 + 跑 OnEnter 副作用，持久化仍由 service 层
// 在同一事务内调 repository.UpdateStatus 完成。状态机是"控制器"，不是"执行器"。
package state

import "errors"

// ErrInvalidStateTransition 非合法转换的统一哨兵。
//
// 触发场景：
//   - current / next 为 nil（防御编程错误）；
//   - 跨类型（DrawOrderState ↔ SettlementState）；
//   - 自迁（s → s，无意义且掩盖 bug）；
//   - 终态试图再次转换（drawn / failed / paid 等）；
//   - 跳跃（如 pending → drawn）；
//   - 逆向（drawn → paid，不在合法表内）。
//
// handler 层映射为 HTTP 409 Conflict（writeSettlementErr / 未来 draw handler 同模式）。
var ErrInvalidStateTransition = errors.New("state: invalid transition")

// State 状态机抽象接口。
//
// 实现类型：
//   - DrawOrderState（internal/domain/state/draw_order_state.go）；
//   - SettlementState（internal/domain/state/settlement_state.go）。
//
// 接口三个方法的契约：
//   - Name 返回当前状态的字符串名（与 DB 列值一致，如 "pending"），便于日志 / 调试；
//   - CanTransitionTo 决定"当前态 → next"是否在合法转换表内；
//   - OnEnter 副作用钩子（MVP 默认 no-op；如未来接 audit log / MQ，覆写此方法）。
type State interface {
	// Name 返回状态字符串名。
	Name() string

	// CanTransitionTo 校验 next 是否是 current 的合法下一态。
	//
	// 实现要点：
	//   - 类型不匹配（next 不是同一种 State）→ return false；
	//   - 终态（DrawnState / PaidState / FailedState）一律 return false；
	//   - 自迁（s → s）→ return false（避免无意义的 "transition"）；
	//   - 跳跃 / 逆向 → return false。
	CanTransitionTo(next State) bool

	// OnEnter 进入该状态时的副作用（写 audit log / 发 MQ 等）。
	//
	// MVP 默认实现：return nil（no-op）。副作用失败 → Transition 返回 err，调用方应处理。
	OnEnter(ctx *Context) error
}

// Context 状态机运行时上下文。
//
// MVP 仅携带 Entity（当前业务实体，便于 OnEnter 副作用读取）；后续接 MQ / 审计
// 日志时扩展字段（如 *gin.Context、logger、publisher）即可。
//
// Entity 形态：
//   - 抽卡订单：*mall.MallDrawOrder；
//   - 结算单：*mall.MallSettlement。
//
// OnEnter 内部按需 type-assert 后读字段；ctx 本身只做"载体"，不做类型约束。
type Context struct {
	// Entity 当前业务实体（*mall.MallDrawOrder / *mall.MallSettlement）。
	Entity any
}

// Transition 把 entity 从 current 推到 next，跑合法校验 + 副作用。
//
// 步骤：
//  1. 校验 current / next 非 nil；
//  2. 调 current.CanTransitionTo(next)，失败 → ErrInvalidStateTransition；
//  3. 调 next.OnEnter(ctx)，副作用失败 → 原样上抛；
//  4. 返回 next，由调用方负责持久化（repository.UpdateStatus 同一事务内执行）。
//
// 设计要点：状态机不直接写库。
//   - 抽卡事务内的 pending → paid → drawn 转换，service 层拿到返回值后立刻
//     调 repo.UpdateStatus，让 UPDATE 与外层事务原子提交 / 回滚；
//   - 状态机的"副作用"是审计 / MQ 这类"写库外的动作"，不是 DB 写。
func Transition(current, next State, ctx *Context) (State, error) {
	if current == nil || next == nil {
		return nil, ErrInvalidStateTransition
	}
	if !current.CanTransitionTo(next) {
		return nil, ErrInvalidStateTransition
	}
	if ctx == nil {
		ctx = &Context{}
	}
	if err := next.OnEnter(ctx); err != nil {
		return nil, err
	}
	return next, nil
}
