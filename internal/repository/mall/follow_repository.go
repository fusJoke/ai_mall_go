package mall

import (
    "errors"
    "time"

    "github.com/gin-gonic/gin"
    "gorm.io/gorm"

    "ai-go-mall/internal/model/mall"
    "ai-go-mall/internal/repository"
)

type FollowRepository interface {
    Follow(c *gin.Context, userID, supplierID int64) error
    Unfollow(c *gin.Context, userID, supplierID int64) error
    IsFollowing(c *gin.Context, userID, supplierID int64) (bool, error)
    ListFollowing(c *gin.Context, userID int64, opts repository.ListOptions) (items []mall.MallSupplier, total int64, err error)
    ListFollowers(c *gin.Context, supplierID int64, opts repository.ListOptions) (items []int64, total int64, err error)
}

type gormFollowRepository struct{}

func NewFollowRepository() FollowRepository {
    return &gormFollowRepository{}
}

func (r *gormFollowRepository) Follow(c *gin.Context, userID, supplierID int64) error {
    if userID <= 0 || supplierID <= 0 {
        return errors.New("follow: invalid ids")
    }
    var existing mall.MallUserFollowSupplier
    err := repository.DB(c).Unscoped().Where("user_id = ? AND supplier_id = ?", userID, supplierID).First(&existing).Error
    if errors.Is(err, gorm.ErrRecordNotFound) {
        return repository.DB(c).Create(&mall.MallUserFollowSupplier{UserID: userID, SupplierID: supplierID}).Error
    }
    if err != nil {
        return err
    }
    if existing.DeletedAt.Valid {
        return repository.DB(c).Unscoped().Model(&mall.MallUserFollowSupplier{}).Where("id = ?", existing.ID).Updates(map[string]any{"deleted_at": nil, "updated_at": time.Now()}).Error
    }
    return nil
}

func (r *gormFollowRepository) Unfollow(c *gin.Context, userID, supplierID int64) error {
    if userID <= 0 || supplierID <= 0 {
        return errors.New("follow: invalid ids")
    }
    return repository.DB(c).Where("user_id = ? AND supplier_id = ? AND deleted_at IS NULL", userID, supplierID).Delete(&mall.MallUserFollowSupplier{}).Error
}

func (r *gormFollowRepository) IsFollowing(c *gin.Context, userID, supplierID int64) (bool, error) {
    if userID <= 0 || supplierID <= 0 {
        return false, nil
    }
    var count int64
    err := repository.DB(c).Model(&mall.MallUserFollowSupplier{}).Where("user_id = ? AND supplier_id = ? AND deleted_at IS NULL", userID, supplierID).Count(&count).Error
    return count > 0, err
}

func (r *gormFollowRepository) ListFollowing(c *gin.Context, userID int64, opts repository.ListOptions) ([]mall.MallSupplier, int64, error) {
    db := repository.DB(c).Table("mall_user_follow_supplier AS f").Joins("JOIN mall_suppliers AS s ON s.id = f.supplier_id AND s.deleted_at IS NULL").Where("f.user_id = ? AND f.deleted_at IS NULL", userID)
    var total int64
    if err := db.Count(&total).Error; err != nil {
        return nil, 0, err
    }
    items := make([]mall.MallSupplier, 0)
    if total == 0 {
        return items, total, nil
    }
    offset := (opts.Page - 1) * opts.PageSize
    if err := db.Select("s.*").Offset(offset).Limit(opts.PageSize).Order("f.created_at DESC").Scan(&items).Error; err != nil {
        return nil, 0, err
    }
    return items, total, nil
}

func (r *gormFollowRepository) ListFollowers(c *gin.Context, supplierID int64, opts repository.ListOptions) ([]int64, int64, error) {
    db := repository.DB(c).Model(&mall.MallUserFollowSupplier{}).Where("supplier_id = ? AND deleted_at IS NULL", supplierID)
    var total int64
    if err := db.Count(&total).Error; err != nil {
        return nil, 0, err
    }
    items := make([]int64, 0)
    if total == 0 {
        return items, total, nil
    }
    offset := (opts.Page - 1) * opts.PageSize
    if err := db.Distinct("user_id").Offset(offset).Limit(opts.PageSize).Order("created_at DESC").Pluck("user_id", &items).Error; err != nil {
        return nil, 0, err
    }
    return items, total, nil
}

var _ FollowRepository = (*gormFollowRepository)(nil)