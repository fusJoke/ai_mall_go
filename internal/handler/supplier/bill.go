// Package supplier — bill.go 实现供应商批次账单路由（List / Detail / Confirm / Reject）。
//
// 所有端点挂 SupplierAuth（router/supplier）；supplierID 一律取
// SupplierAuth 写入的 context 身份，不接受客户端传参 —— ownership
// 由 service 层二次校验，handler 只挡「未登录态」与入参形态。
//
// 账单状态查询参数 status 取值（空 = 全部）：
// generated / confirmed / rejected / failed / paid。
//
// 错误码映射（bill.* 命名空间）：
//   - ErrBillNotFound         → 404 bill.not_found
//   - ErrBillForbidden        → 403 bill.forbidden
//   - state.ErrInvalidStateTransition → 409 bill.invalid_transition
//   - ErrBillStatusConflict   → 409 bill.status_conflict
//   - ErrRejectReasonRequired → 422 bill.reject_reason_required
//   - 其余                    → 500 bill.internal
//
// 409 双码区分：invalid_transition = 预校验期状态机拒绝（如对 confirmed 账单
// 重复确认）；status_conflict = CAS UPDATE 0 行（并发抢先）。前端提示语不同
// （「当前状态不允许」vs「账单已被他人操作，请刷新」），故不合并。
package supplier

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"ai-go-mall/internal/domain/state"
	"ai-go-mall/internal/model/mall"
	supplierSvc "ai-go-mall/internal/service/supplier"
)

// BillHandler 处理供应商批次账单路由。
type BillHandler struct {
	svc supplierSvc.BillService
}

// NewBillHandler 接收 BillService，返回 *BillHandler。
func NewBillHandler(svc supplierSvc.BillService) *BillHandler {
	return &BillHandler{svc: svc}
}

// ConfirmBillRequest 是 POST /supplier/bills/confirm 的请求体。
type ConfirmBillRequest struct {
	ID int64 `json:"id" binding:"required"`
}

// RejectBillRequest 是 POST /supplier/bills/reject 的请求体。
//
// reason 不加 binding:required —— 空白校验在 service 层（ErrRejectReasonRequired
// → 422），避免同一规则双份实现。
type RejectBillRequest struct {
	ID     int64  `json:"id" binding:"required"`
	Reason string `json:"reason"`
}

// validBillStatus 判定 query status 是否为合法账单状态。
func validBillStatus(s string) bool {
	switch mall.MallSettlementBillStatus(s) {
	case mall.BillStatusGenerated, mall.BillStatusConfirmed, mall.BillStatusRejected,
		mall.BillStatusFailed, mall.BillStatusPaid:
		return true
	}
	return false
}

// List 处理 GET /supplier/bills/list。
//
// @Summary      我的批次账单列表
// @Description  当前供应商的结算账单分页列表；status 空 = 全部 tab。
// @Tags         supplier
// @Produce      json
// @Security     BearerAuth
// @Param        status     query  string  false  "账单状态(generated/confirmed/rejected/failed/paid)"
// @Param        page       query  int     false  "页码"
// @Param        page_size  query  int     false  "每页条数"
// @Success      200  {object}  map[string]any  "items/total/page/page_size"
// @Failure      400  {object}  map[string]string  "bill.invalid_input"
// @Failure      401  {object}  map[string]string  "bill.unauthorized"
// @Failure      500  {object}  map[string]string  "bill.internal"
// @Router       /supplier/bills/list [get]
func (h *BillHandler) List(c *gin.Context) {
	supplierID, ok := requireSupplierID(c, "bill")
	if !ok {
		return
	}

	status := mall.MallSettlementBillStatus(c.Query("status"))
	if status != "" && !validBillStatus(string(status)) {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    "bill.invalid_input",
			"message": "invalid bill status",
		})
		return
	}
	opts := parseSupplierListOptions(c)

	items, total, err := h.svc.List(c, supplierID, status, opts)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    "bill.internal",
			"message": "list bills failed",
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

// Detail 处理 GET /supplier/bills/detail。
//
// @Summary      账单详情
// @Description  账单 + 全部订单明细；不属于当前供应商 → 403。
// @Tags         supplier
// @Produce      json
// @Security     BearerAuth
// @Param        id  query  int  true  "账单 ID"
// @Success      200  {object}  map[string]any  "bill/items"
// @Failure      400  {object}  map[string]string  "bill.invalid_input"
// @Failure      401  {object}  map[string]string  "bill.unauthorized"
// @Failure      403  {object}  map[string]string  "bill.forbidden"
// @Failure      404  {object}  map[string]string  "bill.not_found"
// @Failure      500  {object}  map[string]string  "bill.internal"
// @Router       /supplier/bills/detail [get]
func (h *BillHandler) Detail(c *gin.Context) {
	supplierID, ok := requireSupplierID(c, "bill")
	if !ok {
		return
	}

	billID, err := strconv.ParseInt(c.Query("id"), 10, 64)
	if err != nil || billID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    "bill.invalid_input",
			"message": "invalid bill id",
		})
		return
	}

	bill, items, err := h.svc.Detail(c, supplierID, billID)
	if err != nil {
		status, code := mapBillErr(err)
		c.JSON(status, gin.H{"code": code, "message": "get bill detail failed"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"bill":  bill,
		"items": items,
	})
}

// Confirm 处理 POST /supplier/bills/confirm。
//
// @Summary      确认账单
// @Description  generated → confirmed；确认后等待 admin 打款。
// @Tags         supplier
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        req  body  supplier.ConfirmBillRequest  true  "账单 ID"
// @Success      200  {object}  map[string]string  "bill.ok"
// @Failure      400  {object}  map[string]string  "bill.invalid_input"
// @Failure      401  {object}  map[string]string  "bill.unauthorized"
// @Failure      403  {object}  map[string]string  "bill.forbidden"
// @Failure      404  {object}  map[string]string  "bill.not_found"
// @Failure      409  {object}  map[string]string  "bill.invalid_transition / bill.status_conflict"
// @Failure      500  {object}  map[string]string  "bill.internal"
// @Router       /supplier/bills/confirm [post]
func (h *BillHandler) Confirm(c *gin.Context) {
	supplierID, ok := requireSupplierID(c, "bill")
	if !ok {
		return
	}

	var req ConfirmBillRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.ID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    "bill.invalid_input",
			"message": "invalid confirm request",
		})
		return
	}

	if err := h.svc.Confirm(c, supplierID, req.ID); err != nil {
		status, code := mapBillErr(err)
		c.JSON(status, gin.H{"code": code, "message": "confirm bill failed"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": "bill.ok", "message": "ok"})
}

// Reject 处理 POST /supplier/bills/reject。
//
// @Summary      驳回账单
// @Description  generated → rejected；reason 必填（trim 后非空），写入 reject_reason。
// @Tags         supplier
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        req  body  supplier.RejectBillRequest  true  "账单 ID + 原因"
// @Success      200  {object}  map[string]string  "bill.ok"
// @Failure      400  {object}  map[string]string  "bill.invalid_input"
// @Failure      401  {object}  map[string]string  "bill.unauthorized"
// @Failure      403  {object}  map[string]string  "bill.forbidden"
// @Failure      404  {object}  map[string]string  "bill.not_found"
// @Failure      409  {object}  map[string]string  "bill.invalid_transition / bill.status_conflict"
// @Failure      422  {object}  map[string]string  "bill.reject_reason_required"
// @Failure      500  {object}  map[string]string  "bill.internal"
// @Router       /supplier/bills/reject [post]
func (h *BillHandler) Reject(c *gin.Context) {
	supplierID, ok := requireSupplierID(c, "bill")
	if !ok {
		return
	}

	var req RejectBillRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.ID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    "bill.invalid_input",
			"message": "invalid reject request",
		})
		return
	}

	if err := h.svc.Reject(c, supplierID, req.ID, req.Reason); err != nil {
		status, code := mapBillErr(err)
		c.JSON(status, gin.H{"code": code, "message": "reject bill failed"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": "bill.ok", "message": "ok"})
}

// mapBillErr 把 BillService sentinel 映射到 (HTTP status, 业务码)。
func mapBillErr(err error) (int, string) {
	switch {
	case errors.Is(err, supplierSvc.ErrBillNotFound):
		return http.StatusNotFound, "bill.not_found"
	case errors.Is(err, supplierSvc.ErrBillForbidden):
		return http.StatusForbidden, "bill.forbidden"
	case errors.Is(err, state.ErrInvalidStateTransition):
		return http.StatusConflict, "bill.invalid_transition"
	case errors.Is(err, supplierSvc.ErrBillStatusConflict):
		return http.StatusConflict, "bill.status_conflict"
	case errors.Is(err, supplierSvc.ErrRejectReasonRequired):
		return http.StatusUnprocessableEntity, "bill.reject_reason_required"
	default:
		return http.StatusInternalServerError, "bill.internal"
	}
}
