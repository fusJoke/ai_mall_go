// Package admin — supplier.go 实现 admin 对供应商主体的管理（启停 / 推荐位 / 列表筛选）。
//
// 业务规则：
//   - List：admin 管理页筛选（status / is_featured / name 关键字），包含 disabled 行
//     （spec Requirement "Admin sees disabled suppliers"）。
//   - ToggleStatus：active → disabled；disabled → active。disabled 后供应商所有盲盒
//     在前端不可见（service/user.BlindBoxService.List 二次过滤兜底）。
//   - ToggleFeatured：推荐位翻转；featured=true 且 active 且 on_sale 三者同时 true 才进 ES feed。
//
// 与 service/user/client.supplier 不同：
//   - 不做 ownership 自检（admin 是平台运维，可管理所有供应商）；
//   - 不强求 SupplierUser 同步禁用（status 与 MallSupplierUser.status 解耦，admin
//     想连账号一起禁时单独调 SupplierUserRepository.UpdateStatus）。
package admin

import (
	"errors"

	"github.com/gin-gonic/gin"

	"ai-go-mall/internal/model/mall"
	supplierRepo "ai-go-mall/internal/repository/supplier"
)

// 业务错误。
var (
// ErrSupplierNotFound 供应商不存在 / 已软删（ToggleStatus / ToggleFeatured 时碰到）。
//
// 注意：复用 supplierRepo.ErrSupplierNotFound 让 handler 统一映射 404；
// admin service 层不再额外包装一层。
)

// SupplierService 是 admin 视角的供应商管理接口。
type SupplierService interface {
	// List 按 admin 管理页筛选条件拉供应商列表（含 disabled 行）。
	List(c *gin.Context, opts supplierRepo.SupplierListOptions) (items []mall.MallSupplier, total int64, err error)

	// ToggleStatus 切换供应商 Status。
	//
	// 业务规则：
	//  - 先查当前行（取出现有 status）；
	//  - active → disabled；disabled → active；
	//  - 不存在 → supplierRepo.ErrSupplierNotFound（原样返回）。
	ToggleStatus(c *gin.Context, supplierID int64) error

	// ToggleFeatured 切换推荐位（is_featured 列翻转）。
	//
	// 不存在 → supplierRepo.ErrSupplierNotFound。
	ToggleFeatured(c *gin.Context, supplierID int64) error
}

// SupplierServiceDeps 注入 SupplierService 所需的依赖。
type SupplierServiceDeps struct {
	SupplierRepo supplierRepo.SupplierRepository
}

// baseSupplierService 是 SupplierService 的默认实现。
type baseSupplierService struct {
	repo supplierRepo.SupplierRepository
}

// NewSupplierService 接收依赖，返回 SupplierService 接口。
func NewSupplierService(d SupplierServiceDeps) SupplierService {
	return &baseSupplierService{repo: d.SupplierRepo}
}

// List 实现见 SupplierService 注释。
//
// Page / PageSize 由 handler 校验；service 层只负责透传到 repo。
func (s *baseSupplierService) List(c *gin.Context, opts supplierRepo.SupplierListOptions) ([]mall.MallSupplier, int64, error) {
	return s.repo.ListWithFilter(c, opts)
}

// ToggleStatus 实现见 SupplierService 注释。
func (s *baseSupplierService) ToggleStatus(c *gin.Context, supplierID int64) error {
	if supplierID <= 0 {
		return errors.New("admin.supplier: invalid supplier id")
	}
	existing, err := s.repo.GetByID(c, supplierID)
	if err != nil {
		return err // 含 gorm.ErrRecordNotFound（= supplierRepo.ErrSupplierNotFound 的底层 error）
	}
	if existing == nil {
		return supplierRepo.ErrSupplierNotFound
	}

	// active → disabled；disabled → active。
	newStatus := mall.StatusActive
	if existing.Status == mall.StatusActive {
		newStatus = mall.StatusDisabled
	}
	return s.repo.ToggleStatus(c, supplierID, newStatus)
}

// ToggleFeatured 实现见 SupplierService 注释。
func (s *baseSupplierService) ToggleFeatured(c *gin.Context, supplierID int64) error {
	if supplierID <= 0 {
		return errors.New("admin.supplier: invalid supplier id")
	}
	existing, err := s.repo.GetByID(c, supplierID)
	if err != nil {
		return err
	}
	if existing == nil {
		return supplierRepo.ErrSupplierNotFound
	}
	return s.repo.ToggleFeatured(c, supplierID, !existing.IsFeatured)
}

// 编译期断言：baseSupplierService 必须实现 SupplierService。
var _ SupplierService = (*baseSupplierService)(nil)
