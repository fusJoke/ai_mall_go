// Package draw_check — user_limit_check.go 实现「秒杀限购」原子仲裁。
//
// 校验内容（design D20 §一 节点 2 限购规则；spec Requirement "Seckill draw flow"）：
//
//	Redis INCR seckill:user_bought:{sid}:{uid} → n；
//	  n > sec.PerUserLimit → DECR 回滚 + ErrUserLimitExceeded；
//	  n <= sec.PerUserLimit → 通过（side effect = +1，需 service 层在后续失败时 DECR）。
//
// 失败出口：chain.ErrUserLimitExceeded（handler 映射 HTTP 429 seckill.user_limit_exceeded）。
//
// ⚠️ 为什么不放进 Chain：
//
// 本节点是 chain 内**唯一带 Redis 副作用**的节点（INCR）；其他节点都是「读 + 判定」。
// Chain 本身没有补偿栈——一旦 INCR 成功、后续节点失败（如 balance_check），需要
// service 层做一次 DECR 兜底。
//
// 为避免 DECR 在「chain 未跑到 user_limit」时打到空 key（创建出 -1），本节点以
// 自由函数 `AcquireUserLimit` 暴露，**不**作为 chain.Validator。DrawSeckill 的
// 调用顺序：
//
//  1. chain.Validate(SeckillDrawChain)              // 4 个 read-only 校验
//  2. AcquireUserLimit(...)                          // 本函数，INCR + check + DECR-on-reject
//  3. cache.Decr(seckill:stock:{sid}) + check + 回滚 // 库存预扣（既有逻辑）
//  4. MySQL tx                                       // D3 抽卡
//  5. 失败统一回滚（既有逻辑）：INCR stock + DECR user_bought
//
// 这样设计的取舍：
//   - user_limit 维持「独立 + 强副作用」语义（与既有 seckill 事务对齐）；
//   - chain 主体保持「读 + 判定」轻量，便于单测 + 未来扩展；
//   - 单元测试可单独验证本函数的 INCR + 阈值 + DECR-on-reject 行为。
package draw_check

import (
	"context"
	"strconv"

	"ai-go-mall/internal/domain/chain"
)

// Redis key 前缀（与 draw.go / supplier.SeckillService 共享命名）。
//
// 三方实现必须保持完全一致（design D15 决策点 3）：
//
//	seckill:stock:{sid}              — 秒杀剩余名额
//	seckill:user_bought:{sid}:{uid}  — 单用户已购买数
const seckillRedisUserBoughtKey = "seckill:user_bought:"

// redisIncrDecr 抽象 user_limit 需要的 Redis 能力。
//
// 只暴露 Incr / Decr，不暴露无关方法。
// 实际由 *cache.Manager / MultiLevelCache 隐式满足（cache.Cache 接口含 Incr/Decr）；
// 测试可注入 mock 实现。
type redisIncrDecr interface {
	Incr(ctx context.Context, key string) (int64, error)
	Decr(ctx context.Context, key string) (int64, error)
}

// AcquireUserLimit 尝试为本用户在该秒杀活动 +1 购买计数，并对照 per_user_limit。
//
// 返回：
//   - nil：通过（INCR 已生效，调用方需在后续失败时 DECR）；
//   - chain.ErrUserLimitExceeded：超限（INCR 已被 DECR 抵消，无副作用）；
//   - 其他 error：Redis 异常（INCR 已生效，调用方按需 DECR）。
//
// 参数：
//   - perUserLimit <= 0 时一律放行（数据异常兜底，不阻塞业务；正常配置应 > 0）。
func AcquireUserLimit(ctx context.Context, cache redisIncrDecr, seckillID, userID int64, perUserLimit int) error {
	if cache == nil || seckillID <= 0 || userID <= 0 {
		// 防御编程错误：nil cache 或非正 ID 一律按放行处理（不阻断业务路径）。
		return nil
	}
	key := userBoughtKey(seckillID, userID)

	n, err := cache.Incr(ctx, key)
	if err != nil {
		return err
	}
	if perUserLimit > 0 && n > int64(perUserLimit) {
		// 超限：DECR 抵消自己这次 INCR（best-effort；Decr 失败仅 warn）。
		_, _ = cache.Decr(ctx, key)
		return chain.ErrUserLimitExceeded
	}
	return nil
}

// userBoughtKey 拼出单用户在某秒杀活动的 Redis key。
//
// 暴露给同包测试使用（验证 key 格式 + 解析回 round-trip）。
func userBoughtKey(seckillID, userID int64) string {
	return seckillRedisUserBoughtKey +
		strconv.FormatInt(seckillID, 10) + ":" +
		strconv.FormatInt(userID, 10)
}
