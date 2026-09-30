package admin

import (
	"gorm.io/gorm"

	"ai-go-mall/internal/model"
)

// RuleRepository 定义规则域的查询接口。
//
// 设计原则（与 design D5 / D9 / D10 一致）：
//   - 批量查询（ListByIDs / ListActiveIDs / ExistsByIDsAndName）走单条
//     `WHERE id IN (?)` SQL，避免 N+1（design D10）；
//   - ListActiveIDs 把 status=1 过滤推到 SQL 层，用于"超管全规则"路径，
//     不在 Manager 层再二次过滤（性能 + 简洁性 trade-off，design D9 例外）；
//   - ExistsByIDsAndName 同样把 status=1 推到 SQL，避免 Manager 拉行后
//     Go 端二次过滤；返回 bool 而非行集，节省内存。
type RuleRepository interface {
	// ListByIDs 按 id 集合批量取规则行（不过滤 status / deleted_at
	// 之外的任何业务字段；deleted_at 由 GORM 自动过滤）。
	//
	// 空 ids 返回空切片（不查 DB）。
	ListByIDs(ids []uint) ([]model.AdminRule, error)

	// ExistsByIDsAndName 判断"id 集合中存在一条 status=1 且 name=name 的规则"。
	// 用于 Manager.Check 的最后一步。
	ExistsByIDsAndName(ids []uint, name string) (bool, error)

	// ListActiveIDs 拉所有启用规则的 id 升序切片。用于超管路径：
	// Manager 检测到通配符 '*' 后用此接口取全规则 id 集合。
	ListActiveIDs() ([]uint, error)

	// ListActiveAsMenu 拉所有启用的 AdminRule 行，按 weigh ASC, id ASC 排序。
	// 用于后台 init 端点「超管路径」：一次拉齐菜单规则全量数据，避免
	// 先 ListActiveIDs 再 ListByIDs 两次往返。
	ListActiveAsMenu() ([]model.AdminRule, error)
}

// gormRuleRepository 是基于 *gorm.DB 的 RuleRepository 实现。
type gormRuleRepository struct {
	db *gorm.DB
}

// NewRuleRepository 构造一个 RuleRepository。
func NewRuleRepository(db *gorm.DB) RuleRepository {
	return &gormRuleRepository{db: db}
}

// ListByIDs 实现 RuleRepository.ListByIDs。
func (r *gormRuleRepository) ListByIDs(ids []uint) ([]model.AdminRule, error) {
	rules := make([]model.AdminRule, 0)
	if len(ids) == 0 {
		return rules, nil
	}
	err := r.db.Where("id IN ?", ids).Find(&rules).Error
	return rules, err
}

// ExistsByIDsAndName 实现 RuleRepository.ExistsByIDsAndName。
//
// 把 status=1 过滤推到 SQL（Count 走索引更高效）；空 ids 短路返回 false。
func (r *gormRuleRepository) ExistsByIDsAndName(ids []uint, name string) (bool, error) {
	if len(ids) == 0 {
		return false, nil
	}
	var count int64
	err := r.db.Model(&model.AdminRule{}).
		Where("id IN ? AND name = ? AND status = ?", ids, name, 1).
		Count(&count).Error
	return count > 0, err
}

// ListActiveIDs 实现 RuleRepository.ListActiveIDs。
func (r *gormRuleRepository) ListActiveIDs() ([]uint, error) {
	var ids []uint
	err := r.db.Model(&model.AdminRule{}).
		Where("status = ?", 1).
		Order("id ASC").
		Pluck("id", &ids).Error
	return ids, err
}

// ListActiveAsMenu 实现 RuleRepository.ListActiveAsMenu。
//
// 单条 `SELECT * FROM admin_rule WHERE status = 1 ORDER BY weigh ASC, id ASC`，
// GORM 自动过滤软删（deleted_at IS NULL）。返回切片为新分配的，调用方可自由修改。
func (r *gormRuleRepository) ListActiveAsMenu() ([]model.AdminRule, error) {
	rules := make([]model.AdminRule, 0)
	err := r.db.Where("status = ?", 1).
		Order("weigh ASC, id ASC").
		Find(&rules).Error
	return rules, err
}
