// Package user — seckill.go 实现秒杀路由（浏览 List / Detail + 抽卡 DrawSeckill）。
//
// 路由挂载（router/user）：
//
//	GET  /user/seckill/list        公开   当前生效秒杀活动列表
//	GET  /user/seckill/detail      公开   秒杀活动详情（限购 + 实时剩余名额）
//	POST /user/seckill/:id/draw   中间件链 = UserAuth → RateLimitPerMinute("seckill:{uid}", 30)
//
// 路径参数 :id 是秒杀活动 ID（mall_seckill_activities.id），不是盲盒 ID。
// userID 仍取自 UserAuth 写入的 context。
//
// 错误码映射（D12 + D15 spec）：
//   - ErrUserLimitExceeded       → 429 seckill.user_limit_exceeded
//   - ErrSoldOut                 → 409 seckill.sold_out
//   - ErrInsufficientBalance     → 402 seckill.insufficient_balance
//   - ErrSeckillNotInWindow      → 403 seckill.not_in_window
//   - ErrSeckillNotAvailable     → 404 seckill.not_available（不存在 / 已禁用 / 未 init Redis）
//   - ErrBlindBoxNotAvailable    → 404 seckill.blindbox_not_available（秒杀关联盲盒已下架）
//   - ErrUserNotAvailable        → 403 seckill.user_not_available
//   - 其他                        → 500 seckill.internal
package user

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"ai-go-mall/internal/middleware"
	userSvc "ai-go-mall/internal/service/user"
)

// SeckillHandler 处理秒杀路由（浏览 + 抽卡）。
type SeckillHandler struct {
	svc userSvc.SeckillService

	// browse 浏览查询（List / Detail）；nil 仅在旧单测直接构造时出现，路由装配必传。
	browse userSvc.SeckillBrowseService
}

// NewSeckillHandler 接收抽卡与浏览两个 service，返回 *SeckillHandler。
func NewSeckillHandler(svc userSvc.SeckillService, browse userSvc.SeckillBrowseService) *SeckillHandler {
	return &SeckillHandler{svc: svc, browse: browse}
}

// SeckillResponse 是秒杀抽卡成功后的返回：订单号 + 实付价 + 抽中的卡。
//
// 字段形态与 DrawResponse 一致（共用 drawCardResponse 投影类型），
// 仅 JSON tag 同名同含义——前端无需关心是普通抽卡还是秒杀。
type SeckillResponse struct {
	OrderID     int64              `json:"order_id"`
	OrderNo     string             `json:"order_no"`
	ActualPrice float64            `json:"actual_price"`
	Cards       []drawCardResponse `json:"cards"`
}

// List 处理 GET /user/seckill/list（公开路由，浏览不要求登录）。
//
// 返回当前生效的秒杀活动（status=active + 时间窗内 + 盲盒在售 + 供应商未禁用），
// 分页参数与盲盒列表同构：?page=（默认 1）&page_size=（默认 20，上限 200）。
//
// @Summary      秒杀活动列表
// @Description  当前生效的秒杀活动分页列表（含实时剩余名额，-1 = 暂不可知）。
// @Tags         user
// @Produce      json
// @Param        page       query  int  false  "页码，默认 1"
// @Param        page_size  query  int  false  "每页条数，默认 20，上限 200"
// @Success      200  {object}  map[string]any  "items/total/page/page_size"
// @Failure      500  {object}  map[string]string  "seckill.internal"
// @Router       /user/seckill/list [get]
func (h *SeckillHandler) List(c *gin.Context) {
	opts := parseUserListOptions(c)

	items, total, err := h.browse.List(c, opts)
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

// Detail 处理 GET /user/seckill/detail?id=（公开路由）。
//
// 不存在 / 已禁用 / 关联盲盒或供应商不可购 → 404 seckill.not_available。
//
// @Summary      秒杀活动详情
// @Description  含限购规则、时间窗与实时剩余名额。
// @Tags         user
// @Produce      json
// @Param        id  query  int  true  "秒杀活动 ID"
// @Success      200  {object}  user.SeckillDetail  "详情"
// @Failure      400  {object}  map[string]string  "seckill.invalid_input"
// @Failure      404  {object}  map[string]string  "seckill.not_available"
// @Failure      500  {object}  map[string]string  "seckill.internal"
// @Router       /user/seckill/detail [get]
func (h *SeckillHandler) Detail(c *gin.Context) {
	id, err := parseUserPathID(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    "seckill.invalid_input",
			"message": "invalid seckill id",
		})
		return
	}

	detail, err := h.browse.Detail(c, id)
	if err != nil {
		if errors.Is(err, userSvc.ErrSeckillNotAvailable) {
			c.JSON(http.StatusNotFound, gin.H{
				"code":    "seckill.not_available",
				"message": "seckill activity not available",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    "seckill.internal",
			"message": "get seckill detail failed",
		})
		return
	}
	c.JSON(http.StatusOK, detail)
}

// Draw 处理 POST /user/seckill/:id/draw。
//
// :id 是 mall_seckill_activities.id；userID 来自 UserAuth 中间件写入的
// middleware.UserFromContext(c)（兜底 401）。
//
// 限流：30 次/分钟（D15 没指定具体数字，按普通抽卡的 N=30 复用）。
//
// @Summary      秒杀抽卡
// @Description  Redis 限购 + 库存预扣 + MySQL 事务（D3 抽卡 + 写 stock_deduction_log + 结算副作用）；失败回滚 Redis。
// @Tags         user
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id  path  int  true  "秒杀活动 ID"
// @Success      200  {object}  user.SeckillResponse  "秒杀结果"
// @Failure      400  {object}  map[string]string  "seckill.invalid_input"
// @Failure      401  {object}  map[string]string  "seckill.unauthorized"
// @Failure      402  {object}  map[string]string  "seckill.insufficient_balance"
// @Failure      403  {object}  map[string]string  "seckill.not_in_window / seckill.user_not_available"
// @Failure      404  {object}  map[string]string  "seckill.not_available / seckill.blindbox_not_available"
// @Failure      409  {object}  map[string]string  "seckill.sold_out"
// @Failure      429  {object}  map[string]string  "seckill.user_limit_exceeded（限流中间件或业务）"
// @Failure      500  {object}  map[string]string  "seckill.internal"
// @Router       /user/seckill/{id}/draw [post]
func (h *SeckillHandler) Draw(c *gin.Context) {
	u := middleware.UserFromContext(c)
	if u == nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"code":    "seckill.unauthorized",
			"message": "login required",
		})
		return
	}

	idStr := c.Param("id")
	seckillID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || seckillID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    "seckill.invalid_input",
			"message": "invalid seckill id",
		})
		return
	}

	result, err := h.svc.DrawSeckill(c, u.ID, seckillID)
	if err != nil {
		status, code := mapSeckillErr(err)
		c.JSON(status, gin.H{
			"code":    code,
			"message": "seckill draw failed",
		})
		return
	}

	c.JSON(http.StatusOK, toSeckillResponse(result))
}

// mapSeckillErr 把 service sentinel 映射到 (HTTP status, 业务码)。
func mapSeckillErr(err error) (int, string) {
	switch {
	case errors.Is(err, userSvc.ErrUserLimitExceeded):
		return http.StatusTooManyRequests, "seckill.user_limit_exceeded"
	case errors.Is(err, userSvc.ErrSoldOut):
		return http.StatusConflict, "seckill.sold_out"
	case errors.Is(err, userSvc.ErrInsufficientBalance):
		return http.StatusPaymentRequired, "seckill.insufficient_balance"
	case errors.Is(err, userSvc.ErrSeckillNotInWindow):
		return http.StatusForbidden, "seckill.not_in_window"
	case errors.Is(err, userSvc.ErrSeckillNotAvailable):
		return http.StatusNotFound, "seckill.not_available"
	case errors.Is(err, userSvc.ErrBlindBoxNotAvailable):
		return http.StatusNotFound, "seckill.blindbox_not_available"
	case errors.Is(err, userSvc.ErrUserNotAvailable):
		return http.StatusForbidden, "seckill.user_not_available"
	default:
		return http.StatusInternalServerError, "seckill.internal"
	}
}

// toSeckillResponse 把 service 结果投影成对外 DTO。
//
// 共用 draw.go 的 drawCardResponse：秒杀和普通抽卡的 cards 字段语义相同，
// 前端只关心 card_id / rarity / snapshot_* 即可。
func toSeckillResponse(r *userSvc.DrawResult) SeckillResponse {
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
	return SeckillResponse{
		OrderID:     r.OrderID,
		OrderNo:     r.OrderNo,
		ActualPrice: r.ActualPrice,
		Cards:       cards,
	}
}