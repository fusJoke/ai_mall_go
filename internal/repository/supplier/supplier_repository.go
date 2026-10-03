// Package supplier 是供应商域的数据访问层。
//
// 本文件按 CLAUDE.md 分层：repository 层只做 DB ↔ 内存 的搬运，不掺业务。
//
// 设计要点：
//   - supplier_repository.go       —— MallSupplier（mall_suppliers 表）仓储；
//   - supplier_user_repository.go  —— MallSupplierUser（mall_supplier_users 表）仓储；
//   - 不互相内嵌：两个仓储语义独立，账号被禁用不影响供应商主体；
//   - Supplier ID 与 User ID 均为 int64，与 admin 包中 AdminGroup / AdminRule 的
//     uint 类型不同 —— 统一走 int64 是因为 mall_* 表统一 BIGINT。
package supplier

import (
	"errors"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"ai-go-mall/internal/model/mall"
	"ai-go-mall/internal/repository"
)

// SupplierRepository 定义 mall_suppliers 表的数据访问接口。
//
// 设计原则：
//   - 通用 CRUD 由 CRUDRepository[mall.MallSupplier] 提供；
//   - ListByIDs 走单条 WHERE id IN (?) 用于批量拉取（结算 / 详情聚合场景）；
//   - ListWithFilter 走 admin 管理页查询，支持 status / featured / 关键字过滤；
//   - ToggleStatus / ToggleFeatured 走 map 形式 Updates 省一次 SELECT；
//   - UpdateBalanceAndSales 一次性写 balance + total_sales（结算单生成 / 抽卡副作用）。
type SupplierRepository interface {
	repository.CRUDRepository[mall.MallSupplier]

	// GetByID 走主键查单行。MallSupplier.ID 是 int64。
	GetByID(c *gin.Context, id int64) (*mall.MallSupplier, error)

	// ListByIDs 按 id 集合批量取供应商行（IN 单条 SQL）。
	// 空 ids 短路返回空切片。
	ListByIDs(c *gin.Context, ids []int64) ([]mall.MallSupplier, error)

	// ListWithFilter 走 admin 管理页筛选。
	//
	// 过滤规则：
	//   - opts.StatusFilter != "" → status = opts.StatusFilter
	//   - opts.IsFeaturedFilter != nil → is_featured = *opts.IsFeaturedFilter
	//   - opts.NameKeyword != "" → name LIKE %opts.NameKeyword%（GORM 默认 escape）
	ListWithFilter(c *gin.Context, opts SupplierListOptions) (items []mall.MallSupplier, total int64, err error)

	// ToggleStatus 仅更新 Status 列（admin "启用/禁用"路径专用）。
	ToggleStatus(c *gin.Context, id int64, status mall.MallBlindBoxStatus) error

	// ToggleFeatured 仅更新 is_featured 列。
	ToggleFeatured(c *gin.Context, id int64, featured bool) error

	// UpdateBalanceAndSales 一次性写 balance + total_sales（结算单生成 / 抽卡副作用）。
	// SQL：`SET balance = balance + ?, total_sales = total_sales + ? WHERE id = ?`。
	// id 不存在 / 已软删均影响 0 行 → 返回 ErrSupplierNotFound。
	UpdateBalanceAndSales(c *gin.Context, id int64, amount float64) error
}

// SupplierListOptions admin 管理页筛选参数。
type SupplierListOptions struct {
	Page             int
	PageSize         int
	StatusFilter     string // "" = 不过滤
	IsFeaturedFilter *bool   // nil = 不过滤
	NameKeyword      string // "" = 不过滤
}

// ErrSupplierNotFound 供应商不存在 / 已软删（UpdateBalanceAndSales 影响 0 行时返回）。
var ErrSupplierNotFound = errors.New("supplier: not found")

// gormSupplierRepository 是 SupplierRepository 的 *gorm.DB 实现。
type gormSupplierRepository struct {
	repository.CRUDRepository[mall.MallSupplier]
}

// NewSupplierRepository 返回 SupplierRepository 接口，调用方拿到的是接口而非结构体。
func NewSupplierRepository() SupplierRepository {
	return &gormSupplierRepository{
		CRUDRepository: repository.NewBaseRepository[mall.MallSupplier](),
	}
}

// GetByID 按主键查单行；不存在返回 gorm.ErrRecordNotFound。
func (r *gormSupplierRepository) GetByID(c *gin.Context, id int64) (*mall.MallSupplier, error) {
	var s mall.MallSupplier
	if err := repository.DB(c).First(&s, id).Error; err != nil {
		return nil, err
	}
	return &s, nil
}

// ListByIDs 按 id 集合批量取供应商行。
func (r *gormSupplierRepository) ListByIDs(c *gin.Context, ids []int64) ([]mall.MallSupplier, error) {
	rows := make([]mall.MallSupplier, 0)
	if len(ids) == 0 {
		return rows, nil
	}
	err := repository.DB(c).Where("id IN ?", ids).Find(&rows).Error
	return rows, err
}

// ListWithFilter 实现 admin 管理页筛选。
//
// 逐项 Where 拼接：每项条件独立，handler 想筛哪几项就传对应 opts 字段；
// Page/PageSize 由调用方负责兜底（与 admin Repository 风格一致）。
func (r *gormSupplierRepository) ListWithFilter(c *gin.Context, opts SupplierListOptions) ([]mall.MallSupplier, int64, error) {
	db := repository.DB(c).Model(&mall.MallSupplier{})

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

// ToggleStatus 仅更新 Status 列。
func (r *gormSupplierRepository) ToggleStatus(c *gin.Context, id int64, status mall.MallBlindBoxStatus) error {
	return repository.DB(c).
		Model(&mall.MallSupplier{}).
		Where("id = ?", id).
		Update("status", status).Error
}

// ToggleFeatured 仅更新 IsFeatured 列。
func (r *gormSupplierRepository) ToggleFeatured(c *gin.Context, id int64, featured bool) error {
	return repository.DB(c).
		Model(&mall.MallSupplier{}).
		Where("id = ?", id).
		Update("is_featured", featured).Error
}

// UpdateBalanceAndSales 一次性写两列。
//
// 用 gorm.Expr 让 DB 自己算 balance + amount / total_sales + amount。
// id 不存在 / 已软删时返回 ErrSupplierNotFound。
func (r *gormSupplierRepository) UpdateBalanceAndSales(c *gin.Context, id int64, amount float64) error {
	if amount <= 0 {
		return errors.New("supplier: amount must be positive")
	}
	res := repository.DB(c).
		Model(&mall.MallSupplier{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"balance":     gorm.Expr("balance + ?", amount),
			"total_sales": gorm.Expr("total_sales + ?", amount),
		})
	if err := res.Error; err != nil {
		return err
	}
	if res.RowsAffected == 0 {
		return ErrSupplierNotFound
	}
	return nil
}

// 编译期断言：gormSupplierRepository 必须实现 SupplierRepository。
var _ SupplierRepository = (*gormSupplierRepository)(nil)