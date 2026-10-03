package mall

import (
	"errors"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"ai-go-mall/internal/model/mall"
	"ai-go-mall/internal/repository"
)

// PromotionRepository 定义 mall_promotions 表的数据访问接口。
//
// 设计原则：
//   - FindActiveByBlindBoxID 走 (status, end_at) 索引 + blind_box_id 等值条件，
//     抽卡事务 D4 用 LEFT JOIN 调此方法拿到当前活动价；
//   - ListBySupplier 走 supplier_id 等值过滤（供应商活动列表）；
//   - ListTimeOverlap 走时间区间判定，供 service 层 Create 时校验冲突。
type PromotionRepository interface {
	repository.CRUDRepository[mall.MallPromotion]

	// GetByID 按主键查单行。
	GetByID(c *gin.Context, id int64) (*mall.MallPromotion, error)

	// FindActiveByBlindBoxID 找某盲盒当前生效的活动。
	//
	// 生效定义：`status = active AND start_at <= now AND now < end_at`。
	// 同盲盒同时间至多一条活动（service 层防重叠保证），这里只取最新一条。
	FindActiveByBlindBoxID(c *gin.Context, blindBoxID int64, now time.Time) (*mall.MallPromotion, error)

	// ListBySupplier 拉某供应商的全部活动（供应商活动管理页）。
	ListBySupplier(c *gin.Context, supplierID int64, opts repository.ListOptions) (items []mall.MallPromotion, total int64, err error)

	// ListTimeOverlap 拉某盲盒在某时段内已存在的活动（service 层 Create 校验用）。
	//
	// SQL 形态：`WHERE blind_box_id = ? AND deleted_at IS NULL AND start_at < ? AND end_at > ?`
	// （标准区间重叠判定：a.start < b.end AND a.end > b.start）。
	ListTimeOverlap(c *gin.Context, blindBoxID int64, startAt, endAt time.Time) ([]mall.MallPromotion, error)

	// ToggleStatus 仅写 Status 列（admin 强制启停）。
	ToggleStatus(c *gin.Context, id int64, status mall.MallBlindBoxStatus) error
}

// gormPromotionRepository 是 PromotionRepository 的 *gorm.DB 实现。
type gormPromotionRepository struct {
	repository.CRUDRepository[mall.MallPromotion]
}

// NewPromotionRepository 返回 PromotionRepository 接口。
func NewPromotionRepository() PromotionRepository {
	return &gormPromotionRepository{
		CRUDRepository: repository.NewBaseRepository[mall.MallPromotion](),
	}
}

// GetByID 按主键查单行。
func (r *gormPromotionRepository) GetByID(c *gin.Context, id int64) (*mall.MallPromotion, error) {
	var p mall.MallPromotion
	if err := repository.DB(c).First(&p, id).Error; err != nil {
		return nil, err
	}
	return &p, nil
}

// FindActiveByBlindBoxID 找某盲盒当前生效的活动。
func (r *gormPromotionRepository) FindActiveByBlindBoxID(c *gin.Context, blindBoxID int64, now time.Time) (*mall.MallPromotion, error) {
	var p mall.MallPromotion
	err := repository.DB(c).
		Where("blind_box_id = ? AND status = ? AND start_at <= ? AND end_at > ?", blindBoxID, mall.StatusActive, now, now).
		Order("start_at DESC").
		First(&p).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &p, nil
}

// ListBySupplier 拉某供应商的全部活动。
func (r *gormPromotionRepository) ListBySupplier(c *gin.Context, supplierID int64, opts repository.ListOptions) ([]mall.MallPromotion, int64, error) {
	db := repository.DB(c).Model(&mall.MallPromotion{}).Where("supplier_id = ?", supplierID)

	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	items := make([]mall.MallPromotion, 0)
	if total == 0 {
		return items, total, nil
	}

	offset := (opts.Page - 1) * opts.PageSize
	if err := db.Offset(offset).Limit(opts.PageSize).
		Order("start_at DESC").
		Find(&items).Error; err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

// ListTimeOverlap 拉某盲盒在某时段内已存在的活动。
//
// 区间重叠判定 SQL：`a.start_at < b.end_at AND a.end_at > b.start_at`。
// 由于 mall_promotions.UK = (blind_box_id, start_at)，UK 不会拦截「完全重复插入」之外的
// 区间重叠场景，因此 service 层在 Create 时调此接口做时间窗冲突校验。
func (r *gormPromotionRepository) ListTimeOverlap(c *gin.Context, blindBoxID int64, startAt, endAt time.Time) ([]mall.MallPromotion, error) {
	rows := make([]mall.MallPromotion, 0)
	err := repository.DB(c).
		Where("blind_box_id = ? AND start_at < ? AND end_at > ?", blindBoxID, endAt, startAt).
		Find(&rows).Error
	return rows, err
}

// ToggleStatus 仅写 Status 列。
func (r *gormPromotionRepository) ToggleStatus(c *gin.Context, id int64, status mall.MallBlindBoxStatus) error {
	return repository.DB(c).
		Model(&mall.MallPromotion{}).
		Where("id = ?", id).
		Update("status", status).Error
}

// 编译期断言。
var _ PromotionRepository = (*gormPromotionRepository)(nil)