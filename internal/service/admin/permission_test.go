package admin

import (
	"testing"

	"gorm.io/gorm"

	"ai-go-mall/internal/model"
)

// =============================================================================
// Mocks —— 4 个 Repository 接口的 mock 实现，注入到 Manager 测试中。
// =============================================================================

// mockAdminRepository 实现 AdminRepository。用 map[uid]*Admin 模拟"按主键查"。
// 不在 map 中的 uid 返回 (nil, nil)（与真实 GORM 的 ErrRecordNotFound 处理一致）。
type mockAdminRepository struct {
	admins map[uint]*model.Admin
	err    error
}

func (m *mockAdminRepository) GetByID(uid uint) (*model.Admin, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.admins[uid], nil
}

// mockRuleRepository 实现 RuleRepository。
//
//   - rules: 全量规则（含 status=0），用于 ListByIDs；
//   - activeIDs: 启用规则 id 升序切片，用于 ListActiveIDs；
//   - byName: name → 启用规则 id 列表，用于 ExistsByIDsAndName（仅 status=1）。
type mockRuleRepository struct {
	rules     map[uint]*model.AdminRule
	activeIDs []uint
	byName    map[string][]uint
	err       error
}

func (m *mockRuleRepository) ListByIDs(ids []uint) ([]model.AdminRule, error) {
	if m.err != nil {
		return nil, m.err
	}
	out := make([]model.AdminRule, 0, len(ids))
	for _, id := range ids {
		if r, ok := m.rules[id]; ok {
			out = append(out, *r)
		}
	}
	return out, nil
}

func (m *mockRuleRepository) ExistsByIDsAndName(ids []uint, name string) (bool, error) {
	if m.err != nil {
		return false, m.err
	}
	for _, target := range ids {
		for _, id := range m.byName[name] {
			if id == target {
				return true, nil
			}
		}
	}
	return false, nil
}

func (m *mockRuleRepository) ListActiveIDs() ([]uint, error) {
	if m.err != nil {
		return nil, m.err
	}
	out := make([]uint, len(m.activeIDs))
	copy(out, m.activeIDs)
	return out, nil
}

// ListActiveAsMenu 是 init service 用的方法；permission_test 路径不调用，
// 实现为返回 status=1 的子集（与 activeIDs 一致，保持 mock 行为对称）。
func (m *mockRuleRepository) ListActiveAsMenu() ([]model.AdminRule, error) {
	if m.err != nil {
		return nil, m.err
	}
	out := make([]model.AdminRule, 0, len(m.activeIDs))
	for _, id := range m.activeIDs {
		if r, ok := m.rules[id]; ok {
			out = append(out, *r)
		}
	}
	return out, nil
}

// mockGroupRepository 实现 GroupRepository。
//
// 模拟 GORM 自动软删过滤：DeletedAt.Valid == true 的行不在 ListByIDs 返回中。
type mockGroupRepository struct {
	groups map[uint]*model.AdminGroup
	err    error
}

func (m *mockGroupRepository) ListByIDs(ids []uint) ([]model.AdminGroup, error) {
	if m.err != nil {
		return nil, m.err
	}
	out := make([]model.AdminGroup, 0, len(ids))
	for _, id := range ids {
		g, ok := m.groups[id]
		if !ok {
			continue
		}
		if g.DeletedAt.Valid {
			continue
		}
		out = append(out, *g)
	}
	return out, nil
}

// mockAccessRepository 实现 AccessRepository。
type mockAccessRepository struct {
	byUID map[uint][]uint
	err   error
}

func (m *mockAccessRepository) ListGroupIDsByUID(uid uint) ([]uint, error) {
	if m.err != nil {
		return nil, m.err
	}
	ids := m.byUID[uid]
	out := make([]uint, len(ids))
	copy(out, ids)
	return out, nil
}

// =============================================================================
// 测试辅助：构造最小可用的 Repositories
// =============================================================================

// ptr 构造 *string 字面量。
func ptr(s string) *string { return &s }

// rule 构造 *AdminRule。
func rule(id uint, name string, status int8, weigh int) *model.AdminRule {
	return &model.AdminRule{ID: id, Name: name, Status: status, Weigh: weigh}
}

// group 构造 *AdminGroup。
func group(id uint, status int8, rules string) *model.AdminGroup {
	g := &model.AdminGroup{ID: id, Status: status}
	if rules != "" {
		g.Rules = ptr(rules)
	}
	return g
}

// =============================================================================
// TestPermission_IsSuperAdmin —— 覆盖 4 个用例：
//   wildcard / no-wildcard / no-group / disabled-admin
// =============================================================================

func TestPermission_IsSuperAdmin(t *testing.T) {
	cases := []struct {
		name  string
		repos Repositories
		uid   uint
		want  bool
	}{
		{
			name: "wildcard group grants super",
			repos: Repositories{
				Admin:  &mockAdminRepository{admins: map[uint]*model.Admin{1: {ID: 1, Status: 1}}},
				Access: &mockAccessRepository{byUID: map[uint][]uint{1: {10}}},
				Group: &mockGroupRepository{groups: map[uint]*model.AdminGroup{
					10: group(10, 1, "*"),
				}},
			},
			uid:  1,
			want: true,
		},
		{
			name: "non-wildcard rules are not super",
			repos: Repositories{
				Admin:  &mockAdminRepository{admins: map[uint]*model.Admin{1: {ID: 1, Status: 1}}},
				Access: &mockAccessRepository{byUID: map[uint][]uint{1: {10}}},
				Group: &mockGroupRepository{groups: map[uint]*model.AdminGroup{
					10: group(10, 1, "1,2,3"),
				}},
			},
			uid:  1,
			want: false,
		},
		{
			name: "admin with no groups",
			repos: Repositories{
				Admin:  &mockAdminRepository{admins: map[uint]*model.Admin{1: {ID: 1, Status: 1}}},
				Access: &mockAccessRepository{byUID: map[uint][]uint{}},
				Group:  &mockGroupRepository{groups: map[uint]*model.AdminGroup{}},
			},
			uid:  1,
			want: false,
		},
		{
			name: "disabled admin short-circuits to false",
			repos: Repositories{
				Admin: &mockAdminRepository{admins: map[uint]*model.Admin{1: {ID: 1, Status: 0}}},
				Access: &mockAccessRepository{byUID: map[uint][]uint{
					1: {10}, // 即使有 wildcard 分组也无效
				}},
				Group: &mockGroupRepository{groups: map[uint]*model.AdminGroup{
					10: group(10, 1, "*"),
				}},
			},
			uid:  1,
			want: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := New(tc.repos)
			if got := m.IsSuperAdmin(tc.uid); got != tc.want {
				t.Errorf("IsSuperAdmin() = %v, want %v", got, tc.want)
			}
		})
	}
}

// =============================================================================
// TestPermission_GetGroups —— 覆盖 4 个用例：
//   multi-group / no-group / disabled-group-excluded / soft-deleted-group-excluded
// =============================================================================

func TestPermission_GetGroups(t *testing.T) {
	cases := []struct {
		name    string
		repos   Repositories
		uid     uint
		wantIDs []uint
	}{
		{
			name: "admin in multiple groups returns both",
			repos: Repositories{
				Admin: &mockAdminRepository{admins: map[uint]*model.Admin{1: {ID: 1, Status: 1}}},
				Access: &mockAccessRepository{byUID: map[uint][]uint{
					1: {10, 20},
				}},
				Group: &mockGroupRepository{groups: map[uint]*model.AdminGroup{
					10: group(10, 1, "1,2"),
					20: group(20, 1, "*"),
				}},
			},
			uid:     1,
			wantIDs: []uint{10, 20},
		},
		{
			name: "admin with no groups returns empty",
			repos: Repositories{
				Admin:  &mockAdminRepository{admins: map[uint]*model.Admin{1: {ID: 1, Status: 1}}},
				Access: &mockAccessRepository{byUID: map[uint][]uint{}},
				Group:  &mockGroupRepository{groups: map[uint]*model.AdminGroup{}},
			},
			uid:     1,
			wantIDs: nil,
		},
		{
			name: "disabled group excluded",
			repos: Repositories{
				Admin: &mockAdminRepository{admins: map[uint]*model.Admin{1: {ID: 1, Status: 1}}},
				Access: &mockAccessRepository{byUID: map[uint][]uint{
					1: {10, 20},
				}},
				Group: &mockGroupRepository{groups: map[uint]*model.AdminGroup{
					10: group(10, 1, "1"),
					20: group(20, 0, "2"), // status=0
				}},
			},
			uid:     1,
			wantIDs: []uint{10},
		},
		{
			name: "soft-deleted group excluded by Repository",
			repos: Repositories{
				Admin: &mockAdminRepository{admins: map[uint]*model.Admin{1: {ID: 1, Status: 1}}},
				Access: &mockAccessRepository{byUID: map[uint][]uint{
					1: {10, 20},
				}},
				Group: &mockGroupRepository{groups: map[uint]*model.AdminGroup{
					10: group(10, 1, "1"),
					20: func() *model.AdminGroup {
						g := group(20, 1, "2")
						g.DeletedAt = gorm.DeletedAt{Valid: true}
						return g
					}(),
				}},
			},
			uid:     1,
			wantIDs: []uint{10},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := New(tc.repos)
			got := m.GetGroups(tc.uid)
			gotIDs := make([]uint, 0, len(got))
			for _, g := range got {
				gotIDs = append(gotIDs, g.ID)
			}
			if !equalIDs(gotIDs, tc.wantIDs) {
				t.Errorf("GetGroups() ids = %v, want %v", gotIDs, tc.wantIDs)
			}
		})
	}
}

// =============================================================================
// TestPermission_GetRuleIds —— 覆盖 4 个用例：
//   super-admin / regular / no-group / empty-rules
// =============================================================================

func TestPermission_GetRuleIds(t *testing.T) {
	cases := []struct {
		name    string
		repos   Repositories
		uid     uint
		wantIDs []uint
	}{
		{
			name: "wildcard group returns all active rule IDs",
			repos: Repositories{
				Admin: &mockAdminRepository{admins: map[uint]*model.Admin{1: {ID: 1, Status: 1}}},
				Access: &mockAccessRepository{byUID: map[uint][]uint{
					1: {10},
				}},
				Group: &mockGroupRepository{groups: map[uint]*model.AdminGroup{
					10: group(10, 1, "*"),
				}},
				Rule: &mockRuleRepository{activeIDs: []uint{1, 2, 3, 5}},
			},
			uid:     1,
			wantIDs: []uint{1, 2, 3, 5},
		},
		{
			name: "regular admin aggregates deduped union across groups",
			repos: Repositories{
				Admin: &mockAdminRepository{admins: map[uint]*model.Admin{1: {ID: 1, Status: 1}}},
				Access: &mockAccessRepository{byUID: map[uint][]uint{
					1: {10, 20},
				}},
				Group: &mockGroupRepository{groups: map[uint]*model.AdminGroup{
					10: group(10, 1, "1,2,3"),
					20: group(20, 1, "3,4,5"),
				}},
				Rule: &mockRuleRepository{},
			},
			uid:     1,
			wantIDs: []uint{1, 2, 3, 4, 5},
		},
		{
			name: "admin with no groups returns nil",
			repos: Repositories{
				Admin:  &mockAdminRepository{admins: map[uint]*model.Admin{1: {ID: 1, Status: 1}}},
				Access: &mockAccessRepository{byUID: map[uint][]uint{}},
				Group:  &mockGroupRepository{groups: map[uint]*model.AdminGroup{}},
				Rule:   &mockRuleRepository{activeIDs: []uint{1, 2, 3}},
			},
			uid:     1,
			wantIDs: nil,
		},
		{
			name: "groups with empty rules returns nil",
			repos: Repositories{
				Admin: &mockAdminRepository{admins: map[uint]*model.Admin{1: {ID: 1, Status: 1}}},
				Access: &mockAccessRepository{byUID: map[uint][]uint{
					1: {10, 20},
				}},
				Group: &mockGroupRepository{groups: map[uint]*model.AdminGroup{
					10: group(10, 1, ""),
					20: group(10, 1, ""),
				}},
				Rule: &mockRuleRepository{activeIDs: []uint{1, 2, 3}},
			},
			uid:     1,
			wantIDs: nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := New(tc.repos)
			got := m.GetRuleIds(tc.uid)
			if !equalIDs(got, tc.wantIDs) {
				t.Errorf("GetRuleIds() = %v, want %v", got, tc.wantIDs)
			}
		})
	}
}

// =============================================================================
// TestPermission_GetRules —— 覆盖 4 个用例：
//   super-admin / regular-multi-group / no-group / ordering
// =============================================================================

func TestPermission_GetRules(t *testing.T) {
	cases := []struct {
		name    string
		repos   Repositories
		uid     uint
		wantIDs []uint // 按 weigh ASC, id ASC 排序后的 id 序列
	}{
		{
			name: "wildcard returns all active rules sorted",
			repos: Repositories{
				Admin: &mockAdminRepository{admins: map[uint]*model.Admin{1: {ID: 1, Status: 1}}},
				Access: &mockAccessRepository{byUID: map[uint][]uint{
					1: {10},
				}},
				Group: &mockGroupRepository{groups: map[uint]*model.AdminGroup{
					10: group(10, 1, "*"),
				}},
				Rule: &mockRuleRepository{
					activeIDs: []uint{1, 2, 3},
					rules: map[uint]*model.AdminRule{
						1: rule(1, "user.read", 1, 10),
						2: rule(2, "user.write", 1, 5),
						3: rule(3, "user.delete", 1, 15),
					},
				},
			},
			uid:     1,
			wantIDs: []uint{2, 1, 3}, // weigh 5, 10, 15
		},
		{
			name: "regular multi-group aggregates and sorts",
			repos: Repositories{
				Admin: &mockAdminRepository{admins: map[uint]*model.Admin{1: {ID: 1, Status: 1}}},
				Access: &mockAccessRepository{byUID: map[uint][]uint{
					1: {10, 20},
				}},
				Group: &mockGroupRepository{groups: map[uint]*model.AdminGroup{
					10: group(10, 1, "1,2"),
					20: group(20, 1, "3"),
				}},
				Rule: &mockRuleRepository{
					rules: map[uint]*model.AdminRule{
						1: rule(1, "user.read", 1, 10),
						2: rule(2, "user.write", 1, 20),
						3: rule(3, "user.delete", 1, 5),
					},
				},
			},
			uid:     1,
			wantIDs: []uint{3, 1, 2}, // weigh 5, 10, 20
		},
		{
			name: "admin with no groups returns nil",
			repos: Repositories{
				Admin:  &mockAdminRepository{admins: map[uint]*model.Admin{1: {ID: 1, Status: 1}}},
				Access: &mockAccessRepository{byUID: map[uint][]uint{}},
				Group:  &mockGroupRepository{groups: map[uint]*model.AdminGroup{}},
				Rule:   &mockRuleRepository{},
			},
			uid:     1,
			wantIDs: nil,
		},
		{
			name: "same weigh orders by id ASC",
			repos: Repositories{
				Admin: &mockAdminRepository{admins: map[uint]*model.Admin{1: {ID: 1, Status: 1}}},
				Access: &mockAccessRepository{byUID: map[uint][]uint{
					1: {10},
				}},
				Group: &mockGroupRepository{groups: map[uint]*model.AdminGroup{
					10: group(10, 1, "1,2,3"),
				}},
				Rule: &mockRuleRepository{
					rules: map[uint]*model.AdminRule{
						1: rule(1, "a", 1, 100),
						2: rule(2, "b", 1, 100),
						3: rule(3, "c", 1, 100),
					},
				},
			},
			uid:     1,
			wantIDs: []uint{1, 2, 3}, // weigh 相同 → id 升序
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := New(tc.repos)
			rules := m.GetRules(tc.uid)
			gotIDs := make([]uint, 0, len(rules))
			for _, r := range rules {
				gotIDs = append(gotIDs, r.ID)
			}
			if !equalIDs(gotIDs, tc.wantIDs) {
				t.Errorf("GetRules() ids = %v, want %v", gotIDs, tc.wantIDs)
			}
		})
	}
}

// =============================================================================
// TestPermission_Check —— 覆盖 5 个用例：
//   super / has-rule / no-rule / disabled-admin / disabled-rule
// =============================================================================

func TestPermission_Check(t *testing.T) {
	cases := []struct {
		name     string
		repos    Repositories
		uid      uint
		ruleName string
		want     bool
	}{
		{
			name: "wildcard group grants check on any name",
			repos: Repositories{
				Admin: &mockAdminRepository{admins: map[uint]*model.Admin{1: {ID: 1, Status: 1}}},
				Access: &mockAccessRepository{byUID: map[uint][]uint{
					1: {10},
				}},
				Group: &mockGroupRepository{groups: map[uint]*model.AdminGroup{
					10: group(10, 1, "*"),
				}},
				Rule: &mockRuleRepository{},
			},
			uid:      1,
			ruleName: "any.name",
			want:     true,
		},
		{
			name: "admin in group with matching rule",
			repos: Repositories{
				Admin: &mockAdminRepository{admins: map[uint]*model.Admin{1: {ID: 1, Status: 1}}},
				Access: &mockAccessRepository{byUID: map[uint][]uint{
					1: {10},
				}},
				Group: &mockGroupRepository{groups: map[uint]*model.AdminGroup{
					10: group(10, 1, "1,2"),
				}},
				Rule: &mockRuleRepository{
					byName: map[string][]uint{
						"user.read": {1},
					},
				},
			},
			uid:      1,
			ruleName: "user.read",
			want:     true,
		},
		{
			name: "admin in group but rule not held",
			repos: Repositories{
				Admin: &mockAdminRepository{admins: map[uint]*model.Admin{1: {ID: 1, Status: 1}}},
				Access: &mockAccessRepository{byUID: map[uint][]uint{
					1: {10},
				}},
				Group: &mockGroupRepository{groups: map[uint]*model.AdminGroup{
					10: group(10, 1, "5,6"),
				}},
				Rule: &mockRuleRepository{
					byName: map[string][]uint{
						"user.read": {1},
					},
				},
			},
			uid:      1,
			ruleName: "user.read",
			want:     false,
		},
		{
			name: "disabled admin returns false",
			repos: Repositories{
				Admin: &mockAdminRepository{admins: map[uint]*model.Admin{1: {ID: 1, Status: 0}}},
				Access: &mockAccessRepository{byUID: map[uint][]uint{
					1: {10},
				}},
				Group: &mockGroupRepository{groups: map[uint]*model.AdminGroup{
					10: group(10, 1, "*"),
				}},
				Rule: &mockRuleRepository{},
			},
			uid:      1,
			ruleName: "any.name",
			want:     false,
		},
		{
			name: "disabled rule returns false via Repository filter",
			repos: Repositories{
				Admin: &mockAdminRepository{admins: map[uint]*model.Admin{1: {ID: 1, Status: 1}}},
				Access: &mockAccessRepository{byUID: map[uint][]uint{
					1: {10},
				}},
				Group: &mockGroupRepository{groups: map[uint]*model.AdminGroup{
					10: group(10, 1, "5"),
				}},
				Rule: &mockRuleRepository{
					rules: map[uint]*model.AdminRule{
						5: rule(5, "user.read", 0, 0), // status=0
					},
					// byName 不含 "user.read"，模拟 Repository 过滤掉 status=0
					byName: map[string][]uint{},
				},
			},
			uid:      1,
			ruleName: "user.read",
			want:     false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := New(tc.repos)
			got := m.Check(tc.uid, tc.ruleName)
			if got != tc.want {
				t.Errorf("Check() = %v, want %v", got, tc.want)
			}
		})
	}
}

// equalIDs 比较两个 []uint 是否相等。nil 与空切片视为相等。
func equalIDs(a, b []uint) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
