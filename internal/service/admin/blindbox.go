// Package admin — blindbox.go 实现 admin 对盲盒 SKU 的管理（启停 / 在售 / 推荐位）。
//
// 业务规则：
//   - List：admin 管理页查看全量盲盒（含下架 / disabled），无需 ownership 自检。
//   - ToggleStatus：admin 强制禁用 / 启用盲盒（status=active↔disabled），会直接影响
//     C 端 BlindBoxService.List 的二次过滤兜底。
//   - ToggleOnSale：翻转 on_sale 列（true↔false）；on_sale=false 时 C 端不可见，
//     但 admin / 供应商仍能编辑。
//   - ToggleFeatured：翻转 is_featured 列；featured=true 且 status=active 且 on_sale=true
//     三条件 AND 才会被 cmd/es-sync 推到 ES 首页 feed。
//
// 与 service/user/client.blindbox 不同：
//   - 不做 ownership 自检（admin 是平台运维，可管理所有供应商下的盲盒）；
//   - 不过滤 disabled / on_sale=false 行（管理视图）。
package admin

import (
	"errors"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"ai-go-mall/internal/model/mall"
	"ai-go-mall/internal/repository"
	mallRepo "ai-go-mall/internal/repository/mall"
)

// 业务错误。
var (
	// ErrBlindBoxNotFound 盲盒不存在 / 已软删（ToggleStatus / ToggleOnSale /
	// ToggleFeatured 时碰到）。
	//
	// 由 handler 统一映射 404；service 层将 gorm.ErrRecordNotFound 转成此 sentinel，
	// 避免暴露 ORM 细节。
	ErrBlindBoxNotFound = errors.New("admin.blindbox: not found")
)

// BlindBoxService 是 admin 视角的盲盒管理接口。
type BlindBoxService interface {
	// List 拉全量盲盒（不分页盲盒所属 supplier；含 disabled / on_sale=false 行）。
	List(c *gin.Context, opts repository.ListOptions) (items []mall.MallBlindBox, total int64, err error)

	// ToggleStatus 切换盲盒 Status。
	//
	// 业务规则：
	//  - 先查当前行（取出现有 status）；
	//  - active → disabled；disabled → active；
	//  - 不存在 → ErrBlindBoxNotFound。
	ToggleStatus(c *gin.Context, blindBoxID int64) error

	// ToggleOnSale 翻转 on_sale 列（true↔false）。
	//
	// 不存在 → ErrBlindBoxNotFound。
	ToggleOnSale(c *gin.Context, blindBoxID int64) error

	// ToggleFeatured 翻转 is_featured 列（true↔false）。
	//
	// 不存在 → ErrBlindBoxNotFound。
	ToggleFeatured(c *gin.Context, blindBoxID int64) error
}

// BlindBoxServiceDeps 注入 BlindBoxService 所需的依赖。
type BlindBoxServiceDeps struct {
	BlindBoxRepo mallRepo.BlindBoxRepository
}

// baseBlindBoxService 是 BlindBoxService 的默认实现。
type baseBlindBoxService struct {
	repo mallRepo.BlindBoxRepository
}

// NewBlindBoxService 接收依赖，返回 BlindBoxService 接口。
func NewBlindBoxService(d BlindBoxServiceDeps) BlindBoxService {
	return &baseBlindBoxService{repo: d.BlindBoxRepo}
}

// List 实现见 BlindBoxService 注释。
//
// Page / PageSize 由 handler 校验；service 层只负责透传到 repo。
func (s *baseBlindBoxService) List(c *gin.Context, opts repository.ListOptions) ([]mall.MallBlindBox, int64, error) {
	return s.repo.List(c, opts)
}

// ToggleStatus 实现见 BlindBoxService 注释。
func (s *baseBlindBoxService) ToggleStatus(c *gin.Context, blindBoxID int64) error {
	if blindBoxID <= 0 {
		return errors.New("admin.blindbox: invalid blindbox id")
	}
	existing, err := s.repo.GetByID(c, blindBoxID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrBlindBoxNotFound
		}
		return err
	}
	if existing == nil {
		return ErrBlindBoxNotFound
	}

	// active → disabled；disabled → active。
	newStatus := mall.StatusActive
	if existing.Status == mall.StatusActive {
		newStatus = mall.StatusDisabled
	}
	return s.repo.ToggleStatus(c, blindBoxID, newStatus)
}

// ToggleOnSale 实现见 BlindBoxService 注释。
func (s *baseBlindBoxService) ToggleOnSale(c *gin.Context, blindBoxID int64) error {
	if blindBoxID <= 0 {
		return errors.New("admin.blindbox: invalid blindbox id")
	}
	existing, err := s.repo.GetByID(c, blindBoxID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrBlindBoxNotFound
		}
		return err
	}
	if existing == nil {
		return ErrBlindBoxNotFound
	}

	return s.repo.ToggleOnSale(c, blindBoxID, !existing.OnSale)
}

// ToggleFeatured 实现见 BlindBoxService 注释。
func (s *baseBlindBoxService) ToggleFeatured(c *gin.Context, blindBoxID int64) error {
	if blindBoxID <= 0 {
		return errors.New("admin.blindbox: invalid blindbox id")
	}
	existing, err := s.repo.GetByID(c, blindBoxID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrBlindBoxNotFound
		}
		return err
	}
	if existing == nil {
		return ErrBlindBoxNotFound
	}

	return s.repo.ToggleFeatured(c, blindBoxID, !existing.IsFeatured)
}

// 编译期断言：baseBlindBoxService 必须实现 BlindBoxService。
var _ BlindBoxService = (*baseBlindBoxService)(nil)
