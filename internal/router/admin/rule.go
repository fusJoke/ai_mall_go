package admin

import (
	"net/http"

	"github.com/gin-gonic/gin"

	adminHandler "ai-go-mall/internal/handler/admin"
	"ai-go-mall/internal/infra/database"
	"ai-go-mall/internal/middleware"
	adminRepo "ai-go-mall/internal/repository/admin"
	"ai-go-mall/internal/router/registry"
	adminService "ai-go-mall/internal/service/admin"
)

// ruleHandlerInstance 与 admin.go 一致：延后到首次请求时再装配，确保依赖完整。
//
// 不能在包级 var 阶段装配：NewRuleRepository 需要 *gorm.DB，而 DB 由
// database.Init() 在 cmd/serve 启动期注入；首次 HTTP 请求到达时 Init 已完成，
// 此时 database.Get() 才是合法指针。
var ruleHandlerInst *adminHandler.RuleHandler

func ensureRuleDeps() {
	ensureDeps() // 保证 admin service / handler 已装配 —— 当前未依赖，但保持顺序一致便于扩展
	if ruleHandlerInst == nil {
		ruleRepo := adminRepo.NewRuleRepository(database.Get())
		ruleHandlerInst = adminHandler.NewRuleHandler(adminService.NewRuleService(ruleRepo))
	}
}

// registerRuleRoutes 挂载 admin/rule 管理页所需的 8 条路由：
//
//   - 5 条通用 CRUD（BaseHandler 默认）：POST/GET /admin/rule/{create,list,edit,delete}；
//   - 3 条专属：POST /admin/rule/{toggle-status,batch-delete}、GET /admin/rule/all。
//
// 全部挂在 AdminAuth 中间件之后 —— 与现有 /admin/manager 一致。
//
// 为何不在 admin.go 的 init() 里展开：本文件单拆是为了让 admin.go 维持「登录 / 登出
// / init / upload / ping」的基础设施主线，管理页 CRUD 路由聚合在 manager.go / rule.go
// 便于 review。
func registerRuleRoutes() {
	rgPath := "/admin"
	resource := "rule"

	// 通用 CRUD：5 条（BaseHandler 默认方法）。
	registry.Register(rgPath, http.MethodPost, "/"+resource+"/create", func(c *gin.Context) {
		ensureRuleDeps()
		ruleHandlerInst.Create(c)
	}, middleware.AdminAuth())
	registry.Register(rgPath, http.MethodGet, "/"+resource+"/list", func(c *gin.Context) {
		ensureRuleDeps()
		ruleHandlerInst.List(c)
	}, middleware.AdminAuth())
	registry.Register(rgPath, http.MethodGet, "/"+resource+"/edit", func(c *gin.Context) {
		ensureRuleDeps()
		ruleHandlerInst.EditGet(c)
	}, middleware.AdminAuth())
	registry.Register(rgPath, http.MethodPost, "/"+resource+"/edit", func(c *gin.Context) {
		ensureRuleDeps()
		ruleHandlerInst.EditPost(c)
	}, middleware.AdminAuth())
	registry.Register(rgPath, http.MethodPost, "/"+resource+"/delete", func(c *gin.Context) {
		ensureRuleDeps()
		ruleHandlerInst.Delete(c)
	}, middleware.AdminAuth())

	// 专属操作：3 条。
	registry.Register(rgPath, http.MethodPost, "/"+resource+"/toggle-status", func(c *gin.Context) {
		ensureRuleDeps()
		ruleHandlerInst.ToggleStatus(c)
	}, middleware.AdminAuth())
	registry.Register(rgPath, http.MethodPost, "/"+resource+"/batch-delete", func(c *gin.Context) {
		ensureRuleDeps()
		ruleHandlerInst.BatchDelete(c)
	}, middleware.AdminAuth())
	registry.Register(rgPath, http.MethodGet, "/"+resource+"/all", func(c *gin.Context) {
		ensureRuleDeps()
		ruleHandlerInst.ListAll(c)
	}, middleware.AdminAuth())
}
