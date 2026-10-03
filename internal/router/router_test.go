// Package router — router_test.go 验证生产装配回归。
//
// 背景（final review Critical #1）：router/supplier 包新增后，router.go 的
// 空白导入块漏了一行 —— 各子包自己的 route test 都会手动重挂注册函数，
// 所以测试全绿，但生产二进制里 /supplier/* 整体 404。
//
// 本测试站在"生产装配视角"：只 import 本包（router.go 的空白导入链会加载
// 全部子路由包），Setup 后必须能看到三个身份的登录路由。新增子路由包时，
// 这里会强制要求同步加空白导入。
package router

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestSetup_RegistersAllIdentityRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	Setup(engine)

	got := make(map[string]bool)
	for _, r := range engine.Routes() {
		got[r.Method+" "+r.Path] = true
	}

	// 每个身份各取一条代表性路由；login 是公开路由，存在即可，不需要 token。
	for _, key := range []string{
		"POST /admin/login",
		"POST /user/login",
		"GET /user/blindbox/list",
		"POST /supplier/login",
		"GET /supplier/products/list",
	} {
		if !got[key] {
			t.Errorf("route %q not registered — 生产装配缺失子路由包空白导入？", key)
		}
	}

	// 行为抽样：/supplier/login 必须真的可路由（404 检测：路径不存在时
	// gin 返回 404 且 body 为空；存在时至少会进入 handler 链）。
	req := httptest.NewRequest("POST", "/supplier/login", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	if w.Code == 404 {
		t.Errorf("/supplier/login returned 404 — supplier router package not wired into production imports")
	}
}
