// Package user — home.go 实现首页 Feed 路由（Feed）。
//
// 公开路由（不挂 UserAuth）：首页推荐对未登录访客可见。
//
// 错误映射：ES 不可用 → 500 + code=search_unavailable（spec 明确不 fallback
// MySQL；sentinel ErrFeedSearchUnavailable 由 service 层 errors.Join 包装，
// 这里 errors.Is 判定）。空结果回 [] 而非 null（Feed DTO 已保证，handler 透传）。
package user

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	userSvc "ai-go-mall/internal/service/user"
)

// HomeHandler 处理首页 Feed 路由（Feed）。
type HomeHandler struct {
	svc userSvc.FeedService
}

// NewHomeHandler 接收 FeedService，返回 *HomeHandler。
func NewHomeHandler(svc userSvc.FeedService) *HomeHandler {
	return &HomeHandler{svc: svc}
}

// Feed 处理 GET /user/home/feed。
//
// @Summary      首页 Feed
// @Description  ES 推荐流：featured 盲盒（hot_score DESC）+ featured 供应商（featured_rank ASC）。
// @Tags         user
// @Produce      json
// @Success      200  {object}  user.Feed  "blind_boxes / suppliers / fetched_at"
// @Failure      500  {object}  map[string]string  "search_unavailable"
// @Router       /user/home/feed [get]
func (h *HomeHandler) Feed(c *gin.Context) {
	feed, err := h.svc.Feed(c)
	if err != nil {
		if errors.Is(err, userSvc.ErrFeedSearchUnavailable) {
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":    "search_unavailable",
				"message": "feed search unavailable",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    "home.internal",
			"message": "get feed failed",
		})
		return
	}
	c.JSON(http.StatusOK, feed)
}
