// 请求日志与 trace_id 贯穿。
//
//   - 每个请求派生带 trace_id 的子 logger 写入 request context：
//     handler / service / repository / GORM 全链路 FromCtx 即得，字段自动携带。
//   - 请求头带 X-Request-ID 则沿用（网关/压测可串联），否则生成 uuid；
//     响应头回写，前端报障可直接贴 ID 检索。
//   - 每请求一条访问日志：status≥500 → Error，≥400 → Warn，其余 Info；
//     handler 里 c.Error(err) 的内容一并吸收进 err 字段。
//   - Recovery 与 gin.Recovery 同语义：panic 转 500、堆栈进日志不外泄；
//     响应体对齐项目约定 {"code","message"}（code 为点分字符串）。

package middleware

import (
	"fmt"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/gin-gonic/gin"

	"ai-go-mall/internal/infra/log"
)

// HeaderRequestID 请求追踪头，与网关/前端约定保持一致。
const HeaderRequestID = "X-Request-ID"

// RequestLogger 请求日志 + trace_id 注入，注册在路由最外层。
func RequestLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		tid := c.GetHeader(HeaderRequestID)
		if tid == "" {
			tid = log.NewTraceID()
		}
		c.Header(HeaderRequestID, tid)

		lg := log.L().With("trace_id", tid)
		c.Request = c.Request.WithContext(log.IntoCtx(c.Request.Context(), lg))

		c.Next()

		status := c.Writer.Status()
		attrs := []any{
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"status", status,
			"latency_ms", time.Since(start).Milliseconds(),
			"ip", c.ClientIP(),
			"ua", c.Request.UserAgent(),
		}
		// handler 里 c.Error(err) 的内容会进入访问日志的 err 字段。
		if errs := c.Errors.ByType(gin.ErrorTypePrivate); len(errs) > 0 {
			attrs = append(attrs, "err", errs.String())
		}

		switch {
		case status >= 500:
			lg.Error("http request", attrs...)
		case status >= 400:
			lg.Warn("http request", attrs...)
		default:
			lg.Info("http request", attrs...)
		}
	}
}

// Recovery panic 兜底：堆栈进日志，对外只回统一 500，不泄漏内部细节。
func Recovery() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if r := recover(); r != nil {
				log.FromCtx(c.Request.Context()).Error("panic recovered",
					"panic", fmt.Sprint(r),
					"stack", string(debug.Stack()),
					"method", c.Request.Method,
					"path", c.Request.URL.Path,
				)
				// 响应结构对齐项目约定：code 为点分字符串（见 handler/user/auth.go）。
				c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
					"code":    "internal.panic",
					"message": "internal error",
				})
			}
		}()
		c.Next()
	}
}
