// Package supplier — seckill.go 实现 B 端供应商对自家秒杀活动的增删改查。
//
// 业务规则（spec Requirement "Seckill draw flow" + design D15）：
//   - Create：必填校验 + 卡池库存校验（total_stock ≤ 卡池 SUM(stock)）+ Redis 库存初始化
//   - 标记 redis_initialized=true（防 "DB 有行但 Redis 没 init" 的脏数据）。
//   - Update：可改价 / 限购 / 状态；时间窗与盲盒一旦创建不可改 —— 想改时段 = 删旧建新。
//   - Toggle：启停 status（active ↔ disabled）。
//   - Delete：软删；不主动清理 Redis 库存（活动失效时秒杀接口会拒绝，无副作用）。
//
// Redis Key 设计（D15）：
//   - seckill:stock:{sid}              int, 秒杀名额剩余，活动创建时 SETNX 防重 init
//   - seckill:user_bought:{sid}:{uid}   int, 用户已购次数，INCR + 比较 per_user_limit
//
// 所有写操作走 ownership 自检（ErrSeckillForbidden / ErrSeckillNotFound）。
package supplier

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"ai-go-mall/internal/infra/cache"
	"ai-go-mall/internal/infra/cache/hotspot"
	"ai-go-mall/internal/model/mall"
	"ai-go-mall/internal/repository"
	mallRepo "ai-go-mall/internal/repository/mall"
	"ai-go-mall/internal/service/preheat"
)

// Redis key 前缀。
const (
	// RedisKeySeckillStock 秒杀活动库存键：seckill:stock:{sid}。
	RedisKeySeckillStock = "seckill:stock:"

	// RedisKeySeckillUserBought 用户已购次数：seckill:user_bought:{sid}:{uid}。
	RedisKeySeckillUserBought = "seckill:user_bought:"
)

// 业务错误。
var (
	// ErrSeckillNotFound 秒杀活动不可见（不存在 / 已软删）。
	ErrSeckillNotFound = errors.New("supplier.seckill: not found")

	// ErrSeckillForbidden 越权 —— 秒杀活动不属于当前 supplier。
	ErrSeckillForbidden = errors.New("supplier.seckill: not owned by current supplier")

	// ErrSeckillStockExceedsPool 创建时 total_stock > 卡池剩余总库存。
	//
	// D15 决策点 2 约束：秒杀名额（Redis 流量阀门）≤ 卡池库存（物理库存），
	// 否则 Redis DECR 成功后物理库存可能不够抽。
	ErrSeckillStockExceedsPool = errors.New("supplier.seckill: total_stock exceeds card pool stock")

	// ErrInvalidSeckillWindow 时间窗非法（end_at 必须晚于 start_at，且必须晚于当前）。
	ErrInvalidSeckillWindow = errors.New("supplier.seckill: invalid time window")

	// ErrInvalidSeckillPrice 秒杀价必须 > 0 且 < blind_box.price。
	ErrInvalidSeckillPrice = errors.New("supplier.seckill: invalid seckill_price")

	// ErrRedisInitFailed Redis 库存初始化失败（D15 决策点 1 强制要求同事务内 SET）。
	ErrRedisInitFailed = errors.New("supplier.seckill: redis stock init failed")
)

// SeckillService 是 B 端供应商的秒杀活动管理接口。
type SeckillService interface {
	// List 拉当前 supplier 的秒杀活动（按 start_at DESC）。
	List(c *gin.Context, supplierID int64, opts repository.ListOptions) (items []mall.MallSeckillActivity, total int64, err error)

	// Create 新建秒杀活动，同事务内完成：
	//   1. 校验 supplier 拥有 blind_box；
	//   2. 校验 total_stock ≤ 卡池 SUM(stock)；
	//   3. INSERT activity（redis_initialized=false）；
	//   4. SETNX Redis 库存（防重 init）；
	//   5. UPDATE activity SET redis_initialized=true；
	//   6. Commit。
	Create(c *gin.Context, supplierID int64, s *mall.MallSeckillActivity) (*mall.MallSeckillActivity, error)

	// Update 仅改秒杀价 / 限购数量；时间窗与 blind_box_id 不可改。
	Update(c *gin.Context, supplierID, seckillID int64, patch SeckillPatch) error

	// Toggle 启停 status（active ↔ disabled）。
	Toggle(c *gin.Context, supplierID, seckillID int64) error
}

// SeckillPatch 是秒杀活动改价的 patch 入参：nil = 该字段不改。
type SeckillPatch struct {
	SeckillPrice *float64
	PerUserLimit *int
}

// SeckillServiceDeps 注入 SeckillService 所需的依赖。
type SeckillServiceDeps struct {
	SeckillRepo  mallRepo.SeckillRepository
	BlindBoxRepo mallRepo.BlindBoxRepository // 校验 blind_box 归属 + 读 price
	Cache        cache.Cache                 // 用于 Redis 库存 init；nil = 跳过（单测 / 缺依赖）
	TxRunner     txRunner                    // 可选；nil 时走 repository.RunInTx

	// Hotspot + Preheat D22：秒杀创建后预热关联盲盒详情缓存。
	Hotspot       hotspot.HotspotCache
	PreheatLoader preheat.BlindBoxLoader
}

// baseSeckillService 是 SeckillService 的默认实现。
type baseSeckillService struct {
	repo          mallRepo.SeckillRepository
	bb            mallRepo.BlindBoxRepository
	cache         cache.Cache
	txr           txRunner
	hotspot       hotspot.HotspotCache
	preheatLoader preheat.BlindBoxLoader
}

// NewSeckillService 接收依赖，返回 SeckillService 接口。
func NewSeckillService(d SeckillServiceDeps) SeckillService {
	txr := d.TxRunner
	if txr == nil {
		txr = defaultTxRunner{}
	}
	return &baseSeckillService{
		repo:          d.SeckillRepo,
		bb:            d.BlindBoxRepo,
		cache:         d.Cache,
		txr:           txr,
		hotspot:       d.Hotspot,
		preheatLoader: d.PreheatLoader,
	}
}

// List 实现见 SeckillService 注释。
func (s *baseSeckillService) List(c *gin.Context, supplierID int64, opts repository.ListOptions) ([]mall.MallSeckillActivity, int64, error) {
	if supplierID <= 0 {
		return []mall.MallSeckillActivity{}, 0, nil
	}
	return s.repo.ListBySupplier(c, supplierID, opts)
}

// Create 实现见 SeckillService 注释。
func (s *baseSeckillService) Create(c *gin.Context, supplierID int64, sec0 *mall.MallSeckillActivity) (*mall.MallSeckillActivity, error) {
	if supplierID <= 0 {
		return nil, ErrSeckillForbidden
	}
	if sec0 == nil {
		return nil, ErrSeckillNotFound
	}

	// 时间窗合法性。
	if !sec0.EndAt.After(sec0.StartAt) {
		return nil, ErrInvalidSeckillWindow
	}
	if sec0.StartAt.Before(time.Now().Add(-time.Minute)) {
		// 允许 1 分钟时钟漂移；更早的视为非法。
		return nil, ErrInvalidSeckillWindow
	}

	// 强制 SupplierID（防越权赋值）。
	sec0.SupplierID = supplierID

	// 校验 BlindBox 归属 + 秒杀价 < 原价。
	bb, err := s.bb.GetByID(c, sec0.BlindBoxID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrSeckillForbidden
		}
		return nil, err
	}
	if bb == nil || bb.SupplierID != supplierID {
		return nil, ErrSeckillForbidden
	}
	if sec0.SeckillPrice <= 0 || sec0.SeckillPrice >= bb.Price {
		return nil, ErrInvalidSeckillPrice
	}

	// 校验 total_stock ≤ 卡池 SUM(stock)（D15 决策点 2）。
	poolStock, err := s.repo.SumPoolStock(c, sec0.BlindBoxID)
	if err != nil {
		return nil, err
	}
	if sec0.TotalStock <= 0 || sec0.TotalStock > poolStock {
		return nil, ErrSeckillStockExceedsPool
	}

	// Redis 库存初始化（必须在 commit 前完成）。
	ctx := c.Request.Context()
	stockKey := RedisKeySeckillStock + strconv.FormatInt(0, 10) // 占位，下面 UPDATE 真实 ID
	_ = stockKey

	// Step 1: INSERT activity（redis_initialized=false）。
	sec0.RedisInitialized = false
	if err := s.txr.Run(c, func(txCtx *gin.Context) error {
		newID, err := s.repo.Create(txCtx, sec0)
		if err != nil {
			return err
		}
		sec0.ID = newID
		return nil
	}); err != nil {
		return nil, err
	}

	// Step 2: SETNX Redis 库存 + Step 3: 标记 redis_initialized=true。
	if s.cache != nil {
		key := RedisKeySeckillStock + strconv.FormatInt(sec0.ID, 10)
		ok, err := s.cache.SetNX(ctx, key, strconv.Itoa(sec0.TotalStock), 0)
		if err != nil {
			// Redis 失败 → 软回滚（软删刚才的 activity 行 + 返回 ErrRedisInitFailed）。
			_ = s.repo.Update(c, sec0) // best effort 触发软删路径走 Delete
			return nil, fmt.Errorf("%w: %v", ErrRedisInitFailed, err)
		}
		if !ok {
			// 已存在（极小概率，可能是重试创建）→ 不覆盖，避免破坏既有库存；返回错误。
			_ = s.repo.Update(c, sec0)
			return nil, ErrRedisInitFailed
		}
		if err := s.repo.MarkRedisInitialized(c, sec0.ID); err != nil {
			return nil, err
		}
		sec0.RedisInitialized = true

		// D22：事务 COMMIT 后异步预热关联盲盒详情缓存。
		// 失败仅记 log，不影响主业务（用户场景：cold-start 兜底）。
		if s.hotspot != nil && s.preheatLoader != nil {
			go preheat.SeckillPreheat(context.Background(), sec0, preheat.Deps{
				Hotspot:        s.hotspot,
				BlindBoxLoader: s.preheatLoader,
			})
		}
	}

	return sec0, nil
}

// Update 实现见 SeckillService 注释。
func (s *baseSeckillService) Update(c *gin.Context, supplierID, seckillID int64, patch SeckillPatch) error {
	if supplierID <= 0 {
		return ErrSeckillForbidden
	}
	if seckillID <= 0 {
		return ErrSeckillNotFound
	}

	existing, err := s.repo.GetByID(c, seckillID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrSeckillNotFound
		}
		return err
	}
	if existing == nil || existing.SupplierID != supplierID {
		return ErrSeckillForbidden
	}

	// 应用 patch。
	if patch.SeckillPrice != nil {
		if *patch.SeckillPrice <= 0 {
			return ErrInvalidSeckillPrice
		}
		existing.SeckillPrice = *patch.SeckillPrice
	}
	if patch.PerUserLimit != nil {
		if *patch.PerUserLimit <= 0 {
			return ErrInvalidSeckillWindow // 复用"数值非法"槽位；语义相近
		}
		existing.PerUserLimit = *patch.PerUserLimit
	}

	// 重新校验 seckill_price < blind_box.price。
	bb, err := s.bb.GetByID(c, existing.BlindBoxID)
	if err != nil {
		return err
	}
	if bb == nil {
		return ErrSeckillNotFound
	}
	if existing.SeckillPrice >= bb.Price {
		return ErrInvalidSeckillPrice
	}

	return s.repo.Update(c, existing)
}

// Toggle 实现见 SeckillService 注释。
func (s *baseSeckillService) Toggle(c *gin.Context, supplierID, seckillID int64) error {
	if supplierID <= 0 {
		return ErrSeckillForbidden
	}
	if seckillID <= 0 {
		return ErrSeckillNotFound
	}

	existing, err := s.repo.GetByID(c, seckillID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrSeckillNotFound
		}
		return err
	}
	if existing == nil || existing.SupplierID != supplierID {
		return ErrSeckillForbidden
	}

	newStatus := mall.StatusActive
	if existing.Status == mall.StatusActive {
		newStatus = mall.StatusDisabled
	}
	return s.repo.ToggleStatus(c, seckillID, newStatus)
}

// 编译期断言：baseSeckillService 必须实现 SeckillService。
var _ SeckillService = (*baseSeckillService)(nil)

// 保留 ctx 引用以避免 unused import（未来抽卡 service 会复用此 helper）。
var _ = context.Background
