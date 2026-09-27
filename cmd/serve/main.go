// Command serve 启动 ai-go-mall 的 HTTP API 服务。
//
// 启动流程：
//  1. 从项目根目录加载 config/*.yaml 与 .env.yaml
//  2. 初始化数据库（GORM + dbresolver）
//  3. 注册 gin 路由（含 /healthz 与 /api/v1/ping 测试路由）
//  4. dev 模式额外挂载 /swagger/*any（cfg.Server.Mode == "dev" 时启用；
//     默认 release，生产 binary 不挂载 API 文档，避免忘设环境变量导致意外开放）
//  5. 监听 SIGINT / SIGTERM，触发优雅关闭
//
// 后续接入 internal/router 与 internal/handler 后，
// newRouter 会被替换为路由自动发现逻辑。
//
// @title           ai-go-mall API
// @version         1.0
// @description     AI GO MALL 后台 API 文档（仅 dev 环境暴露）。
// @host            localhost:8080
// @BasePath        /
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"

	// 引入 swag init 生成的 docs 包：包级 init() 会把 OpenAPI spec
	// 注册到 swag 全局，ginSwagger.WrapHandler 在第一次请求时读取。
	_ "ai-go-mall/docs"
	"ai-go-mall/internal/infra/config"
	"ai-go-mall/internal/infra/database"
	"ai-go-mall/internal/infra/token"
	"ai-go-mall/internal/middleware"
	"ai-go-mall/internal/router"
)

const shutdownTimeout = 10 * time.Second

func main() {
	if err := config.Init("."); err != nil {
		log.Fatalf("init config: %v", err)
	}
	if err := database.Init(); err != nil {
		log.Fatalf("init database: %v", err)
	}
	if err := token.Init(); err != nil {
		log.Fatalf("init token: %v", err)
	}
	cfg := config.Get()

	srv := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.Server.Port),
		Handler:           newRouter(cfg.Server.Name, cfg.Server.Mode == "dev", database.DBMiddleware()),
		ReadHeaderTimeout: shutdownTimeout,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		log.Printf("%s listening on %s", cfg.Server.Name, srv.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case err := <-errCh:
		if err != nil {
			log.Fatalf("server error: %v", err)
		}
	case <-ctx.Done():
		log.Printf("shutdown signal received")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("graceful shutdown failed: %v", err)
	}
}

// newRouter 注册当前可用的路由。
// 随着 internal/router、internal/middleware、internal/handler 落地，
// 此处会被路由自动发现逻辑替换。
//
// devMode 控制是否挂载 dev-only 路由（/swagger/*any）。
// 由 main() 在启动期根据 cfg.Server.Mode 判定后传入，避免函数本身读全局配置；
// 这样测试也可显式控制路由暴露，不需要污染 config.Init() 的全局状态。
//
// extraMW 用于追加额外中间件（生产环境传 database.DBMiddleware()，
// 单测不传以避免对全局 *gorm.DB 的依赖）。
func newRouter(serverName string, devMode bool, extraMW ...gin.HandlerFunc) http.Handler {
	r := gin.New()
	// 中间件顺序：Logger / Recovery 兜底 → CORS 处理跨域 → DB 中间件接入 *gorm.DB → 业务路由。
	// CORS 插在 DB 之前：OPTIONS 预检无需 DB 上下文，且业务路由前 CORS 头已就绪。
	mw := []gin.HandlerFunc{gin.Logger(), gin.Recovery(), middleware.CORS()}
	mw = append(mw, extraMW...)
	r.Use(mw...)

	// 业务路由：通过 router 包注册的 /ping、/admin/ping、/user/ping 等。
	router.Setup(r)

	// Swagger UI 与 OpenAPI JSON：仅 dev 模式暴露。
	// devMode 由 main() 根据 cfg.Server.Mode 显式传入，
	// 不依赖 gin.Mode() / GIN_MODE —— gin 默认 mode 本身就是 debug，
	// 依赖它意味着"忘设 GIN_MODE=release"就把 /swagger/*any 暴露出去，
	// 违反"仅 dev 暴露"的承诺。
	if devMode {
		r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))
	}

	r.GET("/healthz", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	api := r.Group("/api/v1")
	api.GET("/ping", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"message": "pong",
			"server":  serverName,
			"time":    time.Now().Format(time.RFC3339),
		})
	})

	return r
}
