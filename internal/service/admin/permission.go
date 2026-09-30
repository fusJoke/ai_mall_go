// Package admin 存放管理员域相关的 Service 层实现，按 CLAUDE.md 分层归到
// `internal/service/admin/` 子目录。当前文件：permission.go —— 权限管理
// Manager，对应"权限管理器"的角色，对外暴露 5 个方法（IsSuperAdmin /
// Check / GetGroups / GetRules / GetRuleIds）。
//
// 设计要点：
//   - 依赖注入：构造时接收 Repositories 聚合，调用方可在测试中替换为
//     mock 实现（见 permission_test.go）；
//   - Default() 包级便捷函数从 database.Get() 拉真实 GORM 实例；
//   - 业务过滤（status / deleted_at / '*' 通配）由 Manager 集中处理，
//     Repository 层只做 DB 搬运（与 design D5 / D9 一致）。
package admin

import (
	"sort"

	"ai-go-mall/internal/infra/database"
	"ai-go-mall/internal/model"
	"ai-go-mall/internal/repository/admin"
)

// Repositories 聚合 Manager 所需的全部 Repository。
//
// 4 个 Repository 均为接口类型（定义在 internal/repository/admin），
// 满足"接口隔离 + 实现可替换"。
type Repositories struct {
	Admin  admin.AdminRepository
	Rule   admin.RuleRepository
	Group  admin.GroupRepository
	Access admin.AccessRepository
}

// Manager 权限管理器，对外暴露 5 个公开方法。
//
// 全部方法零开销构造（持有 Repositories 引用，不复制），调用方应只构造
// 一次 Manager 然后复用，参见 Default()。
type Manager struct {
	repos Repositories
}

// New 通过依赖注入构造 Manager。
//
// 通常仅在测试或需要替换 Repository 实现时直接调用 New；生产代码走
// Default()。
func New(repos Repositories) *Manager {
	return &Manager{repos: repos}
}

// Default 从 database.Get() 构造一个 Manager，使用 GORM 真实实现。
//
// 启动期（cmd/serve）一般只调用一次；后续 handler / middleware 持有
// 返回的 *Manager 复用即可。
func Default() *Manager {
	db := database.Get()
	return New(Repositories{
		Admin:  admin.NewAdminRepository(db),
		Rule:   admin.NewRuleRepository(db),
		Group:  admin.NewGroupRepository(db),
		Access: admin.NewAccessRepository(db),
	})
}

// IsSuperAdmin 判断管理员是否为超管。超管定义：管理员所属任一分组的
// admin_group.rules 字段为通配符 '*'。详见 specs/admin-permission。
//
// 返回 false 的场景：
//   - admin 不存在 / 软删（Repository 自动过滤后 nil）；
//   - admin.status = 0；
//   - 管理员不属于任何分组；
//   - 管理员所属分组的 rules 字段均非通配。
func (m *Manager) IsSuperAdmin(uid uint) bool {
	if !m.adminIsActive(uid) {
		return false
	}
	groups := m.activeGroups(uid)
	if len(groups) == 0 {
		return false
	}
	for _, g := range groups {
		if g.Rules == nil {
			continue
		}
		wildcard, _ := admin.ParseRuleIDs(*g.Rules)
		if wildcard {
			return true
		}
	}
	return false
}

// GetGroups 返回管理员所属的全部未禁用分组（GORM 自动过滤软删）。
//
// 返回 nil / 空切片均表示"无权限"，调用方按需处理。
func (m *Manager) GetGroups(uid uint) []model.AdminGroup {
	if !m.adminIsActive(uid) {
		return nil
	}
	return m.activeGroups(uid)
}

// GetRuleIds 返回管理员拥有的全部规则 id 集合（去重 + 升序）。
//
// 三种返回：
//   - 任一分组通配 → 全部启用规则的 id 升序切片（来自 RuleRepository.ListActiveIDs）；
//   - 普通管理员有规则 → 聚合后的去重升序切片；
//   - 无规则 / 无分组 / admin 失效 → nil。
func (m *Manager) GetRuleIds(uid uint) []uint {
	if !m.adminIsActive(uid) {
		return nil
	}
	groups := m.activeGroups(uid)
	if len(groups) == 0 {
		return nil
	}

	wildcard, ids := m.aggregateRuleIDs(groups)
	if wildcard {
		allIDs, err := m.repos.Rule.ListActiveIDs()
		if err != nil {
			return nil
		}
		return allIDs
	}
	if len(ids) == 0 {
		return nil
	}
	return ids
}

// GetRules 返回管理员拥有的全部 AdminRule 记录，按 weigh ASC, id ASC 稳定排序。
//
// 数据流：
//  1. GetRuleIds 拿 id 集合（已含通配语义）；
//  2. RuleRepository.ListByIDs 批量拉行；
//  3. status=1 过滤（Repository 不掺业务过滤，Manager 层做）；
//  4. weigh / id 稳定排序。
//
// 无权限 / 拉取失败 → nil。
func (m *Manager) GetRules(uid uint) []model.AdminRule {
	ids := m.GetRuleIds(uid)
	if len(ids) == 0 {
		return nil
	}
	rules, err := m.repos.Rule.ListByIDs(ids)
	if err != nil {
		return nil
	}
	active := make([]model.AdminRule, 0, len(rules))
	for _, r := range rules {
		if r.Status == 1 {
			active = append(active, r)
		}
	}
	sort.Slice(active, func(i, j int) bool {
		if active[i].Weigh != active[j].Weigh {
			return active[i].Weigh < active[j].Weigh
		}
		return active[i].ID < active[j].ID
	})
	return active
}

// Check 判断管理员是否拥有指定规则名（admin_rule.name）。
//
// 数据流（与 design D11 一致）：
//  1. admin 入口校验（存在 + status=1）；
//  2. 拉管理员所在分组（status=1 过滤）；
//  3. 任一通配短路返回 true；
//  4. 聚合规则 id 集合；
//  5. RuleRepository.ExistsByIDsAndName 单条 COUNT 查询。
//
// 返回 false 的场景：
//   - admin 失效 / 不存在；
//   - 管理员无分组 / 全部分组 rules 为空；
//   - 规则 id 集合为空；
//   - Repository 报错（保守返 false）。
func (m *Manager) Check(uid uint, ruleName string) bool {
	if !m.adminIsActive(uid) {
		return false
	}
	groups := m.activeGroups(uid)
	if len(groups) == 0 {
		return false
	}

	wildcard, ids := m.aggregateRuleIDs(groups)
	if wildcard {
		return true
	}
	if len(ids) == 0 {
		return false
	}

	exists, err := m.repos.Rule.ExistsByIDsAndName(ids, ruleName)
	if err != nil {
		return false
	}
	return exists
}

// adminIsActive 校验 admin 存在且启用。被 5 个公开方法复用。
func (m *Manager) adminIsActive(uid uint) bool {
	a, err := m.repos.Admin.GetByID(uid)
	if err != nil || a == nil {
		return false
	}
	return a.Status == 1
}

// activeGroups 返回管理员所属的全部启用分组（GORM 已自动过滤软删）。
// 被 IsSuperAdmin / GetGroups / GetRuleIds / Check 复用。
func (m *Manager) activeGroups(uid uint) []model.AdminGroup {
	groupIDs, err := m.repos.Access.ListGroupIDsByUID(uid)
	if err != nil || len(groupIDs) == 0 {
		return nil
	}
	groups, err := m.repos.Group.ListByIDs(groupIDs)
	if err != nil {
		return nil
	}
	active := make([]model.AdminGroup, 0, len(groups))
	for _, g := range groups {
		if g.Status == 1 {
			active = append(active, g)
		}
	}
	return active
}

// aggregateRuleIDs 从分组列表里聚合规则 id 集合。
//
// 行为：
//   - 任一分组 rules 解析为通配 → 返回 wildcard=true, ids=nil（短路信号）；
//   - 否则聚合所有分组的非通配 id 集合，去重 + 升序，返回 wildcard=false, ids=[]uint{}。
//   - 无任何规则 → wildcard=false, ids=nil。
//
// 被 GetRuleIds / Check 复用。
func (m *Manager) aggregateRuleIDs(groups []model.AdminGroup) (wildcard bool, ids []uint) {
	var collected []uint
	for _, g := range groups {
		if g.Rules == nil {
			continue
		}
		w, parsed := admin.ParseRuleIDs(*g.Rules)
		if w {
			return true, nil
		}
		collected = append(collected, parsed...)
	}
	if len(collected) == 0 {
		return false, nil
	}
	return false, dedupAndSort(collected)
}

// dedupAndSort 去重并升序排序。空入参返回 nil。
func dedupAndSort(ids []uint) []uint {
	if len(ids) == 0 {
		return nil
	}
	seen := make(map[uint]struct{}, len(ids))
	out := make([]uint, 0, len(ids))
	for _, id := range ids {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}
