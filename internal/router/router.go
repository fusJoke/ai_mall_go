// Package router 是全站业务路由的入口。internal/router 通过 init() 自注册 +
// 空白导入完成子路由的自动发现。
//
// 用法（main.go）：
//
//	r := gin.New()
//	router.Setup(r)
//
// 新增 /xxx/* 路由：
//  1. 在 internal/router/xxx/xxx.go 写一个 init() 调 registry.Register("/xxx", ...)
//  2. 在本文件加一行空白导入：_ "ai-go-mall/internal/router/xxx"
//
// 不需要改 Setup 也不需要改 Apply。
package router

import (
	"github.com/gin-gonic/gin"

	"ai-go-mall/internal/router/registry"

	// 子路由自动发现：空白导入触发各子包 init() 把路由挂载登记到 registry。
	_ "ai-go-mall/internal/router/admin"
	_ "ai-go-mall/internal/router/common"
	_ "ai-go-mall/internal/router/supplier"
	_ "ai-go-mall/internal/router/user"
)

// Setup 把所有延迟登记的挂载下发到 gin 引擎。
//
// 由 main() 在 engine 构造完毕之后调一次；调用前所有子路由的 init() 必须已执行
// （通过空白导入保证顺序）。
func Setup(r *gin.Engine) {
	registry.Apply(r)
}
