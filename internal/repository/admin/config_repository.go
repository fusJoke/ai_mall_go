package admin

import (
	"gorm.io/gorm"

	"ai-go-mall/internal/model"
)

// ConfigRepository 定义 config 表的最小查询接口。
//
// 设计原则（与 access_repository.go 风格一致）：
//   - ListByNames 走单条 `WHERE name IN (?)`，避免 N+1；
//   - 空 names 直接返回空切片，不查 DB；
//   - 不掺业务过滤：软删由 GORM 自动过滤。
type ConfigRepository interface {
	// ListByNames 按 name 集合批量取 config 行。
	//
	// 空 names 返回空切片（不查 DB）。软删的行由 GORM 自动过滤。
	ListByNames(names []string) ([]model.Config, error)
}

// gormConfigRepository 是基于 *gorm.DB 的 ConfigRepository 实现。
type gormConfigRepository struct {
	db *gorm.DB
}

// NewConfigRepository 构造一个 ConfigRepository。
func NewConfigRepository(db *gorm.DB) ConfigRepository {
	return &gormConfigRepository{db: db}
}

// ListByNames 实现 ConfigRepository.ListByNames。
func (r *gormConfigRepository) ListByNames(names []string) ([]model.Config, error) {
	rows := make([]model.Config, 0)
	if len(names) == 0 {
		return rows, nil
	}
	err := r.db.Where("name IN ?", names).Find(&rows).Error
	return rows, err
}
