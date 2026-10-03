// Package admin — mall.go 注册 admin 对供应商 / 盲盒 / 结算管理的 12 条路由。
//
// 全部挂 AdminAuth；handler 与 service 懒装配（首次请求时 sync.Once），
// 与 admin.go 的 ensureDeps 模式一致 —— mall 管理服务只依赖 DB 仓储，
// 不依赖 token / captcha，但统一延后装配保持同构、便于测试。
package admin

import (
	"net/http"
	"sync"

	"github.com/gin-gonic/gin"

	adminHandler "ai-go-mall/internal/handler/admin"
	cacheInfra "ai-go-mall/internal/infra/cache"
	channelInfra "ai-go-mall/internal/infra/channel"
	"ai-go-mall/internal/middleware"
	mallRepo "ai-go-mall/internal/repository/mall"
	supplierRepo "ai-go-mall/internal/repository/supplier"
	"ai-go-mall/internal/router/registry"
	adminSvc "ai-go-mall/internal/service/admin"
)

// mall handlers 的懒装配（独立于 admin.go 的 serviceOnce，避免耦合其 captcha 依赖）。
var (
	mallOnce        sync.Once
	mallSupplierH   *adminHandler.MallSupplierHandler
	mallPromotionH  *adminHandler.MallPromotionHandler
	mallBlindBoxH   *adminHandler.MallBlindBoxHandler
	mallSettlementH *adminHandler.MallSettlementHandler
	mallSeckillH    *adminHandler.MallSeckillHandler
	mallDepsInitErr error
)

func ensureMallDeps() {
	if mallDepsInitErr != nil {
		panic("admin router: mall deps init failed: " + mallDepsInitErr.Error())
	}
	mallOnce.Do(func() {
		supplierSvc := adminSvc.NewSupplierService(adminSvc.SupplierServiceDeps{
			SupplierRepo: supplierRepo.NewSupplierRepository(),
		})
		mallSupplierH = adminHandler.NewMallSupplierHandler(supplierSvc)

		blindBoxSvc := adminSvc.NewBlindBoxService(adminSvc.BlindBoxServiceDeps{
			BlindBoxRepo: mallRepo.NewBlindBoxRepository(),
		})
		mallBlindBoxH = adminHandler.NewMallBlindBoxHandler(blindBoxSvc)

		promotionSvc := adminSvc.NewPromotionService(adminSvc.PromotionServiceDeps{
			PromotionRepo: mallRepo.NewPromotionRepository(),
		})
		mallPromotionH = adminHandler.NewMallPromotionHandler(promotionSvc)

		// 结算管理：依赖 SettlementRepository + SupplierRepository（commission_rate 解析）。
		settlementSvc := adminSvc.NewSettlementService(adminSvc.SettlementServiceDeps{
			SettlementRepo: mallRepo.NewSettlementRepository(),
			SupplierRepo:   supplierRepo.NewSupplierRepository(),
			Channels:       channelInfra.Get(), // D18：MarkPaid 后经 bank 渠道记录打款
		})
		mallSettlementH = adminHandler.NewMallSettlementHandler(settlementSvc)

		// 秒杀管理（D15）：admin 视角全量分页 + 代供应商创建 + 强制启停。
		seckillSvc := adminSvc.NewSeckillService(adminSvc.SeckillServiceDeps{
			SeckillRepo:  mallRepo.NewSeckillRepository(),
			BlindBoxRepo: mallRepo.NewBlindBoxRepository(),
			Cache:        cacheInfra.Get(),
		})
		mallSeckillH = adminHandler.NewMallSeckillHandler(seckillSvc)
	})
}

// wrapMall 把「取 handler 方法」的函数包成「先 ensureMallDeps 再转发」的 gin.HandlerFunc。
//
// 入参刻意是 getter（func() func(*gin.Context)）而不是 handler 方法值：
// registerMallAdminRoutes() 由包 init() 调用，那一刻 mallSupplierH 等变量还是
// nil，直接传 mallSupplierH.List 会得到绑定 nil 接收者的方法值，之后
// ensureMallDeps() 赋值也救不回来 → 首次请求 panic（7.6 验证就是这样暴露的）。
// getter 强制方法值在请求期求值，写错成方法值直接编译不过 —— 用类型系统消灭
// 这一类 bug。router/user、router/supplier 用等价的闭包写法（它们有请求级回归
// 测试兜底；admin 的 12 条路由全挂 AdminAuth，测试环境拿不到合法 token，
// 只能靠这里的类型约束）。
func wrapMall(get func() func(*gin.Context)) gin.HandlerFunc {
	return func(c *gin.Context) {
		ensureMallDeps()
		fn := get()
		if fn == nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"code": "admin.handler_unavailable"})
			return
		}
		fn(c)
	}
}

// registerMallAdminRoutes 声明供应商 / 盲盒 / 结算管理的 12 条路由（测试场景可重挂）。
func registerMallAdminRoutes() {
	// --- 供应商管理 ---
	registry.Register("/admin", http.MethodGet, "/supplier/list", wrapMall(func() func(*gin.Context) { return mallSupplierH.List }), middleware.AdminAuth())
	registry.Register("/admin", http.MethodPost, "/supplier/toggle-status", wrapMall(func() func(*gin.Context) { return mallSupplierH.ToggleStatus }), middleware.AdminAuth())
	registry.Register("/admin", http.MethodPost, "/supplier/toggle-featured", wrapMall(func() func(*gin.Context) { return mallSupplierH.ToggleFeatured }), middleware.AdminAuth())

	// --- 活动管理（只读） ---
	registry.Register("/admin", http.MethodGet, "/promotion/list", wrapMall(func() func(*gin.Context) { return mallPromotionH.List }), middleware.AdminAuth())

	// --- 盲盒管理 ---
	registry.Register("/admin", http.MethodGet, "/blindbox/list", wrapMall(func() func(*gin.Context) { return mallBlindBoxH.List }), middleware.AdminAuth())
	registry.Register("/admin", http.MethodPost, "/blindbox/toggle-status", wrapMall(func() func(*gin.Context) { return mallBlindBoxH.ToggleStatus }), middleware.AdminAuth())
	registry.Register("/admin", http.MethodPost, "/blindbox/toggle-onsale", wrapMall(func() func(*gin.Context) { return mallBlindBoxH.ToggleOnSale }), middleware.AdminAuth())
	registry.Register("/admin", http.MethodPost, "/blindbox/toggle-featured", wrapMall(func() func(*gin.Context) { return mallBlindBoxH.ToggleFeatured }), middleware.AdminAuth())

	// --- 结算管理（spec 8.7：5 条路由） ---
	registry.Register("/admin", http.MethodGet, "/settlement/list", wrapMall(func() func(*gin.Context) { return mallSettlementH.List }), middleware.AdminAuth())
	registry.Register("/admin", http.MethodGet, "/settlement/detail", wrapMall(func() func(*gin.Context) { return mallSettlementH.Detail }), middleware.AdminAuth())
	registry.Register("/admin", http.MethodPost, "/settlement/preview", wrapMall(func() func(*gin.Context) { return mallSettlementH.Preview }), middleware.AdminAuth())
	registry.Register("/admin", http.MethodPost, "/settlement/generate", wrapMall(func() func(*gin.Context) { return mallSettlementH.Generate }), middleware.AdminAuth())
	registry.Register("/admin", http.MethodPost, "/settlement/mark-paid", wrapMall(func() func(*gin.Context) { return mallSettlementH.MarkPaid }), middleware.AdminAuth())

	// --- 秒杀管理（D15：3 条路由） ---
	registry.Register("/admin", http.MethodGet, "/seckill/list", wrapMall(func() func(*gin.Context) { return mallSeckillH.List }), middleware.AdminAuth())
	registry.Register("/admin", http.MethodPost, "/seckill", wrapMall(func() func(*gin.Context) { return mallSeckillH.Create }), middleware.AdminAuth())
	registry.Register("/admin", http.MethodPost, "/seckill/:id/toggle", wrapMall(func() func(*gin.Context) { return mallSeckillH.Toggle }), middleware.AdminAuth())
}

func init() {
	registerMallAdminRoutes()
}
