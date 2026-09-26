// Package middleware 收纳 gin 中间件。
//
// 当前仅含 CORS；后续（日志 / 鉴权 / 限流等）按域拆文件追加。
package middleware

import (
	"net/http"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"

	"ai-go-mall/internal/infra/config"
)

// CORS 返回 gin 跨域中间件，基于 github.com/gin-contrib/cors。
//
// 行为策略（按当前需求）：
//   - 仅对来源域名做限制，AllowOrigins 来自 config.CORS.AllowOrigins
//   - 默认允许所有请求头（AllowHeaders: "*"）与所有常用请求方法
//   - 预检请求（OPTIONS）结果最大缓存 24 小时
//   - 不开启 AllowCredentials（cookie / HTTP auth 跨域场景）；如未来需要
//     须显式打开并配套 AllowOrigins 收紧到精确域名（库强制约束）
func CORS() gin.HandlerFunc {
	// 防御：config 未初始化（cmd/serve 的路由单测）或 AllowOrigins 为空时，
	// 返回 no-op 中间件 —— 不写任何 CORS 头，浏览器侧表现为「跨域被拒」。
	// 这一行为也避免 gin-contrib/cors 在「全空 origins」配置下的 panic。
	//
	// 生产路径 cmd/serve/main.go 已保证 config.Init() 先于 newRouter 调用；
	// 若生产环境 AllowOrigins 误配成空，效果与未配置一致 —— 不允许任何来源。
	cfg := config.Get()
	if cfg == nil || len(cfg.CORS.AllowOrigins) == 0 {
		return func(c *gin.Context) {}
	}
	return cors.New(cors.Config{
		AllowOrigins: cfg.CORS.AllowOrigins,
		AllowMethods: []string{
			http.MethodGet,
			http.MethodPost,
			http.MethodPut,
			http.MethodPatch,
			http.MethodDelete,
			http.MethodHead,
			http.MethodOptions,
		},
		AllowHeaders: []string{"*"},
		MaxAge:       24 * time.Hour,
	})
}