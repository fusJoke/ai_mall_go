package admin

import (
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"ai-go-mall/internal/model"
)

// ptrStr 构造 *string 字面量。
func ptrStrInit(s string) *string { return &s }

// ptrOpenType 构造 *AdminRuleOpenType。
func ptrOpenType(s model.AdminRuleOpenType) *model.AdminRuleOpenType { return &s }

// =============================================================================
// TestRouteFromRule —— 覆盖 design D3 表 + spec ADDED Requirement 6 退化规则。
// =============================================================================

func TestRouteFromRule(t *testing.T) {
	cases := []struct {
		name     string
		rule     model.AdminRule
		wantPath string
		wantName string
		wantComp string
		wantMenu string
		wantOpen any // string 或 nil（不存在）
	}{
		{
			name: "menu with full fields",
			rule: model.AdminRule{
				ID:        10,
				Type:      model.RuleTypeMenu,
				Title:     "用户列表",
				Name:      "user.list",
				Path:      "/admin/user/list",
				Icon:      "lucide:Users",
				OpenType:  ptrOpenType(model.RuleOpenTab),
				Url:       "",
				Component: "/admin/user/list",
				Keepalive: true,
				Extend:    model.RuleExtendNone,
				Weigh:     10,
				Pid:       1,
			},
			wantPath: "/admin/user/list",
			wantName: "user.list",
			wantComp: "/admin/user/list",
			wantMenu: "menu",
			wantOpen: "tab",
		},
		{
			name: "directory dir",
			rule: model.AdminRule{
				ID:        5,
				Type:      model.RuleTypeDir,
				Title:     "系统",
				Name:      "system",
				Path:      "/admin/system",
				Component: "/admin/system/index",
				Weigh:     1,
			},
			wantPath: "/admin/system",
			wantName: "system",
			wantComp: "/admin/system/index",
			wantMenu: "dir",
			wantOpen: nil,
		},
		{
			name: "permission node keeps fields but does not affect output",
			rule: model.AdminRule{
				ID:        20,
				Type:      model.RuleTypeNode,
				Title:     "用户删除",
				Name:      "user.delete",
				Path:      "/api/user/delete",
				Component: "",
				Weigh:     0,
			},
			wantPath: "/api/user/delete",
			wantName: "user.delete",
			wantComp: "404", // 空 component 退化
			wantMenu: "node",
			wantOpen: nil,
		},
		{
			name: "empty component falls back to 404 placeholder",
			rule: model.AdminRule{
				ID:    30,
				Type:  model.RuleTypeMenu,
				Name:  "orphan",
				Path:  "/admin/orphan",
				Title: "孤儿菜单",
			},
			wantPath: "/admin/orphan",
			wantName: "orphan",
			wantComp: "404",
			wantMenu: "menu",
			wantOpen: nil,
		},
		{
			name: "empty name falls back to rule-{id}",
			rule: model.AdminRule{
				ID:        42,
				Type:      model.RuleTypeMenu,
				Name:      "",
				Path:      "/admin/x",
				Component: "/admin/x",
				Title:     "X",
			},
			wantPath: "/admin/x",
			wantName: "rule-42",
			wantComp: "/admin/x",
			wantMenu: "menu",
			wantOpen: nil,
		},
		{
			name: "open_type nil omits field",
			rule: model.AdminRule{
				ID:       50,
				Type:     model.RuleTypeNode,
				Name:     "n",
				Path:     "/api/n",
				OpenType: nil,
			},
			wantPath: "/api/n",
			wantName: "n",
			wantComp: "404",
			wantMenu: "node",
			wantOpen: nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := routeFromRule(&tc.rule)
			if got["path"] != tc.wantPath {
				t.Errorf("path = %v, want %v", got["path"], tc.wantPath)
			}
			if got["name"] != tc.wantName {
				t.Errorf("name = %v, want %v", got["name"], tc.wantName)
			}
			if got["component"] != tc.wantComp {
				t.Errorf("component = %v, want %v", got["component"], tc.wantComp)
			}
			meta, ok := got["meta"].(map[string]any)
			if !ok {
				t.Fatalf("meta not map[string]any: %T", got["meta"])
			}
			if meta["menuType"] != tc.wantMenu {
				t.Errorf("meta.menuType = %v, want %v", meta["menuType"], tc.wantMenu)
			}
			openVal, hasOpen := meta["openType"]
			if tc.wantOpen == nil {
				if hasOpen {
					t.Errorf("meta.openType should be absent, got %v", openVal)
				}
			} else {
				if !hasOpen {
					t.Errorf("meta.openType absent, want %v", tc.wantOpen)
				} else if openVal != tc.wantOpen {
					t.Errorf("meta.openType = %v, want %v", openVal, tc.wantOpen)
				}
			}
		})
	}
}

// =============================================================================
// TestInitService_Init —— 覆盖 4 种核心场景：
//   super-admin / regular / disabled-admin / config-missing
// =============================================================================

// 通用 mock —— 与 permission_test.go 同包，避免跨包复制。
// 注意：这些 mock 也作为权限管理 Manager 复用。

type initMockAdmin struct {
	admins map[uint]*model.Admin
	err    error
}

func (m *initMockAdmin) GetByID(uid uint) (*model.Admin, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.admins[uid], nil
}

type initMockRule struct {
	activeMenu []model.AdminRule
	listByIDs  map[uint]*model.AdminRule
	byName     map[string][]uint
	activeIDs  []uint
	err        error
}

func (m *initMockRule) ListByIDs(ids []uint) ([]model.AdminRule, error) {
	if m.err != nil {
		return nil, m.err
	}
	out := make([]model.AdminRule, 0, len(ids))
	for _, id := range ids {
		if r, ok := m.listByIDs[id]; ok {
			out = append(out, *r)
		}
	}
	return out, nil
}

func (m *initMockRule) ExistsByIDsAndName(ids []uint, name string) (bool, error) {
	return false, nil
}

func (m *initMockRule) ListActiveIDs() ([]uint, error) {
	if m.err != nil {
		return nil, m.err
	}
	out := make([]uint, len(m.activeIDs))
	copy(out, m.activeIDs)
	return out, nil
}

func (m *initMockRule) ListActiveAsMenu() ([]model.AdminRule, error) {
	if m.err != nil {
		return nil, m.err
	}
	out := make([]model.AdminRule, len(m.activeMenu))
	copy(out, m.activeMenu)
	return out, nil
}

type initMockGroup struct {
	groups map[uint]*model.AdminGroup
}

func (m *initMockGroup) ListByIDs(ids []uint) ([]model.AdminGroup, error) {
	out := make([]model.AdminGroup, 0, len(ids))
	for _, id := range ids {
		if g, ok := m.groups[id]; ok && !g.DeletedAt.Valid {
			out = append(out, *g)
		}
	}
	return out, nil
}

type initMockAccess struct {
	byUID map[uint][]uint
}

func (m *initMockAccess) ListGroupIDsByUID(uid uint) ([]uint, error) {
	ids := m.byUID[uid]
	out := make([]uint, len(ids))
	copy(out, ids)
	return out, nil
}

// initMockConfig 仅供 init service 测试使用，复用 config_repository_test.go
// 的 mock 风格。
type initMockConfig struct {
	rows map[string]*model.Config
	err  error
}

func (m *initMockConfig) ListByNames(names []string) ([]model.Config, error) {
	if m.err != nil {
		return nil, m.err
	}
	out := make([]model.Config, 0, len(names))
	for _, name := range names {
		if row, ok := m.rows[name]; ok && row != nil {
			out = append(out, *row)
		}
	}
	return out, nil
}

func TestInitService_Init(t *testing.T) {
	wildcard := "*"
	listRules := "1,2"

	cases := []struct {
		name        string
		admin       *model.Admin
		adminErr    error
		groups      map[uint]*model.AdminGroup
		accessByUID map[uint][]uint
		activeMenu  []model.AdminRule
		listByIDs   map[uint]*model.AdminRule
		activeIDs   []uint
		configRows  map[string]*model.Config
		configErr   error
		ruleErr     error
		wantErr     error
		wantSuper   bool
		wantCfgKey  string // 检查 site_config 中是否含这个键
		wantMenuLen int
	}{
		{
			name: "super admin gets all active menus",
			admin: &model.Admin{
				ID:       1,
				Username: "root",
				Nickname: "Root",
				Status:   1,
			},
			groups: map[uint]*model.AdminGroup{
				10: {ID: 10, Status: 1, Rules: &wildcard},
			},
			accessByUID: map[uint][]uint{1: {10}},
			activeMenu: []model.AdminRule{
				{ID: 1, Type: model.RuleTypeMenu, Name: "dashboard", Path: "/admin/dashboard", Component: "/admin/dashboard", Weigh: 0, Status: 1},
				{ID: 2, Type: model.RuleTypeMenu, Name: "user.list", Path: "/admin/user/list", Component: "/admin/user/list", Weigh: 10, Status: 1},
				{ID: 3, Type: model.RuleTypeNode, Name: "user.delete", Path: "/api/user/delete", Component: "", Status: 1},
			},
			configRows: map[string]*model.Config{
				"name":          {Name: "name", Value: ptrStrInit("AI Mall")},
				"record_number": {Name: "record_number", Value: ptrStrInit("京ICP-1")},
				"version":       {Name: "version", Value: ptrStrInit("v1.0.0")},
			},
			wantSuper:   true,
			wantCfgKey:  "name",
			wantMenuLen: 3, // 含 1 个 node —— spec 要求 menus 数组包含
		},
		{
			name: "regular admin gets aggregated menus from groups",
			admin: &model.Admin{
				ID:       2,
				Username: "alice",
				Nickname: "Alice",
				Status:   1,
			},
			groups: map[uint]*model.AdminGroup{
				10: {ID: 10, Status: 1, Rules: &listRules}, // 1,2
			},
			accessByUID: map[uint][]uint{2: {10}},
			listByIDs: map[uint]*model.AdminRule{
				1: {ID: 1, Type: model.RuleTypeMenu, Name: "dashboard", Path: "/admin/dashboard", Component: "/admin/dashboard", Weigh: 0, Status: 1},
				2: {ID: 2, Type: model.RuleTypeMenu, Name: "user.list", Path: "/admin/user/list", Component: "/admin/user/list", Weigh: 10, Status: 1},
			},
			activeIDs: []uint{1, 2},
			configRows: map[string]*model.Config{
				"name": {Name: "name", Value: ptrStrInit("Mall")},
			},
			wantSuper:   false,
			wantCfgKey:  "name",
			wantMenuLen: 2,
		},
		{
			name: "disabled admin returns ErrAccountDisabled",
			admin: &model.Admin{
				ID:       3,
				Username: "bob",
				Status:   0,
			},
			wantErr:     ErrAccountDisabled,
			wantMenuLen: 0,
		},
		{
			name: "config missing rows produce empty string keys",
			admin: &model.Admin{
				ID:       4,
				Username: "carol",
				Status:   1,
			},
			groups:      map[uint]*model.AdminGroup{},
			accessByUID: map[uint][]uint{4: {}},
			configRows:  map[string]*model.Config{}, // 全部缺失
			wantSuper:   false,
			wantCfgKey:  "version", // 检查任意键都存在（即使为空）
			wantMenuLen: 0,
		},
		{
			name:    "admin not found returns ErrAccountDisabled",
			admin:   nil, // mock map 中查不到
			wantErr: ErrAccountDisabled,
		},
		{
			name:        "config repo error propagates",
			admin:       &model.Admin{ID: 5, Username: "dave", Status: 1},
			groups:      map[uint]*model.AdminGroup{},
			accessByUID: map[uint][]uint{5: {}},
			configRows:  map[string]*model.Config{},
			configErr:   errors.New("db down"),
			wantErr:     errors.New("db down"), // 任何非 nil error
		},
		{
			name:        "rule repo error on super path propagates",
			admin:       &model.Admin{ID: 6, Username: "eve", Status: 1},
			groups:      map[uint]*model.AdminGroup{10: {ID: 10, Status: 1, Rules: &wildcard}},
			accessByUID: map[uint][]uint{6: {10}},
			ruleErr:     errors.New("rules down"),
			wantErr:     errors.New("rules down"),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest("GET", "/admin/init", nil)

			// 组装 mock —— 容忍部分字段为 nil（disabled-admin 路径不查 group）
			var uid uint
			admins := map[uint]*model.Admin{}
			if tc.admin != nil {
				uid = uint(tc.admin.ID)
				admins[uid] = tc.admin
			}
			am := &initMockAdmin{admins: admins}
			if tc.adminErr != nil {
				am.err = tc.adminErr
			}
			groups := tc.groups
			if groups == nil {
				groups = map[uint]*model.AdminGroup{}
			}
			accessByUID := tc.accessByUID
			if accessByUID == nil {
				accessByUID = map[uint][]uint{}
			}
			rm := &initMockRule{
				activeMenu: tc.activeMenu,
				activeIDs:  tc.activeIDs,
				listByIDs:  tc.listByIDs,
				byName:     map[string][]uint{},
			}
			if tc.ruleErr != nil {
				rm.err = tc.ruleErr
			}
			cm := &initMockConfig{rows: tc.configRows}
			if tc.configErr != nil {
				cm.err = tc.configErr
			}

			pm := New(Repositories{
				Admin:  am,
				Rule:   rm,
				Group:  &initMockGroup{groups: groups},
				Access: &initMockAccess{byUID: accessByUID},
			})

			svc := NewInitService(am, pm, rm, cm)
			resp, err := svc.Init(c, uid)

			if tc.wantErr != nil {
				if err == nil {
					t.Fatalf("expected error, got nil (resp=%+v)", resp)
				}
				if tc.wantErr == ErrAccountDisabled && err != ErrAccountDisabled {
					t.Errorf("err = %v, want ErrAccountDisabled", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if resp == nil {
				t.Fatal("resp is nil")
			}
			if resp.Admin.Super != tc.wantSuper {
				t.Errorf("Admin.Super = %v, want %v", resp.Admin.Super, tc.wantSuper)
			}
			if v, ok := resp.SiteConfig[tc.wantCfgKey]; !ok {
				t.Errorf("SiteConfig missing key %q (got %v)", tc.wantCfgKey, resp.SiteConfig)
			} else if v == "" {
				// 空串也算合法
			}
			if len(resp.Menus) != tc.wantMenuLen {
				t.Errorf("Menus len = %d, want %d", len(resp.Menus), tc.wantMenuLen)
			}
		})
	}
}

// helper：避免 lastLoginAt 误用导致的 nil 解引用
