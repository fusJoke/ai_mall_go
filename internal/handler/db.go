// Package handler 负责 HTTP 请求解析、参数校验、序列化响应。
//
// 当前文件仅放跨 handler 共用的小工具；具体业务 handler 后续按业务划分到子目录或子包。
package handler

import (
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"ai-go-mall/internal/infra/database"
)

// DB 取出当前请求 ctx 作用域内的 *gorm.DB，由 DBMiddleware 注入。
//
// 缺失时 panic（视为编程错误：未注册中间件）。在路由前请确认：
//
//	r.Use(gin.Recovery(), database.DBMiddleware())
//
// 调用方一般会在 service 层直接传 db 进去，而非在此 handler 内调用。
// 这里提供快捷访问，主要用于 handler 内需要直接做 DB 校验 / 调试的场景。
func DB(c *gin.Context) *gorm.DB {
	return database.FromContext(c)
}
