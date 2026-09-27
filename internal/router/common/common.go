// Package common 是 /common 前缀下的子路由集合。
//
// 当前承载跨身份可复用的「点选验证码」能力：
//   - GET  /common/captcha/create  → 生成一道点选验证码
//   - POST /common/captcha/verify  → 预检：仅校验坐标精度，不消耗 key
//
// 装配模式与 internal/router/admin 共用 sync.Once 懒初始化：
// infra/captcha.GetManager() 在 cmd/serve 启动期已被调过，
// 但 handler/service 不直接 import infra 包 —— 走 service 抽象更易测试。
package common

import (
	"net/http"
	"sync"

	"github.com/gin-gonic/gin"

	captchaHandler "ai-go-mall/internal/handler/captcha"
	captchaInfra "ai-go-mall/internal/infra/captcha"
	"ai-go-mall/internal/router/registry"
	captchaService "ai-go-mall/internal/service/captcha"
)

var (
	captchaOnce        sync.Once
	captchaSvcInst     captchaService.Service
	captchaHandlerInst *captchaHandler.Handler
)

func ensureCaptchaDeps() (captchaService.Service, *captchaHandler.Handler) {
	captchaOnce.Do(func() {
		mgr, err := captchaInfra.GetManager()
		if err != nil {
			// 启动期 GetManager 失败属致命配置错误；让进程崩溃比带着坏依赖运行更安全。
			panic("common: captcha manager init failed: " + err.Error())
		}
		captchaSvcInst = captchaService.NewService(mgr)
		captchaHandlerInst = captchaHandler.NewHandler(captchaSvcInst)
	})
	return captchaSvcInst, captchaHandlerInst
}

func init() {
	registry.Register("/common", http.MethodGet, "/captcha/create", func(c *gin.Context) {
		_, h := ensureCaptchaDeps()
		h.CreateClick(c)
	})
	registry.Register("/common", http.MethodPost, "/captcha/verify", func(c *gin.Context) {
		_, h := ensureCaptchaDeps()
		h.VerifyClick(c)
	})
}
