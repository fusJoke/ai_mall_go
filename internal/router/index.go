package router

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"ai-go-mall/internal/router/registry"
)

// init 注册根前缀下的路由（Prefix = ""）。
//
// 与 router.go 同包，无需空白导入即可被加载；只要 router 包被引用，
// 本文件的 init() 就会在启动期触发 registry.Register。
func init() {
	registry.Register("", http.MethodGet, "/ping", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "pong"})
	})
}
