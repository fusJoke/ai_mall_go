// Package admin — bill.go 实现 admin 对批次账单的管理路由
// （List / Detail / Regenerate / MarkPaid）。
//
// 与 settlement（周期结算）的区别：批次账单「1 商品 = 1 账单」，由抽卡事务
// 售罄时自动生成（user.draw 7.7 → mall.BillService.FinalizeSoldOut）；
// admin 的职责是核对 rejected 账单（Regenerate 重核金额）、对 confirmed
// 账单打款（MarkPaid：账单置 paid + 扣 supplier.balance + 商品置 settled）。
//
// 错误码形如 admin.bill.<action>.<result>（与 settlement.go 同风格）。
//
// 全部端点挂 AdminAuth（router/admin）。
package admin

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"ai-go-mall/internal/domain/state"
	"ai-go-mall/internal/middleware"
	"ai-go-mall/internal/model/mall"
	"ai-go-mall/internal/repository"
	mallRepo "ai-go-mall/internal/repository/mall"
	mallSvc "ai-go-mall/internal/service/mall"
)

// =============================================================================
// MallBillHandler
// =============================================================================

// MallBillHandler 处理 admin 的批次账单管理路由。
//
// 4 个端点对应 service 层 4 个方法：
//   - List       GET  /admin/bill/list
//   - Detail     GET  /admin/bill/detail?id=N
//   - Regenerate POST /admin/bill/regenerate
//   - MarkPaid   POST /admin/bill/mark-paid
type MallBillHandler struct {
	svc mallSvc.BillService
}

// NewMallBillHandler 接收 mall.BillService，返回 *MallBillHandler。
func NewMallBillHandler(svc mallSvc.BillService) *MallBillHandler {
	return &MallBillHandler{svc: svc}
}

// billRegenerateRequest 是 POST /admin/bill/regenerate 的请求体。
//
// 金额四件套整体快照更新（rejected → generated）；remark 为重核备注（选填）。
type billRegenerateRequest struct {
	ID               int64   `json:"id" binding:"required"`
	TotalAmount      float64 `json:"total_amount"`
	CommissionRate   float64 `json:"commission_rate"`
	CommissionAmount float64 `json:"commission_amount"`
	PayoutAmount     float64 `json:"payout_amount"`
	Remark           string  `json:"remark"`
}

// billMarkPaidRequest 是 POST /admin/bill/mark-paid 的请求体。
type billMarkPaidRequest struct {
	ID int64 `json:"id" binding:"required"`
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

// List 处理 GET /admin/bill/list。
//
// @Summary      批次账单列表
// @Description  admin 视角的全量批次账单分页列表；status / supplier_id 零值不筛。
// @Tags         admin
// @Produce      json
// @Security     BearerAuth
// @Param        status       query  string  false  "账单状态(generated/confirmed/rejected/failed/paid)"
// @Param        supplier_id  query  int     false  "供应商 ID"
// @Param        page         query  int     false  "页码"
// @Param        page_size    query  int     false  "每页条数"
// @Success      200  {object}  map[string]any  "items/total/page/page_size"
// @Failure      400  {object}  map[string]string  "admin.bill.list.invalid_input"
// @Failure      500  {object}  map[string]string  "admin.bill.list.internal"
// @Router       /admin/bill/list [get]
func (h *MallBillHandler) List(c *gin.Context) {
	status := mall.MallSettlementBillStatus(c.Query("status"))
	if status != "" && !validBillStatus(string(status)) {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    "admin.bill.list.invalid_input",
			"message": "invalid bill status",
		})
		return
	}
	supplierID, _ := strconv.ParseInt(c.Query("supplier_id"), 10, 64) // 解析失败 = 不筛
	opts := repository.ListOptions{
		Page:     mallAdminPage(c),
		PageSize: mallAdminPageSize(c),
	}

	items, total, err := h.svc.ListAdmin(c, status, supplierID, opts)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    "admin.bill.list.internal",
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

// Detail 处理 GET /admin/bill/detail?id=N。
//
// @Summary      批次账单详情
// @Description  返回 bill + items 列表（明细 = 逐笔 drawn 订单金额快照）。
// @Tags         admin
// @Produce      json
// @Security     BearerAuth
// @Param        id  query  int  true  "bill ID"
// @Success      200  {object}  map[string]any  "bill/items"
// @Failure      400  {object}  map[string]string  "admin.bill.detail.invalid_input"
// @Failure      404  {object}  map[string]string  "admin.bill.detail.not_found"
// @Failure      500  {object}  map[string]string  "admin.bill.detail.internal"
// @Router       /admin/bill/detail [get]
func (h *MallBillHandler) Detail(c *gin.Context) {
	id, err := strconv.ParseInt(c.Query("id"), 10, 64)
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    "admin.bill.detail.invalid_input",
			"message": "invalid id",
		})
		return
	}
	bill, items, err := h.svc.Detail(c, id)
	if err != nil {
		writeBillErr(c, "admin.bill.detail", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"bill":  bill,
		"items": items,
	})
}

// Regenerate 处理 POST /admin/bill/regenerate。
//
// rejected → generated：金额整体快照更新 + version+1 + 清 reject_reason。
//
// @Summary      重核账单
// @Description  供应商驳回（rejected）后 admin 重核金额，账单回到 generated 待供应商确认。
// @Tags         admin
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        req  body  admin.billRegenerateRequest  true  "bill ID + 金额四件套 + remark"
// @Success      200  {object}  map[string]string  "admin.bill.regenerate.ok"
// @Failure      400  {object}  map[string]string  "admin.bill.regenerate.invalid_input"
// @Failure      404  {object}  map[string]string  "admin.bill.regenerate.not_found"
// @Failure      409  {object}  map[string]string  "admin.bill.regenerate.invalid_transition / status_conflict"
// @Failure      500  {object}  map[string]string  "admin.bill.regenerate.internal"
// @Router       /admin/bill/regenerate [post]
func (h *MallBillHandler) Regenerate(c *gin.Context) {
	var req billRegenerateRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.ID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    "admin.bill.regenerate.invalid_input",
			"message": "invalid request",
		})
		return
	}

	amounts := mallRepo.BillRegenAmounts{
		TotalAmount:      req.TotalAmount,
		CommissionRate:   req.CommissionRate,
		CommissionAmount: req.CommissionAmount,
		PayoutAmount:     req.PayoutAmount,
	}
	if err := h.svc.Regenerate(c, req.ID, amounts, req.Remark); err != nil {
		writeBillErr(c, "admin.bill.regenerate", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"code":    "admin.bill.regenerate.ok",
		"message": "ok",
	})
}

// MarkPaid 处理 POST /admin/bill/mark-paid。
//
// 复合事务：bill confirmed→paid CAS + supplier.balance -= payout + box
// sold_out→settled；余额不够 → 422（事务回滚，账单留在 confirmed）。
// adminID 来自 context（AdminAuth 写入的 *model.Admin）。
//
// @Summary      批次账单打款
// @Description  事务：bill confirmed→paid + 扣 supplier balance + 商品置 settled。
// @Tags         admin
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        req  body  admin.billMarkPaidRequest  true  "bill ID"
// @Success      200  {object}  map[string]string  "admin.bill.mark_paid.ok"
// @Failure      400  {object}  map[string]string  "admin.bill.mark_paid.invalid_input"
// @Failure      401  {object}  map[string]string  "admin.bill.mark_paid.unauthorized"
// @Failure      404  {object}  map[string]string  "admin.bill.mark_paid.not_found"
// @Failure      409  {object}  map[string]string  "admin.bill.mark_paid.invalid_transition / status_conflict / box_lifecycle_conflict"
// @Failure      422  {object}  map[string]string  "admin.bill.mark_paid.insufficient_balance"
// @Failure      500  {object}  map[string]string  "admin.bill.mark_paid.internal"
// @Router       /admin/bill/mark-paid [post]
func (h *MallBillHandler) MarkPaid(c *gin.Context) {
	var req billMarkPaidRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.ID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    "admin.bill.mark_paid.invalid_input",
			"message": "invalid request",
		})
		return
	}
	admin := middleware.AdminFromContext(c)
	if admin == nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"code":    "admin.bill.mark_paid.unauthorized",
			"message": "admin not found in context",
		})
		return
	}
	if err := h.svc.MarkPaid(c, req.ID, admin.ID); err != nil {
		writeBillErr(c, "admin.bill.mark_paid", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"code":    "admin.bill.mark_paid.ok",
		"message": "ok",
	})
}

// =============================================================================
// error mapping
// =============================================================================

// writeBillErr 把 mall.BillService 错误统一映射到 (status, code)。
//
// handler 只 import service/mall 包识别 sentinel（ErrBillNotFound 等 re-export 自
// repo 包）；state.ErrInvalidStateTransition 是状态机包的错误，直接识别。
func writeBillErr(c *gin.Context, codeNS string, err error) {
	switch {
	case errors.Is(err, mallSvc.ErrBillNotFound):
		c.JSON(http.StatusNotFound, gin.H{
			"code":    codeNS + ".not_found",
			"message": "bill not found",
		})
	case errors.Is(err, state.ErrInvalidStateTransition):
		c.JSON(http.StatusConflict, gin.H{
			"code":    codeNS + ".invalid_transition",
			"message": "bill state machine rejected the transition",
		})
	case errors.Is(err, mallSvc.ErrBillStatusConflict):
		c.JSON(http.StatusConflict, gin.H{
			"code":    codeNS + ".status_conflict",
			"message": "bill status changed concurrently",
		})
	case errors.Is(err, mallSvc.ErrBoxLifecycleConflict):
		c.JSON(http.StatusConflict, gin.H{
			"code":    codeNS + ".box_lifecycle_conflict",
			"message": "box lifecycle changed concurrently",
		})
	case errors.Is(err, mallSvc.ErrInsufficientSupplierBalance):
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"code":    codeNS + ".insufficient_balance",
			"message": "supplier balance insufficient for payout",
		})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    codeNS + ".internal",
			"message": "operation failed",
		})
	}
}
