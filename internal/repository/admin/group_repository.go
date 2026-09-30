package admin

import (
	"strconv"
	"strings"

	"gorm.io/gorm"

	"ai-go-mall/internal/model"
)

// GroupRepository 定义分组域的查询接口。
//
// 设计原则：
//   - ListByIDs 走单条 `WHERE id IN (?)`（design D10）；
//   - 不掺业务过滤：status / deleted_at 过滤由 Service 层 Manager 负责。
type GroupRepository interface {
	// ListByIDs 按 id 集合批量取分组行。空 ids 返回空切片。
	ListByIDs(ids []uint) ([]model.AdminGroup, error)
}

// gormGroupRepository 是基于 *gorm.DB 的 GroupRepository 实现。
type gormGroupRepository struct {
	db *gorm.DB
}

// NewGroupRepository 构造一个 GroupRepository。
func NewGroupRepository(db *gorm.DB) GroupRepository {
	return &gormGroupRepository{db: db}
}

// ListByIDs 实现 GroupRepository.ListByIDs。
func (r *gormGroupRepository) ListByIDs(ids []uint) ([]model.AdminGroup, error) {
	groups := make([]model.AdminGroup, 0)
	if len(ids) == 0 {
		return groups, nil
	}
	err := r.db.Where("id IN ?", ids).Find(&groups).Error
	return groups, err
}

// ParseRuleIDs 解析 admin_group.rules 字段的文本内容。
//
// 契约（与 design D4 一致）：
//   - 整串等于 "*"（前后允许空白）→ 返回 wildcard=true, ids=nil；
//   - 空字符串 → 返回 wildcard=false, ids=nil；
//   - 其他字符串按 "," split，每段 TrimSpace 后：
//   - 空段跳过；
//   - 非数字段（strconv.ParseUint 失败）跳过；
//   - 数字段收集为 uint 返回。
//
// 返回的 ids 可能为空切片（所有段都非法）；wildcard 与 ids 互斥：
// wildcard=true 时 ids 必为 nil。
//
// 该函数是纯函数（无 IO / 无 DB 访问），便于单元测试覆盖。
func ParseRuleIDs(s string) (wildcard bool, ids []uint) {
	trimmed := strings.TrimSpace(s)
	if trimmed == "*" {
		return true, nil
	}
	if trimmed == "" {
		return false, nil
	}

	parts := strings.Split(trimmed, ",")
	out := make([]uint, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		n, err := strconv.ParseUint(p, 10, 64)
		if err != nil {
			continue
		}
		out = append(out, uint(n))
	}
	return false, out
}
