// Package user — draw.go 实现抽卡路由（Draw）。
//
// 路由挂载（router/user）：
//
//	POST /user/blindbox/draw   中间件链 = UserAuth → RateLimitPerMinute("draw:{uid}", 30)
//
// 限流挂在 UserAuth 之后（uidExtractor 从 context 拿 user.ID；拿不到放行，
// 由 UserAuth 自己 401）。N=30/分钟来自设计 D8。
//
// 错误码映射（D12；HTTP 状态码对齐 spec 各 Scenario）：
//   - ErrSoldOut            → 409 draw.sold_out
//   - ErrInsufficientBalance → 402 draw.insufficient_balance
//   - ErrBlindBoxNotAvailable → 404 draw.blindbox_not_available（含下架/禁用，模糊存在性）
//   - ErrUserNotAvailable   → 403 draw.user_not_available
//   - ErrPoolNotFound / ErrEmptyPool / 其他 → 500 draw.internal
//     （卡池缺失/空池是数据异常，不是用户可恢复的业务态）
package user

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"ai-go-mall/internal/middleware"
	userSvc "ai-go-mall/internal/service/user"
)

// DrawHandler 处理抽卡路由（Draw）。
type DrawHandler struct {
	svc userSvc.DrawService
}

// NewDrawHandler 接收 DrawService，返回 *DrawHandler。
func NewDrawHandler(svc userSvc.DrawService) *DrawHandler {
	return &DrawHandler{svc: svc}
}

// DrawRequest 是 POST /user/blindbox/draw 的请求体。
type DrawRequest struct {
	// BlindBoxID 要抽的盲盒 ID。
	BlindBoxID int64 `json:"blind_box_id" binding:"required"`
}

// drawCardResponse 是抽中单卡的结果（snake_case，供前端直接渲染）。
type drawCardResponse struct {
	ItemID        int64  `json:"item_id"`
	CardID        int64  `json:"card_id"`
	Rarity        string `json:"rarity"`
	SnapshotName  string `json:"snapshot_name"`
	SnapshotImage string `json:"snapshot_image"`
}

// DrawResponse 是抽卡成功后的返回：订单号 + 实付价 + 抽中的卡。
type DrawResponse struct {
	OrderID     int64              `json:"order_id"`
	OrderNo     string             `json:"order_no"`
	ActualPrice float64            `json:"actual_price"`
	Cards       []drawCardResponse `json:"cards"`
}

// Draw 处理 POST /user/blindbox/draw。
//
// userID 取自 UserAuth 写入的 context（middleware.UserFromContext）；
// context 无用户（理论上中间件已 401，防御性兜底）→ 401。
//
// @Summary      抽卡
// @Description  事务内扣卡池库存 + 扣余额 + 落订单并返回抽中的卡；需 user token；每用户每分钟限 30 次。
// @Tags         user
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        req  body  user.DrawRequest  true  "盲盒 ID"
// @Success      200  {object}  user.DrawResponse  "抽卡结果"
// @Failure      400  {object}  map[string]string  "draw.invalid_input"
// @Failure      401  {object}  map[string]string  "draw.unauthorized"
// @Failure      402  {object}  map[string]string  "draw.insufficient_balance"
// @Failure      403  {object}  map[string]string  "draw.user_not_available"
// @Failure      404  {object}  map[string]string  "draw.blindbox_not_available"
// @Failure      409  {object}  map[string]string  "draw.sold_out"
// @Failure      429  {object}  map[string]string  "rate_limited（限流中间件返回）"
// @Failure      500  {object}  map[string]string  "draw.internal"
// @Router       /user/blindbox/draw [post]
func (h *DrawHandler) Draw(c *gin.Context) {
	u := middleware.UserFromContext(c)
	if u == nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"code":    "draw.unauthorized",
			"message": "login required",
		})
		return
	}

	var req DrawRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.BlindBoxID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    "draw.invalid_input",
			"message": "invalid draw request",
		})
		return
	}

	result, err := h.svc.Draw(c, u.ID, req.BlindBoxID)
	if err != nil {
		status, code := mapDrawErr(err)
		c.JSON(status, gin.H{
			"code":    code,
			"message": "draw failed",
		})
		return
	}

	c.JSON(http.StatusOK, toDrawResponse(result))
}

// mapDrawErr 把 service sentinel 映射到 (HTTP status, 业务码)。
func mapDrawErr(err error) (int, string) {
	switch {
	case errors.Is(err, userSvc.ErrSoldOut):
		return http.StatusConflict, "draw.sold_out"
	case errors.Is(err, userSvc.ErrInsufficientBalance):
		return http.StatusPaymentRequired, "draw.insufficient_balance"
	case errors.Is(err, userSvc.ErrBlindBoxNotAvailable):
		return http.StatusNotFound, "draw.blindbox_not_available"
	case errors.Is(err, userSvc.ErrUserNotAvailable):
		return http.StatusForbidden, "draw.user_not_available"
	default:
		return http.StatusInternalServerError, "draw.internal"
	}
}

// toDrawResponse 把 service 结果投影成对外 DTO。
// Cards 为空时回空数组（保证 JSON 是 [] 而非 null，前端渲染省一次判空）。
func toDrawResponse(r *userSvc.DrawResult) DrawResponse {
	cards := make([]drawCardResponse, 0, len(r.Cards))
	for _, card := range r.Cards {
		cards = append(cards, drawCardResponse{
			ItemID:        card.ItemID,
			CardID:        card.CardID,
			Rarity:        string(card.Rarity),
			SnapshotName:  card.SnapshotName,
			SnapshotImage: card.SnapshotImage,
		})
	}
	return DrawResponse{
		OrderID:     r.OrderID,
		OrderNo:     r.OrderNo,
		ActualPrice: r.ActualPrice,
		Cards:       cards,
	}
}
