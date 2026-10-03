// Package supplier — product.go 实现供应商盲盒管理路由（List / Create / Edit / ToggleOnSale）。
//
// 所有端点挂 SupplierAuth（router/supplier）；supplierID 一律取
// SupplierAuth 写入的 context 身份（mall_supplier_users.supplier_id），
// 不接受客户端传参 —— ownership 由 service 层二次校验，handler 先挡住
// 「未登录态」。
//
// 不提供 GET /edit（取单行）：ProductService 无 Get 单行方法，前端用列表行
// 数据进入编辑态（管理页数据量小，列表即取回全部字段）。
//
// 错误码映射（D12）：
//   - ErrBlindBoxNotFound   → 404 product.not_found
//   - ErrBlindBoxForbidden  → 403 product.forbidden
//   - ErrInvalidWeightSum   → 422 product.invalid_weight_sum
//   - ErrEmptyPool          → 422 product.empty_pool
//   - 其余                  → 500 product.internal
package supplier

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"ai-go-mall/internal/middleware"
	"ai-go-mall/internal/model/mall"
	"ai-go-mall/internal/repository"
	supplierSvc "ai-go-mall/internal/service/supplier"
)

// ProductHandler 处理供应商盲盒管理路由。
type ProductHandler struct {
	svc supplierSvc.ProductService
}

// NewProductHandler 接收 ProductService，返回 *ProductHandler。
func NewProductHandler(svc supplierSvc.ProductService) *ProductHandler {
	return &ProductHandler{svc: svc}
}

// CreateRequest 是 POST /supplier/products/create 的请求体。
type CreateRequest struct {
	Name        string       `json:"name" binding:"required"`
	Cover       string       `json:"cover"`
	Price       float64      `json:"price"`
	Description string       `json:"description"`
	Items       []PoolItemIn `json:"items" binding:"required,min=1"`
}

// PoolItemIn 是卡池条目入参（rarity/weight/stock 在 service 层校验加和）。
type PoolItemIn struct {
	CardID int64  `json:"card_id" binding:"required"`
	Rarity string `json:"rarity" binding:"required,oneof=SSR SR R N"`
	Weight int    `json:"weight"`
	Stock  int    `json:"stock"`
}

// EditRequest 是 POST /supplier/products/edit 的请求体（id 必填）。
type EditRequest struct {
	ID          int64    `json:"id" binding:"required"`
	Name        *string  `json:"name"`
	Cover       *string  `json:"cover"`
	Price       *float64 `json:"price"`
	Description *string  `json:"description"`
}

// ToggleOnSaleRequest 是 POST /supplier/products/toggle-onsale 的请求体。
type ToggleOnSaleRequest struct {
	ID     int64 `json:"id" binding:"required"`
	OnSale bool  `json:"on_sale"`
}

// List 处理 GET /supplier/products/list。
//
// @Summary      我的盲盒列表
// @Description  当前供应商的盲盒分页列表（含下架行）。
// @Tags         supplier
// @Produce      json
// @Security     BearerAuth
// @Param        page       query  int  false  "页码"
// @Param        page_size  query  int  false  "每页条数"
// @Success      200  {object}  map[string]any  "items/total/page/page_size"
// @Failure      401  {object}  map[string]string  "product.unauthorized"
// @Failure      500  {object}  map[string]string  "product.internal"
// @Router       /supplier/products/list [get]
func (h *ProductHandler) List(c *gin.Context) {
	supplierID, ok := requireSupplierID(c, "product")
	if !ok {
		return
	}
	opts := parseSupplierListOptions(c)
	items, total, err := h.svc.List(c, supplierID, opts)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    "product.internal",
			"message": "list products failed",
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

// Create 处理 POST /supplier/products/create（事务内建盲盒 + 卡池 + items）。
//
// @Summary      创建盲盒
// @Description  同一事务内创建盲盒、卡池与卡池条目；weight 加和必须 = 10000。
// @Tags         supplier
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        req  body  supplier.CreateRequest  true  "盲盒 + 卡池条目"
// @Success      200  {object}  mall.MallBlindBox  "新建盲盒（含 ID）"
// @Failure      400  {object}  map[string]string  "product.invalid_input"
// @Failure      401  {object}  map[string]string  "product.unauthorized"
// @Failure      403  {object}  map[string]string  "product.forbidden"
// @Failure      422  {object}  map[string]string  "product.invalid_weight_sum / product.empty_pool"
// @Failure      500  {object}  map[string]string  "product.internal"
// @Router       /supplier/products/create [post]
func (h *ProductHandler) Create(c *gin.Context) {
	supplierID, ok := requireSupplierID(c, "product")
	if !ok {
		return
	}

	var req CreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    "product.invalid_input",
			"message": err.Error(),
		})
		return
	}

	bb := &mall.MallBlindBox{
		SupplierID:  supplierID,
		Name:        req.Name,
		Cover:       req.Cover,
		Price:       req.Price,
		Description: req.Description,
		Status:      mall.StatusActive,
	}
	items := make([]mall.MallCardPoolItem, 0, len(req.Items))
	for _, it := range req.Items {
		items = append(items, mall.MallCardPoolItem{
			CardID: it.CardID,
			Rarity: mall.MallCardRarity(it.Rarity),
			Weight: it.Weight,
			Stock:  it.Stock,
		})
	}

	created, err := h.svc.Create(c, supplierID, bb, items)
	if err != nil {
		status, code := mapProductErr(err)
		c.JSON(status, gin.H{"code": code, "message": "create product failed"})
		return
	}
	c.JSON(http.StatusOK, created)
}

// Edit 处理 POST /supplier/products/edit（不修改 supplier_id / status / is_featured）。
//
// @Summary      编辑盲盒
// @Description  修改自家盲盒（名称/封面/价格/描述）；成功后失效 C 端详情缓存。
// @Tags         supplier
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        req  body  supplier.EditRequest  true  "含 id"
// @Success      200  {object}  map[string]string  "edit.ok"
// @Failure      400  {object}  map[string]string  "product.invalid_input"
// @Failure      401  {object}  map[string]string  "product.unauthorized"
// @Failure      403  {object}  map[string]string  "product.forbidden"
// @Failure      404  {object}  map[string]string  "product.not_found"
// @Failure      500  {object}  map[string]string  "product.internal"
// @Router       /supplier/products/edit [post]
func (h *ProductHandler) Edit(c *gin.Context) {
	supplierID, ok := requireSupplierID(c, "product")
	if !ok {
		return
	}

	var req EditRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.ID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    "product.invalid_input",
			"message": "invalid edit request",
		})
		return
	}

	// 指针字段「缺省 = 不改」：直接把指针交给 service，由它在 DB 原行上合并后再
	// 落库（仓储 Update 是 db.Save 全列写，必须交出完整行；此处不再构造半成品
	// 结构体 —— 那会把未传字段清零并撞 MySQL 1292）。
	patch := supplierSvc.ProductPatch{
		Name:        req.Name,
		Cover:       req.Cover,
		Price:       req.Price,
		Description: req.Description,
	}

	if err := h.svc.Update(c, supplierID, req.ID, patch); err != nil {
		status, code := mapProductErr(err)
		c.JSON(status, gin.H{"code": code, "message": "edit product failed"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": "edit.ok", "message": "ok"})
}

// ToggleOnSale 处理 POST /supplier/products/toggle-onsale。
//
// @Summary      上下架盲盒
// @Description  切换在售状态；on_sale=false 后 C 端列表与详情均不可见。
// @Tags         supplier
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        req  body  supplier.ToggleOnSaleRequest  true  "id + on_sale"
// @Success      200  {object}  map[string]string  "toggle.ok"
// @Failure      400  {object}  map[string]string  "product.invalid_input"
// @Failure      401  {object}  map[string]string  "product.unauthorized"
// @Failure      404  {object}  map[string]string  "product.not_found"
// @Failure      500  {object}  map[string]string  "product.internal"
// @Router       /supplier/products/toggle-onsale [post]
func (h *ProductHandler) ToggleOnSale(c *gin.Context) {
	supplierID, ok := requireSupplierID(c, "product")
	if !ok {
		return
	}

	var req ToggleOnSaleRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.ID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    "product.invalid_input",
			"message": "invalid toggle request",
		})
		return
	}

	if err := h.svc.ToggleOnSale(c, supplierID, req.ID, req.OnSale); err != nil {
		status, code := mapProductErr(err)
		c.JSON(status, gin.H{"code": code, "message": "toggle on sale failed"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": "toggle.ok", "message": "ok"})
}

// requireSupplierID 从 context 取当前供应商 ID；缺失时写出 401 并返回 false。
//
// codePrefix 区分 product / promotion 的错误码命名空间。
func requireSupplierID(c *gin.Context, codePrefix string) (int64, bool) {
	su := middleware.SupplierFromContext(c)
	if su == nil || su.SupplierID <= 0 {
		c.JSON(http.StatusUnauthorized, gin.H{
			"code":    codePrefix + ".unauthorized",
			"message": "login required",
		})
		return 0, false
	}
	return su.SupplierID, true
}

// mapProductErr 把 ProductService sentinel 映射到 (HTTP status, 业务码)。
func mapProductErr(err error) (int, string) {
	switch {
	case errors.Is(err, supplierSvc.ErrBlindBoxNotFound):
		return http.StatusNotFound, "product.not_found"
	case errors.Is(err, supplierSvc.ErrBlindBoxForbidden):
		return http.StatusForbidden, "product.forbidden"
	case errors.Is(err, supplierSvc.ErrInvalidWeightSum):
		return http.StatusUnprocessableEntity, "product.invalid_weight_sum"
	case errors.Is(err, supplierSvc.ErrEmptyPool):
		return http.StatusUnprocessableEntity, "product.empty_pool"
	default:
		return http.StatusInternalServerError, "product.internal"
	}
}

// parseSupplierListOptions 是 handler/supplier 包内的分页解析（与 user 包同构）。
func parseSupplierListOptions(c *gin.Context) repository.ListOptions {
	page := parseIntDefault(c.Query("page"), 1)
	pageSize := parseIntDefault(c.Query("page_size"), 20)
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

// parseIntDefault 解析失败返回默认值。
func parseIntDefault(raw string, def int) int {
	if n, err := strconv.Atoi(raw); err == nil && n > 0 {
		return n
	}
	return def
}
