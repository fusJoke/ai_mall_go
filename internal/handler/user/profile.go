// Package user 的 profile 路由实现（GET /user/profile）。
//
// 本文件对应 openspec/changes/add-uni-h5-client 的
// specs/user-mobile-login/spec.md「Requirement: 当前用户信息接口」。
//
// 存在原因：登录响应里的 `balance` 是登录时刻的快照，客户端在「我的」页
// 或抽卡失败（402 余额不足）后需要拿到**最新**余额，而此前 C 端接口集中
// 没有独立的用户信息查询接口。
package user

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"ai-go-mall/internal/middleware"
)

// Profile 处理 GET /user/profile。
//
// 鉴权由 UserAuth() 中间件承担（未带 / 令牌非法 → 401 auth.*，在进入本函数前
// 就已 abort）。本函数只负责把当前用户投影成与登录响应 `user` 对象**同构**的
// 裸 JSON。
//
// 响应刻意复用 auth.go 的 userInfo 与 toUserInfo，而不是另立 DTO：spec 要求
// 「结构与登录响应的 user 对象同构」，共用同一类型是唯一能保证两者不漂移的
// 做法（字段增删、balance 的 "%.2f" 格式化、内部字段（password /
// login_failure / deleted_at）的排除，全部只有一处定义）。
//
// 关于两次读取：UserAuth 中间件已经按 token 的 uid 查过一次库并把 entity 写进
// context，此处仍再走一次 svc.GetByID。这是刻意的取舍 ——
//  1. 让 spec 列出的 500 user.internal 成为**真实可达**的失败路径（纯用 context
//     实体的话该接口没有任何内部失败分支，spec 的失败码就是空头承诺）；
//  2. 不依赖中间件的实现细节（若将来中间件改为缓存 entity，本接口仍返回实时值）；
//  3. 与其余 C 端 handler 一致地走 service 层。
//
// 代价是每请求多一次主键查询，而本接口只在进「我的」页时调用，可忽略。
//
// @Summary      当前用户信息
// @Description  返回当前登录用户信息（含最新余额）；需 user 类型令牌。响应结构与登录响应的 user 对象同构。
// @Tags         user
// @Produce      json
// @Security     BearerAuth
// @Success      200  {object}  user.userInfo      "当前用户信息"
// @Failure      401  {object}  map[string]string  "auth.*（中间件）/ profile.unauthorized"
// @Failure      500  {object}  map[string]string  "user.internal"
// @Router       /user/profile [get]
func (h *AuthHandler) Profile(c *gin.Context) {
	// 防御性兜底：正常情况下 UserAuth 已 401，走不到这里。
	// 取不到 entity 也可能是「token 合法但用户已被软删」（中间件的 lookupUser
	// 对此返回 (nil, nil) 且不 abort），此时按会话失效处理。
	cur := middleware.UserFromContext(c)
	if cur == nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"code":    "profile.unauthorized",
			"message": "login required",
		})
		return
	}

	u, err := h.svc.GetByID(c, cur.ID)
	if err != nil || u == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    "user.internal",
			"message": "failed to load user profile",
		})
		return
	}

	c.JSON(http.StatusOK, toUserInfo(u))
}
