package admin

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"ai-go-mall/internal/router/registry"
)

// expectedMallAdminRoutes 列出 admin 对供应商 / 盲盒 / 结算管理的 12 条路由。
var expectedMallAdminRoutes = []struct {
	method string
	path   string
}{
	{http.MethodGet, "/admin/supplier/list"},
	{http.MethodPost, "/admin/supplier/toggle-status"},
	{http.MethodPost, "/admin/supplier/toggle-featured"},
	{http.MethodGet, "/admin/blindbox/list"},
	{http.MethodPost, "/admin/blindbox/toggle-status"},
	{http.MethodPost, "/admin/blindbox/toggle-onsale"},
	{http.MethodPost, "/admin/blindbox/toggle-featured"},
	{http.MethodGet, "/admin/promotion/list"},
	// --- 结算管理（spec 8.7） ---
	{http.MethodGet, "/admin/settlement/list"},
	{http.MethodGet, "/admin/settlement/detail"},
	{http.MethodPost, "/admin/settlement/preview"},
	{http.MethodPost, "/admin/settlement/generate"},
	{http.MethodPost, "/admin/settlement/mark-paid"},
}

// TestMallAdminRoutes_AllRegistered 验证 12 条 mall 管理路由全部就位。
//
// 与 manager/rule 测试同构：Apply 前重挂一遍，独立于其他测试的运行顺序。
func TestMallAdminRoutes_AllRegistered(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()

	// 消费 init() 期间登记的挂载（若其他测试已消费则为 no-op），
	// 避免下方重挂时对同一 engine 产生重复路由 panic。
	consume := gin.New()
	registry.Apply(consume)

	registerMallAdminRoutes()
	registry.Apply(engine)

	routes := engine.Routes()
	got := make(map[string]bool, len(routes))
	for _, r := range routes {
		got[r.Method+" "+r.Path] = true
	}

	for _, want := range expectedMallAdminRoutes {
		key := want.method + " " + want.path
		if !got[key] {
			t.Errorf("missing route %q", key)
		}
	}
}

// TestMallAdminRoutes_AllCarryAdminAuth 验证每条 mall 管理路由确实挂了
// AdminAuth：无 token 请求 → 401（AdminAuth 在 handler 装配之前 abort，
// 测试不需要真实基础设施；gin RouteInfo 不暴露中间件链，只能行为验证）。
func TestMallAdminRoutes_AllCarryAdminAuth(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()

	// 消费 init() 期间登记的挂载（若其他测试已消费则为 no-op），
	// 避免下方重挂时对同一 engine 产生重复路由 panic。
	consume := gin.New()
	registry.Apply(consume)

	registerMallAdminRoutes()
	registry.Apply(engine)

	for _, want := range expectedMallAdminRoutes {
		req := httptest.NewRequest(want.method, want.path, nil)
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)
		if w.Code != http.StatusUnauthorized {
			t.Errorf("%s %s: status = %d, want 401 (AdminAuth expected)", want.method, want.path, w.Code)
		}
	}
}
