// Package urlx 提供资源完整 URL 拼装能力。
//
// 函数 FullURL 把相对路径 / 协议地址 / base64 资源统一处理为可对外使用的完整 URL：
//   - 空串、base64（data: 前缀）、协议地址（http:// / https://）→ 原样返回；
//   - 配置了 CDN → 拼 CDN 域名 + CDN params + 资源；
//   - 否则 → 拼当前请求的 scheme + host + port + 资源。
//
// 为什么放在 internal/kit/urlx（不是 pkg/urlx）：
//   - 依赖 gin.Context 取当前请求域名；
//   - 依赖 internal/infra/config 取 CDN 配置；
//     两者都与框架 / 应用层耦合，不适合放进对外公开的 pkg。
package urlx

import (
	"strings"

	"github.com/gin-gonic/gin"

	"ai-go-mall/internal/infra/config"
)

// xForwardedProtoHeader 是反向代理（如 nginx / ALB）转发的协议 header。
// 中间件（如 nginx）会把原始协议（http/https）写到这个 header 上，
// 让后端在 TLS 终止于网关时仍能识别原始协议。
const xForwardedProtoHeader = "X-Forwarded-Proto"

// FullURL 把 resource 拼成对外可用的完整 URL。
//
// 分支按顺序判断：
//  1. 空串 → 返回 ""
//  2. base64 data URI → 原样
//  3. http:// / https:// 绝对地址 → 原样
//  4. CDN 域名已配置 → cdn_url + cdn_url_params + resource
//  5. 否则 → scheme + "://" + c.Request.Host + resource
//
// 配置每次调用实时读（config.Get() 是指针返回，开销可忽略），
// 不缓存 CDN —— 配置热更新语义不在本 change 范围。
func FullURL(c *gin.Context, resource string) string {
	if resource == "" {
		return ""
	}
	if strings.HasPrefix(resource, "data:") {
		return resource
	}
	if strings.HasPrefix(resource, "http://") || strings.HasPrefix(resource, "https://") {
		return resource
	}

	cfg := config.Get().Server
	if cfg.CDNURL != "" {
		// cdn_url_params 由运维侧填写，可能带或不带前导 `/`；
		// 这里统一补齐分隔斜杠，保证 URL 拼出来一定形如 `host/path/resource`。
		return cfg.CDNURL + ensureLeadingSlash(cfg.CDNURLParams) + resource
	}

	return schemeFromRequest(c) + "://" + c.Request.Host + resource
}

// ensureLeadingSlash 在 s 为空时返回空串，否则保证以 `/` 开头。
func ensureLeadingSlash(s string) string {
	if s == "" {
		return ""
	}
	if strings.HasPrefix(s, "/") {
		return s
	}
	return "/" + s
}

// schemeFromRequest 推断当前请求的协议（http / https）。
//
// 优先级：
//  1. c.Request.TLS != nil → TLS 在本进程终结 → "https"
//  2. 反向代理写入的 X-Forwarded-Proto 非空 → 用其值
//  3. 兜底 "http"
//
// 部署侧负责保证反向代理会把原始协议写到该 header 上。
func schemeFromRequest(c *gin.Context) string {
	if c.Request.TLS != nil {
		return "https"
	}
	if proto := c.Request.Header.Get(xForwardedProtoHeader); proto != "" {
		return proto
	}
	return "http"
}
