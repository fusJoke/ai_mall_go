// Package user — order.go 实现 C 端订单查询服务（list + detail）。
//
// 业务规则：
//   - List 只返回当前用户的订单（userID 由 handler 从 token 上下文注入）；
//   - Detail 仅允许用户查自己的订单（越权读 → ErrOrderNotFound，
//     避免泄露订单存在性）。
package user

import (
	"errors"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"ai-go-mall/internal/model/mall"
	"ai-go-mall/internal/repository"
	mallRepo "ai-go-mall/internal/repository/mall"
)

// ErrOrderNotFound 订单不可见（不存在 / 越权读）。
//
// Detail 走统一 404 文案，不暴露「订单不存在」与「订单不属于你」的区别 ——
// 防 ID 枚举攻击。
var ErrOrderNotFound = errors.New("user.order: not found")

// OrderService 是 C 端订单查询业务接口。
type OrderService interface {
	// List 拉当前用户的订单列表（按 created_at DESC）。
	List(c *gin.Context, userID int64, opts repository.ListOptions) (items []mall.MallDrawOrder, total int64, err error)

	// Detail 拉单个订单的详情（含明细 items）。
	// 越权读 → ErrOrderNotFound（不暴露订单存在性）。
	Detail(c *gin.Context, userID, orderID int64) (*mall.MallDrawOrder, []mall.MallDrawOrderItem, error)
}

// OrderServiceDeps 注入 OrderService 所需的依赖。
type OrderServiceDeps struct {
	DrawOrderRepo mallRepo.DrawOrderRepository
}

// baseOrderService 是 OrderService 的默认实现。
type baseOrderService struct {
	order mallRepo.DrawOrderRepository
}

// NewOrderService 接收依赖，返回 OrderService 接口。
func NewOrderService(d OrderServiceDeps) OrderService {
	return &baseOrderService{order: d.DrawOrderRepo}
}

// List 实现见 OrderService 注释。
func (s *baseOrderService) List(c *gin.Context, userID int64, opts repository.ListOptions) ([]mall.MallDrawOrder, int64, error) {
	if userID <= 0 {
		return []mall.MallDrawOrder{}, 0, nil
	}
	return s.order.ListByUser(c, userID, opts)
}

// Detail 实现见 OrderService 注释。
//
// 安全策略：order.UserID != userID → ErrOrderNotFound（不是 gorm.ErrRecordNotFound，
// handler 统一映射成 404）。
func (s *baseOrderService) Detail(c *gin.Context, userID, orderID int64) (*mall.MallDrawOrder, []mall.MallDrawOrderItem, error) {
	if userID <= 0 || orderID <= 0 {
		return nil, nil, ErrOrderNotFound
	}
	order, err := s.order.GetByID(c, orderID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, ErrOrderNotFound
		}
		return nil, nil, err
	}
	if order == nil {
		return nil, nil, ErrOrderNotFound
	}
	if order.UserID != userID {
		// 越权读 —— 故意复用 ErrOrderNotFound，不暴露「订单存在但不属于你」。
		return nil, nil, ErrOrderNotFound
	}
	items, err := s.order.ListItemsByOrderID(c, orderID)
	if err != nil {
		return nil, nil, err
	}
	return order, items, nil
}

// 编译期断言：baseOrderService 必须实现 OrderService。
var _ OrderService = (*baseOrderService)(nil)
