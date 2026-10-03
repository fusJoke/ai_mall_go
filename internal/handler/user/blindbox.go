// Package user — blindbox.go 实现 C 端盲盒浏览路由（List / Detail）。
//
// 两个端点都是公开路由（不挂 UserAuth）：浏览盲盒不要求登录，
// 登录态仅在抽卡 / 看订单时由对应 handler 要求（spec 主流程：
// 用户可先逛首页与详情，抽卡时才需要账号）。
//
// 错误码映射（D12：sentinel → HTTP 集中在 handler 层）：
//   - ErrBlindBoxNotFound → 404 blindbox.not_found
//     （不存在 / 已下架 / 供应商被禁用统一 404，不暴露下架原因，
//     与 service 层注释的「模糊存在性」策略一致）；
//   - 其余 service 错误 → 500 blindbox.internal（不透传 err.Error()）。
package user

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"ai-go-mall/internal/model/mall"
	"ai-go-mall/internal/repository"
	userSvc "ai-go-mall/internal/service/user"
)

// BlindBoxHandler 处理 C 端盲盒浏览路由（List / Detail）。
//
// 仅持有 user.BlindBoxService 接口；装配由 router/user 完成。
type BlindBoxHandler struct {
	svc userSvc.BlindBoxService
}

// NewBlindBoxHandler 接收 BlindBoxService，返回 *BlindBoxHandler。
func NewBlindBoxHandler(svc userSvc.BlindBoxService) *BlindBoxHandler {
	return &BlindBoxHandler{svc: svc}
}

// List 处理 GET /user/blindbox/list。
//
// 分页参数与 BaseHandler.List 同构：?page=（默认 1）&page_size=（默认 20，上限 200）。
// 返回 {items, total, page, page_size}；service 层已做「active + on_sale +
// 供应商未禁用」三条件过滤，handler 不重复业务判断。
//
// @Summary      盲盒列表
// @Description  C 端可见的盲盒分页列表（已过滤下架与被禁供应商）。
// @Tags         user
// @Produce      json
// @Param        page       query  int  false  "页码，默认 1"
// @Param        page_size  query  int  false  "每页条数，默认 20，上限 200"
// @Success      200  {object}  map[string]any  "items/total/page/page_size"
// @Failure      500  {object}  map[string]string  "blindbox.internal"
// @Router       /user/blindbox/list [get]
func (h *BlindBoxHandler) List(c *gin.Context) {
	opts := parseUserListOptions(c)

	items, total, err := h.svc.List(c, opts)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    "blindbox.internal",
			"message": "list blind boxes failed",
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

// Detail 处理 GET /user/blindbox/detail?id=。
//
// id 缺失 / 非法 / 非正数 → 400 blindbox.invalid_id；
// 盲盒不可见（含下架 / 供应商被禁）→ 404 blindbox.not_found；
// 成功 → 200 + BlindBoxDetail（snake_case JSON，含卡池公示 items 与当前活动）。
//
// @Summary      盲盒详情
// @Description  含供应商 / 卡池 items（概率公示）/ 当前限时特价与实际售价。
// @Tags         user
// @Produce      json
// @Param        id  query  int  true  "盲盒 ID"
// @Success      200  {object}  user.BlindBoxDetail  "详情聚合"
// @Failure      400  {object}  map[string]string  "blindbox.invalid_id"
// @Failure      404  {object}  map[string]string  "blindbox.not_found"
// @Failure      500  {object}  map[string]string  "blindbox.internal"
// @Router       /user/blindbox/detail [get]
func (h *BlindBoxHandler) Detail(c *gin.Context) {
	id, err := parseUserPathID(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    "blindbox.invalid_id",
			"message": "invalid blind box id",
		})
		return
	}

	detail, err := h.svc.Detail(c, id)
	if err != nil {
		if errors.Is(err, userSvc.ErrBlindBoxNotFound) {
			c.JSON(http.StatusNotFound, gin.H{
				"code":    "blindbox.not_found",
				"message": "blind box not found",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    "blindbox.internal",
			"message": "get blind box detail failed",
		})
		return
	}
	c.JSON(http.StatusOK, toDetailResponse(detail))
}

// supplierPublicView 是 C 端详情里的供应商白名单投影（final review Important #6）。
//
// MallSupplier 整体序列化会把 balance / total_sales / commission_rate 等
// 商业敏感字段吐给任意访客（admin 列表页需要它们，但 C 端不需要）——
// 白名单显式列出详情页头部要展示的字段，新增敏感字段默认不外泄。
type supplierPublicView struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	Logo string `json:"logo"`
	Bio  string `json:"bio"`
}

// blindBoxDetailResponse 是 Detail 的对外响应形态：与 service.BlindBoxDetail
// 同构，仅 Supplier 换成公开投影。
type blindBoxDetailResponse struct {
	BlindBox        mall.MallBlindBox       `json:"blind_box"`
	Supplier        supplierPublicView      `json:"supplier"`
	Pool            mall.MallCardPool       `json:"pool"`
	Items           []mall.MallCardPoolItem `json:"items"`
	PoolTotalStock  int64                   `json:"pool_total_stock"`
	ActivePromotion *mall.MallPromotion     `json:"active_promotion,omitempty"`
	EffectivePrice  float64                 `json:"effective_price"`
	CachedAt        time.Time               `json:"cached_at"`
}

// toDetailResponse 把 service 聚合投影成对外 DTO。
func toDetailResponse(d *userSvc.BlindBoxDetail) blindBoxDetailResponse {
	return blindBoxDetailResponse{
		BlindBox:        d.BlindBox,
		Supplier:        supplierPublicView{ID: d.Supplier.ID, Name: d.Supplier.Name, Logo: d.Supplier.Logo, Bio: d.Supplier.Bio},
		Pool:            d.Pool,
		Items:           d.Items,
		PoolTotalStock:  d.PoolTotalStock,
		ActivePromotion: d.ActivePromotion,
		EffectivePrice:  d.EffectivePrice,
		CachedAt:        d.CachedAt,
	}
}

// parseUserListOptions 是 handler/user 包内的分页参数解析。
//
// 与 handler/base.go 的 parseListOptions 同构但独立 —— 该 helper 未导出，
// 而 handler/user 与根 handler 包是两个包；auth.go 的 extractBearerToken
// 已确立「子包内复制小 helper、避免跨身份强依赖」的先例。
func parseUserListOptions(c *gin.Context) repository.ListOptions {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
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

// parseUserPathID 从 ?id= 解析正整数 ID；缺失 / 非法 / 非正数都返回 error。
func parseUserPathID(c *gin.Context) (int64, error) {
	raw := c.Query("id")
	if raw == "" {
		return 0, errors.New("missing id")
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return 0, errors.New("invalid id")
	}
	return id, nil
}
