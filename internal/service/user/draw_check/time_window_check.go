// Package draw_check — time_window_check.go 实现「活动时间窗口」校验节点。
//
// 校验内容（design D20 §一 节点 5；spec Requirement "Seckill draw flow"）：
//
//	Source=normal  → 不约束时间窗（普通抽卡无时间限制）；
//	Source=seckill → 要求 now ∈ [seckill.StartAt, seckill.EndAt)；
//	                seckill.status=active AND redis_initialized=true（Redis 库存已 init）。
//
// 失败出口：
//   - 秒杀不在窗口内 → chain.ErrSeckillNotInWindow（HTTP 403 seckill.not_in_window）；
//   - 秒杀不存在 / 已禁用 / 未 init Redis → chain.ErrSeckillNotAvailable（HTTP 404）。
//
// 设计要点：
//   - normal 链路 fast return nil（不查 DB），避免给普通抽卡增加额外读；
//   - seckill 链路复用 SeckillRepository.GetByID（已有索引，性能 OK）；
//   - 与 DrawSeckill 既有 pre-check 行为完全一致（status + redis_initialized + 时间窗）。
package draw_check

import (
	"errors"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"ai-go-mall/internal/domain/chain"
	mallModel "ai-go-mall/internal/model/mall"
	mallRepo "ai-go-mall/internal/repository/mall"
)

// timeWindowCheck 校验活动时间窗口（按 Source 区分）。
type timeWindowCheck struct {
	seckill mallRepo.SeckillRepository
}

// NewTimeWindowCheck 构造 timeWindowCheck Validator。
func NewTimeWindowCheck(seckill mallRepo.SeckillRepository) chain.Validator {
	return &timeWindowCheck{seckill: seckill}
}

// Name 实现 chain.Validator。
func (c *timeWindowCheck) Name() string { return "time_window" }

// Validate 按 input.Source 分派校验。
//
// normal：直接 return nil（普通抽卡无时间约束）。
// seckill：拉 seckill 行，校验 status + redis_initialized + now ∈ [start, end)。
func (c *timeWindowCheck) Validate(ctx *gin.Context, input *chain.DrawInput) error {
	if input.Source != chain.SourceSeckill {
		// normal 链路无时间窗约束（普通抽卡全天可抽）。
		return nil
	}
	if input.SeckillID <= 0 {
		// chain_builder 已保证 SeckillID>0；这里兜底防御编程错误。
		return chain.ErrSeckillNotAvailable
	}
	sec, err := c.seckill.GetByID(ctx, input.SeckillID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return chain.ErrSeckillNotAvailable
		}
		return err
	}
	if sec == nil || sec.Status != mallModel.StatusActive || !sec.RedisInitialized {
		return chain.ErrSeckillNotAvailable
	}

	now := input.Now
	if now.IsZero() {
		now = time.Now()
	}
	// [start, end) 半开区间（与 ListActive 语义一致）。
	if now.Before(sec.StartAt) || !now.Before(sec.EndAt) {
		return chain.ErrSeckillNotInWindow
	}
	return nil
}

// 编译期断言。
var _ chain.Validator = (*timeWindowCheck)(nil)
