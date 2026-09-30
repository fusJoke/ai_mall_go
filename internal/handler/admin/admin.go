// Package admin 是管理员实体的 HTTP 处理层。
//
// Handler 嵌入通用 *handler.BaseHandler[model.Admin]，自动获得 Create /
// List / EditGet / EditPost / Delete 五条路由的处理函数；
// 在此之上扩展管理员专属的 Login 路由处理。
package admin

import (
	"errors"
	"net/http"
	"reflect"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	captchaInfra "ai-go-mall/internal/infra/captcha"
	"ai-go-mall/internal/handler"
	"ai-go-mall/internal/middleware"
	"ai-go-mall/internal/model"
	"ai-go-mall/internal/repository"
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

// --- 内联 helper：与 internal/handler/base.go 同名但保持私有 ---
//
// 把 BaseHandler 的解析逻辑内联在本文件，避免把私有 helper 升级成 export 形式
// 污染 handler 包公共 API；admin 包只需要这三种解析，与 BaseHandler 重复定义的成本可忽略。

// parseID 优先从 query (?id=) 取，其次从 postForm (id=)；都拿不到 / 非法 → 报错。
func parseID(c *gin.Context) (int64, error) {
	raw := c.Query("id")
	if raw == "" {
		raw = c.PostForm("id")
	}
	if raw == "" {
		return 0, errors.New("missing id")
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, errors.New("invalid id")
	}
	return id, nil
}

// parseListOptions 从 query 取 page / page_size，写好默认值与上限。
func parseListOptions(c *gin.Context) repository.ListOptions {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 200 {
		pageSize = 200
	}
	return repository.ListOptions{Page: page, PageSize: pageSize}
}

// requireNonZeroID 通过反射校验 entity 的 ID 字段非零。
// 与 internal/handler/base.go 同实现；只用于 admin EditPost 路径。
func requireNonZeroID(entity any) error {
	v := reflect.ValueOf(entity)
	if v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return errors.New("edit: nil entity")
		}
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return errors.New("edit: entity must be a struct")
	}
	idField := v.FieldByName("ID")
	if !idField.IsValid() {
		return errors.New("edit: entity has no ID field")
	}
	switch idField.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if idField.Int() == 0 {
			return errors.New("edit: missing or zero id")
		}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		if idField.Uint() == 0 {
			return errors.New("edit: missing or zero id")
		}
	default:
		return errors.New("edit: ID field must be an integer")
	}
	return nil
}

// toAdminInfo 把 *model.Admin 投影到不含敏感字段的 adminInfo。
//
// 当前不存在 Password 字段，结构体层面已"省略"；本函数的存在意义是显式表达
// "handler 不应把 model.Admin 原样序列化出去"的契约。
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

// toAdminInfoSlice 批量投影；用于 List 接口的 items。
func toAdminInfoSlice(items []model.Admin) []adminInfo {
	out := make([]adminInfo, len(items))
	for i := range items {
		out[i] = toAdminInfo(&items[i])
	}
	return out
}

// --- 管理页 CRUD 覆盖：去除 password 字段 + Delete 自保护 ---
//
// BaseHandler 默认把 *model.Admin 原样 JSON 出去（含 Password 字段）；
// admin 管理页的 spec "Passwords are never exposed" 要求每个 HTTP 响应都不含密码。
// 下面的覆盖等同于「手动复制 BaseHandler 的同名方法，但出口走 adminInfo」。

// Create 覆盖 BaseHandler.Create：成功 → 200 + adminInfo（不含 password）。
//
// @Summary  创建管理员账号
// @Tags     admin
// @Accept   json
// @Produce  json
// @Security BearerAuth
// @Param    req body model.Admin true "管理员字段（含 username / password 等）"
// @Success  200 {object} admin.adminInfo     "create.ok"
// @Failure  400 {object} map[string]string   "create.invalid_input / create.password_too_short"
// @Failure  500 {object} map[string]string   "create.internal"
// @Router   /admin/admin/create [post]
func (h *Handler) Create(c *gin.Context) {
	var entity model.Admin
	if err := c.ShouldBindJSON(&entity); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "admin.create.invalid_input", "message": err.Error()})
		return
	}
	if err := h.svc.Create(c, &entity); err != nil {
		if errors.Is(err, adminSvc.ErrPasswordTooShort) {
			c.JSON(http.StatusBadRequest, gin.H{
				"code":    "admin.create.password_too_short",
				"message": err.Error(),
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    "admin.create.internal",
			"message": "create failed",
		})
		return
	}
	c.JSON(http.StatusOK, toAdminInfo(&entity))
}

// List 覆盖 BaseHandler.List：items 走 adminInfo，不含 password。
//
// @Summary  管理员列表（搜索 + 分页）
// @Tags     admin
// @Produce  json
// @Security BearerAuth
// @Param    page      query int    false "页码（默认 1）"
// @Param    page_size query int    false "每页条数（默认 20，上限 200）"
// @Success  200 {object} map[string]any "list.ok"
// @Failure  500 {object} map[string]string "list.internal"
// @Router   /admin/admin/list [get]
func (h *Handler) List(c *gin.Context) {
	opts := parseListOptions(c)
	items, total, err := h.svc.List(c, opts)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    "admin.list.internal",
			"message": "list failed",
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"items":     toAdminInfoSlice(items),
		"total":     total,
		"page":      opts.Page,
		"page_size": opts.PageSize,
	})
}

// EditGet 覆盖 BaseHandler.EditGet：单行转 adminInfo。
//
// @Summary  取待编辑管理员行
// @Tags     admin
// @Produce  json
// @Security BearerAuth
// @Param    id query int true "管理员 ID"
// @Success  200 {object} admin.adminInfo  "edit.ok"
// @Failure  400 {object} map[string]string "edit.invalid_input"
// @Failure  404 {object} map[string]string "edit.not_found"
// @Failure  500 {object} map[string]string "edit.internal"
// @Router   /admin/admin/edit [get]
func (h *Handler) EditGet(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "admin.edit.invalid_input", "message": err.Error()})
		return
	}
	entity, err := h.svc.GetByID(c, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{
				"code":    "admin.edit.not_found",
				"message": err.Error(),
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    "admin.edit.internal",
			"message": "get failed",
		})
		return
	}
	c.JSON(http.StatusOK, toAdminInfo(entity))
}

// EditPost 覆盖 BaseHandler.EditPost：成功 → 200 + adminInfo。
//
// @Summary  提交修改管理员
// @Tags     admin
// @Accept   json
// @Produce  json
// @Security BearerAuth
// @Param    req body model.Admin true "管理员字段（必须含 id；password 空表示不改）"
// @Success  200 {object} admin.adminInfo      "edit.ok"
// @Failure  400 {object} map[string]string    "edit.invalid_input / edit.password_too_short"
// @Failure  500 {object} map[string]string    "edit.internal"
// @Router   /admin/admin/edit [post]
func (h *Handler) EditPost(c *gin.Context) {
	var patch model.Admin
	if err := c.ShouldBindJSON(&patch); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "admin.edit.invalid_input", "message": err.Error()})
		return
	}
	if err := requireNonZeroID(&patch); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "admin.edit.invalid_input", "message": err.Error()})
		return
	}
	// 先读原行：避免 client JSON 把 CreatedAt=0000-00-00 / UpdatedAt=zero 等时间戳字段全量回写
	// 触发 "Incorrect datetime value" 错误；也防止覆盖 Password / LastLoginAt 等不该由
	// 编辑接口改的字段（service.Update 会基于 password 字段是否非空决定是否重哈希）。
	entity, err := h.svc.GetByID(c, patch.ID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"code": "admin.edit.not_found", "message": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"code": "admin.edit.internal", "message": "get failed"})
		return
	}
	// 把 patch 里的业务字段选择性覆盖到 entity。
	// Username 在编辑时不可变（避免唯一索引漂移），即使 client 传了也忽略。
	patch.Username = ""
	entity.Nickname = patch.Nickname
	entity.Avatar = patch.Avatar
	entity.Email = patch.Email
	entity.Mobile = patch.Mobile
	entity.Bio = patch.Bio
	entity.Status = patch.Status
	if patch.Password != "" {
		entity.Password = patch.Password
	}
	if err := h.svc.Update(c, entity); err != nil {
		if errors.Is(err, adminSvc.ErrPasswordTooShort) {
			c.JSON(http.StatusBadRequest, gin.H{
				"code":    "admin.edit.password_too_short",
				"message": err.Error(),
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    "admin.edit.internal",
			"message": "edit failed",
		})
		return
	}
	c.JSON(http.StatusOK, toAdminInfo(entity))
}

// Delete 覆盖 BaseHandler.Delete：self-protection。
//
// spec 场景「Delete self → 403 admin.delete.self_protection」要求 admin
// 不能通过 /admin/admin/delete 删除自己；自保护判断放到 service 层会导致
// Delete 操作无法使用 CRUDRepository 的现成 Delete 路径（service 拿不到 self），
// 因此这里在 handler 入口拦截。
//
// @Summary  删除管理员账号（self-protection）
// @Tags     admin
// @Security BearerAuth
// @Param    id query int true "管理员 ID"
// @Success  200 {object} map[string]any      "delete.ok"
// @Failure  400 {object} map[string]string   "delete.invalid_input"
// @Failure  403 {object} map[string]string   "delete.self_protection"
// @Failure  500 {object} map[string]string   "delete.internal"
// @Router   /admin/admin/delete [post]
func (h *Handler) Delete(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "admin.delete.invalid_input", "message": err.Error()})
		return
	}
	if self := middleware.AdminFromContext(c); self != nil && uint(self.ID) == uint(id) {
		c.JSON(http.StatusForbidden, gin.H{
			"code":    "admin.delete.self_protection",
			"message": "cannot delete current admin",
		})
		return
	}
	if err := h.svc.Delete(c, id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    "admin.delete.internal",
			"message": "delete failed",
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// --- 管理页 4 个专属方法 ---

// changePasswordRequest 是 POST /admin/admin/change-password 的请求体。
type changePasswordRequest struct {
	ID          uint   `json:"id" binding:"required"`
	NewPassword string `json:"new_password" binding:"required"`
}

// ChangePassword 处理 POST /admin/admin/change-password。
//
// 错误映射：ErrPasswordTooShort → 400 admin.change_password.password_too_short；
// 其他 → 500 admin.change_password.internal。
//
// @Summary  重置指定管理员的密码（强制吊销其全部 token）
// @Tags     admin
// @Accept   json
// @Produce  json
// @Security BearerAuth
// @Param    req body admin.changePasswordRequest true "管理员 ID + 新密码"
// @Success  200 {object} map[string]string "change_password.ok"
// @Failure  400 {object} map[string]string "change_password.invalid_input / change_password.password_too_short"
// @Failure  500 {object} map[string]string "change_password.internal"
// @Router   /admin/admin/change-password [post]
func (h *Handler) ChangePassword(c *gin.Context) {
	var req changePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    "admin.change_password.invalid_input",
			"message": err.Error(),
		})
		return
	}
	if err := h.svc.ChangePassword(c, req.ID, req.NewPassword); err != nil {
		if errors.Is(err, adminSvc.ErrPasswordTooShort) {
			c.JSON(http.StatusBadRequest, gin.H{
				"code":    "admin.change_password.password_too_short",
				"message": err.Error(),
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    "admin.change_password.internal",
			"message": "change password failed",
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": "admin.change_password.ok", "message": "ok"})
}

// toggleStatusRequest 是 POST /admin/admin/toggle-status 的请求体。
type toggleStatusRequest struct {
	ID uint `json:"id" binding:"required"`
}

// ToggleStatus 处理 POST /admin/admin/toggle-status。
//
// 错误映射：ErrSelfProtection → 403；gorm.ErrRecordNotFound → 404；其他 → 500。
//
// @Summary  切换管理员状态（启用 ↔ 禁用；1→0 吊销 token）
// @Tags     admin
// @Accept   json
// @Produce  json
// @Security BearerAuth
// @Param    req body admin.toggleStatusRequest true "管理员 ID"
// @Success  200 {object} map[string]string "toggle_status.ok"
// @Failure  400 {object} map[string]string "toggle_status.invalid_input"
// @Failure  403 {object} map[string]string "toggle_status.self_protection"
// @Failure  404 {object} map[string]string "toggle_status.not_found"
// @Failure  500 {object} map[string]string "toggle_status.internal"
// @Router   /admin/admin/toggle-status [post]
func (h *Handler) ToggleStatus(c *gin.Context) {
	var req toggleStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    "admin.toggle_status.invalid_input",
			"message": err.Error(),
		})
		return
	}
	if err := h.svc.ToggleStatus(c, req.ID); err != nil {
		switch {
		case errors.Is(err, adminSvc.ErrSelfProtection):
			c.JSON(http.StatusForbidden, gin.H{
				"code":    "admin.toggle_status.self_protection",
				"message": err.Error(),
			})
		case errors.Is(err, gorm.ErrRecordNotFound):
			c.JSON(http.StatusNotFound, gin.H{
				"code":    "admin.toggle_status.not_found",
				"message": err.Error(),
			})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":    "admin.toggle_status.internal",
				"message": "toggle status failed",
			})
		}
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": "admin.toggle_status.ok", "message": "ok"})
}

// unlockRequest 是 POST /admin/admin/unlock 的请求体。
type unlockRequest struct {
	ID uint `json:"id" binding:"required"`
}

// Unlock 处理 POST /admin/admin/unlock。
//
// @Summary  解锁管理员（重置 LoginFailure=0 + Status=1）
// @Tags     admin
// @Accept   json
// @Produce  json
// @Security BearerAuth
// @Param    req body admin.unlockRequest true "管理员 ID"
// @Success  200 {object} map[string]string "unlock.ok"
// @Failure  400 {object} map[string]string "unlock.invalid_input"
// @Failure  500 {object} map[string]string "unlock.internal"
// @Router   /admin/admin/unlock [post]
func (h *Handler) Unlock(c *gin.Context) {
	var req unlockRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    "admin.unlock.invalid_input",
			"message": err.Error(),
		})
		return
	}
	if err := h.svc.Unlock(c, req.ID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    "admin.unlock.internal",
			"message": "unlock failed",
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": "admin.unlock.ok", "message": "ok"})
}

// batchDeleteRequest 是 POST /admin/admin/batch-delete 的请求体。
type batchDeleteRequest struct {
	IDs []uint `json:"ids"`
}

// BatchDelete 处理 POST /admin/admin/batch-delete。
//
// 空 ids / 全是 self → 仍返 200 + {deleted:0, skipped_self:n}（service 已处理）。
//
// @Summary  批量删除管理员（自动剔除 self）
// @Tags     admin
// @Accept   json
// @Produce  json
// @Security BearerAuth
// @Param    req body admin.batchDeleteRequest true "管理员 ID 列表"
// @Success  200 {object} map[string]int "batch_delete.ok"
// @Failure  400 {object} map[string]string "batch_delete.invalid_input"
// @Failure  500 {object} map[string]string "batch_delete.internal"
// @Router   /admin/admin/batch-delete [post]
func (h *Handler) BatchDelete(c *gin.Context) {
	var req batchDeleteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    "admin.batch_delete.invalid_input",
			"message": err.Error(),
		})
		return
	}
	deleted, skippedSelf, err := h.svc.BatchDelete(c, req.IDs)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    "admin.batch_delete.internal",
			"message": "batch delete failed",
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"deleted":       deleted,
		"skipped_self": skippedSelf,
	})
}
