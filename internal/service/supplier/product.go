// Package supplier — product.go 实现 B 端供应商对自家盲盒的增删改查。
//
// 业务规则（spec Requirement "Supplier product management"）：
//   - List：仅返回 supplier_id=当前 supplier 的盲盒，按 id DESC。
//   - Create：单事务内同时建盲盒 + 卡池 + 卡池 items；items.weight 加和 MUST = 10000（D2 关键约束）。
//   - Update：仅允许修改「自家盲盒」（盲盒.SupplierID == supplierID）；成功后失效 C 端详情缓存。
//   - ToggleOnSale：同上 ownership 自检；用于「快速上下架」。
//
// 与 service/user.Draw 共用 txRunner 抽象：tests 可注入 mockTxRunner 同步跑 fn，
// 无需真 DB 也能验证事务内调用是否被正确顺序触发。
package supplier

import (
	"errors"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"ai-go-mall/internal/infra/cache"
	"ai-go-mall/internal/infra/cache/hotspot"
	"ai-go-mall/internal/model/mall"
	"ai-go-mall/internal/repository"
	mallRepo "ai-go-mall/internal/repository/mall"
	userSvc "ai-go-mall/internal/service/user"
)

// 业务错误。
var (
	// ErrBlindBoxNotFound 盲盒不可见（不存在 / 已软删）。
	ErrBlindBoxNotFound = errors.New("supplier.product: blindbox not found")

	// ErrBlindBoxForbidden 越权 —— 盲盒不属于当前 supplier。
	//
	// 越权读 / 改写他人盲盒 → 一律 ErrBlindBoxForbidden；handler 映射 403。
	// 与 service/user.Detail 的「404 模糊存在性」策略相反：
	// 供应商后台是登录态，调用方已知自身 ID，「越权」是可识别的。
	ErrBlindBoxForbidden = errors.New("supplier.product: blindbox not owned by current supplier")

	// ErrInvalidWeightSum 卡池 items 的 weight 加和 ≠ 10000（D2 关键约束）。
	ErrInvalidWeightSum = errors.New("supplier.product: card pool items weight sum must equal 10000")

	// ErrEmptyPool 卡池 items 为空。
	ErrEmptyPool = errors.New("supplier.product: card pool must have at least one item")
)

// 业务常量。
const (
	// weightTotal 卡池 items.weight 加和阈值（设计 D2：10000）。
	weightTotal = 10000
)

// txRunner 复用 service/user.Draw 同样的抽象：transaction 执行的最小抽象。
//
// *repository 默认实现走 repository.RunInTx；测试可注入 mock。
type txRunner interface {
	Run(c *gin.Context, fn func(txCtx *gin.Context) error) error
}

// ProductService 是 B 端供应商的盲盒管理接口。
type ProductService interface {
	// List 拉当前 supplier 的盲盒（按 id DESC）。
	List(c *gin.Context, supplierID int64, opts repository.ListOptions) (items []mall.MallBlindBox, total int64, err error)

	// Create 单事务内建盲盒 + 卡池 + 卡池 items。
	//
	// 业务规则：
	//  - name 必须非空（前端兜底校验，本处双保险）；
	//  - items 必须非空（ErrEmptyPool）；
	//  - items.weight 加和 MUST == 10000（ErrInvalidWeightSum）；
	//  - 成功返回新建 blindbox（含 ID）；
	//  - 事务内任一步失败 → 全部回滚。
	Create(c *gin.Context, supplierID int64, blindBox *mall.MallBlindBox, items []mall.MallCardPoolItem) (*mall.MallBlindBox, error)

	// Update 只改「传入的字段」；nil 字段保持 DB 原值，成功后失效 C 端详情缓存。
	//
	// 业务规则：
	//  - blindBoxID 不允许为空 / 0（ErrBlindBoxNotFound）；
	//  - 盲盒.SupplierID MUST == supplierID（ErrBlindBoxForbidden）；
	//  - 不修改 supplier_id / status / is_featured 字段（这些是 admin 的领域）。
	//
	// 为什么入参是 patch 而不是整个 *MallBlindBox：仓储的 Update 是 db.Save
	// （**全列写**），交给它的一定得是完整行；而 handler 只拿得到「客户端传了
	// 哪些字段」（指针），其余字段在结构体里是 Go 零值。把那种半成品直接 Save
	// 会把未传字段清零，created_at 归零还会直接撞 MySQL 1292 —— 本接口曾因此
	// 在「只改价格」时恒定 500（handler/supplier/edit_integration_test.go 锁定）。
	Update(c *gin.Context, supplierID, blindBoxID int64, patch ProductPatch) error

	// ToggleOnSale 切换在售状态（快速上下架）。
	//
	// 业务规则同 Update：ownership 自检；成功后失效详情缓存。
	ToggleOnSale(c *gin.Context, supplierID, blindBoxID int64, onSale bool) error
}

// ProductPatch 是商品编辑的「只改传入字段」入参：nil = 该字段不改。
//
// 字段与 handler 的 EditRequest 指针一一对应；由 service 在 DB 原行上合并，
// 合并结果才交给仓储（db.Save 全列写）。
type ProductPatch struct {
	Name        *string
	Cover       *string
	Price       *float64
	Description *string
}

// ProductServiceDeps 注入 ProductService 所需的依赖。
type ProductServiceDeps struct {
	BlindBoxRepo mallRepo.BlindBoxRepository
	CardPoolRepo mallRepo.CardPoolRepository
	Cache        cache.Cache          // 用于 Update / ToggleOnSale 失效 C 端详情缓存；nil 时跳过
	Hotspot      hotspot.HotspotCache // D21：写路径同时失效热点详情；nil 时跳过
	TxRunner     txRunner             // 可选；nil 时走 repository.RunInTx
}

// baseProductService 是 ProductService 的默认实现。
type baseProductService struct {
	bb      mallRepo.BlindBoxRepository
	cp      mallRepo.CardPoolRepository
	cache   cache.Cache
	hotspot hotspot.HotspotCache
	txr     txRunner
}

// NewProductService 接收依赖，返回 ProductService 接口。
func NewProductService(d ProductServiceDeps) ProductService {
	txr := d.TxRunner
	if txr == nil {
		txr = defaultTxRunner{}
	}
	return &baseProductService{
		bb:      d.BlindBoxRepo,
		cp:      d.CardPoolRepo,
		cache:   d.Cache,
		hotspot: d.Hotspot,
		txr:     txr,
	}
}

// defaultTxRunner 走 repository.RunInTx（生产路径）。
type defaultTxRunner struct{}

func (defaultTxRunner) Run(c *gin.Context, fn func(txCtx *gin.Context) error) error {
	return repository.RunInTx(c, fn)
}

// List 实现见 ProductService 注释。
func (s *baseProductService) List(c *gin.Context, supplierID int64, opts repository.ListOptions) ([]mall.MallBlindBox, int64, error) {
	if supplierID <= 0 {
		return []mall.MallBlindBox{}, 0, nil
	}
	return s.bb.ListBySupplier(c, supplierID, opts)
}

// Create 实现见 ProductService 注释。
func (s *baseProductService) Create(c *gin.Context, supplierID int64, blindBox *mall.MallBlindBox, items []mall.MallCardPoolItem) (*mall.MallBlindBox, error) {
	if supplierID <= 0 {
		return nil, ErrBlindBoxForbidden
	}
	if blindBox == nil {
		return nil, ErrBlindBoxNotFound
	}
	if blindBox.Name == "" {
		return nil, ErrBlindBoxNotFound // 复用「数据无效」错误（handler 映射 422）
	}
	if len(items) == 0 {
		return nil, ErrEmptyPool
	}

	// D2 校验：items.weight 加和 == 10000。
	var totalWeight int64
	for _, it := range items {
		totalWeight += int64(it.Weight)
	}
	if totalWeight != weightTotal {
		return nil, ErrInvalidWeightSum
	}

	// 强制赋 supplierID = 当前 supplier（防越权赋值）。
	blindBox.SupplierID = supplierID

	// 事务内：INSERT blind_box → INSERT pool → INSERT items。
	err := s.txr.Run(c, func(txCtx *gin.Context) error {
		if err := s.bb.Create(txCtx, blindBox); err != nil {
			return err
		}
		pool := &mall.MallCardPool{BlindBoxID: blindBox.ID}
		if err := s.cp.CreatePool(txCtx, pool); err != nil {
			return err
		}
		// 回填 items.PoolID。
		for i := range items {
			items[i].PoolID = pool.ID
		}
		return s.cp.CreateItems(txCtx, items)
	})
	if err != nil {
		return nil, err
	}
	return blindBox, nil
}

// Update 实现见 ProductService 注释。
func (s *baseProductService) Update(c *gin.Context, supplierID, blindBoxID int64, patch ProductPatch) error {
	if supplierID <= 0 {
		return ErrBlindBoxForbidden
	}
	if blindBoxID <= 0 {
		return ErrBlindBoxNotFound
	}

	// Ownership 自检：先查到原行，比对 SupplierID；原行同时作为字段合并的基准。
	existing, err := s.bb.GetByID(c, blindBoxID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrBlindBoxNotFound
		}
		return err
	}
	if existing == nil {
		return ErrBlindBoxNotFound
	}
	if existing.SupplierID != supplierID {
		return ErrBlindBoxForbidden
	}

	// 只覆盖传入的字段（nil = 不改）。supplier_id / status / is_featured 天然保持
	// 原值（admin 的领域），created_at / updated_at 也来自原行 —— 交给仓储的
	// 因此是完整行，db.Save 不会再把未传字段写成零值。
	if patch.Name != nil {
		existing.Name = *patch.Name
	}
	if patch.Cover != nil {
		existing.Cover = *patch.Cover
	}
	if patch.Price != nil {
		existing.Price = *patch.Price
	}
	if patch.Description != nil {
		existing.Description = *patch.Description
	}

	if err := s.bb.Update(c, existing); err != nil {
		return err
	}

	// 失效 C 端详情缓存（best-effort；失败不阻塞主流程）。
	if s.cache != nil {
		_ = userSvc.InvalidateDetail(c.Request.Context(), s.cache, s.hotspot, existing.ID)
	}
	return nil
}

// ToggleOnSale 实现见 ProductService 注释。
func (s *baseProductService) ToggleOnSale(c *gin.Context, supplierID, blindBoxID int64, onSale bool) error {
	if supplierID <= 0 {
		return ErrBlindBoxForbidden
	}
	if blindBoxID <= 0 {
		return ErrBlindBoxNotFound
	}

	// Ownership 自检（同 Update）。
	existing, err := s.bb.GetByID(c, blindBoxID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrBlindBoxNotFound
		}
		return err
	}
	if existing == nil {
		return ErrBlindBoxNotFound
	}
	if existing.SupplierID != supplierID {
		return ErrBlindBoxForbidden
	}

	if err := s.bb.ToggleOnSale(c, blindBoxID, onSale); err != nil {
		return err
	}

	if s.cache != nil {
		_ = userSvc.InvalidateDetail(c.Request.Context(), s.cache, s.hotspot, blindBoxID)
	}
	return nil
}

// 编译期断言：baseProductService 必须实现 ProductService。
var _ ProductService = (*baseProductService)(nil)
