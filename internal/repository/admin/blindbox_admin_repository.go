package admin

import (
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"ai-go-mall/internal/model/mall"
)

// AdminBlindBoxRepository 定义 admin 端对盲盒（mall_blind_boxes）的查询/管理接口。
//
// 设计原则：
//   - ListWithFilter 支持 supplier_id / status / featured 过滤，admin 视角
//     默认跨供应商查全部，由 opts 控制筛选；
//   - ToggleStatus / ToggleOnSale / ToggleFeatured 仅写指定列。
type AdminBlindBoxRepository interface {
	// ListWithFilter 走 admin 管理页筛选。
	//
	// 过滤规则：
	//   - opts.SupplierID > 0 → supplier_id = opts.SupplierID
	//   - opts.StatusFilter != "" → status = opts.StatusFilter
	//   - opts.IsFeaturedFilter != nil → is_featured = *opts.IsFeaturedFilter
	ListWithFilter(c *gin.Context, opts AdminBlindBoxListOptions) (items []mall.MallBlindBox, total int64, err error)

	// ToggleStatus 仅写 status 列。
	ToggleStatus(c *gin.Context, id int64, status mall.MallBlindBoxStatus) error

	// ToggleOnSale 仅写 on_sale 列。
	ToggleOnSale(c *gin.Context, id int64, onSale bool) error

	// ToggleFeatured 仅写 is_featured 列。
	ToggleFeatured(c *gin.Context, id int64, featured bool) error
}

// AdminBlindBoxListOptions admin 管理页筛选参数。
type AdminBlindBoxListOptions struct {
	Page             int
	PageSize         int
	SupplierID       int64  // 0 = 不过滤
	StatusFilter     string // "" = 不过滤
	IsFeaturedFilter *bool   // nil = 不过滤
}

// gormAdminBlindBoxRepository 是 AdminBlindBoxRepository 的 *gorm.DB 实现。
type gormAdminBlindBoxRepository struct {
	db *gorm.DB
}

// NewAdminBlindBoxRepository 构造一个 AdminBlindBoxRepository。
func NewAdminBlindBoxRepository(db *gorm.DB) AdminBlindBoxRepository {
	return &gormAdminBlindBoxRepository{db: db}
}

// ListWithFilter 实现 admin 管理页筛选。
func (r *gormAdminBlindBoxRepository) ListWithFilter(c *gin.Context, opts AdminBlindBoxListOptions) ([]mall.MallBlindBox, int64, error) {
	db := r.db.Model(&mall.MallBlindBox{})

	if opts.SupplierID > 0 {
		db = db.Where("supplier_id = ?", opts.SupplierID)
	}
	if opts.StatusFilter != "" {
		db = db.Where("status = ?", opts.StatusFilter)
	}
	if opts.IsFeaturedFilter != nil {
		db = db.Where("is_featured = ?", *opts.IsFeaturedFilter)
	}

	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	items := make([]mall.MallBlindBox, 0)
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
func (r *gormAdminBlindBoxRepository) ToggleStatus(c *gin.Context, id int64, status mall.MallBlindBoxStatus) error {
	return r.db.
		Model(&mall.MallBlindBox{}).
		Where("id = ?", id).
		Update("status", status).Error
}

// ToggleOnSale 仅写 on_sale 列。
func (r *gormAdminBlindBoxRepository) ToggleOnSale(c *gin.Context, id int64, onSale bool) error {
	return r.db.
		Model(&mall.MallBlindBox{}).
		Where("id = ?", id).
		Update("on_sale", onSale).Error
}

// ToggleFeatured 仅写 is_featured 列。
func (r *gormAdminBlindBoxRepository) ToggleFeatured(c *gin.Context, id int64, featured bool) error {
	return r.db.
		Model(&mall.MallBlindBox{}).
		Where("id = ?", id).
		Update("is_featured", featured).Error
}

// 编译期断言。
var _ AdminBlindBoxRepository = (*gormAdminBlindBoxRepository)(nil)