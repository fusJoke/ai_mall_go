// Package supplier — promotion.go 实现供应商限时特价管理路由
// （List / Create / Edit / Toggle / Delete）。
//
// 全部端点挂 SupplierAuth（router/supplier）；supplierID 一律取 context 身份。
// Create 时即使 body 伪造 supplier_id 也被强制覆盖为 context 身份（防越权挂活动）。
//
// 时间窗字段用 RFC3339 JSON 字符串（如 2026-10-01T00:00:00Z），由
// encoding/json 原生解析到 time.Time；格式非法 → 400 invalid_input。
//
// 错误码映射（D12）：
//   - ErrPromotionNotFound    → 404 promotion.not_found
//   - ErrPromotionForbidden   → 403 promotion.forbidden
//   - ErrPromotionTimeOverlap → 422 promotion.time_overlap
//   - ErrInvalidTimeWindow    → 422 promotion.invalid_time_window
//   - ErrInvalidPrice         → 422 promotion.invalid_price
//   - 其余                    → 500 promotion.internal
package supplier

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"ai-go-mall/internal/model/mall"
	supplierSvc "ai-go-mall/internal/service/supplier"
)

// PromotionHandler 处理供应商限时特价管理路由。
type PromotionHandler struct {
	svc supplierSvc.PromotionService
}

// NewPromotionHandler 接收 PromotionService，返回 *PromotionHandler。
func NewPromotionHandler(svc supplierSvc.PromotionService) *PromotionHandler {
	return &PromotionHandler{svc: svc}
}

// CreatePromotionRequest 是 POST /supplier/promotions/create 的请求体。
type CreatePromotionRequest struct {
	BlindBoxID    int64     `json:"blind_box_id" binding:"required"`
	OriginalPrice float64   `json:"original_price"`
	PromoPrice    float64   `json:"promo_price"`
	StartAt       time.Time `json:"start_at" binding:"required"`
	EndAt         time.Time `json:"end_at" binding:"required"`
}

// EditPromotionRequest 是 POST /supplier/promotions/edit 的请求体（仅价格字段）。
type EditPromotionRequest struct {
	ID            int64    `json:"id" binding:"required"`
	OriginalPrice *float64 `json:"original_price"`
	PromoPrice    *float64 `json:"promo_price"`
}

// TargetIDRequest 是 toggle / delete 共用的请求体。
type TargetIDRequest struct {
	ID int64 `json:"id" binding:"required"`
}

// List 处理 GET /supplier/promotions/list。
//
// @Summary      我的活动列表
// @Description  当前供应商的限时特价活动分页列表（start_at DESC）。
// @Tags         supplier
// @Produce      json
// @Security     BearerAuth
// @Param        page       query  int  false  "页码"
// @Param        page_size  query  int  false  "每页条数"
// @Success      200  {object}  map[string]any  "items/total/page/page_size"
// @Failure      401  {object}  map[string]string  "promotion.unauthorized"
// @Failure      500  {object}  map[string]string  "promotion.internal"
// @Router       /supplier/promotions/list [get]
func (h *PromotionHandler) List(c *gin.Context) {
	supplierID, ok := requireSupplierID(c, "promotion")
	if !ok {
		return
	}
	opts := parseSupplierListOptions(c)
	items, total, err := h.svc.List(c, supplierID, opts)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    "promotion.internal",
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

// Create 处理 POST /supplier/promotions/create。
//
// @Summary      创建活动
// @Description  为自家盲盒创建限时特价；时间窗重叠 → 422。
// @Tags         supplier
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        req  body  supplier.CreatePromotionRequest  true  "活动信息（RFC3339 时间）"
// @Success      200  {object}  mall.MallPromotion  "新建活动（含 ID）"
// @Failure      400  {object}  map[string]string  "promotion.invalid_input"
// @Failure      401  {object}  map[string]string  "promotion.unauthorized"
// @Failure      403  {object}  map[string]string  "promotion.forbidden"
// @Failure      422  {object}  map[string]string  "promotion.time_overlap / promotion.invalid_time_window / promotion.invalid_price"
// @Failure      500  {object}  map[string]string  "promotion.internal"
// @Router       /supplier/promotions/create [post]
func (h *PromotionHandler) Create(c *gin.Context) {
	supplierID, ok := requireSupplierID(c, "promotion")
	if !ok {
		return
	}

	var req CreatePromotionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    "promotion.invalid_input",
			"message": err.Error(),
		})
		return
	}

	// SupplierID 强制 context 身份 —— body 里的 supplier_id 一律忽略。
	promo := &mall.MallPromotion{
		SupplierID:    supplierID,
		BlindBoxID:    req.BlindBoxID,
		OriginalPrice: req.OriginalPrice,
		PromoPrice:    req.PromoPrice,
		StartAt:       req.StartAt,
		EndAt:         req.EndAt,
		Status:        mall.StatusActive,
	}

	created, err := h.svc.Create(c, supplierID, promo)
	if err != nil {
		status, code := mapPromotionErr(err)
		c.JSON(status, gin.H{"code": code, "message": "create promotion failed"})
		return
	}
	c.JSON(http.StatusOK, created)
}

// Edit 处理 POST /supplier/promotions/edit（仅价格字段；改时段 = 删旧建新）。
//
// @Summary      编辑活动价格
// @Description  仅修改 promo_price / original_price；时段与归属盲盒不可改。
// @Tags         supplier
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        req  body  supplier.EditPromotionRequest  true  "含 id"
// @Success      200  {object}  map[string]string  "edit.ok"
// @Failure      400  {object}  map[string]string  "promotion.invalid_input"
// @Failure      404  {object}  map[string]string  "promotion.not_found"
// @Failure      422  {object}  map[string]string  "promotion.invalid_price"
// @Failure      500  {object}  map[string]string  "promotion.internal"
// @Router       /supplier/promotions/edit [post]
func (h *PromotionHandler) Edit(c *gin.Context) {
	supplierID, ok := requireSupplierID(c, "promotion")
	if !ok {
		return
	}

	var req EditPromotionRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.ID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    "promotion.invalid_input",
			"message": "invalid edit request",
		})
		return
	}

	// 只传传入的价格字段（nil = 不改），由 service 在 DB 原行上合并后再落库。
	patch := supplierSvc.PromotionPatch{
		OriginalPrice: req.OriginalPrice,
		PromoPrice:    req.PromoPrice,
	}

	if err := h.svc.Update(c, supplierID, req.ID, patch); err != nil {
		status, code := mapPromotionErr(err)
		c.JSON(status, gin.H{"code": code, "message": "edit promotion failed"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": "edit.ok", "message": "ok"})
}

// Toggle 处理 POST /supplier/promotions/toggle（active ↔ disabled）。
//
// @Summary      启停活动
// @Description  切换活动状态；disabled 后活动价不参与计价。
// @Tags         supplier
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        req  body  supplier.TargetIDRequest  true  "活动 ID"
// @Success      200  {object}  map[string]string  "toggle.ok"
// @Failure      404  {object}  map[string]string  "promotion.not_found"
// @Failure      500  {object}  map[string]string  "promotion.internal"
// @Router       /supplier/promotions/toggle [post]
func (h *PromotionHandler) Toggle(c *gin.Context) {
	supplierID, ok := requireSupplierID(c, "promotion")
	if !ok {
		return
	}

	var req TargetIDRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.ID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    "promotion.invalid_input",
			"message": "invalid toggle request",
		})
		return
	}

	if err := h.svc.Toggle(c, supplierID, req.ID); err != nil {
		status, code := mapPromotionErr(err)
		c.JSON(status, gin.H{"code": code, "message": "toggle promotion failed"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": "toggle.ok", "message": "ok"})
}

// Delete 处理 POST /supplier/promotions/delete（软删）。
//
// @Summary      删除活动
// @Description  软删活动并失效相关详情缓存。
// @Tags         supplier
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        req  body  supplier.TargetIDRequest  true  "活动 ID"
// @Success      200  {object}  map[string]string  "delete.ok"
// @Failure      404  {object}  map[string]string  "promotion.not_found"
// @Failure      500  {object}  map[string]string  "promotion.internal"
// @Router       /supplier/promotions/delete [post]
func (h *PromotionHandler) Delete(c *gin.Context) {
	supplierID, ok := requireSupplierID(c, "promotion")
	if !ok {
		return
	}

	var req TargetIDRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.ID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    "promotion.invalid_input",
			"message": "invalid delete request",
		})
		return
	}

	if err := h.svc.Delete(c, supplierID, req.ID); err != nil {
		status, code := mapPromotionErr(err)
		c.JSON(status, gin.H{"code": code, "message": "delete promotion failed"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": "delete.ok", "message": "ok"})
}

// mapPromotionErr 把 PromotionService sentinel 映射到 (HTTP status, 业务码)。
func mapPromotionErr(err error) (int, string) {
	switch {
	case errors.Is(err, supplierSvc.ErrPromotionNotFound):
		return http.StatusNotFound, "promotion.not_found"
	case errors.Is(err, supplierSvc.ErrPromotionForbidden):
		return http.StatusForbidden, "promotion.forbidden"
	case errors.Is(err, supplierSvc.ErrPromotionTimeOverlap):
		return http.StatusUnprocessableEntity, "promotion.time_overlap"
	case errors.Is(err, supplierSvc.ErrInvalidTimeWindow):
		return http.StatusUnprocessableEntity, "promotion.invalid_time_window"
	case errors.Is(err, supplierSvc.ErrInvalidPrice):
		return http.StatusUnprocessableEntity, "promotion.invalid_price"
	default:
		return http.StatusInternalServerError, "promotion.internal"
	}
}
