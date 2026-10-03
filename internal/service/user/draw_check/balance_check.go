// Package draw_check — balance_check.go 实现「余额是否充足」校验节点。
//
// 校验内容（spec Requirement "Draw transaction atomicity" 前置校验 #4）：
//
//	user.balance >= actual_price
//
// actual_price 口径：
//   - Source=normal  → 优先走活动价（FindActiveByBlindBoxID），回退 bb.Price；
//   - Source=seckill → sec.SeckillPrice（活动价对秒杀不适用）。
//
// 失败出口：chain.ErrInsufficientBalance（handler 映射 HTTP 402 draw.insufficient_balance）。
//
// ⚠️ 双重防御的设计动机：
//
// 本节点是「预过滤」——避免余额为 0 的用户进到事务后被 DecrementBalance 拒掉。
// 真实扣减仍由事务内 user.DecrementBalance 完成（条件 UPDATE，余额又被改过的
// 极端并发场景由 DB 兜底）。两层语义不同：
//
//	chain balance_check：用户进 tx 前先看一眼，提前拒绝；
//	tx DecrementBalance：原子条件扣减，影响行=0 → ErrInsufficientBalance。
//
// 不删除 tx 内的 DecrementBalance：它在 tx 内确保「扣不到数据就没钱」，是
// 「抽卡事务原子性」（D3）的硬约束；chain 检查只是 UX 优化。
package draw_check

import (
	"errors"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"ai-go-mall/internal/domain/chain"
	mallModel "ai-go-mall/internal/model/mall"
	mallRepo "ai-go-mall/internal/repository/mall"
	userRepo "ai-go-mall/internal/repository/user"
)

// balanceCheck 校验余额充足。
type balanceCheck struct {
	users     userRepo.UserRepository
	bb        mallRepo.BlindBoxRepository
	promotion mallRepo.PromotionRepository
	seckill   mallRepo.SeckillRepository
}

// NewBalanceCheck 构造 balanceCheck Validator。
//
// normal 链路需要 bb + promotion；seckill 链路需要 seckill。注入全套即可。
func NewBalanceCheck(users userRepo.UserRepository, bb mallRepo.BlindBoxRepository, promotion mallRepo.PromotionRepository, seckill mallRepo.SeckillRepository) chain.Validator {
	return &balanceCheck{
		users:     users,
		bb:        bb,
		promotion: promotion,
		seckill:   seckill,
	}
}

// Name 实现 chain.Validator。
func (c *balanceCheck) Name() string { return "balance" }

// Validate 读用户余额 + 解析 actual_price，比较。
//
// 通过：return nil。
// 拒绝：return chain.ErrInsufficientBalance（wrap 由 Chain.Validate 完成）。
//
// 注：user / blindbox 不存在已由前置节点（user_active / blindbox_buyable）拒绝；
// 本节点假定输入合法。价格解析失败（如 DB 抖动）按 chain.ErrInsufficientBalance
// 兜底——拒绝比通过更安全，避免假阳性「以为有钱实则没拿到价」放行。
func (c *balanceCheck) Validate(ctx *gin.Context, input *chain.DrawInput) error {
	if input.UserID <= 0 {
		return chain.ErrInsufficientBalance
	}
	u, err := c.users.GetByID(ctx, input.UserID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// user_active 已拦过；这里再防一次：用户被删除 → 视为没钱。
			return chain.ErrInsufficientBalance
		}
		return err
	}
	if u == nil {
		return chain.ErrInsufficientBalance
	}

	price, err := c.resolvePrice(ctx, input)
	if err != nil {
		// 价格拿不到 → 视为余额不够（保守拒绝，与「价格永远合法」的乐观假设相反）。
		return chain.ErrInsufficientBalance
	}
	if u.Balance < price {
		return chain.ErrInsufficientBalance
	}
	return nil
}

// resolvePrice 按 Source 解析 actual_price。
//
// normal：优先活动价（now ∈ [start_at, end_at)），回退 bb.Price；
// seckill：sec.SeckillPrice。
func (c *balanceCheck) resolvePrice(ctx *gin.Context, input *chain.DrawInput) (float64, error) {
	switch input.Source {
	case chain.SourceSeckill:
		if input.SeckillID <= 0 {
			return 0, gorm.ErrRecordNotFound
		}
		sec, err := c.seckill.GetByID(ctx, input.SeckillID)
		if err != nil {
			return 0, err
		}
		if sec == nil {
			return 0, gorm.ErrRecordNotFound
		}
		return sec.SeckillPrice, nil

	default: // SourceNormal
		if input.BlindBoxID <= 0 {
			return 0, gorm.ErrRecordNotFound
		}
		bb, err := c.bb.GetByID(ctx, input.BlindBoxID)
		if err != nil {
			return 0, err
		}
		if bb == nil {
			return 0, gorm.ErrRecordNotFound
		}
		now := input.Now
		if now.IsZero() {
			now = time.Now()
		}
		p, err := c.promotion.FindActiveByBlindBoxID(ctx, bb.ID, now)
		if err != nil {
			return 0, err
		}
		if p != nil && p.Status == mallModel.StatusActive {
			return p.PromoPrice, nil
		}
		return bb.Price, nil
	}
}

// 编译期断言。
var _ chain.Validator = (*balanceCheck)(nil)
