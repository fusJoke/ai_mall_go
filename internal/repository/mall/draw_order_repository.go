package mall

import (
	"errors"
	"time"

	"github.com/gin-gonic/gin"

	"ai-go-mall/internal/model/mall"
	"ai-go-mall/internal/repository"
)

// DrawOrderRepository 定义 mall_draw_orders + mall_draw_order_items 表的数据访问接口。
//
// 设计原则：
//   - 通用 CRUD 由 CRUDRepository[mall.MallDrawOrder] 提供；
//   - CreateOrder + CreateItems 走事务内调用（service 层包 tx.Begin/Commit）；
//   - GetByOrderNo 走 UK，支付回调 / 幂等判定用；
//   - ListByUser 按 (user_id, created_at DESC) 索引查用户订单列表；
//   - ListSettledOrdersBySupplierInRange 按 supplier_id + created_at + status
//     拉某个周期内可结算的订单（D14 settlement 关键）。
type DrawOrderRepository interface {
	repository.CRUDRepository[mall.MallDrawOrder]

	// GetByID 按主键查单行。
	GetByID(c *gin.Context, id int64) (*mall.MallDrawOrder, error)

	// GetByOrderNo 按 UK 查唯一订单（支付回调 / 幂等）。
	GetByOrderNo(c *gin.Context, orderNo string) (*mall.MallDrawOrder, error)

	// CreateOrder 插入订单。entity 为 nil 时报错。
	CreateOrder(c *gin.Context, order *mall.MallDrawOrder) error

	// ListByUser 拉某用户的订单列表（按 created_at DESC）。
	ListByUser(c *gin.Context, userID int64, opts repository.ListOptions) (items []mall.MallDrawOrder, total int64, err error)

	// UpdateStatus 仅写 Status 列（订单状态机迁移专用）。
	UpdateStatus(c *gin.Context, id int64, status mall.MallDrawOrderStatus) error

	// --- 抽卡算法 / 结算相关 ---

	// ListItemsByOrderID 拉某订单的明细。
	ListItemsByOrderID(c *gin.Context, orderID int64) ([]mall.MallDrawOrderItem, error)

	// CreateItems 批量插入订单明细。
	CreateItems(c *gin.Context, items []mall.MallDrawOrderItem) error

	// ListSettledOrdersBySupplierInRange 拉某供应商在 [startAt, endAt) 内
	// 状态为 drawn 的订单，供 service 层生成结算单。
	ListSettledOrdersBySupplierInRange(c *gin.Context, supplierID int64, startAt, endAt time.Time) ([]mall.MallDrawOrder, error)
}

// gormDrawOrderRepository 是 DrawOrderRepository 的 *gorm.DB 实现。
type gormDrawOrderRepository struct {
	repository.CRUDRepository[mall.MallDrawOrder]
}

// NewDrawOrderRepository 返回 DrawOrderRepository 接口。
func NewDrawOrderRepository() DrawOrderRepository {
	return &gormDrawOrderRepository{
		CRUDRepository: repository.NewBaseRepository[mall.MallDrawOrder](),
	}
}

// GetByID 按主键查单行。
func (r *gormDrawOrderRepository) GetByID(c *gin.Context, id int64) (*mall.MallDrawOrder, error) {
	var o mall.MallDrawOrder
	if err := repository.DB(c).First(&o, id).Error; err != nil {
		return nil, err
	}
	return &o, nil
}

// GetByOrderNo 按 UK 查唯一订单。
func (r *gormDrawOrderRepository) GetByOrderNo(c *gin.Context, orderNo string) (*mall.MallDrawOrder, error) {
	var o mall.MallDrawOrder
	if err := repository.DB(c).Where("order_no = ?", orderNo).First(&o).Error; err != nil {
		return nil, err
	}
	return &o, nil
}

// CreateOrder 插入订单。
func (r *gormDrawOrderRepository) CreateOrder(c *gin.Context, order *mall.MallDrawOrder) error {
	if order == nil {
		return errors.New("draw_order: nil order")
	}
	return repository.DB(c).Create(order).Error
}

// ListByUser 拉某用户的订单列表。
func (r *gormDrawOrderRepository) ListByUser(c *gin.Context, userID int64, opts repository.ListOptions) ([]mall.MallDrawOrder, int64, error) {
	db := repository.DB(c).Model(&mall.MallDrawOrder{}).Where("user_id = ?", userID)

	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	items := make([]mall.MallDrawOrder, 0)
	if total == 0 {
		return items, total, nil
	}

	offset := (opts.Page - 1) * opts.PageSize
	if err := db.Offset(offset).Limit(opts.PageSize).
		Order("created_at DESC").
		Find(&items).Error; err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

// UpdateStatus 仅写 Status 列。
func (r *gormDrawOrderRepository) UpdateStatus(c *gin.Context, id int64, status mall.MallDrawOrderStatus) error {
	return repository.DB(c).
		Model(&mall.MallDrawOrder{}).
		Where("id = ?", id).
		Update("status", status).Error
}

// ListItemsByOrderID 拉某订单的明细。
func (r *gormDrawOrderRepository) ListItemsByOrderID(c *gin.Context, orderID int64) ([]mall.MallDrawOrderItem, error) {
	items := make([]mall.MallDrawOrderItem, 0)
	err := repository.DB(c).
		Where("order_id = ?", orderID).
		Order("id ASC").
		Find(&items).Error
	return items, err
}

// CreateItems 批量插入订单明细。空 items 短路返回 nil。
func (r *gormDrawOrderRepository) CreateItems(c *gin.Context, items []mall.MallDrawOrderItem) error {
	if len(items) == 0 {
		return nil
	}
	return repository.DB(c).Create(&items).Error
}

// ListSettledOrdersBySupplierInRange 拉某供应商在 [startAt, endAt) 内 drawn 订单。
//
// SQL：`WHERE supplier_id = ? AND status = 'drawn' AND created_at >= ? AND created_at < ?`。
// 索引走 (supplier_id, status, created_at) 复合索引，结算场景高选择性。
func (r *gormDrawOrderRepository) ListSettledOrdersBySupplierInRange(c *gin.Context, supplierID int64, startAt, endAt time.Time) ([]mall.MallDrawOrder, error) {
	rows := make([]mall.MallDrawOrder, 0)
	err := repository.DB(c).
		Where("supplier_id = ? AND status = ? AND created_at >= ? AND created_at < ?", supplierID, mall.OrderStatusDrawn, startAt, endAt).
		Order("created_at ASC").
		Find(&rows).Error
	return rows, err
}

// 编译期断言。
var _ DrawOrderRepository = (*gormDrawOrderRepository)(nil)