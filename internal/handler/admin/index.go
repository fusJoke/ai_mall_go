// Package admin 是管理员实体的 HTTP 处理层。
//
// Handler 嵌入通用 *handler.BaseHandler[model.Admin]，自动获得 Create /
// List / EditGet / EditPost / Delete 五条路由的处理函数；
// 在此之上扩展管理员专属的 Login 路由处理。
package admin

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

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
}

// LoginResponse 是登录成功后的返回。
//
// 注意：adminInfo 故意不包含 Password / LoginFailure 等敏感字段，由本方法显式挑选。
// token 字段当前是占位（"stub-token"），真实实现由 token 包出 JWT / Session。
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

// Login 处理 POST /admin/login。
//
// body 缺字段 → 400；用户名 / 密码错 → 401（不区分）；账号禁用 → 403；成功 → 200。
func (h *Handler) Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	adm, err := h.svc.Login(c, req.Username, req.Password)
	if err != nil {
		switch {
		case errors.Is(err, adminSvc.ErrInvalidCredentials):
			c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		case errors.Is(err, adminSvc.ErrAccountDisabled):
			c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}

	c.JSON(http.StatusOK, LoginResponse{
		Admin: toAdminInfo(adm),
		Token: "stub-token", // TODO: 换成 internal/infra/token 的产物
	})
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
