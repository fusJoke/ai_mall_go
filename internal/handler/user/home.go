// Package user — home.go 实现首页 Feed 路由（Feed）。
//
// 公开路由（不挂 UserAuth）：首页推荐对未登录访客可见。
//
// 错误映射：ES 与 MySQL 降级查询**均失败** → 500 + code=search_unavailable
// （sentinel ErrFeedSearchUnavailable 由 service 层 errors.Join 包装，
// 这里 errors.Is 判定）。空结果回 [] 而非 null（Feed DTO 已保证，handler 透传）。
//
// 降级可观测性：响应携带 X-Feed-Source 头（es / mysql），仅供排障与灰度观察；
// 客户端 MUST NOT 依赖它做渲染分支（spec Requirement "降级可观测性"）。
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
// @Description  ES 推荐流：在售盲盒（hot_score DESC）+ featured 供应商（featured_rank ASC）。ES 不可用时自动降级 MySQL，响应头 X-Feed-Source 标识来源。
// @Tags         user
// @Produce      json
// @Success      200  {object}  user.Feed  "blind_boxes / suppliers / fetched_at / source"
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

	// 数据来源响应头。service 层保证两路径都填 Source；空值兜底为 es
	// （缓存项由 source 字段上线前的版本写入时可能出现空串）。
	source := feed.Source
	if source == "" {
		source = userSvc.FeedSourceES
	}
	c.Header(userSvc.FeedSourceHeader, source)

	c.JSON(http.StatusOK, feed)
}
