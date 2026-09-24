// Package user 是 /user 前缀下的子路由集合。
//
// 通过 init() 自注册到 internal/router/registry；router.Setup 调用时由
// Apply 懒创建 /user 对应的 r.Group 并挂载到引擎。
//
// 新增 /user/* 路由 = 在本包内加一个 init() 调用 registry.Register("/user", ...)。
package user

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"ai-go-mall/internal/router/registry"
)

func init() {
	registry.Register("/user", http.MethodGet, "/ping", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "user pong"})
	})
}
