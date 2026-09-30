package admin

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"ai-go-mall/internal/middleware"
	adminSvc "ai-go-mall/internal/service/admin"
)

// InitHandler 处理 `/admin/init` 端点。
//
// 仅持有 InitService；当前管理员记录由 middleware.AdminFromContext(c) 读取
// （由 AdminAuth 中间件在 token 校验通过后写入 context）。
type InitHandler struct {
	initSvc adminSvc.InitService
}

// NewInitHandler 接收 init service，返回 *InitHandler。
func NewInitHandler(initSvc adminSvc.InitService) *InitHandler {
	return &InitHandler{initSvc: initSvc}
}

// Init 处理 GET /admin/init。
//
// 数据流：
//  1. 从 middleware.AdminFromContext(c) 读 admin；为 nil 说明 admin 已被并发删除 → 401；
//  2. 调 h.initSvc.Init(c, uint(adm.ID))；
//  3. 按 service 返回的 error 分类：
//     - nil: 200 + InitResponse（service 已组好结构，handler 直接 JSON 序列化）；
//     - ErrAccountDisabled: 403 admin.account_disabled；
//     - 其他: 500 admin.init.internal（不回显 err.Error()）。
//
// @Summary      后台初始化
// @Description  登录后调用一次，聚合当前管理员信息、站点配置（site name / record number / version）、权限菜单规则；前端据此填充 store + 注册动态路由。
// @Tags         admin
// @Produce      json
// @Security     BearerAuth
// @Success      200  {object}  admin.InitResponse  "admin.init.ok"
// @Failure      401  {object}  map[string]string  "auth.*（中间件处理）"
// @Failure      403  {object}  map[string]string  "admin.account_disabled"
// @Failure      500  {object}  map[string]string  "admin.init.internal"
// @Router       /admin/init [get]
func (h *InitHandler) Init(c *gin.Context) {
	adm := middleware.AdminFromContext(c)
	if adm == nil {
		// admin 已被删除 / token 签发后并发消失 —— 按"账号不存在"语义处理，
		// 走 403 而非 401（token 本身合法）。
		c.JSON(http.StatusForbidden, gin.H{
			"code":    "admin.account_disabled",
			"message": "admin: account disabled",
		})
		return
	}

	resp, err := h.initSvc.Init(c, uint(adm.ID))
	if err != nil {
		if errors.Is(err, adminSvc.ErrAccountDisabled) {
			c.JSON(http.StatusForbidden, gin.H{
				"code":    "admin.account_disabled",
				"message": err.Error(),
			})
			return
		}
		// 内部错误：不回显 err.Error() 细节（可能含 SQL / 驱动层文本）。
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    "admin.init.internal",
			"message": "init failed",
		})
		return
	}

	c.JSON(http.StatusOK, resp)
}
