//go:build integration

// Package admin 规则仓储的真连库对照测试。
//
// 存在理由（消除「mock 假绿」）：add-admin-rule-management 的 9.6 复验在真机上
// 发现 `create` 可以写入 `pid` 指向不存在节点的孤儿规则，而单测全绿 ——
// 因为当时的单测要么用 sqlmock **自己指定**返回 gorm.ErrRecordNotFound
// （真机 Pluck 根本产生不了这个错误），要么用 stub service 把错误直接喂给
// handler，**都没有真正走过 GORM 的行数判断**。
//
// 本文件让真实的 MySQL + 真实 GORM 参与判定：
//   - GetPID 对不存在的 id 必须返回 gorm.ErrRecordNotFound（旧实现用 Pluck，
//     查不到行时 err=nil，本文件的 TestIntegration_GetPID_NotFound 会失败）；
//   - HasChildren 对「不存在」与「无子」都必须返回 false 且 err=nil；
//   - List 的 Filters 必须真的落到 SQL（total 与 items 同口径）。
//
// 必须挂 //go:build integration，默认 `go test ./...` 不跑。
// 运行方式（**必须**用专用测试库，库名含 "test"）：
//
//	MYSQL_TEST_DSN='root:root123@tcp(127.0.0.1:3307)/ai_go_mall_test?charset=utf8mb4&parseTime=true' \
//	  go test -tags=integration ./internal/repository/admin/...
//
// 库需要预先存在（本测试只 AutoMigrate 表结构 + 清理自己的行，**不 DROP 库**）。
package admin

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"

	"ai-go-mall/internal/infra/database"
	"ai-go-mall/internal/model"
	"ai-go-mall/internal/repository"
)

// newRuleIntegrationContext 连真库并把 *gorm.DB 注入 gin ctx（repository.DB(c) 的来源）。
//
// 未设置 MYSQL_TEST_DSN → skip，与 service/admin 的集成测试约定一致。
func newRuleIntegrationContext(t *testing.T) *gin.Context {
	t.Helper()

	dsn := os.Getenv("MYSQL_TEST_DSN")
	if dsn == "" {
		t.Skip("MYSQL_TEST_DSN not set; skipping integration test")
	}

	// 安全闸：本测试会 DELETE admin_rule 全表，库名必须明显是测试库。
	dbName := dsnDBName(t, dsn)
	if !strings.Contains(strings.ToLower(dbName), "test") {
		t.Fatalf("refusing to touch database %q: integration test requires a dedicated test database (name must contain \"test\")", dbName)
	}

	gdb, err := gorm.Open(mysql.Open(dsn), &gorm.Config{
		// 与 internal/infra/database 的生产配置保持一致（SingularTable → admin_rule）。
		NamingStrategy: schema.NamingStrategy{SingularTable: true},
	})
	if err != nil {
		t.Fatalf("gorm.Open(%s): %v", dbName, err)
	}
	raw, err := gdb.DB()
	if err != nil {
		t.Fatalf("gdb.DB(): %v", err)
	}
	t.Cleanup(func() { _ = raw.Close() })

	if err := gdb.AutoMigrate(&model.AdminRule{}); err != nil {
		t.Fatalf("AutoMigrate admin_rule: %v", err)
	}

	// 清理：硬删本表全部行，保证断言不受历史数据影响。
	if err := gdb.Exec("DELETE FROM `admin_rule`").Error; err != nil {
		t.Fatalf("clean admin_rule: %v", err)
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c.Set(database.CtxKey, gdb)
	return c
}

// dsnDBName 从 DSN 里取出库名，形如 user:pass@tcp(host:port)/dbname?params。
func dsnDBName(t *testing.T, dsn string) string {
	t.Helper()
	slash := strings.LastIndex(dsn, "/")
	if slash < 0 {
		t.Fatalf("malformed MYSQL_TEST_DSN (no '/'): %s", dsn)
	}
	rest := dsn[slash+1:]
	if q := strings.Index(rest, "?"); q >= 0 {
		rest = rest[:q]
	}
	if rest == "" {
		t.Fatalf("malformed MYSQL_TEST_DSN (empty db name): %s", dsn)
	}
	return rest
}

// seededTree 记录 seedRuleTree 建出来的真实主键。
//
// 不能假设自增从 1 开始：`DELETE FROM` 不重置 AUTO_INCREMENT，
// 所以必须用 Create 回填的 id，否则断言会随历史数据漂移。
type seededTree struct {
	parentID uint // 有子节点的父
	childID  uint // 挂在 parentID 下
	leafID   uint // 独立叶子（无子）
	all      int  // 总行数
}

// seedRuleTree 造一棵最小规则树 + 一条独立叶子。
//
//	parent(dir)  ← child(node)
//	leaf(menu)
//	extra(menu)
func seedRuleTree(t *testing.T, c *gin.Context) seededTree {
	t.Helper()
	r := newTestRuleRepository()

	create := func(rule *model.AdminRule) uint {
		t.Helper()
		if err := r.Create(c, rule); err != nil {
			t.Fatalf("seed Create(%s): %v", rule.Name, err)
		}
		if rule.ID == 0 {
			t.Fatalf("seed Create(%s) 未回填 id —— 后续断言无法定位行", rule.Name)
		}
		return rule.ID
	}

	parentID := create(&model.AdminRule{Type: model.RuleTypeDir, Title: "管理员账号", Name: "admin/manager"})
	create(&model.AdminRule{Type: model.RuleTypeMenu, Title: "菜单规则", Name: "admin/rule"})
	leafID := create(&model.AdminRule{Type: model.RuleTypeMenu, Title: "订单管理", Name: "mall/order"})
	childID := create(&model.AdminRule{
		Type: model.RuleTypeNode, Title: "详情", Name: "admin/manager/detail", Pid: parentID,
	})

	return seededTree{parentID: parentID, childID: childID, leafID: leafID, all: 4}
}

func intPtr(v int) *int { return &v }

var _ = intPtr // 供后续扩展使用

// TestIntegration_GetPID_NotFound 是 D3 的**对照测试**。
//
// 旧实现（Pluck）查不到行时 err=nil、pid=0，本用例会失败；
// 新实现（Select+Take）返回 gorm.ErrRecordNotFound，本用例通过。
// 这条分支正是 service.ValidatePID 判定「父规则不存在」的唯一依据。
func TestIntegration_GetPID_NotFound(t *testing.T) {
	c := newRuleIntegrationContext(t)
	seedRuleTree(t, c)

	_, err := newTestRuleRepository().GetPID(c, 999999)
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("GetPID(999999) err = %v, want gorm.ErrRecordNotFound —— "+
			"若为 nil，说明仓储把「父规则不存在」当成了「pid=0（顶级）」，"+
			"ValidatePID 的 ErrPIDNotFound 分支将不可达，孤儿节点会被写进库", err)
	}
}

// TestIntegration_GetPID_HappyPath 验证存在时返回真实 pid（含 pid=0 的顶级）。
func TestIntegration_GetPID_HappyPath(t *testing.T) {
	c := newRuleIntegrationContext(t)
	tree := seedRuleTree(t, c)

	repo := newTestRuleRepository()

	top, err := repo.GetPID(c, tree.parentID)
	if err != nil {
		t.Fatalf("GetPID(%d): %v", tree.parentID, err)
	}
	if top != 0 {
		t.Errorf("GetPID(%d) = %d, want 0（顶级）", tree.parentID, top)
	}

	child, err := repo.GetPID(c, tree.childID)
	if err != nil {
		t.Fatalf("GetPID(%d): %v", tree.childID, err)
	}
	if child != tree.parentID {
		t.Errorf("GetPID(%d) = %d, want %d", tree.childID, child, tree.parentID)
	}
}

// TestIntegration_HasChildren 覆盖「有子 / 无子 / 不存在」三种情况。
//
// 第三种的判据与 GetPID 同源：不能靠「字段值非零」推断存在性。
func TestIntegration_HasChildren(t *testing.T) {
	c := newRuleIntegrationContext(t)
	tree := seedRuleTree(t, c)

	repo := newTestRuleRepository()

	cases := []struct {
		name string
		id   uint
		want bool
	}{
		{"有子节点", tree.parentID, true},
		{"无子节点", tree.leafID, false},
		{"不存在的 id", 999999, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := repo.HasChildren(c, tc.id)
			if err != nil {
				t.Fatalf("HasChildren(%d): %v", tc.id, err)
			}
			if got != tc.want {
				t.Errorf("HasChildren(%d) = %v, want %v", tc.id, got, tc.want)
			}
		})
	}
}

// TestIntegration_List_Filters 是 D1 的真库对照：?title= / ?name= 必须真的落 SQL。
func TestIntegration_List_Filters(t *testing.T) {
	c := newRuleIntegrationContext(t)
	tree := seedRuleTree(t, c)

	repo := newTestRuleRepository()

	t.Run("无过滤返回全量", func(t *testing.T) {
		items, total, err := repo.List(c, repository.ListOptions{Page: 1, PageSize: 20})
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if total != int64(tree.all) || len(items) != tree.all {
			t.Errorf("total=%d len=%d, want %d/%d", total, len(items), tree.all, tree.all)
		}
	})

	t.Run("title 模糊匹配（子串 + 大小写）", func(t *testing.T) {
		items, total, err := repo.List(c, repository.ListOptions{
			Page: 1, PageSize: 20, Filters: map[string]string{"title": "管理"},
		})
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if total != 2 {
			t.Fatalf("total = %d, want 2（管理员账号 / 订单管理）", total)
		}
		for _, it := range items {
			if !strings.Contains(it.Title, "管理") {
				t.Errorf("item %q does not contain 管理", it.Title)
			}
		}
	})

	t.Run("name 模糊匹配", func(t *testing.T) {
		items, total, err := repo.List(c, repository.ListOptions{
			Page: 1, PageSize: 20, Filters: map[string]string{"name": "admin/"},
		})
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if total != 3 {
			t.Fatalf("total = %d, want 3（admin/manager, admin/rule, admin/manager/detail）", total)
		}
		if len(items) != 3 {
			t.Errorf("len(items) = %d, want 3（total 与当前页必须同口径）", len(items))
		}
	})

	t.Run("不匹配的关键词返回空集", func(t *testing.T) {
		items, total, err := repo.List(c, repository.ListOptions{
			Page: 1, PageSize: 20, Filters: map[string]string{"title": "zzz_no_such"},
		})
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if total != 0 || len(items) != 0 {
			t.Errorf("total=%d len=%d, want 0/0", total, len(items))
		}
	})

	t.Run("通配符按字面量处理", func(t *testing.T) {
		_, total, err := repo.List(c, repository.ListOptions{
			Page: 1, PageSize: 20, Filters: map[string]string{"title": "%"},
		})
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if total != 0 {
			t.Errorf("total = %d, want 0 —— 关键词里的 %% 必须被转义成字面量，不能当通配符匹配全部", total)
		}
	})

	t.Run("title 与 name 同时给出按 AND 组合", func(t *testing.T) {
		_, total, err := repo.List(c, repository.ListOptions{
			Page: 1, PageSize: 20, Filters: map[string]string{"title": "管理", "name": "admin/manager"},
		})
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if total != 1 {
			t.Errorf("total = %d, want 1（只有「管理员账号」同时满足）", total)
		}
	})

	t.Run("不认识的 filter key 被忽略", func(t *testing.T) {
		_, total, err := repo.List(c, repository.ListOptions{
			Page: 1, PageSize: 20, Filters: map[string]string{"nonexistent_column": "x"},
		})
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if total != int64(tree.all) {
			t.Errorf("total = %d, want %d（无法识别的 key 必须被忽略而不是报错）", total, tree.all)
		}
	})
}

// 说明：service 层的 ValidatePID 把 GetPID 的 not-found 映射成 ErrPIDNotFound，
// 该映射本身已有 mock 单测覆盖；本文件只负责证明**仓储在真库上确实会返回
// gorm.ErrRecordNotFound** —— 缺失的正是这一环（旧单测用 mock 自己指定该错误，
// 掩盖了 Pluck 的真机行为）。端到端效果由 9.6 的真机复验（verify_96.py）把关。
