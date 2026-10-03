// Package admin — settlement.go 实现 admin 对结算单的管理路由
// （List / Detail / Preview / Generate / MarkPaid）。
//
// 错误码形如 admin.settlement.<action>.<result>（namespace.action.result）。
//
// 全部端点挂 AdminAuth（router/admin）。
package admin

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"ai-go-mall/internal/domain/state"
	"ai-go-mall/internal/middleware"
	"ai-go-mall/internal/repository"
	adminSvc "ai-go-mall/internal/service/admin"
)

// =============================================================================
// MallSettlementHandler
// =============================================================================

// MallSettlementHandler 处理 admin 的结算单管理路由。
//
// 5 个端点对应 service 层 5 个方法：
//   - List       GET  /admin/settlement/list
//   - Detail     GET  /admin/settlement/detail?id=N
//   - Preview    POST /admin/settlement/preview
//   - Generate   POST /admin/settlement/generate
//   - MarkPaid   POST /admin/settlement/mark-paid
type MallSettlementHandler struct {
	svc adminSvc.SettlementService
}

// NewMallSettlementHandler 接收 admin.SettlementService，返回 *MallSettlementHandler。
func NewMallSettlementHandler(svc adminSvc.SettlementService) *MallSettlementHandler {
	return &MallSettlementHandler{svc: svc}
}

// List 处理 GET /admin/settlement/list（全量结算单，按 ID DESC）。
//
// @Summary      结算单列表
// @Description  admin 视角的全量结算单分页列表（按 ID DESC）。
// @Tags         admin
// @Produce      json
// @Security     BearerAuth
// @Param        page       query  int  false  "页码"
// @Param        page_size  query  int  false  "每页条数"
// @Success      200  {object}  map[string]any  "items/total/page/page_size"
// @Failure      500  {object}  map[string]string  "admin.settlement.list.internal"
// @Router       /admin/settlement/list [get]
func (h *MallSettlementHandler) List(c *gin.Context) {
	opts := repository.ListOptions{
		Page:     mallAdminPage(c),
		PageSize: mallAdminPageSize(c),
	}
	items, total, err := h.svc.List(c, opts)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    "admin.settlement.list.internal",
			"message": "list settlements failed",
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

// Detail 处理 GET /admin/settlement/detail?id=N。
//
// @Summary      结算单详情
// @Description  返回 settlement + items 列表（详情 / 行展开）。
// @Tags         admin
// @Produce      json
// @Security     BearerAuth
// @Param        id  query  int  true  "settlement ID"
// @Success      200  {object}  map[string]any  "settlement + items"
// @Failure      400  {object}  map[string]string  "admin.settlement.detail.invalid_input"
// @Failure      404  {object}  map[string]string  "admin.settlement.detail.not_found"
// @Failure      500  {object}  map[string]string  "admin.settlement.detail.internal"
// @Router       /admin/settlement/detail [get]
func (h *MallSettlementHandler) Detail(c *gin.Context) {
	id, err := strconv.ParseInt(c.Query("id"), 10, 64)
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    "admin.settlement.detail.invalid_input",
			"message": "invalid id",
		})
		return
	}
	settlement, items, err := h.svc.Detail(c, id)
	if err != nil {
		writeSettlementErr(c, "admin.settlement.detail", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"settlement": settlement,
		"items":      items,
	})
}

// settlementPeriodRequest 是 Preview / Generate 的请求体（共用结构）。
//
// period_start / period_end 用 RFC3339 字符串（time.Time 自动解析）；
// supplier_id 必填且 > 0；period_end 必须 > period_start（service 层再校验兜底）。
type settlementPeriodRequest struct {
	SupplierID  int64  `json:"supplier_id" binding:"required"`
	PeriodStart string `json:"period_start" binding:"required"`
	PeriodEnd   string `json:"period_end" binding:"required"`
}

// parseSettlementPeriod 解析 Preview/Generate 请求 + 转 periods 边界。
//
// 任意字段非法 → (nil, 400 code string)；handler 拿到 false 直接写 400。
func parseSettlementPeriod(c *gin.Context, codeNS string) (supplierID int64, start, end time.Time, ok bool) {
	var req settlementPeriodRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    codeNS + ".invalid_input",
			"message": "invalid request",
		})
		return 0, time.Time{}, time.Time{}, false
	}
	if req.SupplierID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    codeNS + ".invalid_input",
			"message": "invalid supplier_id",
		})
		return 0, time.Time{}, time.Time{}, false
	}
	start, err1 := time.Parse(time.RFC3339, req.PeriodStart)
	end, err2 := time.Parse(time.RFC3339, req.PeriodEnd)
	if err1 != nil || err2 != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    codeNS + ".invalid_input",
			"message": "invalid period format (RFC3339 required)",
		})
		return 0, time.Time{}, time.Time{}, false
	}
	return req.SupplierID, start, end, true
}

// Preview 处理 POST /admin/settlement/preview。
//
// 不写库，仅返回预览：supplier + period 内可结算订单的总额 / 佣金 / 打款额。
//
// @Summary      结算预览
// @Description  不写库，列某 supplier + 周期内可结算订单 + 算金额，供 admin 决策。
// @Tags         admin
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        req  body  admin.settlementPeriodRequest  true  "supplier_id + period_start + period_end (RFC3339)"
// @Success      200  {object}  admin.PreviewSettlement  "preview payload"
// @Failure      400  {object}  map[string]string  "admin.settlement.preview.invalid_input"
// @Failure      404  {object}  map[string]string  "admin.settlement.preview.supplier_not_found"
// @Failure      422  {object}  map[string]string  "admin.settlement.preview.invalid_period"
// @Failure      500  {object}  map[string]string  "admin.settlement.preview.internal"
// @Router       /admin/settlement/preview [post]
func (h *MallSettlementHandler) Preview(c *gin.Context) {
	supplierID, start, end, ok := parseSettlementPeriod(c, "admin.settlement.preview")
	if !ok {
		return
	}
	preview, err := h.svc.Preview(c, supplierID, start, end)
	if err != nil {
		writeSettlementErr(c, "admin.settlement.preview", err)
		return
	}
	c.JSON(http.StatusOK, preview)
}

// Generate 处理 POST /admin/settlement/generate。
//
// 事务：列 eligible orders + 算金额 + INSERT settlement + items。
// UK (supplier_id + period_start + period_end) 冲突 → 409。
//
// @Summary      生成结算单
// @Description  事务：列某 supplier + 周期内 drawn 且未结算订单 → INSERT settlement + items。
// @Tags         admin
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        req  body  admin.settlementPeriodRequest  true  "supplier_id + period_start + period_end (RFC3339)"
// @Success      200  {object}  map[string]any  "settlement + code/message"
// @Failure      400  {object}  map[string]string  "admin.settlement.generate.invalid_input"
// @Failure      404  {object}  map[string]string  "admin.settlement.generate.supplier_not_found"
// @Failure      409  {object}  map[string]string  "admin.settlement.generate.conflict"
// @Failure      422  {object}  map[string]string  "admin.settlement.generate.invalid_period / no_orders"
// @Failure      500  {object}  map[string]string  "admin.settlement.generate.internal"
// @Router       /admin/settlement/generate [post]
func (h *MallSettlementHandler) Generate(c *gin.Context) {
	supplierID, start, end, ok := parseSettlementPeriod(c, "admin.settlement.generate")
	if !ok {
		return
	}
	settlement, err := h.svc.Generate(c, supplierID, start, end)
	if err != nil {
		writeSettlementErr(c, "admin.settlement.generate", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"code":       "admin.settlement.generate.ok",
		"message":    "settlement generated",
		"settlement": settlement,
	})
}

// MarkPaid 处理 POST /admin/settlement/mark-paid。
//
// 事务：标 settlement paid + 扣 supplier balance。
// 二次 mark_paid → 409；supplier.balance < payout → 422。
// adminID 来自 context（AdminAuth 写入的 *model.Admin）。
//
// @Summary      标记结算单已打款
// @Description  事务：UPDATE settlement status='paid' + UPDATE supplier balance。
// @Tags         admin
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        req  body  admin.mallSettlementIDRequest  true  "settlement ID"
// @Success      200  {object}  map[string]string  "admin.settlement.mark_paid.ok"
// @Failure      400  {object}  map[string]string  "admin.settlement.mark_paid.invalid_input"
// @Failure      404  {object}  map[string]string  "admin.settlement.mark_paid.not_found"
// @Failure      409  {object}  map[string]string  "admin.settlement.mark_paid.already_paid"
// @Failure      422  {object}  map[string]string  "admin.settlement.mark_paid.insufficient_balance"
// @Failure      500  {object}  map[string]string  "admin.settlement.mark_paid.internal"
// @Router       /admin/settlement/mark-paid [post]
func (h *MallSettlementHandler) MarkPaid(c *gin.Context) {
	var req mallSettlementIDRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.ID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    "admin.settlement.mark_paid.invalid_input",
			"message": "invalid request",
		})
		return
	}
	admin := middleware.AdminFromContext(c)
	if admin == nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"code":    "admin.settlement.mark_paid.unauthorized",
			"message": "admin not found in context",
		})
		return
	}
	if err := h.svc.MarkPaid(c, req.ID, admin.ID); err != nil {
		writeSettlementErr(c, "admin.settlement.mark_paid", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"code":    "admin.settlement.mark_paid.ok",
		"message": "ok",
	})
}

// mallSettlementIDRequest 是 MarkPaid 的请求体。
type mallSettlementIDRequest struct {
	ID int64 `json:"id" binding:"required"`
}

// =============================================================================
// error mapping
// =============================================================================

// writeSettlementErr 把 service 层错误统一映射到 (status, code)。
//
// 设计：handler 只 import service 包（service 已 re-export repo 同名 var），
// 避免跨 import 拿不到哨兵；额外识别 ErrInvalidPeriod / ErrNoOrdersToSettle
// 这两个 service-only 哨兵。
func writeSettlementErr(c *gin.Context, codeNS string, err error) {
	switch {
	case errors.Is(err, adminSvc.ErrSettlementNotFound):
		c.JSON(http.StatusNotFound, gin.H{
			"code":    codeNS + ".not_found",
			"message": "settlement not found",
		})
	case errors.Is(err, adminSvc.ErrSupplierNotFound):
		c.JSON(http.StatusNotFound, gin.H{
			"code":    codeNS + ".supplier_not_found",
			"message": "supplier not found",
		})
	case errors.Is(err, adminSvc.ErrSettlementConflict):
		c.JSON(http.StatusConflict, gin.H{
			"code":    codeNS + ".conflict",
			"message": "settlement already exists for this supplier + period",
		})
	case errors.Is(err, adminSvc.ErrSettlementAlreadyPaid):
		c.JSON(http.StatusConflict, gin.H{
			"code":    codeNS + ".already_paid",
			"message": "settlement already paid",
		})
	case errors.Is(err, state.ErrInvalidStateTransition):
		// spec 14.7：状态机拒绝非法转换 → HTTP 409。
		// 触发场景：MarkPaid 时 settlement.status ∈ {pending, paid, failed}；
		// 或管理员在错误状态下尝试操作。状态机在「写库前」拦截，错误名固定。
		c.JSON(http.StatusConflict, gin.H{
			"code":    codeNS + ".invalid_state_transition",
			"message": "settlement state machine rejected the transition",
		})
	case errors.Is(err, adminSvc.ErrInsufficientSupplierBalance):
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"code":    codeNS + ".insufficient_balance",
			"message": "supplier balance insufficient for payout",
		})
	case errors.Is(err, adminSvc.ErrNoOrdersToSettle):
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"code":    codeNS + ".no_orders",
			"message": "no orders to settle in this period",
		})
	case errors.Is(err, adminSvc.ErrInvalidPeriod):
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"code":    codeNS + ".invalid_period",
			"message": "period_end must be after period_start",
		})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    codeNS + ".internal",
			"message": "operation failed",
		})
	}
}