// Package supplier 是 /supplier 前缀下的子路由集合。
//
// 通过 init() 自注册到 internal/router/registry；router.Setup 调用时由
// Apply 懒创建 /supplier 对应的 r.Group 并挂载到引擎。
//
// 路由清单（15 条）：
//
//	POST /supplier/login                    公开     供应商登录
//	POST /supplier/logout                   公开     供应商登出（幂等）
//	GET  /supplier/products/list            登录态   我的盲盒列表
//	POST /supplier/products/create          登录态   创建盲盒 + 卡池（事务）
//	POST /supplier/products/edit            登录态   编辑盲盒（失效详情缓存）
//	POST /supplier/products/toggle-onsale   登录态   上下架
//	GET  /supplier/promotions/list          登录态   我的活动列表
//	POST /supplier/promotions/create        登录态   创建限时特价（冲突校验）
//	POST /supplier/promotions/edit          登录态   改活动价格
//	POST /supplier/promotions/toggle        登录态   启停活动
//	POST /supplier/promotions/delete        登录态   删除活动
//	GET  /supplier/seckill/list             登录态   我的秒杀活动列表
//	POST /supplier/seckill/create           登录态   创建秒杀（卡池校验 + Redis init）
//	POST /supplier/seckill/edit             登录态   改秒杀价 / 限购
//	POST /supplier/seckill/toggle           登录态   启停秒杀
package supplier

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	supplierHandler "ai-go-mall/internal/handler/supplier"
	"ai-go-mall/internal/infra/cache"
	hotspotInfra "ai-go-mall/internal/infra/cache/hotspot"
	captchaInfra "ai-go-mall/internal/infra/captcha"
	"ai-go-mall/internal/infra/config"
	"ai-go-mall/internal/infra/database"
	"ai-go-mall/internal/infra/token"
	"ai-go-mall/internal/middleware"
	mallRepo "ai-go-mall/internal/repository/mall"
	supplierRepo "ai-go-mall/internal/repository/supplier"
	"ai-go-mall/internal/router/registry"
	"ai-go-mall/internal/service/preheat"
	supplierSvc "ai-go-mall/internal/service/supplier"
	userSvc "ai-go-mall/internal/service/user"
)

// handlers 收纳首次请求时装配的全部 B 端 handler（与 router/user 同模式）。
var (
	depsOnce    sync.Once
	authH       *supplierHandler.AuthHandler
	productH    *supplierHandler.ProductHandler
	promotionH  *supplierHandler.PromotionHandler
	seckillH    *supplierHandler.SeckillHandler
	depsInitErr error
)

func ensureDeps() {
	if depsInitErr != nil {
		panic("supplier router: deps init failed: " + depsInitErr.Error())
	}
	depsOnce.Do(func() {
		mgr, err := captchaInfra.GetManager()
		if err != nil {
			depsInitErr = fmt.Errorf("captcha manager init failed: %w", err)
			return
		}

		// auth：登录 / 登出。
		authSvc := supplierSvc.NewAuthService(supplierRepo.NewSupplierUserRepository(), supplierRepo.NewSupplierRepository(), token.Get(), mgr)
		authH = supplierHandler.NewAuthHandler(authSvc)

		// 商品管理（Update / ToggleOnSale 失效 C 端详情缓存）。
		productSvc := supplierSvc.NewProductService(supplierSvc.ProductServiceDeps{
			BlindBoxRepo: mallRepo.NewBlindBoxRepository(),
			CardPoolRepo: mallRepo.NewCardPoolRepository(),
			Cache:        cache.Get(),
			Hotspot:      hotspotInfra.Get(), // D21：写路径同步失效热点详情
		})
		productH = supplierHandler.NewProductHandler(productSvc)

		// 活动管理（Toggle / Delete 失效详情缓存）。
		promotionSvc := supplierSvc.NewPromotionService(supplierSvc.PromotionServiceDeps{
			PromotionRepo: mallRepo.NewPromotionRepository(),
			BlindBoxRepo:  mallRepo.NewBlindBoxRepository(),
			Cache:         cache.Get(),
			Hotspot:       hotspotInfra.Get(), // D21：写路径同步失效热点详情
			PreheatLoader: supplierBlindBoxDetailLoader(
				hotspotInfra.Get(), cache.Get(),
				config.Get().Cache.TTL.BlindBoxDetail.L2,
			), // D22：活动创建后预热盲盒详情
		})
		promotionH = supplierHandler.NewPromotionHandler(promotionSvc)

		// 秒杀管理（Create 同事务 SETNX Redis 库存 + 卡池库存校验）。
		seckillSvc := supplierSvc.NewSeckillService(supplierSvc.SeckillServiceDeps{
			SeckillRepo:  mallRepo.NewSeckillRepository(),
			BlindBoxRepo: mallRepo.NewBlindBoxRepository(),
			Cache:        cache.Get(),
			Hotspot:      hotspotInfra.Get(), // D22：写后预热
			PreheatLoader: supplierBlindBoxDetailLoader(
				hotspotInfra.Get(), cache.Get(),
				config.Get().Cache.TTL.BlindBoxDetail.L2,
			),
		})
		seckillH = supplierHandler.NewSeckillHandler(seckillSvc)
	})
}

// wrap 把 handler 方法包成「先 ensureDeps 再转发」的 gin.HandlerFunc。
//
// 关键：fn 必须是「请求期才读取包级 handler 变量」的闭包，**不能**直接传
// authH.Login 这类方法值。registerSupplierRoutes() 由包 init() 调用，那一刻
// authH 等变量还是 nil；init 期取到的方法值会永久绑定 nil 接收者，之后
// ensureDeps() 给变量赋了值也救不回来 —— 首次请求必定在 handler 内解引用
// nil → panic。写法与 internal/router/user 保持一致。
func wrap(fn func(*gin.Context)) gin.HandlerFunc {
	return func(c *gin.Context) {
		ensureDeps()
		fn(c)
	}
}

// registerSupplierRoutes 声明全部 /supplier 路由（测试场景可重挂）。
func registerSupplierRoutes() {
	// --- 鉴权（公开） ---
	registry.Register("/supplier", http.MethodPost, "/login", wrap(func(c *gin.Context) { authH.Login(c) }))
	registry.Register("/supplier", http.MethodPost, "/logout", wrap(func(c *gin.Context) { authH.Logout(c) }))

	// --- 商品管理（登录态） ---
	registry.Register("/supplier", http.MethodGet, "/products/list", wrap(func(c *gin.Context) { productH.List(c) }), middleware.SupplierAuth())
	registry.Register("/supplier", http.MethodPost, "/products/create", wrap(func(c *gin.Context) { productH.Create(c) }), middleware.SupplierAuth())
	registry.Register("/supplier", http.MethodPost, "/products/edit", wrap(func(c *gin.Context) { productH.Edit(c) }), middleware.SupplierAuth())
	registry.Register("/supplier", http.MethodPost, "/products/toggle-onsale", wrap(func(c *gin.Context) { productH.ToggleOnSale(c) }), middleware.SupplierAuth())

	// --- 活动管理（登录态） ---
	registry.Register("/supplier", http.MethodGet, "/promotions/list", wrap(func(c *gin.Context) { promotionH.List(c) }), middleware.SupplierAuth())
	registry.Register("/supplier", http.MethodPost, "/promotions/create", wrap(func(c *gin.Context) { promotionH.Create(c) }), middleware.SupplierAuth())
	registry.Register("/supplier", http.MethodPost, "/promotions/edit", wrap(func(c *gin.Context) { promotionH.Edit(c) }), middleware.SupplierAuth())
	registry.Register("/supplier", http.MethodPost, "/promotions/toggle", wrap(func(c *gin.Context) { promotionH.Toggle(c) }), middleware.SupplierAuth())
	registry.Register("/supplier", http.MethodPost, "/promotions/delete", wrap(func(c *gin.Context) { promotionH.Delete(c) }), middleware.SupplierAuth())

	// --- 秒杀管理（登录态） ---
	registry.Register("/supplier", http.MethodGet, "/seckill/list", wrap(func(c *gin.Context) { seckillH.List(c) }), middleware.SupplierAuth())
	registry.Register("/supplier", http.MethodPost, "/seckill/create", wrap(func(c *gin.Context) { seckillH.Create(c) }), middleware.SupplierAuth())
	registry.Register("/supplier", http.MethodPost, "/seckill/edit", wrap(func(c *gin.Context) { seckillH.Edit(c) }), middleware.SupplierAuth())
	registry.Register("/supplier", http.MethodPost, "/seckill/toggle", wrap(func(c *gin.Context) { seckillH.Toggle(c) }), middleware.SupplierAuth())
}

func init() {
	registerSupplierRoutes()
}

// supplierBlindBoxDetailLoader 构造一个用于 D22 预热的盲盒详情 loader。
//
// 入参全部传 depsOnce 内构造好的 repo/cache，避免 loader 闭包引用请求级 c.Request.Context()。
//
// loader 内部用 BlindBoxService.Detail 走完「热点 + TTL 缓存」原路径，
// 调用 preheat.PromotionPreheat / SeckillPreheat 时效果等同于「模拟一次冷读」，
// 下一次真正的 Get 直接命中。
func supplierBlindBoxDetailLoader(
	hotspot hotspotInfra.HotspotCache,
	c cache.Cache,
	cacheTTL time.Duration,
) preheat.BlindBoxLoader {
	return func(ctx context.Context, blindBoxID int64) (any, error) {
		bbSvc := userSvc.NewBlindBoxService(userSvc.BlindBoxServiceDeps{
			BlindBoxRepo:  mallRepo.NewBlindBoxRepository(),
			CardPoolRepo:  mallRepo.NewCardPoolRepository(),
			PromotionRepo: mallRepo.NewPromotionRepository(),
			SupplierRepo:  supplierRepo.NewSupplierRepository(),
			Hotspot:       hotspot,
			Cache:         c,
			CacheTTL:      cacheTTL,
		})
		// 脱离请求生命周期的 gin.Context（database.DetachedGinContext 内部
		// 调 context.WithoutCancel 保留 trace 值但去掉 cancel 信号）。
		dc := database.DetachedGinContext(ctx)
		return bbSvc.Detail(dc, blindBoxID)
	}
}
