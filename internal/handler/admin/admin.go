// Package admin 是管理员实体的 HTTP 处理层。
//
// Handler 嵌入通用 *handler.BaseHandler[model.Admin]，自动获得 Create /
// List / EditGet / EditPost / Delete 五条路由的处理函数；
// 在此之上扩展管理员专属的 Login 路由处理。
package admin

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	captchaInfra "ai-go-mall/internal/infra/captcha"
	"ai-go-mall/internal/handler"
	"ai-go-mall/internal/model"
	adminSvc "ai-go-mall/internal/service/admin"
)

// Handler 是管理员实体的控制器。
//
// 通过嵌入 *handler.BaseHandler[model.Admin] 复用通用 CRUD 的五个方法；
// 额外持有 svc（adminSvc.Service）以便在 Login 中调用 Service 接口的 Login 方法
// —— BaseHandler 字段被类型擦除为 service.CRUDService[T]，访问不到扩展方法。
type Handler struct {
	*handler.BaseHandler[model.Admin]
	svc adminSvc.Service
}

// NewHandler 接收 admin service，返回 *Handler。
//
// 注意：BaseHandler 已经持有同一个 svc（构造时强转 CRUDService），所以
// h.svc 与 h.BaseHandler.CRUDService 指向同一对象。
func NewHandler(svc adminSvc.Service) *Handler {
	return &Handler{
		BaseHandler: handler.NewBaseHandler[model.Admin](svc),
		svc:         svc,
	}
}

// LoginRequest 是 POST /admin/login 的请求体。
type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`

	// CaptchaKey 来源于点选验证码预检通过后回调透传的 key；
	// 缺 / 空 走 400，service 层做二次校验（consume 语义）。
	CaptchaKey string `json:"captcha_key" binding:"required"`

	// Points 是用户在图片坐标系下的点击坐标（按 elements 顺序）。
	// 与 captchaKey 一起透传给 service，做二次校验。
	Points []CaptchaPoint `json:"points" binding:"required"`

	// Remember 记住我标记；true 表示 token 有效期 30 天，false（缺省）表示 3 天。
	// 前端不传时 Go 零值是 false，无需 omitempty。
	Remember bool `json:"remember"`
}

// CaptchaPoint 是 LoginRequest 内嵌的点选坐标。
type CaptchaPoint struct {
	X int `json:"x"`
	Y int `json:"y"`
}

// LoginResponse 是登录成功后的返回。
//
// 注意：adminInfo 故意不包含 Password / LoginFailure 等敏感字段，由本方法显式挑选。
// Token 字段由 service 层签发，handler 原样回传（明文 token 一次性下发，前端自行保管）。
type LoginResponse struct {
	Admin adminInfo `json:"admin"`
	Token string    `json:"token"`
}

type adminInfo struct {
	ID          int64   `json:"id"`
	Username    string  `json:"username"`
	Nickname    string  `json:"nickname"`
	Avatar      string  `json:"avatar"`
	Email       *string `json:"email,omitempty"`
	Mobile      *string `json:"mobile,omitempty"`
	LastLoginAt string  `json:"last_login_at,omitempty"`
	LastLoginIp string  `json:"last_login_ip,omitempty"`
	Bio         string  `json:"bio"`
	Status      int8    `json:"status"`
}

// bearerLogoutPrefix 是 Logout 解析时接受的 Bearer scheme 前缀（RFC 6750 大小写不敏感）。
//
// 放在文件顶部的 Login 注释块上方 —— swag 解析时以「最近的 // 注释块」为锚定，
// 任何 const / var / type 声明插在 @Router 注解和 handler 函数之间都会让 swag
// 丢失该注解，导致接口不出现在 swagger 文档里。
const bearerLogoutPrefix = "Bearer "

// Login 处理 POST /admin/login。
//
// body 缺字段 → 400；用户名 / 密码错 → 401（不区分）；账号禁用 → 403；成功 → 200。
//
// @Summary      管理员登录
// @Description  校验点选验证码 → 校验账号 → 签发 token；前端拿到 token 后自行保存并放入后续请求头。
// @Tags         admin
// @Accept       json
// @Produce      json
// @Param        req  body      admin.LoginRequest   true  "登录请求（含用户名 / 密码 / 点选坐标）"
// @Success      200  {object}  admin.LoginResponse  "登录成功"
// @Failure      400  {object}  map[string]string    "login.invalid_input"
// @Failure      401  {object}  map[string]string    "login.invalid_credentials / login.invalid_captcha"
// @Failure      403  {object}  map[string]string    "login.account_disabled"
// @Failure      500  {object}  map[string]string    "login.internal"
// @Router       /admin/login [post]
func (h *Handler) Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    "login.invalid_input",
			"message": err.Error(),
		})
		return
	}

	points := make([]captchaInfra.Point, len(req.Points))
	for i, p := range req.Points {
		points[i] = captchaInfra.Point{X: p.X, Y: p.Y}
	}

	adm, rawToken, err := h.svc.Login(c, req.Username, req.Password, req.CaptchaKey, points, req.Remember)
	if err != nil {
		switch {
		case errors.Is(err, adminSvc.ErrInvalidCredentials):
			c.JSON(http.StatusUnauthorized, gin.H{
				"code":    "login.invalid_credentials",
				"message": err.Error(),
			})
		case errors.Is(err, adminSvc.ErrAccountDisabled):
			c.JSON(http.StatusForbidden, gin.H{
				"code":    "login.account_disabled",
				"message": err.Error(),
			})
		case errors.Is(err, adminSvc.ErrInvalidCaptcha):
			c.JSON(http.StatusUnauthorized, gin.H{
				"code":    "login.invalid_captcha",
				"message": err.Error(),
			})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":    "login.internal",
				"message": err.Error(),
			})
		}
		return
	}

	c.JSON(http.StatusOK, LoginResponse{
		Admin: toAdminInfo(adm),
		Token: rawToken,
	})
}

// Logout 处理 POST /admin/logout。
//
// Bearer token 解析内联在 handler 顶部 —— 唯一调用点，不抽 helper。
// 幂等：header 缺失 / 格式错误 / token 不存在 / 已过期 / 已登出 → 一律 200；
// token 有效 → 软删除 → 200；只有存储驱动出错才返回 500。
//
// @Summary      管理员登出
// @Description  软删除当前调用方持有的 admin token；幂等 —— 缺失 / 格式错误 / 已过期 / 不存在都返回 200。
// @Tags         admin
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Success      200  {object}  map[string]string  "logout.ok"
// @Failure      500  {object}  map[string]string  "logout.internal"
// @Router       /admin/logout [post]
func (h *Handler) Logout(c *gin.Context) {
	authz := c.GetHeader("Authorization")
	rawToken := extractBearerToken(authz)
	if rawToken == "" {
		// header 缺失 / 非 Bearer / 截取后为空：视作幂等成功，不落库。
		c.JSON(http.StatusOK, gin.H{
			"code":    "logout.ok",
			"message": "no active session",
		})
		return
	}
	if err := h.svc.Logout(c.Request.Context(), rawToken); err != nil {
		// 500 不回显 err.Error() —— 驱动层细节可能暴露存储路径 / SQL 状态。
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    "logout.internal",
			"message": "logout failed",
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"code":    "logout.ok",
		"message": "ok",
	})
}

// extractBearerToken 从 Authorization 头里抠出明文 token；scheme 大小写不敏感。
// 不匹配 / 截取为空 → 返回空串（调用方按幂等成功处理）。
func extractBearerToken(authz string) string {
	if len(authz) < len(bearerLogoutPrefix) {
		return ""
	}
	if !strings.EqualFold(authz[:len(bearerLogoutPrefix)], bearerLogoutPrefix) {
		return ""
	}
	return strings.TrimSpace(authz[len(bearerLogoutPrefix):])
}

// toAdminInfo 把 *model.Admin 投影到不含敏感字段的 adminInfo。
func toAdminInfo(adm *model.Admin) adminInfo {
	info := adminInfo{
		ID:          adm.ID,
		Username:    adm.Username,
		Nickname:    adm.Nickname,
		Avatar:      adm.Avatar,
		Email:       adm.Email,
		Mobile:      adm.Mobile,
		LastLoginIp: adm.LastLoginIp,
		Bio:         adm.Bio,
		Status:      adm.Status,
	}
	if adm.LastLoginAt != nil {
		info.LastLoginAt = adm.LastLoginAt.Format("2006-01-02T15:04:05Z07:00")
	}
	return info
}
