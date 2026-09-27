// Package admin 是 /admin 前缀下的子路由集合。
//
// 通过 init() 自注册到 internal/router/registry；router.Setup 调用时由
// Apply 懒创建 /admin 对应的 r.Group 并挂载到引擎。
//
// 新增 /admin/* 路由 = 在本包内加一个 init() 调用 registry.Register("/admin", ...)。
package admin

import (
	"net/http"
	"sync"

	"github.com/gin-gonic/gin"

	"ai-go-mall/internal/handler/admin"
	captchaInfra "ai-go-mall/internal/infra/captcha"
	"ai-go-mall/internal/infra/token"
	adminRepo "ai-go-mall/internal/repository/admin"
	"ai-go-mall/internal/router/registry"
	adminService "ai-go-mall/internal/service/admin"
)

// adminRepoInstance 仍可在包级初始化（不依赖延迟资源）。
var adminRepoInstance = adminRepo.NewRepository()

// service/handler 延后到首次请求时再装配：
// 它们依赖 token.Get() 与 captchaInfra.GetManager()，两者均由 cmd/serve/main()
// 启动期调用 Init() 完成；包级 var 阶段拿到的可能仍是 nil，会让 Login 走到
// token / captcha 写入时空指针 panic。
//
// 用 sync.Once 锁住首调装配，保证并发安全 + 只装配一次。
var (
	serviceOnce      sync.Once
	adminSvcInstance adminService.Service
	adminHandlerInst *admin.Handler
)

func ensureDeps() {
	serviceOnce.Do(func() {
		mgr, err := captchaInfra.GetManager()
		if err != nil {
			// 启动期 captcha 配置不健全 → 让进程崩比带着坏依赖运行更安全。
			panic("admin: captcha manager init failed: " + err.Error())
		}
		adminSvcInstance = adminService.NewService(adminRepoInstance, token.Get(), mgr)
		adminHandlerInst = admin.NewHandler(adminSvcInstance)
	})
}

func init() {
	registry.Register("/admin", http.MethodGet, "/ping", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "admin pong"})
	})

	// POST /admin/login —— 管理员登录。
	// 包一层确保依赖装配好再转发给 handler，避免 token.Manager 还未初始化就被捕获。
	registry.Register("/admin", http.MethodPost, "/login", func(c *gin.Context) {
		ensureDeps()
		adminHandlerInst.Login(c)
	})

	// POST /admin/logout —— 管理员登出（公开、幂等；handler 内部已处理无 token / token 无效情况）。
	// 这里同样包 ensureDeps() 保证 service / handler 已就绪。
	registry.Register("/admin", http.MethodPost, "/logout", func(c *gin.Context) {
		ensureDeps()
		adminHandlerInst.Logout(c)
	})
}