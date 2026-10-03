package mall

import (
	"github.com/gin-gonic/gin"

	"ai-go-mall/internal/model/mall"
	"ai-go-mall/internal/repository"
)

// BlindBoxRepository 定义 mall_blind_boxes 表的数据访问接口。
//
// 设计原则：
//   - 通用 CRUD 由 CRUDRepository[mall.MallBlindBox] 提供；
//   - ListBySupplier 走单条 WHERE supplier_id = ? ORDER BY id DESC；
//   - ListFeaturedOnSale 走 WHERE is_featured=1 AND status=active AND on_sale=true，
//     用于供应商 / admin 列表页；
//   - ListActiveOnSaleBySupplierIDs 按供应商 id 集合批量取「在售启用」行；
//   - ToggleStatus / ToggleOnSale / ToggleFeatured 走 map 形式 Updates 省一次 SELECT。
type BlindBoxRepository interface {
	repository.CRUDRepository[mall.MallBlindBox]

	// GetByID 按主键查单行。
	GetByID(c *gin.Context, id int64) (*mall.MallBlindBox, error)

	// ListBySupplier 拉某供应商的全部盲盒（管理页 / 供应商后台）。
	ListBySupplier(c *gin.Context, supplierID int64, opts repository.ListOptions) (items []mall.MallBlindBox, total int64, err error)

	// ListFeaturedOnSale 拉「推荐 + 启用 + 在售」盲盒（前端首页推荐位）。
	ListFeaturedOnSale(c *gin.Context, opts repository.ListOptions) (items []mall.MallBlindBox, total int64, err error)

	// ListActiveOnSaleBySupplierIDs 按供应商 id 集合批量取「启用 + 在售」盲盒。
	// 用于跨供应商推荐 feed；空 ids 短路返回空切片。
	ListActiveOnSaleBySupplierIDs(c *gin.Context, supplierIDs []int64, opts repository.ListOptions) (items []mall.MallBlindBox, total int64, err error)

	// ListByIDs 按 id 集合批量取盲盒（结算 / 详情聚合）。
	ListByIDs(c *gin.Context, ids []int64) ([]mall.MallBlindBox, error)

	// ToggleStatus 仅写 Status 列。
	ToggleStatus(c *gin.Context, id int64, status mall.MallBlindBoxStatus) error

	// ToggleOnSale 仅写 OnSale 列。
	ToggleOnSale(c *gin.Context, id int64, onSale bool) error

	// ToggleFeatured 仅写 IsFeatured 列。
	ToggleFeatured(c *gin.Context, id int64, featured bool) error
}

// gormBlindBoxRepository 是 BlindBoxRepository 的 *gorm.DB 实现。
type gormBlindBoxRepository struct {
	repository.CRUDRepository[mall.MallBlindBox]
}

// NewBlindBoxRepository 返回 BlindBoxRepository 接口。
func NewBlindBoxRepository() BlindBoxRepository {
	return &gormBlindBoxRepository{
		CRUDRepository: repository.NewBaseRepository[mall.MallBlindBox](),
	}
}

// GetByID 按主键查单行。
func (r *gormBlindBoxRepository) GetByID(c *gin.Context, id int64) (*mall.MallBlindBox, error) {
	var b mall.MallBlindBox
	if err := repository.DB(c).First(&b, id).Error; err != nil {
		return nil, err
	}
	return &b, nil
}

// ListBySupplier 拉某供应商的全部盲盒。
func (r *gormBlindBoxRepository) ListBySupplier(c *gin.Context, supplierID int64, opts repository.ListOptions) ([]mall.MallBlindBox, int64, error) {
	db := repository.DB(c).Model(&mall.MallBlindBox{}).Where("supplier_id = ?", supplierID)

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

// ListFeaturedOnSale 拉「推荐 + 启用 + 在售」盲盒。
func (r *gormBlindBoxRepository) ListFeaturedOnSale(c *gin.Context, opts repository.ListOptions) ([]mall.MallBlindBox, int64, error) {
	db := repository.DB(c).Model(&mall.MallBlindBox{}).
		Where("is_featured = ? AND status = ? AND on_sale = ?", true, mall.StatusActive, true)

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

// ListActiveOnSaleBySupplierIDs 按供应商 id 集合批量取「启用 + 在售」盲盒。
func (r *gormBlindBoxRepository) ListActiveOnSaleBySupplierIDs(c *gin.Context, supplierIDs []int64, opts repository.ListOptions) ([]mall.MallBlindBox, int64, error) {
	if len(supplierIDs) == 0 {
		return make([]mall.MallBlindBox, 0), 0, nil
	}
	db := repository.DB(c).Model(&mall.MallBlindBox{}).
		Where("supplier_id IN ? AND status = ? AND on_sale = ?", supplierIDs, mall.StatusActive, true)

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

// ListByIDs 按 id 集合批量取盲盒。
func (r *gormBlindBoxRepository) ListByIDs(c *gin.Context, ids []int64) ([]mall.MallBlindBox, error) {
	rows := make([]mall.MallBlindBox, 0)
	if len(ids) == 0 {
		return rows, nil
	}
	err := repository.DB(c).Where("id IN ?", ids).Find(&rows).Error
	return rows, err
}

// ToggleStatus 仅写 Status 列。
func (r *gormBlindBoxRepository) ToggleStatus(c *gin.Context, id int64, status mall.MallBlindBoxStatus) error {
	return repository.DB(c).
		Model(&mall.MallBlindBox{}).
		Where("id = ?", id).
		Update("status", status).Error
}

// ToggleOnSale 仅写 OnSale 列。
func (r *gormBlindBoxRepository) ToggleOnSale(c *gin.Context, id int64, onSale bool) error {
	return repository.DB(c).
		Model(&mall.MallBlindBox{}).
		Where("id = ?", id).
		Update("on_sale", onSale).Error
}

// ToggleFeatured 仅写 IsFeatured 列。
func (r *gormBlindBoxRepository) ToggleFeatured(c *gin.Context, id int64, featured bool) error {
	return repository.DB(c).
		Model(&mall.MallBlindBox{}).
		Where("id = ?", id).
		Update("is_featured", featured).Error
}

// 编译期断言。
var _ BlindBoxRepository = (*gormBlindBoxRepository)(nil)