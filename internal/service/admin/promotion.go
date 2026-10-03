// Package admin — promotion.go 实现 admin 视角的限时特价只读列表。
//
// MVP 范围（任务 6.11 / spec）：
//   - admin 端活动管理是查看 / 联动视图（禁供应商 → 前端列表过滤由
//     service/user 层保证）；启停 / 删除 / 改价由供应商端自行操作，
//     admin 不越俎代庖，所以只暴露 List。
package admin

import (
	"github.com/gin-gonic/gin"

	"ai-go-mall/internal/model/mall"
	"ai-go-mall/internal/repository"
	mallRepo "ai-go-mall/internal/repository/mall"
)

// PromotionService 是 admin 视角的限时特价只读接口。
type PromotionService interface {
	// List 拉全量活动（分页，start_at 倒序由 repo 的 CRUD List 决定）。
	List(c *gin.Context, opts repository.ListOptions) (items []mall.MallPromotion, total int64, err error)
}

// PromotionServiceDeps 注入 PromotionService 所需的依赖。
type PromotionServiceDeps struct {
	PromotionRepo mallRepo.PromotionRepository
}

// basePromotionService 是 PromotionService 的默认实现。
type basePromotionService struct {
	repo mallRepo.PromotionRepository
}

// NewPromotionService 接收依赖，返回 PromotionService 接口。
func NewPromotionService(d PromotionServiceDeps) PromotionService {
	return &basePromotionService{repo: d.PromotionRepo}
}

// List 实现见 PromotionService 注释；透传 repo 的通用分页 List。
func (s *basePromotionService) List(c *gin.Context, opts repository.ListOptions) ([]mall.MallPromotion, int64, error) {
	return s.repo.List(c, opts)
}
