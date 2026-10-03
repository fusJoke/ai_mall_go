package user

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	userHandler "ai-go-mall/internal/handler/user"
	captchaInfra "ai-go-mall/internal/infra/captcha"
	"ai-go-mall/internal/model"
	"ai-go-mall/internal/repository"
	"ai-go-mall/internal/router/registry"
	userSvc "ai-go-mall/internal/service/user"
)

// expectedUserRoutes 列出 C 端 /user 的全部路由（含公开与 UserAuth 保护）。
var expectedUserRoutes = []struct {
	method string
	path   string
}{
	{http.MethodPost, "/user/login"},
	{http.MethodPost, "/user/logout"},
	{http.MethodGet, "/user/blindbox/list"},
	{http.MethodGet, "/user/blindbox/detail"},
	{http.MethodPost, "/user/blindbox/draw"},
	{http.MethodGet, "/user/orders/list"},
	{http.MethodGet, "/user/orders/detail"},
	{http.MethodGet, "/user/home/feed"},
}

// TestUserRoutes_AllRegistered 验证 8 条 C 端路由全部就位。
//
// 与 admin router 测试同构：Apply 前重挂一遍，独立于其他测试的运行顺序。
func TestUserRoutes_AllRegistered(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()

	// 消费 init() 期间登记的挂载（若其他测试已消费则为 no-op），
	// 避免下方重挂时对同一 engine 产生重复路由 panic。
	consume := gin.New()
	registry.Apply(consume)

	registerUserRoutes()
	registry.Apply(engine)

	routes := engine.Routes()
	got := make(map[string]bool, len(routes))
	for _, r := range routes {
		got[r.Method+" "+r.Path] = true
	}

	for _, want := range expectedUserRoutes {
		key := want.method + " " + want.path
		if !got[key] {
			t.Errorf("missing route %q", key)
		}
	}
}

// TestUserRoutes_ProtectedRoutes401WithoutToken 验证登录态路由确实挂了
// UserAuth：无 token 请求 → 401（UserAuth 在 handler 装配之前 abort，
// 因此测试不需要真实 token / captcha / DB 基础设施）。
//
// 说明：gin 的 RouteInfo 不暴露中间件链，无法静态断言"挂了哪个中间件"，
// 只能用行为验证 —— 401 即证明 UserAuth 已挂载且在 handler 之前生效。
func TestUserRoutes_ProtectedRoutes401WithoutToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()

	// 消费 init() 期间登记的挂载（若其他测试已消费则为 no-op），
	// 避免下方重挂时对同一 engine 产生重复路由 panic。
	consume := gin.New()
	registry.Apply(consume)

	registerUserRoutes()
	registry.Apply(engine)

	protected := []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/user/blindbox/draw"},
		{http.MethodGet, "/user/orders/list"},
		{http.MethodGet, "/user/orders/detail"},
	}
	for _, p := range protected {
		req := httptest.NewRequest(p.method, p.path, nil)
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)
		if w.Code != http.StatusUnauthorized {
			t.Errorf("%s %s: status = %d, want 401 (UserAuth expected)", p.method, p.path, w.Code)
		}
	}
}

// stubUserService 是 userSvc.Service 的最小桩：Login 固定返回
// ErrInvalidCredentials（映射 401），其余方法在本测试中不会被触达。
type stubUserService struct{}

func (stubUserService) Create(*gin.Context, *model.User) error { return nil }

func (stubUserService) List(*gin.Context, repository.ListOptions) ([]model.User, int64, error) {
	return nil, 0, nil
}

func (stubUserService) GetByID(*gin.Context, int64) (*model.User, error) { return nil, nil }

func (stubUserService) Update(*gin.Context, *model.User) error { return nil }

func (stubUserService) Delete(*gin.Context, int64) error { return nil }

func (stubUserService) Login(*gin.Context, string, string, string, []captchaInfra.Point, bool) (*model.User, string, error) {
	return nil, "", userSvc.ErrInvalidCredentials
}

func (stubUserService) Logout(context.Context, string) error { return nil }

// TestUserRoutes_HandlerResolvedAtRequestTime 是「nil 接收者」回归测试。
//
// registerUserRoutes() 由包 init() 调用，那一刻 authH 等包级 handler 变量还是
// nil。若把 authH.Login 这类**方法值**直接交给 wrap，方法值在 init 期就绑定了
// nil 接收者；请求期即使 ensureDeps() 已经给 authH 赋了值，被调用的仍是那个旧
// 方法值，于是 h.svc 解引用 nil → panic → 500（本次 7.4 端到端验证即由此暴露）。
//
// 本测试刻意在「注册之后」才设置 authH，因此只有真正在请求期读取变量的实现
// 才能走到业务分支：桩返回 ErrInvalidCredentials，期望 401。若实现回退成在
// init 期捕获方法值，这里会拿到 500（panic 被 gin Recovery 兜住）。
func TestUserRoutes_HandlerResolvedAtRequestTime(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()

	// 让 ensureDeps() 成为 no-op（真实依赖需要 token/captcha/DB，测试环境没有）。
	depsOnce.Do(func() {})
	depsInitErr = nil

	// 复现包 init() 的时序，且不依赖测试执行顺序：
	//   1) 注册时 handler 变量还是 nil —— 与 init() 期一致；
	//   2) 注册之后才把 handler 赋值 —— 与首次请求时 ensureDeps() 一致。
	authH = nil
	consume := gin.New()
	registry.Apply(consume) // 清空挂载表，保证下面登记的是本次这一批
	registerUserRoutes()
	authH = userHandler.NewAuthHandler(stubUserService{})

	registry.Apply(engine)

	body := `{"username":"u","password":"p","captcha_key":"k","points":[{"x":1,"y":1}]}`
	req := httptest.NewRequest(http.MethodPost, "/user/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 (stub ErrInvalidCredentials → 401); body=%s", w.Code, w.Body.String())
	}
}
