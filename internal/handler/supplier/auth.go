// Package supplier 是 B 端供应商实体的 HTTP 处理层。
//
// 本文件实现 auth 路由（Login / Logout），与 handler/user/auth.go 同构。
// 业务差异：
//   - Login 调 service/supplier 的 Login（验证码 + bcrypt + token type="supplier"）；
//   - 登录失败统一 401（不区分用户名错 / 密码错 / captcha 错），防用户名探测；
//   - 供应商主体被禁用（mall_suppliers.status=disabled）→ 403 account_disabled。
package supplier

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	captchaInfra "ai-go-mall/internal/infra/captcha"
	"ai-go-mall/internal/model/mall"
	supplierSvc "ai-go-mall/internal/service/supplier"
)

// AuthHandler 处理供应商的鉴权路由（Login / Logout）。
type AuthHandler struct {
	svc supplierSvc.AuthService
}

// NewAuthHandler 接收 supplier AuthService，返回 *AuthHandler。
func NewAuthHandler(svc supplierSvc.AuthService) *AuthHandler {
	return &AuthHandler{svc: svc}
}

// LoginRequest 是 POST /supplier/login 的请求体（与 user.LoginRequest 同构）。
type LoginRequest struct {
	Username   string         `json:"username" binding:"required"`
	Password   string         `json:"password" binding:"required"`
	CaptchaKey string         `json:"captcha_key" binding:"required"`
	Points     []CaptchaPoint `json:"points" binding:"required"`
	Remember   bool           `json:"remember"`
}

// CaptchaPoint 是 LoginRequest 内嵌的点选坐标。
type CaptchaPoint struct {
	X int `json:"x"`
	Y int `json:"y"`
}

// LoginResponse 是登录成功后的返回。
type LoginResponse struct {
	User  supplierUserInfo `json:"user"`
	Token string           `json:"token"`
}

// supplierUserInfo 是供应商账号的安全投影。
//
// 不含 Password（模型已 json:"-"，这里显式白名单双保险）。
// SupplierID 是供应商后台一切业务操作的归属 ID，前端登录后即持有。
type supplierUserInfo struct {
	ID          int64   `json:"id"`
	SupplierID  int64   `json:"supplier_id"`
	Username    string  `json:"username"`
	Status      int8    `json:"status"`
	LastLoginAt string  `json:"last_login_at,omitempty"`
	LastLoginIp string  `json:"last_login_ip,omitempty"`
	ContactMail *string `json:"contact_email,omitempty"`
}

// bearerLogoutPrefix 是 Logout 解析时接受的 Bearer scheme 前缀（RFC 6750 大小写不敏感）。
const bearerLogoutPrefix = "Bearer "

// Login 处理 POST /supplier/login。
//
// @Summary      供应商登录
// @Description  校验点选验证码 → 校验账号 → 签发 supplier token。
// @Tags         supplier
// @Accept       json
// @Produce      json
// @Param        req  body  supplier.LoginRequest  true  "登录请求"
// @Success      200  {object}  supplier.LoginResponse  "登录成功"
// @Failure      400  {object}  map[string]string  "login.invalid_input"
// @Failure      401  {object}  map[string]string  "login.invalid_credentials / login.invalid_captcha"
// @Failure      403  {object}  map[string]string  "login.account_disabled"
// @Failure      500  {object}  map[string]string  "login.internal"
// @Router       /supplier/login [post]
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
		case errors.Is(err, supplierSvc.ErrInvalidCredentials):
			c.JSON(http.StatusUnauthorized, gin.H{
				"code":    "login.invalid_credentials",
				"message": err.Error(),
			})
		case errors.Is(err, supplierSvc.ErrAccountDisabled):
			c.JSON(http.StatusForbidden, gin.H{
				"code":    "login.account_disabled",
				"message": err.Error(),
			})
		case errors.Is(err, supplierSvc.ErrInvalidCaptcha):
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
		User:  toSupplierUserInfo(u),
		Token: rawToken,
	})
}

// Logout 处理 POST /supplier/logout（幂等，同 user.Logout 语义）。
//
// @Summary      供应商登出
// @Description  软删除当前 supplier token；幂等。
// @Tags         supplier
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Success      200  {object}  map[string]string  "logout.ok"
// @Failure      500  {object}  map[string]string  "logout.internal"
// @Router       /supplier/logout [post]
func (h *AuthHandler) Logout(c *gin.Context) {
	authz := c.GetHeader("Authorization")
	rawToken := extractBearerToken(authz)
	if rawToken == "" {
		c.JSON(http.StatusOK, gin.H{
			"code":    "logout.ok",
			"message": "no active session",
		})
		return
	}
	if err := h.svc.Logout(c.Request.Context(), rawToken); err != nil {
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

// extractBearerToken 从 Authorization 头抠出明文 token；与 handler/user 同构。
func extractBearerToken(authz string) string {
	if len(authz) < len(bearerLogoutPrefix) {
		return ""
	}
	if !strings.EqualFold(authz[:len(bearerLogoutPrefix)], bearerLogoutPrefix) {
		return ""
	}
	return strings.TrimSpace(authz[len(bearerLogoutPrefix):])
}

// toSupplierUserInfo 投影 MallSupplierUser → 安全 DTO。
func toSupplierUserInfo(u *mall.MallSupplierUser) supplierUserInfo {
	info := supplierUserInfo{
		ID:          u.ID,
		SupplierID:  u.SupplierID,
		Username:    u.Username,
		Status:      u.Status,
		LastLoginIp: u.LastLoginIp,
	}
	if u.LastLoginAt != nil {
		info.LastLoginAt = u.LastLoginAt.Format("2006-01-02T15:04:05Z07:00")
	}
	return info
}
