package admin

import (
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"

	"ai-go-mall/internal/router/registry"
)

// expectedManagerRoutes 列出 9 条 admin 管理页路由的标准 method+path。
// 与 manager.go 的注册声明保持一一对应。
var expectedManagerRoutes = []struct {
	method string
	path   string
}{
	{http.MethodPost, "/admin/admin/create"},
	{http.MethodGet, "/admin/admin/list"},
	{http.MethodGet, "/admin/admin/edit"},
	{http.MethodPost, "/admin/admin/edit"},
	{http.MethodPost, "/admin/admin/delete"},
	{http.MethodPost, "/admin/admin/change-password"},
	{http.MethodPost, "/admin/admin/toggle-status"},
	{http.MethodPost, "/admin/admin/unlock"},
	{http.MethodPost, "/admin/admin/batch-delete"},
}

// TestManagerRoutes_AllRegistered 验证：registry.Apply 把 admin 管理页的 9 条
// 路由挂到 gin 引擎，engine.Routes() 能列出每一条。
//
// 设计：包级 init() 已经调用 registerManagerRoutes()；registry.Apply 把所有
// prefix 的挂载一次性下发到引擎。本测试只关心"admin 管理页 9 条路由是否就位"，
// 不去碰其他包注册的路由 —— 后者由各自的 router_test.go 覆盖。
//
// 中间件（AdminAuth）"挂上与否"的验证放在 internal/middleware/auth_test.go
// 内，那里能替换 checkToken hook 验证 401 拦截。
func TestManagerRoutes_AllRegistered(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()

	// 消费既有挂载（若其他测试已消费则为 no-op）+ 重挂本组路由 ——
	// 与 rule_test.go 同模式：让本测试独立于同包其他测试的执行顺序
	// （新增 mall_routes_test.go 后原本的"首个 Apply 消费一切"假设不再成立）。
	consume := gin.New()
	registry.Apply(consume)
	registerManagerRoutes()
	registry.Apply(engine)

	routes := engine.Routes()
	got := make(map[string]bool, len(routes))
	for _, r := range routes {
		got[r.Method+" "+r.Path] = true
	}

	for _, want := range expectedManagerRoutes {
		key := want.method + " " + want.path
		if !got[key] {
			t.Errorf("missing route %q", key)
		}
	}
}
