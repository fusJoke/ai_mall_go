// Package user — follow.go 实现 C 端关注供应商路由（Follow / Unfollow / ListFollowing）。
//
// 路由挂载（router/user）：
//
//	POST   /user/follow             登录   关注供应商（幂等 upsert）
//	DELETE /user/follow             登录   取关供应商（幂等）
//	GET    /user/follow/following   登录   我的关注分页列表
//
// 鉴权：全部走 UserAuth，handler 从 context 拿当前 user.ID 作为 follow.user_id。
// supplier_id 来源：POST/DELETE 走 ?supplier_id= 查询参数（与 /blindbox/detail?id=
// 同构，避免 body 解析 + DELETE 少有 body 解析器兼容陷阱）。
//
// 错误码映射（D12；HTTP 状态码对齐 spec 各 Scenario）：
//   - supplier_id 缺失 / 非法 / 非正数 → 400 follow.invalid_id
//   - 其余 service 错误 → 500 follow.internal（不透传 err.Error()）
package user

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"ai-go-mall/internal/middleware"
	userSvc "ai-go-mall/internal/service/user"
)

// FollowHandler 处理 C 端关注供应商路由（Follow / Unfollow / ListFollowing）。
//
// 仅持有 user.FollowService 接口；装配由 router/user 完成。
type FollowHandler struct {
	svc userSvc.FollowService
}

// NewFollowHandler 接收 FollowService，返回 *FollowHandler。
func NewFollowHandler(svc userSvc.FollowService) *FollowHandler {
	return &FollowHandler{svc: svc}
}

// Follow 处理 POST /user/follow?supplier_id=。
//
// 幂等 upsert：重复关注 / 取关后再次关注均不报错。
//
// @Summary      关注供应商
// @Description  幂等 upsert；user_id 取自登录态。
// @Tags         user
// @Produce      json
// @Security     BearerAuth
// @Param        supplier_id  query  int  true  "供应商 ID"
// @Success      200  {object}  map[string]string  "ok"
// @Failure      400  {object}  map[string]string  "follow.invalid_id"
// @Failure      401  {object}  map[string]string  "follow.unauthorized"
// @Failure      500  {object}  map[string]string  "follow.internal"
// @Router       /user/follow [post]
func (h *FollowHandler) Follow(c *gin.Context) {
	u := middleware.UserFromContext(c)
	if u == nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"code":    "follow.unauthorized",
			"message": "login required",
		})
		return
	}
	supplierID, err := parseSupplierID(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    "follow.invalid_id",
			"message": "invalid supplier id",
		})
		return
	}
	if err := h.svc.Follow(c, u.ID, supplierID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    "follow.internal",
			"message": "follow supplier failed",
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "ok"})
}

// Unfollow 处理 DELETE /user/follow?supplier_id=。
//
// 幂等：未关注 / 重复取关均为 no-op。
//
// @Summary      取关供应商
// @Description  幂等软删；user_id 取自登录态。
// @Tags         user
// @Produce      json
// @Security     BearerAuth
// @Param        supplier_id  query  int  true  "供应商 ID"
// @Success      200  {object}  map[string]string  "ok"
// @Failure      400  {object}  map[string]string  "follow.invalid_id"
// @Failure      401  {object}  map[string]string  "follow.unauthorized"
// @Failure      500  {object}  map[string]string  "follow.internal"
// @Router       /user/follow [delete]
func (h *FollowHandler) Unfollow(c *gin.Context) {
	u := middleware.UserFromContext(c)
	if u == nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"code":    "follow.unauthorized",
			"message": "login required",
		})
		return
	}
	supplierID, err := parseSupplierID(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    "follow.invalid_id",
			"message": "invalid supplier id",
		})
		return
	}
	if err := h.svc.Unfollow(c, u.ID, supplierID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    "follow.internal",
			"message": "unfollow supplier failed",
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "ok"})
}

// ListFollowing 处理 GET /user/follow/following。
//
// 分页参数与 BlindBoxHandler.List 同构：?page=&page_size=。
// 返回 {items, total, page, page_size}。
//
// @Summary      我的关注
// @Description  当前用户关注的供应商分页列表。
// @Tags         user
// @Produce      json
// @Security     BearerAuth
// @Param        page       query  int  false  "页码，默认 1"
// @Param        page_size  query  int  false  "每页条数，默认 20，上限 200"
// @Success      200  {object}  map[string]any  "items/total/page/page_size"
// @Failure      401  {object}  map[string]string  "follow.unauthorized"
// @Failure      500  {object}  map[string]string  "follow.internal"
// @Router       /user/follow/following [get]
func (h *FollowHandler) ListFollowing(c *gin.Context) {
	u := middleware.UserFromContext(c)
	if u == nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"code":    "follow.unauthorized",
			"message": "login required",
		})
		return
	}
	opts := parseUserListOptions(c)
	items, total, err := h.svc.ListFollowing(c, u.ID, opts)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    "follow.internal",
			"message": "list following failed",
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

// parseSupplierID 从 ?supplier_id= 解析正整数；缺失 / 非法 / 非正数都返回 error。
func parseSupplierID(c *gin.Context) (int64, error) {
	raw := c.Query("supplier_id")
	if raw == "" {
		return 0, errInvalidSupplierID
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return 0, errInvalidSupplierID
	}
	return id, nil
}

// errInvalidSupplierID 仅作为 parseSupplierID 的哨兵；handler 不做 errors.Is 判断，
// 一律映射 400 follow.invalid_id。
var errInvalidSupplierID = &invalidSupplierIDError{}

type invalidSupplierIDError struct{}

func (*invalidSupplierIDError) Error() string { return "invalid supplier id" }