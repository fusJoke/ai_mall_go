// Package chain — errors.go 定义责任链校验节点的 sentinel 错误。
//
// 设计动机：
//
//	draw_check 子包需要返回业务错误，但 draw.go（user 包）也要 import draw_check
//	构造 chain。如果 draw_check 直接 import user 包就形成循环（user → draw_check → user）。
//
//	把 sentinel 集中在 chain 包：draw_check → chain（OK），user/draw.go → draw_check
//	（OK），user 包不直接依赖 chain 而是用别名 re-export（保持 handler 的
//	errors.Is(err, userSvc.ErrXxx) 写法不变）。
package chain

import "errors"

// 业务 sentinel 错误：节点拒绝时返回。
//
// 与 service/user 包同名 var 是 alias 关系（见 user/draw.go 的 re-export）；
// handler 端继续用 userSvc.ErrSoldOut 不受影响；chain 内部 + draw_check 用
// 这里的定义，避免子包反向 import 父包造成的循环。
var (
	// ErrBlindBoxNotAvailable 盲盒不可抽（不存在 / 已下架 / 供应商被禁用）。
	//
	// handler 映射 HTTP 404 draw.blindbox_not_available。
	ErrBlindBoxNotAvailable = errors.New("chain: blindbox not available")

	// ErrUserNotAvailable 用户账号不可用（不存在 / 已禁用）。
	//
	// handler 映射 HTTP 403 draw.user_not_available。
	ErrUserNotAvailable = errors.New("chain: user not available")

	// ErrSeckillNotAvailable 秒杀不可用（不存在 / 已禁用 / 未 init Redis）。
	//
	// handler 映射 HTTP 404 seckill.not_available。
	ErrSeckillNotAvailable = errors.New("chain: seckill not available")

	// ErrSeckillNotInWindow 秒杀不在时间窗口内（未开始 / 已结束）。
	//
	// handler 映射 HTTP 403 seckill.not_in_window。
	ErrSeckillNotInWindow = errors.New("chain: seckill not in time window")

	// ErrUserLimitExceeded 超过 per_user_limit。
	//
	// handler 映射 HTTP 429 seckill.user_limit_exceeded。
	ErrUserLimitExceeded = errors.New("chain: per-user limit exceeded")

	// ErrSoldOut 卡池 / 秒杀 已售罄。
	//
	// 注：chain 层理论上不该返回（卡池售罄是事务内的随机命中结果，不在 chain
	// 校验范围）；保留是为了「节点也想 short-circuit 库存不足」场景（如未来
	// 加一个「库存预占」节点）。
	//
	// handler 映射 HTTP 409 draw.sold_out。
	ErrSoldOut = errors.New("chain: sold out")

	// ErrInsufficientBalance 余额不足（balance < actual_price）。
	//
	// handler 映射 HTTP 402 draw.insufficient_balance。
	ErrInsufficientBalance = errors.New("chain: insufficient balance")
)