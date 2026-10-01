package admin

import (
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"

	"ai-go-mall/internal/router/registry"
)

// expectedRuleRoutes 列出 8 条菜单规则管理页路由的标准 method+path。
// 与 rule.go 的注册声明保持一一对应。
var expectedRuleRoutes = []struct {
	method string
	path   string
}{
	{http.MethodPost, "/admin/rule/create"},
	{http.MethodGet, "/admin/rule/list"},
	{http.MethodGet, "/admin/rule/edit"},
	{http.MethodPost, "/admin/rule/edit"},
	{http.MethodPost, "/admin/rule/delete"},
	{http.MethodPost, "/admin/rule/toggle-status"},
	{http.MethodPost, "/admin/rule/batch-delete"},
	{http.MethodGet, "/admin/rule/all"},
}

// TestRuleRoutes_AllRegistered 验证：registry.Apply 把菜单规则的 8 条路由挂到
// gin 引擎，engine.Routes() 能列出每一条。
//
// 设计：包级 init() 已经调用 registerRuleRoutes()；registry.Apply 把所有
// prefix 的挂载一次性下发到引擎。本测试只关心"菜单规则 8 条路由是否就位"，
// 不去碰其他包注册的路由 —— 后者由各自的 router_test.go 覆盖。
//
// 已知 trade-off：registry.Apply 调用一次后会清空 mounts（见
// registry.Apply 文档）。当本测试与 manager_test.go 的 TestManagerRoutes_AllRegistered
// 在同一进程内并发运行，第二个跑到的测试会因 mounts 已被消费而失败。
// 解决办法：本测试在调用 Apply 前再次调用 registerRuleRoutes() 重挂一遍 ——
// 该函数幂等（重复注册同一组路由在 router_test 之外不会被调用，仅测试场景用）。
//
// 中间件（AdminAuth）"挂上与否"的验证放在 internal/middleware/auth_test.go
// 内，那里能替换 checkToken hook 验证 401 拦截。
func TestRuleRoutes_AllRegistered(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()

	// 见上方文档：补一次 registerRuleRoutes() 让本测试独立于其他测试的运行顺序。
	registerRuleRoutes()
	registry.Apply(engine)

	routes := engine.Routes()
	got := make(map[string]bool, len(routes))
	for _, r := range routes {
		got[r.Method+" "+r.Path] = true
	}

	for _, want := range expectedRuleRoutes {
		key := want.method + " " + want.path
		if !got[key] {
			t.Errorf("missing route %q", key)
		}
	}

	// 反向断言：已注册的路由数应 >= 8 条（本测试关心"至少这 8 条就位"）。
	if len(routes) < len(expectedRuleRoutes) {
		t.Errorf("engine.Routes() returned %d entries, want >= %d", len(routes), len(expectedRuleRoutes))
	}
}
