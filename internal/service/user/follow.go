// Package user — follow.go 实现 C 端「关注供应商」服务。
//
// 业务边界：
//   - Follow：幂等 upsert（取关后再次关注会复活原行），不在 service 层做权限校验，
//     UserAuth 中间件已保证 userID 来自合法登录态。
//   - Unfollow：软删（deleted_at 由 GORM 自动写入），幂等 —— 重复取关为 no-op。
//   - ListFollowing：分页拉会员关注的供应商，service 不再二次过滤（关注列表
//     即历史快照，供应商被禁用后仍展示；前端按需标灰）。
//
// 不放缓存（D22 仅预热 Promotion/Seckill）；follow 是低频写 + 中频读，
// 直接走 DB + dbresolver 读副本足够。
package user

import (
	"github.com/gin-gonic/gin"

	"ai-go-mall/internal/model/mall"
	"ai-go-mall/internal/repository"
	mallRepo "ai-go-mall/internal/repository/mall"
)

// FollowService 是 C 端关注供应商的业务接口。
type FollowService interface {
	// Follow 关注 supplierID。
	//
	// 已关注 / 取关后再关注均为幂等 upsert；底层走 Unscoped 查询 + revive 软删行。
	// 入参 supplierID <= 0 视为业务错误，返回 ErrFollowInvalidID。
	Follow(c *gin.Context, userID, supplierID int64) error

	// Unfollow 取关 supplierID。
	//
	// 已取关 / 未关注均为 no-op（软删 SQL WHERE 过滤 deleted_at IS NULL）。
	Unfollow(c *gin.Context, userID, supplierID int64) error

	// ListFollowing 拉用户关注的供应商分页列表。
	ListFollowing(c *gin.Context, userID int64, opts repository.ListOptions) (items []mall.MallSupplier, total int64, err error)
}

// FollowServiceDeps 注入 FollowService 所需依赖。
type FollowServiceDeps struct {
	FollowRepo mallRepo.FollowRepository
}

// baseFollowService 是 FollowService 的默认实现。
type baseFollowService struct {
	follow mallRepo.FollowRepository
}

// NewFollowService 接收依赖，返回 FollowService 接口。
func NewFollowService(d FollowServiceDeps) FollowService {
	return &baseFollowService{follow: d.FollowRepo}
}

// Follow 直接转发到 FollowRepository。
func (s *baseFollowService) Follow(c *gin.Context, userID, supplierID int64) error {
	return s.follow.Follow(c, userID, supplierID)
}

// Unfollow 直接转发到 FollowRepository。
func (s *baseFollowService) Unfollow(c *gin.Context, userID, supplierID int64) error {
	return s.follow.Unfollow(c, userID, supplierID)
}

// ListFollowing 直接转发到 FollowRepository。
func (s *baseFollowService) ListFollowing(c *gin.Context, userID int64, opts repository.ListOptions) ([]mall.MallSupplier, int64, error) {
	return s.follow.ListFollowing(c, userID, opts)
}

var _ FollowService = (*baseFollowService)(nil)