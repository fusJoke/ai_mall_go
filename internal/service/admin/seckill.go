// Package admin — seckill.go 实现 admin 视角的秒杀活动管理。
//
// MVP 范围（任务 9.8）：
//   - List 全量分页（无 supplier_id 过滤；与 supplier.serveckill 按自家视角
//     过滤形成对照）。
//   - Create：admin 可代供应商创建（请求体携带 supplier_id）；同 supplier 版
//     一套（Redis 库存 init + 卡池库存校验 + 时间窗校验）。
//   - Toggle：admin 可强制启停任意秒杀活动，不做 ownership 校验
//     （admin 视角 = 全量管理权）。
//
// 与 supplier.SeckillService 的差异：
//   - List 无 supplier_id 过滤；
//   - Create 入参 sec0.SupplierID 由请求体指定（不是从登录态透出）；
//   - Toggle 无 supplierID 参数（admin 视角不需要 ownership check）。
package admin

import (
	"context"
	"errors"
	"strconv"

	"github.com/gin-gonic/gin"

	"ai-go-mall/internal/infra/cache"
	"ai-go-mall/internal/model/mall"
	"ai-go-mall/internal/repository"
	mallRepo "ai-go-mall/internal/repository/mall"
)

// 业务错误。
var (
	// ErrSeckillSupplierIDRequired admin Create 时 sec0.SupplierID <= 0。
	ErrSeckillSupplierIDRequired = errors.New("admin.seckill: supplier_id required")

	// ErrSeckillNotFound admin Toggle 时 id 不存在。
	ErrSeckillNotFound = errors.New("admin.seckill: not found")
)

// SeckillService 是 admin 视角的秒杀管理接口。
type SeckillService interface {
	// List 全量分页（按 start_at DESC）。
	List(c *gin.Context, opts repository.ListOptions) ([]mall.MallSeckillActivity, int64, error)

	// Create 由 admin 代供应商创建（请求体携带 supplier_id）。
	//
	// 复用 supplier 服务的校验规则（时间窗 / seckill_price / 卡池库存 / Redis init），
	// 仅入参形态不同 —— admin 提供 SupplierID，supplier 版从登录态注入。
	Create(c *gin.Context, sec0 *mall.MallSeckillActivity) (*mall.MallSeckillActivity, error)

	// Toggle 启停任意秒杀活动（admin 视角无 supplier 归属约束）。
	Toggle(c *gin.Context, seckillID int64) error
}

// SeckillServiceDeps 注入 SeckillService 所需的依赖。
type SeckillServiceDeps struct {
	SeckillRepo  mallRepo.SeckillRepository
	BlindBoxRepo mallRepo.BlindBoxRepository
	Cache        cache.Cache // 用于 Create 时 Redis 库存 init
}

// baseSeckillService 是 SeckillService 的默认实现。
type baseSeckillService struct {
	repo  mallRepo.SeckillRepository
	bb    mallRepo.BlindBoxRepository
	cache cache.Cache
}

// NewSeckillService 接收依赖，返回 SeckillService 接口。
func NewSeckillService(d SeckillServiceDeps) SeckillService {
	return &baseSeckillService{
		repo:  d.SeckillRepo,
		bb:    d.BlindBoxRepo,
		cache: d.Cache,
	}
}

// List 实现见 SeckillService 注释。
func (s *baseSeckillService) List(c *gin.Context, opts repository.ListOptions) ([]mall.MallSeckillActivity, int64, error) {
	return s.repo.List(c, opts)
}

// Create 实现见 SeckillService 注释。
//
// 与 supplier 版的差异点：
//   - 入参 SupplierID 由 sec0 携带（admin 视角指定供应商）；
//   - 校验 supplierID > 0 + 关联盲盒归属于该 supplier；
//   - 时间窗 / 价格 / 卡池校验沿用 supplier 规则（详见 supplier.seckill.go）。
func (s *baseSeckillService) Create(c *gin.Context, sec0 *mall.MallSeckillActivity) (*mall.MallSeckillActivity, error) {
	if sec0 == nil || sec0.SupplierID <= 0 {
		return nil, ErrSeckillSupplierIDRequired
	}

	// 复用 supplier 的核心校验 + Redis init 流程（封装在共享内部 helper 的话会让包依赖变得复杂，
	// 这里直接 inline：代码 ~30 行，与 supplier 版本差距仅入参形态）。
	//
	// 复用 supplier helper 暂不抽取：当前 admin 与 supplier 的 seckill 流程未来还可能加
	// "admin 可以为任意 supplier 设置 featured" 等管理差异，强制共享反而限制扩展。

	// 1. 时间窗合法性（admin 版本也强制 start_at > end_at）。
	if !sec0.EndAt.After(sec0.StartAt) {
		return nil, errors.New("admin.seckill: invalid time window")
	}

	// 2. 校验 BlindBox 归属 + 秒杀价 < 原价。
	bb, err := s.bb.GetByID(c, sec0.BlindBoxID)
	if err != nil {
		return nil, err
	}
	if bb == nil || bb.SupplierID != sec0.SupplierID {
		return nil, errors.New("admin.seckill: blindbox not owned by supplier")
	}
	if sec0.SeckillPrice <= 0 || sec0.SeckillPrice >= bb.Price {
		return nil, errors.New("admin.seckill: invalid seckill_price")
	}

	// 3. 卡池库存校验。
	poolStock, err := s.repo.SumPoolStock(c, sec0.BlindBoxID)
	if err != nil {
		return nil, err
	}
	if sec0.TotalStock <= 0 || sec0.TotalStock > poolStock {
		return nil, errors.New("admin.seckill: total_stock exceeds card pool stock")
	}

	// 4. INSERT + Redis SETNX + MarkRedisInitialized（与 supplier 版同模式）。
	sec0.RedisInitialized = false
	if _, err := s.repo.Create(c, sec0); err != nil {
		return nil, err
	}
	if s.cache != nil {
		ctx := c.Request.Context()
		key := "seckill:stock:" + strconv.FormatInt(sec0.ID, 10)
		ok, err := s.cache.SetNX(ctx, key, strconv.Itoa(sec0.TotalStock), 0)
		if err != nil || !ok {
			return nil, errors.New("admin.seckill: redis stock init failed")
		}
		if err := s.repo.MarkRedisInitialized(c, sec0.ID); err != nil {
			return nil, err
		}
		sec0.RedisInitialized = true
	}
	return sec0, nil
}

// Toggle 实现见 SeckillService 注释（admin 视角无 ownership 校验）。
func (s *baseSeckillService) Toggle(c *gin.Context, seckillID int64) error {
	if seckillID <= 0 {
		return ErrSeckillNotFound
	}
	existing, err := s.repo.GetByID(c, seckillID)
	if err != nil {
		return ErrSeckillNotFound
	}
	if existing == nil {
		return ErrSeckillNotFound
	}
	newStatus := mall.StatusActive
	if existing.Status == mall.StatusActive {
		newStatus = mall.StatusDisabled
	}
	return s.repo.ToggleStatus(c, seckillID, newStatus)
}

// 编译期断言：baseSeckillService 必须实现 SeckillService。
var _ SeckillService = (*baseSeckillService)(nil)

// 避免 context 包被 unused 编译器警告（cache 上层路径用 ctx）。
var _ = context.Background
