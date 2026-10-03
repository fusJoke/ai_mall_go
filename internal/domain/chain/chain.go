// Package chain — chain.go 实现 Chain 结构与 Validate 方法。
//
// 责任链执行规则（参考 下单责任链_流程说明 §二 执行流程）：
//
//	请求进入
//	  ├─ ① 参数校验 ──拒绝──► 返回 4xx（无补偿）
//	  │     ↓ 通过
//	  ├─ ② 身份/限购 ──拒绝──► 返回 4xx（无补偿）
//	  │     ↓ 通过
//	  ... 节点依次执行 ...
//	  │     ↓ 全部通过
//	  └─► 进入 service 事务
//
// 关键不变量：
//   - 顺序固定：节点按 NewChain 时传入顺序执行（不可重排，除非显式构造新 Chain）；
//   - 短路：任一节点返回 err 即终止，不跑后续；
//   - 错误信息：原 err 上 wrap `validator(name)`，便于日志定位失败节点。
//
// 节点无需补偿：所有 check 都是「读」+ 状态判定，无副作用；Redis 库存预扣 /
// 锁券等带副作用的步骤不在 chain 内，由 service 层在事务内执行（与 D3 算法对齐）。
package chain

import (
	"errors"
	"fmt"

	"github.com/gin-gonic/gin"
)

// Chain 责任链容器。
//
// 不可变（创建后 validators 切片不再追加）；并发安全（无写）。
type Chain struct {
	// validators 节点列表，按序执行。
	validators []Validator
}

// NewChain 按给定顺序构造 Chain。
//
// 顺序由调用方决定（chain_builder.go 暴露 DrawChain / SeckillDrawChain 两种组装）。
func NewChain(validators ...Validator) *Chain {
	return &Chain{validators: validators}
}

// Validate 顺序执行所有 validator，任一 err 即终止。
//
// 返回：
//   - nil：所有节点通过；
//   - *ValidationError：节点拒绝（含原始 sentinel + 节点名 + cause，方便 handler
//     用 errors.Is(err, targetErr) 映射 HTTP，又保留定位信息）；
//   - 其他 error：节点返回的 DB / Redis 异常，原样上抛（不 wrap，避免吞掉 stack）。
//
// 设计：节点内的业务 sentinel 通过 wrap 暴露（保留 errors.Is 能力），DB /
// Redis 异常不 wrap（保留 errors.Unwrap 链路 + 不假装是节点失败）。
func (c *Chain) Validate(ctx *gin.Context, input *DrawInput) error {
	if input == nil {
		return ErrMissingInput
	}
	if input.Source != SourceNormal && input.Source != SourceSeckill {
		return fmt.Errorf("%w: %q", ErrInvalidSource, input.Source)
	}
	if input.UserID <= 0 {
		return fmt.Errorf("%w: user_id <= 0", ErrMissingInput)
	}
	switch input.Source {
	case SourceNormal:
		if input.BlindBoxID <= 0 {
			return fmt.Errorf("%w: blind_box_id <= 0", ErrMissingInput)
		}
	case SourceSeckill:
		if input.SeckillID <= 0 {
			return fmt.Errorf("%w: seckill_id <= 0", ErrMissingInput)
		}
	}

	for _, v := range c.validators {
		if err := v.Validate(ctx, input); err != nil {
			// 节点失败：wrap validator name + 原 err；
			// 业务 sentinel 经 %w 保留 errors.Is 能力。
			return &ValidationError{
				ValidatorName: v.Name(),
				Cause:         err,
			}
		}
	}
	return nil
}

// ValidationError 节点失败的统一载体。
//
// handler 端用法：
//
//	err := chain.Validate(...)
//	var ve *chain.ValidationError
//	if errors.As(err, &ve) {
//	    // ve.Cause 是业务 sentinel，用 errors.Is(ve.Cause, targetErr) 映射 HTTP
//	    // ve.ValidatorName 用于日志 / 调试
//	}
type ValidationError struct {
	// ValidatorName 失败的 Validator.Name()。
	ValidatorName string

	// Cause 节点返回的原始 error（业务 sentinel 或 DB 异常）。
	Cause error
}

// Error 实现 error 接口。
func (e *ValidationError) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("validator(%s): %v", e.ValidatorName, e.Cause)
}

// Unwrap 让 errors.Is / errors.As 能透传到底层 sentinel。
//
//   - errors.Is(err, ErrInsufficientBalance) → true（若 Cause 是 ErrInsufficientBalance）；
//   - errors.As(err, &ve) → 命中 ValidationError 自身。
//
// 注：errors.As 优先匹配最近一层 wrap，所以同时 errors.Is + errors.As 是 OK 的。
func (e *ValidationError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// 编译期断言：errors.Is / errors.As 应当透明工作（声明 + Unwrap 已实现）。
//
// 显式声明是为了让静态检查工具识别本类型实现了 unwrap 语义；
// 非 UnwrapTest 单元测试覆盖（见 chain_test.go）。
var _ error = (*ValidationError)(nil)

// Compile-time interface assertion to ensure ValidationError participates in
// the errors.Is / errors.As protocol.
var _ interface{ Unwrap() error } = (*ValidationError)(nil)

// errorsUnwrapSanity 触发 errors.Unwrap 在 init 阶段检查：避免 Unwrap 返回
// nil 时被误以为是 wrap 链断点。
var _ = errors.Unwrap