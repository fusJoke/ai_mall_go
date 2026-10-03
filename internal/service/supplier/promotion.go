// Package supplier — promotion.go 实现 B 端供应商对自家限时特价活动的增删改查。
//
// 业务规则（spec Requirement "Supplier promotion management" + design D10）：
//   - List：仅返回 supplier_id=当前 supplier 的活动，按 start_at DESC。
//   - Create：必填校验 + 时间重叠校验（同一盲盒同时间至多 1 个生效活动，D10 关键约束）。
//   - Update：仅允许修改价格字段（PromoPrice / OriginalPrice）；时间窗 / blind_box_id / supplier_id
//     不可改 —— 想改时段 = 删旧建新。
//   - Toggle：status 切换（active ↔ disabled）；同 blindbox 验证 ownership。
//   - Delete：软删；成功后失效 C 端详情缓存。
//
// 所有写操作走 ownership 自检：promo.SupplierID MUST == currentSupplierID，
// 否则 ErrPromotionForbidden。供应商后台是登录态，越权可识别，不需要「404 模糊」。
package supplier

import (
	"context"
	"errors"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"ai-go-mall/internal/infra/cache"
	"ai-go-mall/internal/infra/cache/hotspot"
	"ai-go-mall/internal/model/mall"
	"ai-go-mall/internal/repository"
	mallRepo "ai-go-mall/internal/repository/mall"
	"ai-go-mall/internal/service/preheat"
	userSvc "ai-go-mall/internal/service/user"
)

// 业务错误。
var (
	// ErrPromotionNotFound 活动不可见（不存在 / 已软删）。
	ErrPromotionNotFound = errors.New("supplier.promotion: not found")

	// ErrPromotionForbidden 越权 —— 活动不属于当前 supplier。
	ErrPromotionForbidden = errors.New("supplier.promotion: not owned by current supplier")

	// ErrPromotionTimeOverlap 新建活动与既有活动时间窗冲突（D10）。
	//
	// 同盲盒同时只允许 1 个生效活动 —— 由 UK (blind_box_id, start_at) 只能防
	// "完全重复插入"，本错误覆盖「时段重叠但起始时间不同」的场景。
	ErrPromotionTimeOverlap = errors.New("supplier.promotion: time window overlaps with existing")

	// ErrInvalidTimeWindow 时间窗非法（end_at <= start_at）。
	ErrInvalidTimeWindow = errors.New("supplier.promotion: end_at must be after start_at")

	// ErrInvalidPrice 活动价 >= 原价（活动价必须 < 原价才有意义）。
	ErrInvalidPrice = errors.New("supplier.promotion: promo_price must be less than original_price")
)

// PromotionService 是 B 端供应商的活动管理接口。
type PromotionService interface {
	// List 拉当前 supplier 的活动（按 start_at DESC）。
	List(c *gin.Context, supplierID int64, opts repository.ListOptions) (items []mall.MallPromotion, total int64, err error)

	// Create 新建活动（同时段冲突校验）。
	//
	// 业务规则：
	//  - promo.SupplierID 强制 = supplierID（防越权赋值）；
	//  - promo.BlindBoxID 必须属于 supplierID（不属于自己的盲盒不能挂活动）；
	//  - promo.StartAt < promo.EndAt（ErrInvalidTimeWindow）；
	//  - promo.PromoPrice < promo.OriginalPrice（ErrInvalidPrice）；
	//  - ListTimeOverlap 命中已有活动 → ErrPromotionTimeOverlap。
	Create(c *gin.Context, supplierID int64, promo *mall.MallPromotion) (*mall.MallPromotion, error)

	// Update 只改传入的价格字段；nil 字段保持 DB 原值，成功后失效详情缓存。
	//
	// 业务规则：
	//  - ownership 自检（ErrPromotionNotFound / ErrPromotionForbidden）；
	//  - 不修改 StartAt / EndAt / BlindBoxID / SupplierID / Status
	//    （改时段 = 删旧建新；status 只由 Toggle 改）；
	//  - 用**合并后**的价格重新校验 PromoPrice < OriginalPrice（ErrInvalidPrice）：
	//    只传一个价格时，另一个取 DB 原值。
	//
	// 入参是 patch 的理由同 ProductService.Update：仓储 Update 是 db.Save 全列写，
	// 半成品结构体会把未传字段清零、created_at 归零并撞 MySQL 1292。
	Update(c *gin.Context, supplierID, promotionID int64, patch PromotionPatch) error

	// Toggle 切换 status（active ↔ disabled），并失效详情缓存。
	Toggle(c *gin.Context, supplierID, promotionID int64) error

	// Delete 软删活动，并失效详情缓存。
	Delete(c *gin.Context, supplierID, promotionID int64) error
}

// PromotionPatch 是活动改价的「只改传入字段」入参：nil = 该字段不改。
type PromotionPatch struct {
	OriginalPrice *float64
	PromoPrice    *float64
}

// PromotionServiceDeps 注入 PromotionService 所需的依赖。
type PromotionServiceDeps struct {
	PromotionRepo mallRepo.PromotionRepository
	BlindBoxRepo  mallRepo.BlindBoxRepository // 用于校验 promo.BlindBoxID 归属
	Cache         cache.Cache                 // 用于 Toggle / Delete 失效详情缓存；nil 跳过
	Hotspot       hotspot.HotspotCache        // D21：写路径同时失效热点详情；nil 跳过

	// PreheatLoader D22：活动创建后预热盲盒详情缓存（loader 由 caller 注入，
	// 通常封装 BlindBoxService.Detail）。nil 时 Create 不触发预热。
	PreheatLoader preheat.BlindBoxLoader
}

// basePromotionService 是 PromotionService 的默认实现。
type basePromotionService struct {
	repo          mallRepo.PromotionRepository
	bb            mallRepo.BlindBoxRepository
	cache         cache.Cache
	hotspot       hotspot.HotspotCache
	preheatLoader preheat.BlindBoxLoader
}

// NewPromotionService 接收依赖，返回 PromotionService 接口。
func NewPromotionService(d PromotionServiceDeps) PromotionService {
	return &basePromotionService{
		repo:          d.PromotionRepo,
		bb:            d.BlindBoxRepo,
		cache:         d.Cache,
		hotspot:       d.Hotspot,
		preheatLoader: d.PreheatLoader,
	}
}

// List 实现见 PromotionService 注释。
func (s *basePromotionService) List(c *gin.Context, supplierID int64, opts repository.ListOptions) ([]mall.MallPromotion, int64, error) {
	if supplierID <= 0 {
		return []mall.MallPromotion{}, 0, nil
	}
	return s.repo.ListBySupplier(c, supplierID, opts)
}

// Create 实现见 PromotionService 注释。
func (s *basePromotionService) Create(c *gin.Context, supplierID int64, promo *mall.MallPromotion) (*mall.MallPromotion, error) {
	if supplierID <= 0 {
		return nil, ErrPromotionForbidden
	}
	if promo == nil {
		return nil, ErrPromotionNotFound
	}

	// 时间窗合法性。
	if !promo.EndAt.After(promo.StartAt) {
		return nil, ErrInvalidTimeWindow
	}

	// 价格合法性。
	if promo.PromoPrice >= promo.OriginalPrice {
		return nil, ErrInvalidPrice
	}

	// 强制 SupplierID（防越权赋值）。
	promo.SupplierID = supplierID

	// 校验 BlindBoxID 属于当前 supplier（不属于自己的盲盒不能挂活动）。
	bb, err := s.bb.GetByID(c, promo.BlindBoxID)
	if err != nil {
		return nil, ErrPromotionForbidden // 复用「不在我的范围」错误
	}
	if bb == nil || bb.SupplierID != supplierID {
		return nil, ErrPromotionForbidden
	}

	// 时间窗重叠校验（D10）：同盲盒同时段至多一条活动。
	overlaps, err := s.repo.ListTimeOverlap(c, promo.BlindBoxID, promo.StartAt, promo.EndAt)
	if err != nil {
		return nil, err
	}
	if len(overlaps) > 0 {
		return nil, ErrPromotionTimeOverlap
	}

	if err := s.repo.Create(c, promo); err != nil {
		return nil, err
	}

	// D22：事务 COMMIT 后异步预热关联盲盒详情缓存（活动开始瞬间缓存即热）。
	// 失败仅记 log，不影响主业务（用户场景：cold-start 兜底）。
	if s.hotspot != nil && s.preheatLoader != nil {
		go preheat.PromotionPreheat(context.Background(), promo, preheat.Deps{
			Hotspot:        s.hotspot,
			BlindBoxLoader: s.preheatLoader,
		})
	}

	return promo, nil
}

// Update 实现见 PromotionService 注释。
func (s *basePromotionService) Update(c *gin.Context, supplierID, promotionID int64, patch PromotionPatch) error {
	if supplierID <= 0 {
		return ErrPromotionForbidden
	}
	if promotionID <= 0 {
		return ErrPromotionNotFound
	}

	// Ownership 自检（原行同时作为合并基准）。
	existing, err := s.repo.GetByID(c, promotionID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrPromotionNotFound
		}
		return err
	}
	if existing == nil {
		return ErrPromotionNotFound
	}
	if existing.SupplierID != supplierID {
		return ErrPromotionForbidden
	}

	// 只覆盖传入的价格字段；StartAt / EndAt / BlindBoxID / SupplierID / Status
	// 一律保持 DB 原值（含 created_at，避免 db.Save 全列写把它写成零值）。
	if patch.OriginalPrice != nil {
		existing.OriginalPrice = *patch.OriginalPrice
	}
	if patch.PromoPrice != nil {
		existing.PromoPrice = *patch.PromoPrice
	}

	// 价格合法性用**合并后**的值判断：只传一个价格时另一个取原值。
	if existing.PromoPrice >= existing.OriginalPrice {
		return ErrInvalidPrice
	}

	if err := s.repo.Update(c, existing); err != nil {
		return err
	}

	// 失效详情缓存（活动价变更影响 C 端 effective_price）。
	if s.cache != nil {
		_ = userSvc.InvalidateDetail(c.Request.Context(), s.cache, s.hotspot, existing.BlindBoxID)
	}
	return nil
}

// Toggle 实现见 PromotionService 注释。
func (s *basePromotionService) Toggle(c *gin.Context, supplierID, promotionID int64) error {
	if supplierID <= 0 {
		return ErrPromotionForbidden
	}
	if promotionID <= 0 {
		return ErrPromotionNotFound
	}

	existing, err := s.repo.GetByID(c, promotionID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrPromotionNotFound
		}
		return err
	}
	if existing == nil {
		return ErrPromotionNotFound
	}
	if existing.SupplierID != supplierID {
		return ErrPromotionForbidden
	}

	// active → disabled；disabled → active。
	newStatus := mall.StatusActive
	if existing.Status == mall.StatusActive {
		newStatus = mall.StatusDisabled
	}
	if err := s.repo.ToggleStatus(c, promotionID, newStatus); err != nil {
		return err
	}

	if s.cache != nil {
		_ = userSvc.InvalidateDetail(c.Request.Context(), s.cache, s.hotspot, existing.BlindBoxID)
	}
	return nil
}

// Delete 实现见 PromotionService 注释。
func (s *basePromotionService) Delete(c *gin.Context, supplierID, promotionID int64) error {
	if supplierID <= 0 {
		return ErrPromotionForbidden
	}
	if promotionID <= 0 {
		return ErrPromotionNotFound
	}

	existing, err := s.repo.GetByID(c, promotionID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrPromotionNotFound
		}
		return err
	}
	if existing == nil {
		return ErrPromotionNotFound
	}
	if existing.SupplierID != supplierID {
		return ErrPromotionForbidden
	}

	if err := s.repo.Delete(c, promotionID); err != nil {
		return err
	}

	if s.cache != nil {
		_ = userSvc.InvalidateDetail(c.Request.Context(), s.cache, s.hotspot, existing.BlindBoxID)
	}
	return nil
}

// 编译期断言：basePromotionService 必须实现 PromotionService。
var _ PromotionService = (*basePromotionService)(nil)
