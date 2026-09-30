package admin

import (
	"gorm.io/gorm"

	"ai-go-mall/internal/model"
)

// AccessRepository 定义管理员-分组关系表的查询接口。
//
// 设计原则：
//   - ListGroupIDsByUID 走单条 SELECT，distinct 防御性处理（虽然复合 PK
//     (uid, group_id) 天然不会重复）；
//   - 软删除的行由 GORM 自动从结果中过滤（deleted_at IS NULL）。
type AccessRepository interface {
	// ListGroupIDsByUID 查某管理员所属分组的 id 集合（去重）。
	// 未命中返回空切片。Manager 入口用此判断"是否属于任何分组"。
	ListGroupIDsByUID(uid uint) ([]uint, error)
}

// gormAccessRepository 是基于 *gorm.DB 的 AccessRepository 实现。
type gormAccessRepository struct {
	db *gorm.DB
}

// NewAccessRepository 构造一个 AccessRepository。
func NewAccessRepository(db *gorm.DB) AccessRepository {
	return &gormAccessRepository{db: db}
}

// ListGroupIDsByUID 实现 AccessRepository.ListGroupIDsByUID。
//
// 用 DISTINCT 防御性去重；正常情况下复合 PK 已经保证不重复。
func (r *gormAccessRepository) ListGroupIDsByUID(uid uint) ([]uint, error) {
	groupIDs := make([]uint, 0)
	err := r.db.Model(&model.AdminGroupAccess{}).
		Where("uid = ?", uid).
		Distinct("group_id").
		Pluck("group_id", &groupIDs).Error
	return groupIDs, err
}
