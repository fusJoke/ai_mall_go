// Package user 是 /user 前缀下的子路由集合。
//
// 通过 init() 自注册到 internal/router/registry；router.Setup 调用时由
// Apply 懒创建 /user 对应的 r.Group 并挂载到引擎。
//
// 新增 /user/* 路由 = 在本包内加一个 init() 调用 registry.Register("/user", ...)。
//
// 路由清单（11 条）：
//
//	POST /user/login                  公开   会员登录
//	POST /user/logout                 公开   会员登出（幂等）
//	GET  /user/blindbox/list          公开   盲盒列表
//	GET  /user/blindbox/detail        公开   盲盒详情（概率公示 + 当前活动）
//	POST /user/blindbox/draw          登录   抽卡（UserAuth + 限流 30 次/分钟）
//	GET  /user/seckill/list           公开   当前生效秒杀活动列表
//	GET  /user/seckill/detail         公开   秒杀活动详情（限购 + 实时剩余名额）
//	POST /user/seckill/:id/draw       登录   秒杀抽卡（UserAuth + 限流 30 次/分钟）
//	GET  /user/orders/list            登录   我的订单
//	GET  /user/orders/detail          登录   订单详情
//	GET  /user/home/feed              公开   ES 首页推荐流
//	POST /user/follow                 登录   关注供应商（幂等 upsert）
//	DELETE /user/follow               登录   取关供应商（幂等）
//	GET  /user/follow/following       登录   我的关注分页列表
package user

import (
	"fmt"
	"net/http"
	"sync"

	"github.com/gin-gonic/gin"

	userHandler "ai-go-mall/internal/handler/user"
	"ai-go-mall/internal/infra/cache"
	hotspotInfra "ai-go-mall/internal/infra/cache/hotspot"
	captchaInfra "ai-go-mall/internal/infra/captcha"
	channelInfra "ai-go-mall/internal/infra/channel"
	"ai-go-mall/internal/infra/config"
	"ai-go-mall/internal/infra/mq"
	"ai-go-mall/internal/infra/search"
	"ai-go-mall/internal/infra/token"
	"ai-go-mall/internal/middleware"
	mallRepo "ai-go-mall/internal/repository/mall"
	supplierRepo "ai-go-mall/internal/repository/supplier"
	userRepo "ai-go-mall/internal/repository/user"
	"ai-go-mall/internal/router/registry"
	userSvc "ai-go-mall/internal/service/user"
)

// handlers 收纳首次请求时装配的全部 C 端 handler。
//
// 装配依赖 token.Get() / captchaInfra.GetManager() / cache.Get() /
// search.Get()，它们由 cmd/serve/main() 启动期 Init()；包级 var 阶段
// 可能仍是 nil。与 router/admin 的 ensureDeps 同一模式：sync.Once +
// 首次请求装配。
var (
	depsOnce    sync.Once
	authH       *userHandler.AuthHandler
	blindBoxH   *userHandler.BlindBoxHandler
	drawH       *userHandler.DrawHandler
	seckillH    *userHandler.SeckillHandler
	orderH      *userHandler.OrderHandler
	homeH       *userHandler.HomeHandler
	followH     *userHandler.FollowHandler
	depsInitErr error
)

func ensureDeps() {
	if depsInitErr != nil {
		panic("user router: deps init failed: " + depsInitErr.Error())
	}
	depsOnce.Do(func() {
		mgr, err := captchaInfra.GetManager()
		if err != nil {
			depsInitErr = fmt.Errorf("captcha manager init failed: %w", err)
			return
		}

		// auth：登录 / 登出。
		authSvc := userSvc.NewService(userRepo.NewRepository(), token.Get(), mgr)
		authH = userHandler.NewAuthHandler(authSvc)

		// 盲盒浏览（详情缓存走 cache infra；TTL 来自 config.Cache.TTL.BlindBoxDetail.L2）。
		blindBoxSvc := userSvc.NewBlindBoxService(userSvc.BlindBoxServiceDeps{
			BlindBoxRepo:  mallRepo.NewBlindBoxRepository(),
			CardPoolRepo:  mallRepo.NewCardPoolRepository(),
			PromotionRepo: mallRepo.NewPromotionRepository(),
			SupplierRepo:  supplierRepo.NewSupplierRepository(),
			Hotspot:       hotspotInfra.Get(), // D21：详情走热点缓存（30s 逻辑过期）
			Cache:         cache.Get(),
			CacheTTL:      config.Get().Cache.TTL.BlindBoxDetail.L2,
		})
		blindBoxH = userHandler.NewBlindBoxHandler(blindBoxSvc)

		// 抽卡事务。
		drawSvc := userSvc.NewDrawService(userSvc.DrawServiceDeps{
			BlindBoxRepo:   mallRepo.NewBlindBoxRepository(),
			CardPoolRepo:   mallRepo.NewCardPoolRepository(),
			PromotionRepo:  mallRepo.NewPromotionRepository(),
			SupplierRepo:   supplierRepo.NewSupplierRepository(),
			UserRepo:       userRepo.NewRepository(),
			DrawOrderRepo:  mallRepo.NewDrawOrderRepository(),
			CardRepo:       mallRepo.NewCardRepository(),
			SettlementRepo: mallRepo.NewSettlementRepository(), // spec 8.5 步骤 7.6：累加 total_revenue
			Channels:       channelInfra.Get(),                 // D18：事务后经 payment 渠道记录模拟支付
		})
		drawH = userHandler.NewDrawHandler(drawSvc)

		// 秒杀抽卡事务（D15）：复用 draw 的 repo + 加 SeckillRepo / Cache（Redis 限购 + 库存预扣）。
		seckillSvc := userSvc.NewSeckillService(userSvc.SeckillServiceDeps{
			BlindBoxRepo:   mallRepo.NewBlindBoxRepository(),
			CardPoolRepo:   mallRepo.NewCardPoolRepository(),
			CardRepo:       mallRepo.NewCardRepository(),
			SupplierRepo:   supplierRepo.NewSupplierRepository(),
			UserRepo:       userRepo.NewRepository(),
			DrawOrderRepo:  mallRepo.NewDrawOrderRepository(),
			SettlementRepo: mallRepo.NewSettlementRepository(),
			SeckillRepo:    mallRepo.NewSeckillRepository(),
			Cache:          cache.Get(),
			Publisher:      mq.Get(),           // D17：事务后异步 publish stock.deduction.sync
			Channels:       channelInfra.Get(), // D18：事务后经 payment 渠道记录模拟支付
		})

		// 秒杀浏览（List / Detail）：只读聚合，Redis 拿实时剩余名额、DB 兜底。
		seckillBrowseSvc := userSvc.NewSeckillBrowseService(userSvc.SeckillBrowseServiceDeps{
			SeckillRepo:  mallRepo.NewSeckillRepository(),
			BlindBoxRepo: mallRepo.NewBlindBoxRepository(),
			SupplierRepo: supplierRepo.NewSupplierRepository(),
			Cache:        cache.Get(),
		})
		seckillH = userHandler.NewSeckillHandler(seckillSvc, seckillBrowseSvc)

		// 订单查询。
		orderSvc := userSvc.NewOrderService(userSvc.OrderServiceDeps{
			DrawOrderRepo: mallRepo.NewDrawOrderRepository(),
		})
		orderH = userHandler.NewOrderHandler(orderSvc)

		// 首页 feed（ES 只读 + 多层缓存 L1/L2）。
		feedSvc := userSvc.NewFeedService(userSvc.FeedServiceDeps{
			Search:   search.Get(),
			Hotspot:  hotspotInfra.Get(), // D21：feed 走热点缓存（60s 逻辑过期）
			Cache:    cache.Get(),
			CacheTTL: config.Get().Cache.TTL.HomeFeed.L2,
		})
		homeH = userHandler.NewHomeHandler(feedSvc)

		// 关注供应商（D22 / Phase 18）：直接 DB 读写，不挂缓存。
		followSvc := userSvc.NewFollowService(userSvc.FollowServiceDeps{
			FollowRepo: mallRepo.NewFollowRepository(),
		})
		followH = userHandler.NewFollowHandler(followSvc)
	})
}

// wrap 把 handler 方法包成「先 ensureDeps 再转发」的 gin.HandlerFunc。
//
// 关键：fn 必须是「请求期才读取包级 handler 变量」的闭包，**不能**直接传
// authH.Login 这类方法值。registerUserRoutes() 由包 init() 调用，那一刻
// authH 等变量还是 nil；init 期取到的方法值会永久绑定 nil 接收者，之后
// ensureDeps() 给变量赋了值也救不回来 —— 首次请求必定在 handler 内解引用
// nil → panic（本次 7.4 端到端验证就是这样暴露的）。写成
// `func(c *gin.Context) { authH.Login(c) }` 让变量在请求期才被读取。
func wrap(fn func(*gin.Context)) gin.HandlerFunc {
	return func(c *gin.Context) {
		ensureDeps()
		fn(c)
	}
}

// drawRateLimitKey 从 context 抽取限流标识 "draw:{uid}"；无登录态返回空串
// （由 UserAuth 中间件自己 401，限流器 fail-open 放行）。
func drawRateLimitKey(c *gin.Context) string {
	u := middleware.UserFromContext(c)
	if u == nil {
		return ""
	}
	return fmt.Sprintf("draw:%d", u.ID)
}

// seckillRateLimitKey 与 draw 共享 N=30/min 限流策略；key 前缀分桶
// "seckill:{uid}" 避免与普通抽卡的 "draw:{uid}" 互相干扰（同一用户 1 分钟内
// 可以抽 30 次 + 抢 30 次秒杀，互相独立计数）。
func seckillRateLimitKey(c *gin.Context) string {
	u := middleware.UserFromContext(c)
	if u == nil {
		return ""
	}
	return fmt.Sprintf("seckill:%d", u.ID)
}

// registerUserRoutes 声明全部 /user 路由。独立成函数以便测试场景重挂
// （registry.Apply 会清空 mounts，见 registry 文档）。
func registerUserRoutes() {
	// --- 鉴权（公开） ---
	registry.Register("/user", http.MethodPost, "/login", wrap(func(c *gin.Context) { authH.Login(c) }))
	registry.Register("/user", http.MethodPost, "/logout", wrap(func(c *gin.Context) { authH.Logout(c) }))

	// --- 盲盒浏览（公开） ---
	registry.Register("/user", http.MethodGet, "/blindbox/list", wrap(func(c *gin.Context) { blindBoxH.List(c) }))
	registry.Register("/user", http.MethodGet, "/blindbox/detail", wrap(func(c *gin.Context) { blindBoxH.Detail(c) }))

	// --- 抽卡（登录态 + 限流：中间件顺序 = UserAuth → RateLimit，后者从
	// context 拿 uid，必须排在 auth 之后） ---
	registry.Register("/user", http.MethodPost, "/blindbox/draw", wrap(func(c *gin.Context) { drawH.Draw(c) }),
		middleware.UserAuth(),
		middleware.RateLimitPerMinute(drawRateLimitKey, 30),
	)

	// --- 秒杀浏览（公开） ---
	registry.Register("/user", http.MethodGet, "/seckill/list", wrap(func(c *gin.Context) { seckillH.List(c) }))
	registry.Register("/user", http.MethodGet, "/seckill/detail", wrap(func(c *gin.Context) { seckillH.Detail(c) }))

	// --- 秒杀抽卡（登录态 + 限流：与普通抽卡独立计数） ---
	registry.Register("/user", http.MethodPost, "/seckill/:id/draw", wrap(func(c *gin.Context) { seckillH.Draw(c) }),
		middleware.UserAuth(),
		middleware.RateLimitPerMinute(seckillRateLimitKey, 30),
	)

	// --- 订单（登录态） ---
	registry.Register("/user", http.MethodGet, "/orders/list", wrap(func(c *gin.Context) { orderH.List(c) }), middleware.UserAuth())
	registry.Register("/user", http.MethodGet, "/orders/detail", wrap(func(c *gin.Context) { orderH.Detail(c) }), middleware.UserAuth())

	// --- 首页 feed（公开） ---
	registry.Register("/user", http.MethodGet, "/home/feed", wrap(func(c *gin.Context) { homeH.Feed(c) }))

	// --- 关注供应商（登录态） ---
	registry.Register("/user", http.MethodPost, "/follow", wrap(func(c *gin.Context) { followH.Follow(c) }), middleware.UserAuth())
	registry.Register("/user", http.MethodDelete, "/follow", wrap(func(c *gin.Context) { followH.Unfollow(c) }), middleware.UserAuth())
	registry.Register("/user", http.MethodGet, "/follow/following", wrap(func(c *gin.Context) { followH.ListFollowing(c) }), middleware.UserAuth())
}

func init() {
	// GET /user/ping —— 健康探针。
	registry.Register("/user", http.MethodGet, "/ping", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "user pong"})
	})

	registerUserRoutes()
}
