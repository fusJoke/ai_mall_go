// Package supplier — seckill.go 实现供应商秒杀活动管理路由
// （List / Create / Edit / Toggle）。
//
// 全部端点挂 SupplierAuth（router/supplier）；supplierID 一律取 context 身份，
// Create 时 body 伪造 supplier_id 也被 service 强制覆盖（防越权）。
//
// 业务规则（service/supplier/seckill.go）：
//   - Create：时间窗校验（end > start 且 start 不早于 now-1min）+ 归属校验
//     + 秒杀价 < 原价 + total_stock ≤ 卡池库存 + 同事务 SETNX Redis 库存；
//   - Edit：仅 seckill_price / per_user_limit；时间窗与盲盒创建后不可改；
//   - Toggle：active ↔ disabled。
//
// 错误码映射（对齐 spec "Seckill activity schema" 的 422 约定）：
//   - ErrSeckillNotFound        → 404 seckill.not_found
//   - ErrSeckillForbidden       → 403 seckill.forbidden
//   - ErrSeckillStockExceedsPool → 422 seckill.stock_exceeds_pool
//   - ErrInvalidSeckillWindow   → 422 seckill.invalid_time_window
//   - ErrInvalidSeckillPrice    → 422 seckill.invalid_price
//   - ErrRedisInitFailed        → 500 seckill.redis_init_failed
//   - 其余                      → 500 seckill.internal
package supplier

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"ai-go-mall/internal/model/mall"
	supplierSvc "ai-go-mall/internal/service/supplier"
)

// SeckillHandler 处理供应商秒杀活动管理路由。
type SeckillHandler struct {
	svc supplierSvc.SeckillService
}

// NewSeckillHandler 接收 SeckillService，返回 *SeckillHandler。
func NewSeckillHandler(svc supplierSvc.SeckillService) *SeckillHandler {
	return &SeckillHandler{svc: svc}
}

// CreateSeckillRequest 是 POST /supplier/seckill/create 的请求体。
type CreateSeckillRequest struct {
	BlindBoxID   int64     `json:"blind_box_id" binding:"required"`
	SeckillPrice float64   `json:"seckill_price" binding:"required,gt=0"`
	TotalStock   int       `json:"total_stock" binding:"required,gt=0"`
	PerUserLimit int       `json:"per_user_limit" binding:"required,gt=0"`
	StartAt      time.Time `json:"start_at" binding:"required"`
	EndAt        time.Time `json:"end_at" binding:"required"`
}

// EditSeckillRequest 是 POST /supplier/seckill/edit 的请求体（仅价 / 限购字段）。
type EditSeckillRequest struct {
	ID           int64    `json:"id" binding:"required"`
	SeckillPrice *float64 `json:"seckill_price"`
	PerUserLimit *int     `json:"per_user_limit"`
}

// List 处理 GET /supplier/seckill/list。
//
// @Summary      我的秒杀活动列表
// @Description  当前供应商的秒杀活动分页列表（start_at DESC）。
// @Tags         supplier
// @Produce      json
// @Security     BearerAuth
// @Param        page       query  int  false  "页码"
// @Param        page_size  query  int  false  "每页条数"
// @Success      200  {object}  map[string]any  "items/total/page/page_size"
// @Failure      401  {object}  map[string]string  "seckill.unauthorized"
// @Failure      500  {object}  map[string]string  "seckill.internal"
// @Router       /supplier/seckill/list [get]
func (h *SeckillHandler) List(c *gin.Context) {
	supplierID, ok := requireSupplierID(c, "seckill")
	if !ok {
		return
	}
	opts := parseSupplierListOptions(c)
	items, total, err := h.svc.List(c, supplierID, opts)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    "seckill.internal",
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

// Create 处理 POST /supplier/seckill/create。
//
// @Summary      创建秒杀活动
// @Description  为自家盲盒创建秒杀；total_stock 超卡池库存 → 422；时间窗非法 → 422。
// @Tags         supplier
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        req  body  supplier.CreateSeckillRequest  true  "秒杀参数（RFC3339 时间）"
// @Success      200  {object}  mall.MallSeckillActivity  "新建活动（含 ID）"
// @Failure      400  {object}  map[string]string  "seckill.invalid_input"
// @Failure      401  {object}  map[string]string  "seckill.unauthorized"
// @Failure      403  {object}  map[string]string  "seckill.forbidden"
// @Failure      422  {object}  map[string]string  "seckill.invalid_time_window / seckill.invalid_price / seckill.stock_exceeds_pool"
// @Failure      500  {object}  map[string]string  "seckill.redis_init_failed / seckill.internal"
// @Router       /supplier/seckill/create [post]
func (h *SeckillHandler) Create(c *gin.Context) {
	supplierID, ok := requireSupplierID(c, "seckill")
	if !ok {
		return
	}

	var req CreateSeckillRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    "seckill.invalid_input",
			"message": err.Error(),
		})
		return
	}

	// SupplierID 由 service 强制 context 身份 —— body 里的值一律忽略。
	sec := &mall.MallSeckillActivity{
		BlindBoxID:   req.BlindBoxID,
		SeckillPrice: req.SeckillPrice,
		TotalStock:   req.TotalStock,
		PerUserLimit: req.PerUserLimit,
		StartAt:      req.StartAt,
		EndAt:        req.EndAt,
		Status:       mall.StatusActive,
	}

	created, err := h.svc.Create(c, supplierID, sec)
	if err != nil {
		status, code := mapSeckillErr(err)
		c.JSON(status, gin.H{"code": code, "message": "create seckill failed"})
		return
	}
	c.JSON(http.StatusOK, created)
}

// Edit 处理 POST /supplier/seckill/edit（仅 seckill_price / per_user_limit）。
//
// @Summary      编辑秒杀活动
// @Description  仅修改秒杀价与限购数量；时间窗与归属盲盒不可改（改时段 = 删旧建新）。
// @Tags         supplier
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        req  body  supplier.EditSeckillRequest  true  "含 id"
// @Success      200  {object}  map[string]string  "edit.ok"
// @Failure      400  {object}  map[string]string  "seckill.invalid_input"
// @Failure      401  {object}  map[string]string  "seckill.unauthorized"
// @Failure      403  {object}  map[string]string  "seckill.forbidden"
// @Failure      404  {object}  map[string]string  "seckill.not_found"
// @Failure      422  {object}  map[string]string  "seckill.invalid_price"
// @Failure      500  {object}  map[string]string  "seckill.internal"
// @Router       /supplier/seckill/edit [post]
func (h *SeckillHandler) Edit(c *gin.Context) {
	supplierID, ok := requireSupplierID(c, "seckill")
	if !ok {
		return
	}

	var req EditSeckillRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.ID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    "seckill.invalid_input",
			"message": "invalid edit request",
		})
		return
	}

	// nil = 该字段不改，由 service 在 DB 原行上合并后再落库。
	patch := supplierSvc.SeckillPatch{
		SeckillPrice: req.SeckillPrice,
		PerUserLimit: req.PerUserLimit,
	}

	if err := h.svc.Update(c, supplierID, req.ID, patch); err != nil {
		status, code := mapSeckillErr(err)
		c.JSON(status, gin.H{"code": code, "message": "edit seckill failed"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": "edit.ok", "message": "ok"})
}

// Toggle 处理 POST /supplier/seckill/toggle（active ↔ disabled）。
//
// @Summary      启停秒杀活动
// @Description  切换活动状态；disabled 后 C 端不可见、秒杀接口拒绝。
// @Tags         supplier
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        req  body  supplier.TargetIDRequest  true  "秒杀活动 ID"
// @Success      200  {object}  map[string]string  "toggle.ok"
// @Failure      400  {object}  map[string]string  "seckill.invalid_input"
// @Failure      403  {object}  map[string]string  "seckill.forbidden"
// @Failure      404  {object}  map[string]string  "seckill.not_found"
// @Failure      500  {object}  map[string]string  "seckill.internal"
// @Router       /supplier/seckill/toggle [post]
func (h *SeckillHandler) Toggle(c *gin.Context) {
	supplierID, ok := requireSupplierID(c, "seckill")
	if !ok {
		return
	}

	var req TargetIDRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.ID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    "seckill.invalid_input",
			"message": "invalid toggle request",
		})
		return
	}

	if err := h.svc.Toggle(c, supplierID, req.ID); err != nil {
		status, code := mapSeckillErr(err)
		c.JSON(status, gin.H{"code": code, "message": "toggle seckill failed"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": "toggle.ok", "message": "ok"})
}

// mapSeckillErr 把 supplier.SeckillService sentinel 映射到 (HTTP status, 业务码)。
func mapSeckillErr(err error) (int, string) {
	switch {
	case errors.Is(err, supplierSvc.ErrSeckillNotFound):
		return http.StatusNotFound, "seckill.not_found"
	case errors.Is(err, supplierSvc.ErrSeckillForbidden):
		return http.StatusForbidden, "seckill.forbidden"
	case errors.Is(err, supplierSvc.ErrSeckillStockExceedsPool):
		return http.StatusUnprocessableEntity, "seckill.stock_exceeds_pool"
	case errors.Is(err, supplierSvc.ErrInvalidSeckillWindow):
		return http.StatusUnprocessableEntity, "seckill.invalid_time_window"
	case errors.Is(err, supplierSvc.ErrInvalidSeckillPrice):
		return http.StatusUnprocessableEntity, "seckill.invalid_price"
	case errors.Is(err, supplierSvc.ErrRedisInitFailed):
		return http.StatusInternalServerError, "seckill.redis_init_failed"
	default:
		return http.StatusInternalServerError, "seckill.internal"
	}
}
