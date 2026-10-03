// database — detached.go 提供脱离请求生命周期的 gin 上下文构造。
//
// 消费者：后台异步任务里跑 repository 层（如 hotspot 异步刷新、MQ 消费者）。
// 请求级 DB 会话绑定 c.Request.Context()，请求结束后被取消 —— 异步任务
// 继续用它查询会得到 context canceled。本 helper 用 context.WithoutCancel
// 的 ctx 重新绑定全局 *gorm.DB，让异步任务安全复用连接池。
package database

import (
	"context"

	"github.com/gin-gonic/gin"
)

// DetachedGinContext 构造一个带 DB 会话、但不随任何请求结束而取消的 gin.Context。
//
// 行为：
//   - ctx 用于派生 gorm 会话（context.WithoutCancel 去掉取消信号，保留 trace 值）；
//   - 返回的 *gin.Context 只保证 repository.DB() 可用，不含请求参数 / header；
//   - database.Get() 为 nil（未 Init，单测环境）时返回不带 DB 的空上下文，
//     由调用方注入的 fake repository 兜底。
func DetachedGinContext(ctx context.Context) *gin.Context {
	c := &gin.Context{}
	if g := Get(); g != nil {
		c.Set(CtxKey, g.WithContext(context.WithoutCancel(ctx)))
	}
	return c
}
