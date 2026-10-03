// Package user — draw.go 实现 C 端盲盒抽卡事务（spec Requirement "Draw transaction atomicity"）。
//
// 核心算法（design D3）：
//
//	BEGIN TX
//	  1. SELECT pool items FOR UPDATE WHERE stock > 0
//	  2. Σ(weight_i)；若 Σ = 0 → ROLLBACK, ErrSoldOut
//	  3. rand(0, Σ) → 命中 item
//	  4. UPDATE pool_items SET stock = stock - 1 WHERE id = ? AND stock > 0
//	     影响行数 = 0 → 重试 1 次（极端并发）
//	  5. 拿实际售价（活动价优先，回退原价）
//	  6. UPDATE mall_users SET balance = balance - ? WHERE id = ? AND balance >= ?
//	     影响行数 = 0 → ROLLBACK, ErrInsufficientBalance
//	  7. UPDATE mall_suppliers SET balance = balance + ?, total_sales = total_sales + ?
//	  7.5/7.6. 累加 platform ledger（spec 8.5：total_revenue += actualPrice，
//	         给结算 / 报表做平台总收入只读快照；同样在事务内保证「抽不到数据就没钱」）。
//	  8. INSERT mall_draw_orders (status=pending)
//	  8.5 状态机 Transition pending→paid + repo.UpdateStatus（commit 前）
//	  9. INSERT mall_draw_order_items (snapshot_*)
//	  9.5 状态机 Transition paid→drawn + repo.UpdateStatus
//	COMMIT
//
// 并发安全靠两件事：
//   - SELECT ... FOR UPDATE（行锁）
//   - 库存与余额的条件更新（避免超卖与超额扣款）
//
// 乐观锁版本号不适用：「库存 / 余额」单调递减，ABA 风险不存在。
//
// 订单状态机（spec 14）：pending → paid → drawn；任何一步失败 → 整笔 ROLLBACK，
// 状态机在"写库前"校验合法性，非法跳跃（编程错误）→ ErrInvalidStateTransition。
// 因为 Draw 整体在单事务内，外部读到的最终态恒为 drawn；pending/paid 是事务内
// 状态机的中间态，主要价值是 catch 编程错误 + 维持未来异步化（解耦支付与抽卡）
// 时的状态契约。
package user

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"ai-go-mall/internal/domain/chain"
	"ai-go-mall/internal/domain/state"
	"ai-go-mall/internal/infra/cache"
	"ai-go-mall/internal/infra/channel"
	"ai-go-mall/internal/model"
	"ai-go-mall/internal/model/mall"
	"ai-go-mall/internal/repository"
	mallRepo "ai-go-mall/internal/repository/mall"
	supplierRepo "ai-go-mall/internal/repository/supplier"
	userRepo "ai-go-mall/internal/repository/user"
	"ai-go-mall/internal/service/user/draw_check"
)

// topicStockDeductionSync 是秒杀扣减日志异步对账的 MQ topic。
//
// 与 cmd/stock-sync 的 Subscribe topic 必须一字不差：
// 两边任一边写错就形成「发布方写了 X，消费方订了 Y」的对角线错位，
// 业务上消息失踪，对账永久漏推。
const topicStockDeductionSync = "stock.deduction.sync"

// stockDeductionPayload 是 MQ 消息体的 JSON 形态。
//
// cmd/stock-sync 端用同样的结构反序列化（参见 cmd/stock-sync/main.go）。
// 这里只挑出对账必需的 LogID，避免序列化整个对象引入无关字段耦合
// （如 CreatedAt、DeductedAt 与 synced 无关；snapshot_* 字段已经持久化）。
type stockDeductionPayload struct {
	ID int64 `json:"id"`
}

// 业务错误。
//
// 与 chain 包同名 var 是 alias 关系（task 15 责任链重构）：
// 业务 sentinel 集中在 internal/domain/chain/errors.go；这里 re-export 保持
// handler 端 userSvc.ErrXxx 的写法不变（errors.Is(err, userSvc.ErrXxx) 仍
// 命中 chain.ErrXxx）。
//
// chain 不含的 sentinel（ErrPoolNotFound / ErrEmptyPool）留在本包。
var (
	// ErrBlindBoxNotAvailable 盲盒不可抽（不存在 / 已下架 / 供应商被禁用）。
	ErrBlindBoxNotAvailable = chain.ErrBlindBoxNotAvailable

	// ErrUserNotAvailable 用户账号不可用（不存在 / 被禁用）。
	ErrUserNotAvailable = chain.ErrUserNotAvailable

	// ErrPoolNotFound 卡池缺失（数据异常：创建盲盒必须建池）。
	ErrPoolNotFound = errors.New("user.draw: card pool not found")

	// ErrSoldOut 卡池已售罄（所有 item stock=0）。
	ErrSoldOut = chain.ErrSoldOut

	// ErrInsufficientBalance 余额不足。
	ErrInsufficientBalance = chain.ErrInsufficientBalance

	// ErrEmptyPool 卡池里没有任何 items（理论上不应该发生）。
	ErrEmptyPool = errors.New("user.draw: empty pool")
)

// DrawnCard 单次抽中的卡牌结果（含快照字段，供订单明细持久化）。
type DrawnCard struct {
	// ItemID 卡池条目 ID（mall_card_pool_items.id）。
	ItemID int64

	// CardID 卡牌 ID（mall_cards.id）。
	CardID int64

	// Rarity 稀有度快照（SSR/SR/R/N）。
	Rarity mall.MallCardRarity

	// SnapshotName 卡名快照。
	SnapshotName string

	// SnapshotImage 卡图快照。
	SnapshotImage string
}

// DrawResult 抽卡事务返回值。
type DrawResult struct {
	// OrderID 落库的订单 ID。
	OrderID int64

	// OrderNo 落库的订单号（业务唯一键）。
	OrderNo string

	// ActualPrice 实际扣款金额（活动价或原价）。
	ActualPrice float64

	// Cards 本次抽中的卡牌（通常 = 1 张；后续批量抽支持 N 张）。
	Cards []DrawnCard
}

// txRunner 是 transaction 执行的最小抽象。
//
// *repository 默认实现走 repository.RunInTx；测试可注入 mock，让 fn 同步执行。
type txRunner interface {
	Run(c *gin.Context, fn func(txCtx *gin.Context) error) error
}

// DrawService 是抽卡事务的业务接口。
//
// 单方法：Draw(c, userID, blindBoxID)。
// 多个方法会污染接口语义；后续加 seckill 时另起 SeckillService。
type DrawService interface {
	Draw(c *gin.Context, userID, blindBoxID int64) (*DrawResult, error)
}

// DrawServiceDeps 注入 DrawService 所需的依赖。
type DrawServiceDeps struct {
	BlindBoxRepo   mallRepo.BlindBoxRepository
	CardPoolRepo   mallRepo.CardPoolRepository
	PromotionRepo  mallRepo.PromotionRepository
	SupplierRepo   supplierRepo.SupplierRepository
	UserRepo       userRepo.UserRepository
	DrawOrderRepo  mallRepo.DrawOrderRepository
	CardRepo       mallRepo.CardRepository
	SettlementRepo mallRepo.SettlementRepository // spec 8.5 步骤 7.6：累加 total_revenue

	// CheckChain 下单责任链（task 15 / design D20）；事务前调 Validate。
	// nil 时 service 仍能跑（凭多驱动兼容旧单测），但跳过校验 → 等价旧行为。
	CheckChain *chain.Chain

	// Channels 渠道工厂（D18）：事务提交后经 "payment" 渠道记录模拟支付。
	// MVP 是 mock driver（仅记录）；nil 时跳过（兼容旧单测 / 缺依赖装配）。
	Channels channel.ChannelFactory

	// TxRunner 可选；nil 时走 repository.RunInTx。
	TxRunner txRunner
}

// baseDrawService 是 DrawService 的默认实现。
type baseDrawService struct {
	bb         mallRepo.BlindBoxRepository
	cp         mallRepo.CardPoolRepository
	promo      mallRepo.PromotionRepository
	sup        supplierRepo.SupplierRepository
	user       userRepo.UserRepository
	order      mallRepo.DrawOrderRepository
	card       mallRepo.CardRepository
	settlement mallRepo.SettlementRepository
	checkChain *chain.Chain
	channels   channel.ChannelFactory
	txr        txRunner
}

// NewDrawService 接收依赖，返回 DrawService 接口。
func NewDrawService(d DrawServiceDeps) DrawService {
	txr := d.TxRunner
	if txr == nil {
		txr = defaultTxRunner{}
	}
	return &baseDrawService{
		bb:         d.BlindBoxRepo,
		cp:         d.CardPoolRepo,
		promo:      d.PromotionRepo,
		sup:        d.SupplierRepo,
		user:       d.UserRepo,
		order:      d.DrawOrderRepo,
		card:       d.CardRepo,
		settlement: d.SettlementRepo,
		checkChain: d.CheckChain,
		channels:   d.Channels,
		txr:        txr,
	}
}

// defaultTxRunner 走 repository.RunInTx（生产路径）。
type defaultTxRunner struct{}

func (defaultTxRunner) Run(c *gin.Context, fn func(txCtx *gin.Context) error) error {
	return repository.RunInTx(c, fn)
}

// Draw 实现抽卡事务（spec Requirement "Draw transaction atomicity"）。
//
// 前置校验（事务外，由责任链完成；task 15 / design D20）：
//  1. user_active — mall_users.status = 1；
//  2. blindbox_buyable — blind_box.status=active + on_sale=true + supplier.status=active；
//  3. time_window — normal 链路 fast return nil；
//  4. balance — user.balance >= actual_price（活动价优先）。
//
// 事务内（详见包注释 D3）：
//   - SELECT items FOR UPDATE → 随机按 weight 选 → 条件扣减 stock（重试 1 次）；
//   - 拿活动价优先的实际售价；
//   - 条件扣减用户余额（影响行=0 → ErrInsufficientBalance，双重防御最后一关）；
//   - 增加供应商余额与销售额；
//   - INSERT order + items。
func (s *baseDrawService) Draw(c *gin.Context, userID, blindBoxID int64) (*DrawResult, error) {
	if userID <= 0 || blindBoxID <= 0 {
		return nil, ErrBlindBoxNotAvailable
	}

	// --- 责任链校验（事务外；替换原有内联 check） ---
	if s.checkChain != nil {
		input := &chain.DrawInput{
			Source:     chain.SourceNormal,
			UserID:     userID,
			BlindBoxID: blindBoxID,
			Now:        time.Now(),
		}
		if err := s.checkChain.Validate(c, input); err != nil {
			return nil, err
		}
	}

	// --- 事务内 D3 算法 ---
	var result *DrawResult
	err := s.txr.Run(c, func(txCtx *gin.Context) error {
		// 0. tx 内再读一次盲盒（防御 chain 通过后到 tx 之间 admin 下架）。
		//    chain 已校验 status=active + on_sale + supplier 启用；这里只取
		//    SupplierID / Price 等字段，TOCTOU 窗口由盲盒事务兜底。
		bb, err := s.bb.GetByID(txCtx, blindBoxID)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrBlindBoxNotAvailable
			}
			return err
		}
		if bb == nil || bb.Status != mall.StatusActive || !bb.OnSale {
			return ErrBlindBoxNotAvailable
		}

		// 1. 拿卡池（含 pool_id）。
		pool, err := s.cp.GetPoolByBlindBoxID(txCtx, blindBoxID)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrPoolNotFound
			}
			return err
		}

		// 2. SELECT items FOR UPDATE WHERE stock > 0。
		items, err := s.cp.ListItemsForUpdateByPoolID(txCtx, pool.ID)
		if err != nil {
			return err
		}
		if len(items) == 0 {
			return ErrSoldOut
		}

		// 3. 随机按 weight 命中 item（重试至多 1 次）。
		chosenItem, err := pickWeightedItemWithRetry(txCtx, s.cp, pool.ID, items)
		if err != nil {
			return err
		}

		// 4. 拿卡牌快照（name / image）。
		cardRow, err := s.card.GetByID(txCtx, chosenItem.CardID)
		if err != nil {
			return err
		}
		drawn := DrawnCard{
			ItemID:        chosenItem.ID,
			CardID:        chosenItem.CardID,
			Rarity:        chosenItem.Rarity,
			SnapshotName:  cardRow.Name,
			SnapshotImage: cardRow.Image,
		}

		// 5. 拿活动价优先的实际售价（同事务 LEFT JOIN —— design D4）。
		now := time.Now()
		actualPrice, err := resolveActualPrice(txCtx, bb, s.promo, now)
		if err != nil {
			return err
		}

		// 6. 条件扣减用户余额（ErrInsufficientBalance 影响 0 行时由 repo 返回）。
		if err := s.user.DecrementBalance(txCtx, userID, actualPrice); err != nil {
			if errors.Is(err, userRepo.ErrInsufficientBalance) {
				return ErrInsufficientBalance
			}
			return err
		}

		// 7. 增加供应商余额 + 销售额（事务内保证 supplier "幻觉收入" 不存在）。
		if err := s.sup.UpdateBalanceAndSales(txCtx, bb.SupplierID, actualPrice); err != nil {
			return err
		}

		// 7.5/7.6. 累加 platform ledger 的 total_revenue（spec 8.5）：
		//   INSERT INTO mall_platform_ledger (counter_key, amount) VALUES ('total_revenue', ?)
		//   ON DUPLICATE KEY UPDATE amount = amount + VALUES(amount)
		// 失败 → 整笔事务回滚，保证「抽卡没产生」与「ledger 金额不同步」互斥。
		// 注意：counterKey / amount 由 settlement_repository.go IncrementLedger 兜底，
		// 这里只填业务字段；空 counterKey / 步长 ≤0 等异常场景已在 repo 层报错。
		if err := s.settlement.IncrementLedger(txCtx, "total_revenue", actualPrice); err != nil {
			return err
		}

		// 8. 落订单（uuid v7 作 order_no；初始 status=pending，paid/drawn 由
		//    状态机校验后通过 repo.UpdateStatus 推进）。状态机包 internal/domain/state。
		orderNo, err := newOrderNo()
		if err != nil {
			return err
		}
		order := &mall.MallDrawOrder{
			OrderNo:    orderNo,
			UserID:     userID,
			BlindBoxID: blindBoxID,
			SupplierID: bb.SupplierID,
			Price:      actualPrice,
			Status:     mall.OrderStatusPending,
			Source:     mall.SourceNormal,
		}
		if err := s.order.CreateOrder(txCtx, order); err != nil {
			return err
		}

		// 状态机推进 pending → paid（在事务 commit 前，余额已扣、订单已建）。
		// 校验失败（编程错误如跳跃 / 跨类型）→ abort 整笔事务。
		if _, err := state.Transition(state.DrawOrderStates.Pending, state.DrawOrderStates.Paid, &state.Context{Entity: order}); err != nil {
			return err
		}
		if err := s.order.UpdateStatus(txCtx, order.ID, mall.OrderStatusPaid); err != nil {
			return err
		}

		// 9. 落订单明细。
		orderItems := []mall.MallDrawOrderItem{
			{
				OrderID:       order.ID,
				CardID:        drawn.CardID,
				Rarity:        drawn.Rarity,
				SnapshotName:  drawn.SnapshotName,
				SnapshotImage: drawn.SnapshotImage,
			},
		}
		if err := s.order.CreateItems(txCtx, orderItems); err != nil {
			return err
		}

		// 状态机推进 paid → drawn（步骤 9 之后，订单明细已落库）。
		if _, err := state.Transition(state.DrawOrderStates.Paid, state.DrawOrderStates.Drawn, &state.Context{Entity: order}); err != nil {
			return err
		}
		if err := s.order.UpdateStatus(txCtx, order.ID, mall.OrderStatusDrawn); err != nil {
			return err
		}

		result = &DrawResult{
			OrderID:     order.ID,
			OrderNo:     orderNo,
			ActualPrice: actualPrice,
			Cards:       []DrawnCard{drawn},
		}
		return nil
	})

	if err != nil {
		return nil, err
	}
	// 渠道层记录模拟支付（D18；事务外，失败仅 warn，见 channel_hooks.go）。
	recordChannelPayment(s.channels, c.Request.Context(), userID, result, "盲盒抽卡")
	return result, nil
}

// pickWeightedItemWithRetry 实现 D3 步骤 4 的「条件扣减失败重试 1 次」逻辑。
//
// 取库存 items → 按 weight 命中 → DecrementStock(stock > 0) → 失败则重抽一次（最多 1 次）。
// 两次都失败（极端并发下别的 tx 抢走）→ ErrSoldOut。
func pickWeightedItemWithRetry(c *gin.Context, cp mallRepo.CardPoolRepository, poolID int64, items []mall.MallCardPoolItem) (*mall.MallCardPoolItem, error) {
	for attempt := range 2 {
		// 重新拉一次 items（FOR UPDATE 在前面 SQL 已经锁住了，这里重读即可）。
		// 若 attempt == 0 用传入的 items；attempt == 1 重新拉。
		working := items
		if attempt == 1 {
			fresh, err := cp.ListItemsByPoolID(c, poolID)
			if err != nil {
				return nil, err
			}
			// 仅保留 stock > 0
			working = make([]mall.MallCardPoolItem, 0, len(fresh))
			for _, it := range fresh {
				if it.Stock > 0 {
					working = append(working, it)
				}
			}
			if len(working) == 0 {
				return nil, ErrSoldOut
			}
		}

		// 按 weight 求和（已过滤 stock > 0）。
		var total int64
		for _, it := range working {
			total += int64(it.Weight)
		}
		if total <= 0 {
			return nil, ErrSoldOut
		}

		// 随机命中。
		target := rand.Int63n(total) // [0, total)
		var acc int64
		var picked *mall.MallCardPoolItem
		for i := range working {
			acc += int64(working[i].Weight)
			if target < acc {
				picked = &working[i]
				break
			}
		}
		if picked == nil {
			// 浮点累计误差理论上不应该发生（都是整数），兜底。
			picked = &working[len(working)-1]
		}

		// 条件扣减 stock。
		if err := cp.DecrementStock(c, picked.ID); err != nil {
			if errors.Is(err, mallRepo.ErrStockEmpty) {
				// 极端并发：item 在我们锁定后被别的 tx 抽走。重试。
				continue
			}
			return nil, err
		}
		return picked, nil
	}
	return nil, ErrSoldOut
}

// resolveActualPrice 拿活动价优先的实际售价（同事务 LEFT JOIN 语义 —— design D4）。
//
// 优先顺序：
//  1. 若存在「active 且 now ∈ [start_at, end_at)」的活动 → 用 PromoPrice；
//  2. 否则 → 用盲盒 Price。
//
// 简化：用 PromotionRepository.FindActiveByBlindBoxID 已经实现「同窗口单条」语义。
// 若返回 nil 表示无活动，回退原价。
func resolveActualPrice(c *gin.Context, bb *mall.MallBlindBox, promo mallRepo.PromotionRepository, now time.Time) (float64, error) {
	p, err := promo.FindActiveByBlindBoxID(c, bb.ID, now)
	if err != nil {
		return 0, err
	}
	if p == nil {
		return bb.Price, nil
	}
	return p.PromoPrice, nil
}

// newOrderNo 生成业务订单号：uuid v7 的 32 位十六进制（无连字符）形态。
//
// 为什么不是 uuid.String()：spec「mall_draw_orders field schema」、迁移 000009
// 与 model 都把 order_no 定为 varchar(32)，而 String() 是 36 字符（带 4 个
// 连字符），直接落库会 `Error 1406 Data too long for column 'order_no'`。
// uuid v7 的时间有序性不受影响 —— 去连字符只是编码形态变化。
func newOrderNo() (string, error) {
	uid, err := uuid.NewV7()
	if err != nil {
		return "", err
	}
	return strings.ReplaceAll(uid.String(), "-", ""), nil
}

// 编译期断言：baseDrawService 必须实现 DrawService。
var _ DrawService = (*baseDrawService)(nil)

// =============================================================================
// 秒杀抽卡（design D15 + spec Requirement "Seckill draw flow"）
// =============================================================================
//
// 业务路径（D15 流程）：
//
//  1. 预校验：seckill.status=active AND redis_initialized=true AND NOW() ∈ [start_at, end_at)
//  2. 限购：INCR seckill:user_bought:{sid}:{uid}；> per_user_limit → DECR 回滚 → ErrUserLimitExceeded
//  3. 库存预扣：DECR seckill:stock:{sid}；< 0 → INCR stock + DECR user_bought 回滚 → ErrSoldOut
//  4. MySQL 事务（d3 抽卡 + 写 stock_deduction_log + source='seckill' + 结算副作用）
//  5. 失败：ROLLBACK + Redis INCR stock + DECR user_bought
//
// 与 Draw() 的差异：
//   - 入口参数是 seckillID 而非 blindBoxID；价格走 seckill_price 而非 activity 价；
//   - 多写 stock_deduction_log；order.source='seckill'；
//   - 抽卡前必须 Redis 限购 + 库存两道闸口（Redis 端先扛并发 → SQL 层抽卡）。
//
// 状态字段定值（Ruling 9.5）：spec D15 描述里写 "INSERT status='paid'"，
// 但同 tx 内已 INSERT mall_draw_order_items（snapshot_*），且 4.e 步骤紧跟其后，
// 状态实际反映"已支付 + 已抽中"双重事实；与现有 Draw() 一致使用 OrderStatusDrawn。
// 这样前端 source='seckill' + status='drawn' 一眼可识别秒杀已完结订单。
//
// MQ 发布（task 11.7）：本文件不实现，事务提交后由 cmd/stock-sync 兜底对账，
// 后续 task 11.7 在这里加 `go mq.Publish(...)` 异步 publish。

// 秒杀业务错误（独立 sentinel 以便 handler 映射 HTTP 状态码）。
var (
	// ErrSeckillNotAvailable 秒杀不可用（不存在 / 已禁用 / 未 init Redis / 盲盒/供应商已下架）。
	ErrSeckillNotAvailable = errors.New("user.draw: seckill not available")

	// ErrSeckillNotInWindow 秒杀不在时间窗口内（未开始 / 已结束）。
	ErrSeckillNotInWindow = errors.New("user.draw: seckill not in time window")

	// ErrUserLimitExceeded 超过 per_user_limit。
	ErrUserLimitExceeded = errors.New("user.draw: per-user limit exceeded")
)

// Redis key 前缀（与 supplier.SeckillService 共享命名）。
//
// 这里是字面常量复述，避免在 draw.go 内反向依赖 supplier 包；
// 两边的实现必须保持完全一致（d15 决策点 3）。
const (
	seckillRedisStockKey      = "seckill:stock:"
	seckillRedisUserBoughtKey = "seckill:user_bought:"
)

// SeckillService 是秒杀抽卡的业务接口。
//
// 单方法：DrawSeckill(c, userID, seckillID)。
type SeckillService interface {
	DrawSeckill(c *gin.Context, userID, seckillID int64) (*DrawResult, error)
}

// SeckillServiceDeps 注入 SeckillService 所需的依赖。
//
// 复用 DrawService 的盲盒/卡池/用户/订单/结算 repo，
// 另加 seckillRepo、cache（Redis 限购 + 库存预扣）、publisher（事务后异步对账）。
type SeckillServiceDeps struct {
	BlindBoxRepo   mallRepo.BlindBoxRepository
	CardPoolRepo   mallRepo.CardPoolRepository
	CardRepo       mallRepo.CardRepository
	SupplierRepo   supplierRepo.SupplierRepository
	UserRepo       userRepo.UserRepository
	DrawOrderRepo  mallRepo.DrawOrderRepository
	SettlementRepo mallRepo.SettlementRepository
	SeckillRepo    mallRepo.SeckillRepository // seckill activity + stock_deduction_log
	Cache          cache.Cache                // Redis 限购 + 库存预扣
	Publisher      Publisher                  // 事务后异步 publish stock.deduction.sync（D17）
	Channels       channel.ChannelFactory     // D18：事务提交后经 "payment" 渠道记录模拟支付；nil 跳过

	// CheckChain 下单责任链（task 15 / design D20）；事务前调 Validate。
	// nil 时 service 仍能跑（兼容旧单测），但跳过 chain 校验 → 等价旧行为。
	CheckChain *chain.Chain

	// TxRunner 可选；nil 时走 repository.RunInTx。
	TxRunner txRunner
}

// Publisher 是 SeckillService 用到的 MQ 发布能力抽象。
//
// 只暴露 Publish（service 不应能 subscribe / ack）；由 *mq.Manager 隐式满足，
// 测试可注入 mock 实现。
type Publisher interface {
	Publish(ctx context.Context, topic string, payload []byte) error
}

// baseSeckillService 是 SeckillService 的默认实现。
type baseSeckillService struct {
	bb         mallRepo.BlindBoxRepository
	cp         mallRepo.CardPoolRepository
	card       mallRepo.CardRepository
	sup        supplierRepo.SupplierRepository
	user       userRepo.UserRepository
	order      mallRepo.DrawOrderRepository
	settlement mallRepo.SettlementRepository
	seckill    mallRepo.SeckillRepository
	cache      cache.Cache
	publisher  Publisher
	checkChain *chain.Chain
	channels   channel.ChannelFactory
	txr        txRunner
}

// NewSeckillService 接收依赖，返回 SeckillService 接口。
func NewSeckillService(d SeckillServiceDeps) SeckillService {
	txr := d.TxRunner
	if txr == nil {
		txr = defaultTxRunner{}
	}
	return &baseSeckillService{
		bb:         d.BlindBoxRepo,
		cp:         d.CardPoolRepo,
		card:       d.CardRepo,
		sup:        d.SupplierRepo,
		user:       d.UserRepo,
		order:      d.DrawOrderRepo,
		settlement: d.SettlementRepo,
		seckill:    d.SeckillRepo,
		cache:      d.Cache,
		publisher:  d.Publisher,
		checkChain: d.CheckChain,
		channels:   d.Channels,
		txr:        txr,
	}
}

// DrawSeckill 实现秒杀抽卡事务（spec Requirement "Seckill draw flow"）。
//
// 步骤对应 D15 流程；事务内复用 Draw() 的 D3 抽卡算法（D3 + 限购 + 价格）。
// 失败回滚：Redis INCR stock + DECR user_bought（即使事务已 ROLLBACK）。
//
// 责任链校验（task 15 / design D20）：事务前先跑 SeckillDrawChain
// （user_active + blindbox_buyable + time_window + balance），任一失败 fast return。
// user_limit 走 draw_check.AcquireUserLimit（Redis INCR + 阈值比较 + 自拒回滚），
// 不进 chain（带副作用，详见 user_limit_check.go 包注释）。
func (s *baseSeckillService) DrawSeckill(c *gin.Context, userID, seckillID int64) (*DrawResult, error) {
	if userID <= 0 || seckillID <= 0 {
		return nil, ErrSeckillNotAvailable
	}

	// --- 1. 责任链校验（事务外；替换原有内联 pre-check） ---
	//    chain 会拉 seckill + blindbox + user 完成校验；这里先 fetch 一次拿到
	//    BlindBoxID + PerUserLimit 用于下面的 Redis 限购 + 库存预扣。
	sec, err := s.seckill.GetByID(c, seckillID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrSeckillNotAvailable
		}
		return nil, err
	}
	if sec == nil {
		return nil, ErrSeckillNotAvailable
	}

	if s.checkChain != nil {
		input := &chain.DrawInput{
			Source:     chain.SourceSeckill,
			UserID:     userID,
			SeckillID:  seckillID,
			BlindBoxID: sec.BlindBoxID, // 供 blindbox_buyable_check 用
			Now:        time.Now(),
		}
		if err := s.checkChain.Validate(c, input); err != nil {
			return nil, err
		}
	}

	// --- 2. 限购校验（Redis INCR + 比较 + 必要 DECR 回滚） ---
	//    user_limit 不进 chain（带副作用，详见 draw_check.AcquireUserLimit）。
	ctx := c.Request.Context()
	userBoughtKey := seckillRedisUserBoughtKey + strconv.FormatInt(sec.ID, 10) + ":" + strconv.FormatInt(userID, 10)
	stockKey := seckillRedisStockKey + strconv.FormatInt(sec.ID, 10)

	if err := draw_check.AcquireUserLimit(ctx, s.cache, sec.ID, userID, sec.PerUserLimit); err != nil {
		return nil, err
	}

	// --- 3. Redis 库存预扣（DECR + 必要时 INCR 回滚） ---
	remaining, err := s.cache.Decr(ctx, stockKey)
	if err != nil {
		// Redis 异常，回滚限购计数。
		_, _ = s.cache.Decr(ctx, userBoughtKey)
		return nil, err
	}
	if remaining < 0 {
		// 名额被抢光，回滚库存 + 限购。
		_, _ = s.cache.Incr(ctx, stockKey)
		_, _ = s.cache.Decr(ctx, userBoughtKey)
		return nil, ErrSoldOut
	}

	// --- 4. MySQL 事务（d3 + stock_deduction_log + 结算副作用） ---
	var result *DrawResult
	txErr := s.txr.Run(c, func(txCtx *gin.Context) error {
		// 4.a 校验盲盒 + 供应商 + 用户（防御性二次校验，防秒杀创建后被 admin 下架）。
		bb, err := s.bb.GetByID(txCtx, sec.BlindBoxID)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrBlindBoxNotAvailable
			}
			return err
		}
		if bb == nil || bb.Status != mall.StatusActive || !bb.OnSale {
			return ErrBlindBoxNotAvailable
		}
		sup, err := s.sup.GetByID(txCtx, bb.SupplierID)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrBlindBoxNotAvailable
			}
			return err
		}
		if sup == nil || sup.Status == mall.StatusDisabled {
			return ErrBlindBoxNotAvailable
		}
		u, err := s.user.GetByID(txCtx, userID)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrUserNotAvailable
			}
			return err
		}
		if u == nil || u.Status != 1 {
			return ErrUserNotAvailable
		}

		// 4.b 拿卡池 + 抽卡（复用 D3 算法）。
		pool, err := s.cp.GetPoolByBlindBoxID(txCtx, bb.ID)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrPoolNotFound
			}
			return err
		}
		items, err := s.cp.ListItemsForUpdateByPoolID(txCtx, pool.ID)
		if err != nil {
			return err
		}
		if len(items) == 0 {
			return ErrSoldOut
		}
		chosenItem, err := pickWeightedItemWithRetry(txCtx, s.cp, pool.ID, items)
		if err != nil {
			return err
		}
		cardRow, err := s.card.GetByID(txCtx, chosenItem.CardID)
		if err != nil {
			return err
		}
		drawn := DrawnCard{
			ItemID:        chosenItem.ID,
			CardID:        chosenItem.CardID,
			Rarity:        chosenItem.Rarity,
			SnapshotName:  cardRow.Name,
			SnapshotImage: cardRow.Image,
		}

		// 4.f 条件扣减用户余额（秒杀价 seckill_price；影响 0 行 → ErrInsufficientBalance）。
		if err := s.user.DecrementBalance(txCtx, userID, sec.SeckillPrice); err != nil {
			if errors.Is(err, userRepo.ErrInsufficientBalance) {
				return ErrInsufficientBalance
			}
			return err
		}

		// 4.g 增加供应商余额 + 销售额。
		if err := s.sup.UpdateBalanceAndSales(txCtx, bb.SupplierID, sec.SeckillPrice); err != nil {
			return err
		}

		// 4.g+ 累加 platform ledger 的 total_revenue（spec 8.5）。
		if err := s.settlement.IncrementLedger(txCtx, "total_revenue", sec.SeckillPrice); err != nil {
			return err
		}

		// 4.d 落订单（source='seckill'；初始 status=pending，由状态机推到 paid/drawn）。
		orderNo, err := newOrderNo()
		if err != nil {
			return err
		}
		order := &mall.MallDrawOrder{
			OrderNo:    orderNo,
			UserID:     userID,
			BlindBoxID: bb.ID,
			SupplierID: bb.SupplierID,
			Price:      sec.SeckillPrice,
			Status:     mall.OrderStatusPending,
			Source:     mall.SourceSeckill,
		}
		if err := s.order.CreateOrder(txCtx, order); err != nil {
			return err
		}

		// 状态机推进 pending → paid（同 Draw()，事务 commit 前）。
		if _, err := state.Transition(state.DrawOrderStates.Pending, state.DrawOrderStates.Paid, &state.Context{Entity: order}); err != nil {
			return err
		}
		if err := s.order.UpdateStatus(txCtx, order.ID, mall.OrderStatusPaid); err != nil {
			return err
		}

		// 4.e 落订单明细（snapshot_*）。
		orderItems := []mall.MallDrawOrderItem{
			{
				OrderID:       order.ID,
				CardID:        drawn.CardID,
				Rarity:        drawn.Rarity,
				SnapshotName:  drawn.SnapshotName,
				SnapshotImage: drawn.SnapshotImage,
			},
		}
		if err := s.order.CreateItems(txCtx, orderItems); err != nil {
			return err
		}

		// 状态机推进 paid → drawn（同 Draw()，步骤 9 后）。
		if _, err := state.Transition(state.DrawOrderStates.Paid, state.DrawOrderStates.Drawn, &state.Context{Entity: order}); err != nil {
			return err
		}
		if err := s.order.UpdateStatus(txCtx, order.ID, mall.OrderStatusDrawn); err != nil {
			return err
		}

		// 4.c 写 stock_deduction_log（cmd/stock-sync 异步对账的源头）。
		log := &mall.MallStockDeductionLog{
			SeckillID:      sec.ID,
			UserID:         userID,
			BlindBoxID:     bb.ID,
			CardID:         drawn.CardID,
			Rarity:         drawn.Rarity,
			SnapshotName:   drawn.SnapshotName,
			SnapshotImage:  drawn.SnapshotImage,
			DeductedAt:     time.Now(),
			SyncedToPoolAt: nil, // NULL = 未同步；stock-sync 后回填
		}
		if err := s.seckill.InsertDeductionLog(txCtx, log); err != nil {
			return err
		}

		result = &DrawResult{
			OrderID:     order.ID,
			OrderNo:     orderNo,
			ActualPrice: sec.SeckillPrice,
			Cards:       []DrawnCard{drawn},
		}
		return nil
	})

	if txErr != nil {
		// 失败回滚 Redis 计数（即使 txErr != nil，事务已 ROLLBACK）。
		_, _ = s.cache.Incr(ctx, stockKey)
		_, _ = s.cache.Decr(ctx, userBoughtKey)
		return nil, txErr
	}

	// 异步 publish MQ 通知 cmd/stock-sync 标记 synced_to_pool_at。
	//
	// 为什么放在事务外 + goroutine（design D17 §秒杀路径集成）：
	//   - 事务内同步 publish 增加事务持有时间，秒杀高并发下放大延迟；
	//   - MQ 失败不应阻塞业务主路径（事务已 COMMIT，库存已落库）；
	//   - cron 兜底（cmd/stock-sync 模式 both）能追平 publish 失败。
	//
	// publisher 为 nil 时跳过（兼容旧单测 / 不强依赖 MQ）。
	// payload 只含 LogID（cmd/stock-sync 端只需 ID 即可走 MarkDeductionLogSynced）。
	if s.publisher != nil {
		go func() {
			payload, err := json.Marshal(stockDeductionPayload{ID: result.OrderID})
			if err != nil {
				log.Printf("user.draw: marshal stock deduction payload failed: %v", err)
				return
			}
			if err := s.publisher.Publish(context.Background(), topicStockDeductionSync, payload); err != nil {
				log.Printf("user.draw: publish stock deduction sync failed (will rely on cron fallback): %v", err)
			}
		}()
	}

	// 渠道层记录模拟支付（D18；事务外，失败仅 warn，见 channel_hooks.go）。
	recordChannelPayment(s.channels, c.Request.Context(), userID, result, "秒杀抽卡")

	return result, nil
}

// 编译期断言：baseSeckillService 必须实现 SeckillService。
var _ SeckillService = (*baseSeckillService)(nil)

// 避免 context / model / fmt 包被 unused 编译器警告。
var (
	_ = context.Background
	_ = (*model.User)(nil)
	_ = fmt.Sprintf
)
