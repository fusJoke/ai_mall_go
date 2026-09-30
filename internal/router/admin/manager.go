package admin

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"ai-go-mall/internal/middleware"
	"ai-go-mall/internal/router/registry"
)

// registerManagerRoutes 挂载 admin 管理页所需的 9 条路由：
//
//   - 5 条通用 CRUD（BaseHandler.RegisterRoutes）：POST/GET /admin/admin/{create,list,edit,delete}；
//   - 4 条专属：POST /admin/admin/{change-password,toggle-status,unlock,batch-delete}。
//
// 全部挂在 AdminAuth 中间件之后 —— 与现有 /admin/ping /init 行为一致。
//
// 为何不在 admin.go 的 init() 里展开：本文件单拆是为了让 admin.go 维持「登录 / 登出
// / init / upload / ping」的基础设施主线，管理页 CRUD 路由聚合在 manager.go 便于 review。
func registerManagerRoutes() {
	rgPath := "/admin"
	resource := "admin"

	// 通用 CRUD：5 条。
	registry.Register(rgPath, http.MethodPost, "/"+resource+"/create", func(c *gin.Context) {
		ensureDeps()
		adminHandlerInst.Create(c)
	}, middleware.AdminAuth())
	registry.Register(rgPath, http.MethodGet, "/"+resource+"/list", func(c *gin.Context) {
		ensureDeps()
		adminHandlerInst.List(c)
	}, middleware.AdminAuth())
	registry.Register(rgPath, http.MethodGet, "/"+resource+"/edit", func(c *gin.Context) {
		ensureDeps()
		adminHandlerInst.EditGet(c)
	}, middleware.AdminAuth())
	registry.Register(rgPath, http.MethodPost, "/"+resource+"/edit", func(c *gin.Context) {
		ensureDeps()
		adminHandlerInst.EditPost(c)
	}, middleware.AdminAuth())
	registry.Register(rgPath, http.MethodPost, "/"+resource+"/delete", func(c *gin.Context) {
		ensureDeps()
		adminHandlerInst.Delete(c)
	}, middleware.AdminAuth())

	// 专属操作：4 条。
	registry.Register(rgPath, http.MethodPost, "/"+resource+"/change-password", func(c *gin.Context) {
		ensureDeps()
		adminHandlerInst.ChangePassword(c)
	}, middleware.AdminAuth())
	registry.Register(rgPath, http.MethodPost, "/"+resource+"/toggle-status", func(c *gin.Context) {
		ensureDeps()
		adminHandlerInst.ToggleStatus(c)
	}, middleware.AdminAuth())
	registry.Register(rgPath, http.MethodPost, "/"+resource+"/unlock", func(c *gin.Context) {
		ensureDeps()
		adminHandlerInst.Unlock(c)
	}, middleware.AdminAuth())
	registry.Register(rgPath, http.MethodPost, "/"+resource+"/batch-delete", func(c *gin.Context) {
		ensureDeps()
		adminHandlerInst.BatchDelete(c)
	}, middleware.AdminAuth())
}
