package admin

import (
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"ai-go-mall/internal/model/mall"
	"ai-go-mall/internal/repository"
)

// AdminSupplierRepository 定义 admin 端对供应商（mall_suppliers）的查询/管理接口。
//
// 设计原则：
//   - ListWithFilter 支持 status / featured / 名称关键字 过滤，admin 视角
//     默认包含全部状态（含 disabled），由 opts.StatusFilter 控制；
//   - ToggleStatus / ToggleFeatured 仅写指定列；
//   - 与 supplier.SupplierRepository 的差别：这里是「admin 跨供应商管理」专用，
//   业务字段查询（如 ListByIDs）由 supplier 包负责。
//
// 注意：本接口不复用 supplier.SupplierRepository —— service 层既可注入两者，
// 也可按需在 service.admin.supplier 注入 admin 版本。
type AdminSupplierRepository interface {
	// ListWithFilter 走 admin 管理页筛选。
	//
	// 过滤规则：
	//   - opts.StatusFilter != "" → status = opts.StatusFilter
	//   - opts.IsFeaturedFilter != nil → is_featured = *opts.IsFeaturedFilter
	//   - opts.NameKeyword != "" → name LIKE %opts.NameKeyword%（GORM 默认 escape）
	ListWithFilter(c *gin.Context, opts AdminSupplierListOptions) (items []mall.MallSupplier, total int64, err error)

	// ToggleStatus 仅写 status 列。
	ToggleStatus(c *gin.Context, id int64, status mall.MallBlindBoxStatus) error

	// ToggleFeatured 仅写 is_featured 列。
	ToggleFeatured(c *gin.Context, id int64, featured bool) error
}

// AdminSupplierListOptions admin 管理页筛选参数。
type AdminSupplierListOptions struct {
	Page             int
	PageSize         int
	StatusFilter     string // "" = 不过滤（含全部）
	IsFeaturedFilter *bool   // nil = 不过滤
	NameKeyword      string // "" = 不过滤
}

// gormAdminSupplierRepository 是 AdminSupplierRepository 的 *gorm.DB 实现。
type gormAdminSupplierRepository struct {
	db *gorm.DB
}

// NewAdminSupplierRepository 构造一个 AdminSupplierRepository。
func NewAdminSupplierRepository(db *gorm.DB) AdminSupplierRepository {
	return &gormAdminSupplierRepository{db: db}
}

// ListWithFilter 实现 admin 管理页筛选。
//
// 与 supplier.SupplierRepository.ListWithFilter 形态一致；这里保持独立
// 实现以体现 admin 域的查询语义（不依赖 supplier 包）。
func (r *gormAdminSupplierRepository) ListWithFilter(c *gin.Context, opts AdminSupplierListOptions) ([]mall.MallSupplier, int64, error) {
	db := r.db.Model(&mall.MallSupplier{})

	if opts.StatusFilter != "" {
		db = db.Where("status = ?", opts.StatusFilter)
	}
	if opts.IsFeaturedFilter != nil {
		db = db.Where("is_featured = ?", *opts.IsFeaturedFilter)
	}
	if opts.NameKeyword != "" {
		db = db.Where("name LIKE ?", "%"+opts.NameKeyword+"%")
	}

	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	items := make([]mall.MallSupplier, 0)
	if total == 0 {
		return items, total, nil
	}

	offset := (opts.Page - 1) * opts.PageSize
	if err := db.Offset(offset).Limit(opts.PageSize).
		Order("id DESC").
		Find(&items).Error; err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

// ToggleStatus 仅写 status 列。
func (r *gormAdminSupplierRepository) ToggleStatus(c *gin.Context, id int64, status mall.MallBlindBoxStatus) error {
	return r.db.
		Model(&mall.MallSupplier{}).
		Where("id = ?", id).
		Update("status", status).Error
}

// ToggleFeatured 仅写 is_featured 列。
func (r *gormAdminSupplierRepository) ToggleFeatured(c *gin.Context, id int64, featured bool) error {
	return r.db.
		Model(&mall.MallSupplier{}).
		Where("id = ?", id).
		Update("is_featured", featured).Error
}

// 编译期断言。
var _ AdminSupplierRepository = (*gormAdminSupplierRepository)(nil)

// 避免 repository 引用被 unused 编译器警告。
var _ repository.ListOptions = repository.ListOptions{}