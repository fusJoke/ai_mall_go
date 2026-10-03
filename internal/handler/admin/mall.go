// Package admin — mall.go 实现 admin 对供应商 / 盲盒的管理路由
// （供应商：List / ToggleStatus / ToggleFeatured；盲盒：List / ToggleStatus /
// ToggleOnSale / ToggleFeatured）。
//
// 与本包 admin.go（管理员自身管理）的风格保持一致：
//   - 列表 GET + query 分页；toggle POST + JSON body {id}；
//   - 错误码形如 admin.supplier.toggle_status.*（namespace.action.result）。
//
// 全部端点挂 AdminAuth（router/admin）。
package admin

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"ai-go-mall/internal/model/mall"
	"ai-go-mall/internal/repository"
	supplierRepo "ai-go-mall/internal/repository/supplier"
	adminSvc "ai-go-mall/internal/service/admin"
)

// =============================================================================
// 供应商管理
// =============================================================================

// MallSupplierHandler 处理 admin 的供应商管理路由。
type MallSupplierHandler struct {
	svc adminSvc.SupplierService
}

// NewMallSupplierHandler 接收 admin.SupplierService，返回 *MallSupplierHandler。
func NewMallSupplierHandler(svc adminSvc.SupplierService) *MallSupplierHandler {
	return &MallSupplierHandler{svc: svc}
}

// mallSupplierIDRequest 是供应商 toggle 类路由的请求体。
type mallSupplierIDRequest struct {
	ID int64 `json:"id" binding:"required"`
}

// List 处理 GET /admin/supplier/list。
//
// 筛选参数：?status=active|disabled（缺省不过滤）、?is_featured=true|false
// （缺省不过滤）、?name=关键字（模糊）。
//
// @Summary      供应商列表
// @Description  admin 管理页供应商筛选列表（含 disabled 行）。
// @Tags         admin
// @Produce      json
// @Security     BearerAuth
// @Param        page         query  int     false  "页码"
// @Param        page_size    query  int     false  "每页条数"
// @Param        status       query  string  false  "active / disabled"
// @Param        is_featured  query  bool    false  "true / false"
// @Param        name         query  string  false  "名称关键字"
// @Success      200  {object}  map[string]any  "items/total/page/page_size"
// @Failure      500  {object}  map[string]string  "admin.supplier.list.internal"
// @Router       /admin/supplier/list [get]
func (h *MallSupplierHandler) List(c *gin.Context) {
	opts := supplierRepo.SupplierListOptions{
		Page:         mallAdminPage(c),
		PageSize:     mallAdminPageSize(c),
		StatusFilter: c.Query("status"),
		NameKeyword:  c.Query("name"),
	}
	switch raw := c.Query("is_featured"); raw {
	case "true":
		v := true
		opts.IsFeaturedFilter = &v
	case "false":
		v := false
		opts.IsFeaturedFilter = &v
	}

	items, total, err := h.svc.List(c, opts)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    "admin.supplier.list.internal",
			"message": "list suppliers failed",
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"items":     items,
		"total":     total,
		"page":      opts.Page,
		"page_size": opts.PageSize,
	})
}

// ToggleStatus 处理 POST /admin/supplier/toggle-status。
//
// @Summary      切换供应商状态
// @Description  active ↔ disabled；禁用后该供应商盲盒在前端不可见。
// @Tags         admin
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        req  body  admin.mallSupplierIDRequest  true  "供应商 ID"
// @Success      200  {object}  map[string]string  "admin.supplier.toggle_status.ok"
// @Failure      400  {object}  map[string]string  "admin.supplier.toggle_status.invalid_input"
// @Failure      404  {object}  map[string]string  "admin.supplier.toggle_status.not_found"
// @Failure      500  {object}  map[string]string  "admin.supplier.toggle_status.internal"
// @Router       /admin/supplier/toggle-status [post]
func (h *MallSupplierHandler) ToggleStatus(c *gin.Context) {
	id, ok := bindMallAdminID(c, "admin.supplier.toggle_status")
	if !ok {
		return
	}
	if err := h.svc.ToggleStatus(c, id); err != nil {
		writeMallAdminToggleErr(c, "admin.supplier.toggle_status", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": "admin.supplier.toggle_status.ok", "message": "ok"})
}

// ToggleFeatured 处理 POST /admin/supplier/toggle-featured。
//
// @Summary      切换供应商推荐位
// @Description  is_featured 翻转；featured 且 active 才进 ES feed。
// @Tags         admin
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        req  body  admin.mallSupplierIDRequest  true  "供应商 ID"
// @Success      200  {object}  map[string]string  "admin.supplier.toggle_featured.ok"
// @Failure      400  {object}  map[string]string  "admin.supplier.toggle_featured.invalid_input"
// @Failure      404  {object}  map[string]string  "admin.supplier.toggle_featured.not_found"
// @Failure      500  {object}  map[string]string  "admin.supplier.toggle_featured.internal"
// @Router       /admin/supplier/toggle-featured [post]
func (h *MallSupplierHandler) ToggleFeatured(c *gin.Context) {
	id, ok := bindMallAdminID(c, "admin.supplier.toggle_featured")
	if !ok {
		return
	}
	if err := h.svc.ToggleFeatured(c, id); err != nil {
		writeMallAdminToggleErr(c, "admin.supplier.toggle_featured", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": "admin.supplier.toggle_featured.ok", "message": "ok"})
}

// =============================================================================
// 盲盒管理
// =============================================================================

// MallBlindBoxHandler 处理 admin 的盲盒管理路由。
type MallBlindBoxHandler struct {
	svc adminSvc.BlindBoxService
}

// NewMallBlindBoxHandler 接收 admin.BlindBoxService，返回 *MallBlindBoxHandler。
func NewMallBlindBoxHandler(svc adminSvc.BlindBoxService) *MallBlindBoxHandler {
	return &MallBlindBoxHandler{svc: svc}
}

// List 处理 GET /admin/blindbox/list（全量，含 disabled / 下架行）。
//
// @Summary      盲盒列表
// @Description  admin 视角的全量盲盒分页列表。
// @Tags         admin
// @Produce      json
// @Security     BearerAuth
// @Param        page       query  int  false  "页码"
// @Param        page_size  query  int  false  "每页条数"
// @Success      200  {object}  map[string]any  "items/total/page/page_size"
// @Failure      500  {object}  map[string]string  "admin.blindbox.list.internal"
// @Router       /admin/blindbox/list [get]
func (h *MallBlindBoxHandler) List(c *gin.Context) {
	opts := repository.ListOptions{
		Page:     mallAdminPage(c),
		PageSize: mallAdminPageSize(c),
	}
	items, total, err := h.svc.List(c, opts)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    "admin.blindbox.list.internal",
			"message": "list blind boxes failed",
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"items":     items,
		"total":     total,
		"page":      opts.Page,
		"page_size": opts.PageSize,
	})
}

// MallBlindBoxIDRequest 是盲盒 toggle 类路由的请求体。
type MallBlindBoxIDRequest struct {
	ID int64 `json:"id" binding:"required"`
}

// ToggleStatus 处理 POST /admin/blindbox/toggle-status。
//
// @Summary      切换盲盒状态
// @Description  active ↔ disabled（admin 强制上下架，与供应商在售解耦）。
// @Tags         admin
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        req  body  admin.MallBlindBoxIDRequest  true  "盲盒 ID"
// @Success      200  {object}  map[string]string  "admin.blindbox.toggle_status.ok"
// @Failure      400  {object}  map[string]string  "admin.blindbox.toggle_status.invalid_input"
// @Failure      404  {object}  map[string]string  "admin.blindbox.toggle_status.not_found"
// @Failure      500  {object}  map[string]string  "admin.blindbox.toggle_status.internal"
// @Router       /admin/blindbox/toggle-status [post]
func (h *MallBlindBoxHandler) ToggleStatus(c *gin.Context) {
	id, ok := bindMallAdminID(c, "admin.blindbox.toggle_status")
	if !ok {
		return
	}
	if err := h.svc.ToggleStatus(c, id); err != nil {
		writeMallAdminToggleErr(c, "admin.blindbox.toggle_status", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": "admin.blindbox.toggle_status.ok", "message": "ok"})
}

// ToggleOnSale 处理 POST /admin/blindbox/toggle-onsale。
//
// @Summary      切换盲盒在售
// @Description  on_sale 翻转（强制上下架）。
// @Tags         admin
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        req  body  admin.MallBlindBoxIDRequest  true  "盲盒 ID"
// @Success      200  {object}  map[string]string  "admin.blindbox.toggle_onsale.ok"
// @Failure      404  {object}  map[string]string  "admin.blindbox.toggle_onsale.not_found"
// @Failure      500  {object}  map[string]string  "admin.blindbox.toggle_onsale.internal"
// @Router       /admin/blindbox/toggle-onsale [post]
func (h *MallBlindBoxHandler) ToggleOnSale(c *gin.Context) {
	id, ok := bindMallAdminID(c, "admin.blindbox.toggle_onsale")
	if !ok {
		return
	}
	if err := h.svc.ToggleOnSale(c, id); err != nil {
		writeMallAdminToggleErr(c, "admin.blindbox.toggle_onsale", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": "admin.blindbox.toggle_onsale.ok", "message": "ok"})
}

// ToggleFeatured 处理 POST /admin/blindbox/toggle-featured。
//
// @Summary      切换盲盒推荐位
// @Description  is_featured 翻转；featured + active + on_sale 才进 ES feed。
// @Tags         admin
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        req  body  admin.MallBlindBoxIDRequest  true  "盲盒 ID"
// @Success      200  {object}  map[string]string  "admin.blindbox.toggle_featured.ok"
// @Failure      404  {object}  map[string]string  "admin.blindbox.toggle_featured.not_found"
// @Failure      500  {object}  map[string]string  "admin.blindbox.toggle_featured.internal"
// @Router       /admin/blindbox/toggle-featured [post]
func (h *MallBlindBoxHandler) ToggleFeatured(c *gin.Context) {
	id, ok := bindMallAdminID(c, "admin.blindbox.toggle_featured")
	if !ok {
		return
	}
	if err := h.svc.ToggleFeatured(c, id); err != nil {
		writeMallAdminToggleErr(c, "admin.blindbox.toggle_featured", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": "admin.blindbox.toggle_featured.ok", "message": "ok"})
}

// =============================================================================
// 共用 helper
// =============================================================================

// bindMallAdminID 解析 toggle 请求体 {id}；失败时直接写出 400 响应。
func bindMallAdminID(c *gin.Context, codeNS string) (int64, bool) {
	var req struct {
		ID int64 `json:"id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.ID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    codeNS + ".invalid_input",
			"message": "invalid request",
		})
		return 0, false
	}
	return req.ID, true
}

// writeMallAdminToggleErr 把 toggle 类 service 错误写成统一响应。
func writeMallAdminToggleErr(c *gin.Context, codeNS string, err error) {
	switch {
	case errors.Is(err, supplierRepo.ErrSupplierNotFound), errors.Is(err, adminSvc.ErrBlindBoxNotFound):
		c.JSON(http.StatusNotFound, gin.H{
			"code":    codeNS + ".not_found",
			"message": "target not found",
		})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    codeNS + ".internal",
			"message": "toggle failed",
		})
	}
}

// mallAdminPage / mallAdminPageSize 分页参数（默认 1 / 20，上限 200）。
func mallAdminPage(c *gin.Context) int {
	n, err := strconv.Atoi(c.DefaultQuery("page", "1"))
	if err != nil || n < 1 {
		return 1
	}
	return n
}

func mallAdminPageSize(c *gin.Context) int {
	n, err := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if err != nil || n < 1 {
		return 20
	}
	if n > 200 {
		return 200
	}
	return n
}

// =============================================================================
// 活动管理（只读）
// =============================================================================

// MallPromotionHandler 处理 admin 的限时特价只读路由。
type MallPromotionHandler struct {
	svc adminSvc.PromotionService
}

// NewMallPromotionHandler 接收 admin.PromotionService，返回 *MallPromotionHandler。
func NewMallPromotionHandler(svc adminSvc.PromotionService) *MallPromotionHandler {
	return &MallPromotionHandler{svc: svc}
}

// List 处理 GET /admin/promotion/list（全量活动，分页）。
//
// @Summary      活动列表
// @Description  admin 视角的全量限时特价列表（只读；管理动作在供应商端）。
// @Tags         admin
// @Produce      json
// @Security     BearerAuth
// @Param        page       query  int  false  "页码"
// @Param        page_size  query  int  false  "每页条数"
// @Success      200  {object}  map[string]any  "items/total/page/page_size"
// @Failure      500  {object}  map[string]string  "admin.promotion.list.internal"
// @Router       /admin/promotion/list [get]
func (h *MallPromotionHandler) List(c *gin.Context) {
	opts := repository.ListOptions{
		Page:     mallAdminPage(c),
		PageSize: mallAdminPageSize(c),
	}
	items, total, err := h.svc.List(c, opts)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    "admin.promotion.list.internal",
			"message": "list promotions failed",
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"items":     items,
		"total":     total,
		"page":      opts.Page,
		"page_size": opts.PageSize,
	})
}

// =============================================================================
// 秒杀活动管理（D15）
// =============================================================================

// MallSeckillHandler 处理 admin 的秒杀活动管理路由。
type MallSeckillHandler struct {
	svc adminSvc.SeckillService
}

// NewMallSeckillHandler 接收 admin.SeckillService，返回 *MallSeckillHandler。
func NewMallSeckillHandler(svc adminSvc.SeckillService) *MallSeckillHandler {
	return &MallSeckillHandler{svc: svc}
}

// seckillCreateRequest 是 POST /admin/seckill 的请求体（admin 代供应商创建）。
type seckillCreateRequest struct {
	SupplierID   int64     `json:"supplier_id" binding:"required"`
	BlindBoxID   int64     `json:"blind_box_id" binding:"required"`
	SeckillPrice float64   `json:"seckill_price" binding:"required,gt=0"`
	TotalStock   int       `json:"total_stock" binding:"required,gt=0"`
	PerUserLimit int       `json:"per_user_limit" binding:"required,gt=0"`
	StartAt      time.Time `json:"start_at" binding:"required"`
	EndAt        time.Time `json:"end_at" binding:"required"`
}

// List 处理 GET /admin/seckill/list（全量分页）。
//
// @Summary      秒杀活动列表
// @Description  admin 视角的全量秒杀活动列表（按 start_at DESC）。
// @Tags         admin
// @Produce      json
// @Security     BearerAuth
// @Param        page       query  int  false  "页码"
// @Param        page_size  query  int  false  "每页条数"
// @Success      200  {object}  map[string]any  "items/total/page/page_size"
// @Failure      500  {object}  map[string]string  "admin.seckill.list.internal"
// @Router       /admin/seckill/list [get]
func (h *MallSeckillHandler) List(c *gin.Context) {
	opts := repository.ListOptions{
		Page:     mallAdminPage(c),
		PageSize: mallAdminPageSize(c),
	}
	items, total, err := h.svc.List(c, opts)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    "admin.seckill.list.internal",
			"message": "list seckill activities failed",
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"items":     items,
		"total":     total,
		"page":      opts.Page,
		"page_size": opts.PageSize,
	})
}

// Create 处理 POST /admin/seckill（admin 代供应商创建）。
//
// @Summary      创建秒杀活动
// @Description  admin 代指定 supplier 创建；同 supplier 同接口的校验（时间窗 / 价格 / 卡池 / Redis init）。
// @Tags         admin
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        req  body  admin.seckillCreateRequest  true  "秒杀活动参数"
// @Success      200  {object}  map[string]any  "活动详情"
// @Failure      400  {object}  map[string]string  "admin.seckill.create.invalid_input"
// @Failure      500  {object}  map[string]string  "admin.seckill.create.internal"
// @Router       /admin/seckill [post]
func (h *MallSeckillHandler) Create(c *gin.Context) {
	var req seckillCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    "admin.seckill.create.invalid_input",
			"message": "invalid seckill request",
		})
		return
	}

	sec := &mall.MallSeckillActivity{
		SupplierID:   req.SupplierID,
		BlindBoxID:   req.BlindBoxID,
		SeckillPrice: req.SeckillPrice,
		TotalStock:   req.TotalStock,
		PerUserLimit: req.PerUserLimit,
		StartAt:      req.StartAt,
		EndAt:        req.EndAt,
		Status:       mall.StatusActive,
	}

	out, err := h.svc.Create(c, sec)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    "admin.seckill.create.internal",
			"message": err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, out)
}

// Toggle 处理 POST /admin/seckill/:id/toggle（admin 强制启停，无 ownership 校验）。
//
// @Summary      启停秒杀活动
// @Description  admin 视角无 supplier 归属约束；路径 :id 是 mall_seckill_activities.id。
// @Tags         admin
// @Produce      json
// @Security     BearerAuth
// @Param        id  path  int  true  "秒杀活动 ID"
// @Success      200  {object}  map[string]string  "admin.seckill.toggle.ok"
// @Failure      400  {object}  map[string]string  "admin.seckill.toggle.invalid_id"
// @Failure      404  {object}  map[string]string  "admin.seckill.toggle.not_found"
// @Failure      500  {object}  map[string]string  "admin.seckill.toggle.internal"
// @Router       /admin/seckill/{id}/toggle [post]
func (h *MallSeckillHandler) Toggle(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    "admin.seckill.toggle.invalid_id",
			"message": "invalid seckill id",
		})
		return
	}
	if err := h.svc.Toggle(c, id); err != nil {
		if errors.Is(err, adminSvc.ErrSeckillNotFound) {
			c.JSON(http.StatusNotFound, gin.H{
				"code":    "admin.seckill.toggle.not_found",
				"message": "seckill not found",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    "admin.seckill.toggle.internal",
			"message": "toggle seckill failed",
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"code":    "admin.seckill.toggle.ok",
		"message": "ok",
	})
}
