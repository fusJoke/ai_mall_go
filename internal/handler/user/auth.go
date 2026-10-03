// Package user 是 C 端会员实体的 HTTP 处理层。
//
// 本文件实现 auth 路由（Login / Logout），与 handler/admin 的 admin login 同构。
// 业务差异：
//   - Login 调用 service/user 的 Login（验证码 + bcrypt + token type="user"）；
//   - Login 失败统一回 401（不区分用户名错 / 密码错 / captcha 错），
//     避免脚本探测用户名是否存在（spec security requirement）。
package user

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	captchaInfra "ai-go-mall/internal/infra/captcha"
	"ai-go-mall/internal/model"
	userSvc "ai-go-mall/internal/service/user"
)

// AuthHandler 处理 C 端会员的鉴权路由（Login / Logout）。
//
// 仅持有 user.Service（具体是 AuthService 接口）；通过 Login 调登录能力。
type AuthHandler struct {
	svc userSvc.Service
}

// NewAuthHandler 接收 user service，返回 *AuthHandler。
func NewAuthHandler(svc userSvc.Service) *AuthHandler {
	return &AuthHandler{svc: svc}
}

// =============================================================================
// Login
// =============================================================================

// LoginRequest 是 POST /user/login 的请求体。
//
// 与 admin.LoginRequest 同构：username / password / captcha_key / points / remember。
// 字段顺序 / binding tag 与 admin 一致，便于前端共用类型生成代码。
type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`

	// CaptchaKey 来源于点选验证码预检通过后回调透传的 key；
	// 缺 / 空 走 400，service 层做二次校验（consume 语义）。
	CaptchaKey string `json:"captcha_key" binding:"required"`

	// Points 用户在图片坐标系下的点击坐标（按 elements 顺序）。
	Points []CaptchaPoint `json:"points" binding:"required"`

	// Remember 记住我标记；true 表示 token 有效期 30 天，false（缺省）表示 3 天。
	Remember bool `json:"remember"`
}

// CaptchaPoint 是 LoginRequest 内嵌的点选坐标。
type CaptchaPoint struct {
	X int `json:"x"`
	Y int `json:"y"`
}

// LoginResponse 是登录成功后的返回。
//
// userInfo 故意不包含 Password / LoginFailure 等敏感字段；Balance 在前端展示
// 余额需要，但 spec 要求脱敏精度（小数点后两位），由本方法显式格式化。
// Token 字段由 service 层签发，handler 原样回传（明文 token 一次性下发，
// 前端自行保管）。
type LoginResponse struct {
	User  userInfo `json:"user"`
	Token string   `json:"token"`
}

type userInfo struct {
	ID          int64   `json:"id"`
	Username    string  `json:"username"`
	Nickname    string  `json:"nickname"`
	Avatar      string  `json:"avatar"`
	Email       *string `json:"email,omitempty"`
	Mobile      *string `json:"mobile,omitempty"`
	Balance     string  `json:"balance"` // 格式化为 "%.2f"，避免 float64 精度问题
	LastLoginAt string  `json:"last_login_at,omitempty"`
	LastLoginIp string  `json:"last_login_ip,omitempty"`
	Status      int8    `json:"status"`
}

// bearerLogoutPrefix 是 Logout 解析时接受的 Bearer scheme 前缀（RFC 6750 大小写不敏感）。
//
// 放在文件顶部的 Login 注释块上方 —— swag 解析时以「最近的 // 注释块」为锚定，
// 任何 const / var / type 声明插在 @Router 注解和 handler 函数之间都会让 swag
// 丢失该注解，导致接口不出现在 swagger 文档里。
const bearerLogoutPrefix = "Bearer "

// Login 处理 POST /user/login。
//
// body 缺字段 → 400；用户名 / 密码错 → 401（不区分）；账号禁用 → 403；成功 → 200。
//
// @Summary      会员登录
// @Description  校验点选验证码 → 校验账号 → 签发 token；前端拿到 token 后自行保存并放入后续请求头。
// @Tags         user
// @Accept       json
// @Produce      json
// @Param        req  body      user.LoginRequest   true  "登录请求（含用户名 / 密码 / 点选坐标）"
// @Success      200  {object}  user.LoginResponse  "登录成功"
// @Failure      400  {object}  map[string]string   "login.invalid_input"
// @Failure      401  {object}  map[string]string   "login.invalid_credentials / login.invalid_captcha"
// @Failure      403  {object}  map[string]string   "login.account_disabled"
// @Failure      500  {object}  map[string]string   "login.internal"
// @Router       /user/login [post]
func (h *AuthHandler) Login(c *gin.Context) {
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

	u, rawToken, err := h.svc.Login(c, req.Username, req.Password, req.CaptchaKey, points, req.Remember)
	if err != nil {
		switch {
		case errors.Is(err, userSvc.ErrInvalidCredentials):
			c.JSON(http.StatusUnauthorized, gin.H{
				"code":    "login.invalid_credentials",
				"message": err.Error(),
			})
		case errors.Is(err, userSvc.ErrAccountDisabled):
			c.JSON(http.StatusForbidden, gin.H{
				"code":    "login.account_disabled",
				"message": err.Error(),
			})
		case errors.Is(err, userSvc.ErrInvalidCaptcha):
			c.JSON(http.StatusUnauthorized, gin.H{
				"code":    "login.invalid_captcha",
				"message": err.Error(),
			})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":    "login.internal",
				"message": "login failed",
			})
		}
		return
	}

	c.JSON(http.StatusOK, LoginResponse{
		User:  toUserInfo(u),
		Token: rawToken,
	})
}

// Logout 处理 POST /user/logout。
//
// 幂等：header 缺失 / 格式错误 / token 不存在 / 已过期 / 已登出 → 一律 200；
// token 有效 → 软删除 → 200；只有存储驱动出错才返回 500。
//
// @Summary      会员登出
// @Description  软删除当前调用方持有的 user token；幂等 —— 缺失 / 格式错误 / 已过期 / 不存在都返回 200。
// @Tags         user
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Success      200  {object}  map[string]string  "logout.ok"
// @Failure      500  {object}  map[string]string  "logout.internal"
// @Router       /user/logout [post]
func (h *AuthHandler) Logout(c *gin.Context) {
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
//
// 与 handler/admin.extractBearerToken 同构；handler 包不暴露公共 helper 是为
// 了避免跨身份 handler 之间的强依赖（C 端与 B 端的登录逻辑是独立的）。
func extractBearerToken(authz string) string {
	if len(authz) < len(bearerLogoutPrefix) {
		return ""
	}
	if !strings.EqualFold(authz[:len(bearerLogoutPrefix)], bearerLogoutPrefix) {
		return ""
	}
	return strings.TrimSpace(authz[len(bearerLogoutPrefix):])
}

// toUserInfo 把 *model.User 投影到不含敏感字段的 userInfo。
//
// 存在意义：显式表达"handler 不应把 model.User 原样序列化出去"的契约。
// Balance 字段从 float64 改为 "%.2f" 字符串，避免 JSON 序列化端精度问题
// （float64 → JSON 数字可能产生未披露精度差异）。
func toUserInfo(u *model.User) userInfo {
	info := userInfo{
		ID:          u.ID,
		Username:    u.Username,
		Nickname:    u.Nickname,
		Avatar:      u.Avatar,
		Email:       u.Email,
		Mobile:      u.Mobile,
		Balance:     fmtBalance(u.Balance),
		LastLoginIp: u.LastLoginIp,
		Status:      u.Status,
	}
	if u.LastLoginAt != nil {
		info.LastLoginAt = u.LastLoginAt.Format("2006-01-02T15:04:05Z07:00")
	}
	return info
}

// fmtBalance 把 float64 格式化为 2 位精度的字符串（用于 JSON 输出）。
//
// 用 strconv.FormatFloat 显式控制精度与格式（不依赖 fmt.Sprintf 的 %f 默认行为，
// 避免因 locale / runtime 不同产生歧义）。
func fmtBalance(b float64) string {
	return strconv.FormatFloat(b, 'f', 2, 64)
}
