// Package chain 是下单前的校验责任链抽象（design D20）。
//
// 设计动机（参考 下单责任链_流程说明_无代码版.md）：
//
//	把下单前的校验与准备动作拆成一串独立节点，依次执行，任一节点拒绝即终止。
//	每个节点内做原子仲裁，只有「通过」或「拒绝」两种出口；失败统一走业务 sentinel
//	错误收敛（service 层映射 HTTP）。
//
// 与「一个大 if-else 函数」的差异：
//   - 节点可独立测试、可插拔（秒杀开关、限购上下调只需调整 chain 顺序）；
//   - 失败出口统一（每个 check 返回 sentinel，handler 集中映射）；
//   - 后续加新校验（如黑名单、设备指纹）只需新加一个 Validator，零修改调用方。
//
// 不直接执行副作用：校验节点只做「读」 + 状态判定；DB 写、Redis 库存预扣
// 等副作用动作仍由 service 层在事务内完成（与 D3 算法保持一致）。
package chain

import (
	"errors"
	"time"

	"github.com/gin-gonic/gin"
)

// Source 标识本次下单的来源（普通抽卡 vs 秒杀）。
//
// 不同 Source 在 chain 内的处理差异：
//   - user_limit_check 仅 seckill 校验（normal 跳过）；
//   - time_window_check：normal 不约束，seckill 强制 [start_at, end_at)。
type Source string

const (
	// SourceNormal 普通抽卡（Draw 入口）。
	SourceNormal Source = "normal"

	// SourceSeckill 秒杀抽卡（DrawSeckill 入口）。
	SourceSeckill Source = "seckill"
)

// 业务 sentinel 错误（chain 层校验失败时返回）。
//
// 设计：chain 不重新发明错误，复用 user service 已有的 ErrUserNotAvailable /
// ErrBlindBoxNotAvailable / ErrSoldOut / ErrInsufficientBalance /
// ErrSeckillNotAvailable / ErrSeckillNotInWindow / ErrUserLimitExceeded 等。
// 这里只重新 export，避免 chain 包反向 import service 包造成循环依赖。
var (
	// ErrInvalidSource Source 不在 enum 内（防御编程错误，handler 500）。
	ErrInvalidSource = errors.New("chain: invalid source")

	// ErrMissingInput 必填字段缺失（如 BlindBoxID<=0 / UserID<=0）。
	ErrMissingInput = errors.New("chain: missing required input")
)

// DrawInput 责任链的输入参数。
//
// 字段按需填写（不同 Source 关注不同字段）：
//   - Source=normal 时：UserID / BlindBoxID / Now；
//   - Source=seckill 时：UserID / SeckillID / Now（SeckillID 用于解析 blind_box_id + 时间窗）；
//
// Now 由 service 层注入 time.Now()，便于测试替换。
// SeckillID 在 seckill 链路中用于查找 blind_box_id + 时间窗；normal 链路忽略。
// BlindBoxID 两种链路都有，含义明确：被抽的盲盒 ID。
//
// 字段全部非 nil-able：chain 内调用方已保证 UserID > 0。
// chain.Validate 入口校验缺失字段并 fast return ErrMissingInput（防御编程错误）。
type DrawInput struct {
	// Source 入口来源（normal / seckill）。
	Source Source

	// UserID 当前用户 ID。
	UserID int64

	// BlindBoxID 要抽的盲盒 ID。
	//
	// normal 链路：直接使用；
	// seckill 链路：先按 SeckillID 解析，再写入本字段供下游 check 使用。
	BlindBoxID int64

	// SeckillID 秒杀活动 ID（仅 seckill 链路使用）。
	//
	// normal 链路必须 = 0；seckill 链路必须 > 0。
	SeckillID int64

	// Now 校验当前时间（用于 time_window_check）。service 层传 time.Now()。
	Now time.Time
}

// Validator 校验节点接口。
//
// 实现位于 internal/service/user/draw_check/ 下；chain 包只定义抽象。
//
// 设计要点（参考 state.State 接口风格）：
//   - Name 返回节点字符串名（日志 / 错误 wrap 用，与 chain 内的 err log 简短对齐）；
//   - Validate 做原子仲裁：通过 → return nil；拒绝 → return 业务 sentinel；
//   - 节点内部不许「顺手」做下一节点的事；禁止半成品状态。
type Validator interface {
	// Name 节点名（如 "user_active"），用于日志与错误 wrap。
	Name() string

	// Validate 做本节点的校验。
	//
	// 参数：c（gin context，含 db session）；input（chain 输入）。
	//
	// 返回：
	//   - nil：通过；
	//   - 业务 sentinel：拒绝（handler 集中映射 HTTP）；
	//   - 其他 error：DB / Redis 异常（handler 500）。
	Validate(c *gin.Context, input *DrawInput) error
}