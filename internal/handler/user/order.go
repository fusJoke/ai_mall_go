// Package user — order.go 实现 C 端订单查询路由（List / Detail）。
//
// 两个端点都挂 UserAuth（router/user）：订单是登录态私有数据，
// userID 一律取 token 身份，不接受客户端传参 —— 防越权枚举。
//
// 错误码映射（D12）：
//   - ErrOrderNotFound → 404 order.not_found（不存在与越权读统一 404，
//     不暴露订单存在性，与 service 层防 ID 枚举策略一致）；
//   - 其余 → 500 order.internal。
package user

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"ai-go-mall/internal/middleware"
	userSvc "ai-go-mall/internal/service/user"
)

// OrderHandler 处理 C 端订单查询路由（List / Detail）。
type OrderHandler struct {
	svc userSvc.OrderService
}

// NewOrderHandler 接收 OrderService，返回 *OrderHandler。
func NewOrderHandler(svc userSvc.OrderService) *OrderHandler {
	return &OrderHandler{svc: svc}
}

// List 处理 GET /user/orders/list。
//
// @Summary      我的订单列表
// @Description  当前登录用户的抽卡订单分页列表（created_at DESC）。
// @Tags         user
// @Produce      json
// @Security     BearerAuth
// @Param        page       query  int  false  "页码，默认 1"
// @Param        page_size  query  int  false  "每页条数，默认 20，上限 200"
// @Success      200  {object}  map[string]any  "items/total/page/page_size"
// @Failure      401  {object}  map[string]string  "order.unauthorized"
// @Failure      500  {object}  map[string]string  "order.internal"
// @Router       /user/orders/list [get]
func (h *OrderHandler) List(c *gin.Context) {
	u := middleware.UserFromContext(c)
	if u == nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"code":    "order.unauthorized",
			"message": "login required",
		})
		return
	}

	opts := parseUserListOptions(c)
	items, total, err := h.svc.List(c, u.ID, opts)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    "order.internal",
			"message": "list orders failed",
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

// Detail 处理 GET /user/orders/detail?id=。
//
// @Summary      订单详情
// @Description  单个抽卡订单 + 抽中明细（卡牌快照）；仅能查看自己的订单。
// @Tags         user
// @Produce      json
// @Security     BearerAuth
// @Param        id  query  int  true  "订单 ID"
// @Success      200  {object}  map[string]any  "order + items"
// @Failure      400  {object}  map[string]string  "order.invalid_id"
// @Failure      401  {object}  map[string]string  "order.unauthorized"
// @Failure      404  {object}  map[string]string  "order.not_found"
// @Failure      500  {object}  map[string]string  "order.internal"
// @Router       /user/orders/detail [get]
func (h *OrderHandler) Detail(c *gin.Context) {
	u := middleware.UserFromContext(c)
	if u == nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"code":    "order.unauthorized",
			"message": "login required",
		})
		return
	}

	orderID, err := parseUserPathID(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    "order.invalid_id",
			"message": "invalid order id",
		})
		return
	}

	order, items, err := h.svc.Detail(c, u.ID, orderID)
	if err != nil {
		if errors.Is(err, userSvc.ErrOrderNotFound) {
			c.JSON(http.StatusNotFound, gin.H{
				"code":    "order.not_found",
				"message": "order not found",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    "order.internal",
			"message": "get order detail failed",
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"order": order,
		"items": items,
	})
}
