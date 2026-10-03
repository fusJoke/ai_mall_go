package supplier

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	supplierHandler "ai-go-mall/internal/handler/supplier"
	captchaInfra "ai-go-mall/internal/infra/captcha"
	"ai-go-mall/internal/model/mall"
	"ai-go-mall/internal/router/registry"
	supplierSvc "ai-go-mall/internal/service/supplier"
)

// expectedSupplierRoutes 列出 B 端 /supplier 的全部路由。
var expectedSupplierRoutes = []struct {
	method string
	path   string
}{
	{http.MethodPost, "/supplier/login"},
	{http.MethodPost, "/supplier/logout"},

	{http.MethodGet, "/supplier/products/list"},
	{http.MethodPost, "/supplier/products/create"},
	{http.MethodPost, "/supplier/products/edit"},
	{http.MethodPost, "/supplier/products/toggle-onsale"},

	{http.MethodGet, "/supplier/promotions/list"},
	{http.MethodPost, "/supplier/promotions/create"},
	{http.MethodPost, "/supplier/promotions/edit"},
	{http.MethodPost, "/supplier/promotions/toggle"},
	{http.MethodPost, "/supplier/promotions/delete"},
}

// TestSupplierRoutes_AllRegistered 验证 11 条 B 端路由全部就位。
func TestSupplierRoutes_AllRegistered(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()

	// 消费 init() 期间登记的挂载（若其他测试已消费则为 no-op），
	// 避免下方重挂时对同一 engine 产生重复路由 panic。
	consume := gin.New()
	registry.Apply(consume)

	registerSupplierRoutes()
	registry.Apply(engine)

	routes := engine.Routes()
	got := make(map[string]bool, len(routes))
	for _, r := range routes {
		got[r.Method+" "+r.Path] = true
	}

	for _, want := range expectedSupplierRoutes {
		key := want.method + " " + want.path
		if !got[key] {
			t.Errorf("missing route %q", key)
		}
	}
}

// TestSupplierRoutes_AuthRequiredExceptLoginLogout 验证除 login / logout 外
// 全部挂 SupplierAuth：无 token 请求 → 401（SupplierAuth 在 handler 装配之前
// abort，测试不需要真实基础设施；gin RouteInfo 不暴露中间件链，只能行为验证）。
func TestSupplierRoutes_AuthRequiredExceptLoginLogout(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()

	// 消费 init() 期间登记的挂载（若其他测试已消费则为 no-op），
	// 避免下方重挂时对同一 engine 产生重复路由 panic。
	consume := gin.New()
	registry.Apply(consume)

	registerSupplierRoutes()
	registry.Apply(engine)

	for _, want := range expectedSupplierRoutes {
		if want.path == "/supplier/login" || want.path == "/supplier/logout" {
			continue
		}
		req := httptest.NewRequest(want.method, want.path, nil)
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)
		if w.Code != http.StatusUnauthorized {
			t.Errorf("%s %s: status = %d, want 401 (SupplierAuth expected)", want.method, want.path, w.Code)
		}
	}
}

// stubAuthService 是 supplierSvc.AuthService 的最小桩：Login 固定返回
// ErrInvalidCredentials（映射 401）。
type stubAuthService struct{}

func (stubAuthService) Login(*gin.Context, string, string, string, []captchaInfra.Point, bool) (*mall.MallSupplierUser, string, error) {
	return nil, "", supplierSvc.ErrInvalidCredentials
}

func (stubAuthService) Logout(context.Context, string) error { return nil }

// TestSupplierRoutes_HandlerResolvedAtRequestTime 是「nil 接收者」回归测试
// （与 router/user 的同名测试同构）。
//
// registerSupplierRoutes() 由包 init() 调用，那一刻 authH 仍是 nil。若把
// authH.Login 这类**方法值**直接交给 wrap，方法值在 init 期就绑定 nil 接收者，
// 请求期被调用的仍是那个旧方法值 → h.svc 解引用 nil → panic → 500。
//
// 本测试显式复现该时序（先以 nil 注册、再赋值），因此只有「请求期读取变量」的
// 实现能走到业务分支：桩返回 ErrInvalidCredentials，期望 401。
func TestSupplierRoutes_HandlerResolvedAtRequestTime(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()

	// ensureDeps() 变 no-op（真实依赖需要 token / captcha / DB）。
	depsOnce.Do(func() {})
	depsInitErr = nil

	authH = nil
	consume := gin.New()
	registry.Apply(consume)
	registerSupplierRoutes()
	authH = supplierHandler.NewAuthHandler(stubAuthService{})

	registry.Apply(engine)

	body := `{"username":"u","password":"p","captcha_key":"k","points":[{"x":1,"y":1}]}`
	req := httptest.NewRequest(http.MethodPost, "/supplier/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 (stub ErrInvalidCredentials → 401); body=%s", w.Code, w.Body.String())
	}
}
