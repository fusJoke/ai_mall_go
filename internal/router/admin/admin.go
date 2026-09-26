// Package admin 是 /admin 前缀下的子路由集合。
//
// 通过 init() 自注册到 internal/router/registry；router.Setup 调用时由
// Apply 懒创建 /admin 对应的 r.Group 并挂载到引擎。
//
// 新增 /admin/* 路由 = 在本包内加一个 init() 调用 registry.Register("/admin", ...)。
package admin

import (
	"net/http"

	"github.com/gin-gonic/gin"

	adminHandler "ai-go-mall/internal/handler/admin"
	adminRepo "ai-go-mall/internal/repository/admin"
	"ai-go-mall/internal/router/registry"
	adminService "ai-go-mall/internal/service/admin"
	"ai-go-mall/internal/infra/token"
)

// 在包加载期一次性把 admin 的 repo → tokenMgr → service → handler 链路接好。
// router 层持有依赖装配逻辑，业务包保持纯净（不感知 HTTP）。
var (
	adminRepoInstance    = adminRepo.NewRepository()
	adminTokenInstance   = token.Get()
	adminServiceInstance = adminService.NewService(adminRepoInstance, adminTokenInstance)
	adminHandlerInstance = adminHandler.NewHandler(adminServiceInstance)
)

func init() {
	registry.Register("/admin", http.MethodGet, "/ping", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "admin pong"})
	})

	// POST /admin/login —— 管理员登录。
	registry.Register("/admin", http.MethodPost, "/login", adminHandlerInstance.Login)
}
