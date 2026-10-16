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
	//
	// 无 CAS：仅适用于「同事务内刚建单后的初始化流转」——该事务内不存在其他写者。
	// 需要跨请求推进状态（支付回调 / 自动取消 / 结算）时**必须**改用 UpdateStatusCAS，
	// 否则「先读再写」会让两个并发流程都通过前置校验、双双写成功（OCR 审查 P2-2）。
	UpdateStatus(c *gin.Context, id int64, status mall.MallDrawOrderStatus) error

	// UpdateStatusCAS 条件推进状态：WHERE id=? AND status=from → SET status=to。
	//
	// 返回 RowsAffected：0 = 当前状态不是 from（并发抢先 / 非法流转 / 行不存在），
	// 由调用方裁决（通常映射成 409 冲突或幂等短路），不要当成错误吞掉。
	//
	// 形态与 settlement_bill_repository.UpdateStatus 一致 —— 那是本仓库已验证过的
	// CAS 写法，此处推广到订单域。
	UpdateStatusCAS(c *gin.Context, id int64, from, to mall.MallDrawOrderStatus) (int64, error)

	// --- 抽卡算法 / 结算相关 ---

	// ListItemsByOrderID 拉某订单的明细。
	ListItemsByOrderID(c *gin.Context, orderID int64) ([]mall.MallDrawOrderItem, error)

	// CreateItems 批量插入订单明细。
	CreateItems(c *gin.Context, items []mall.MallDrawOrderItem) error

	// ListSettledOrdersBySupplierInRange 拉某供应商在 [startAt, endAt) 内
	// 状态为 drawn 的订单，供 service 层生成结算单。
	ListSettledOrdersBySupplierInRange(c *gin.Context, supplierID int64, startAt, endAt time.Time) ([]mall.MallDrawOrder, error)

	// ListBoxDrawnOrders 拉某商品全部「drawn 且未被批次账单结算」订单
	// （GenerateForBox 聚合用，openspec: add-batch-settlement）。
	ListBoxDrawnOrders(c *gin.Context, boxID int64) ([]mall.MallDrawOrder, error)

	// CountDrawnByBoxIDs 按 box id 集合批量统计各自 drawn 订单数（销量口径，
	// 供应商商品列表 PoolStats 用）。返回 map 缺 key = 0 单。空集合短路。
	CountDrawnByBoxIDs(c *gin.Context, boxIDs []int64) (map[int64]int64, error)
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
//
// 无 CAS，仅供同事务内的初始化流转使用；跨请求推进状态请用 UpdateStatusCAS。
func (r *gormDrawOrderRepository) UpdateStatus(c *gin.Context, id int64, status mall.MallDrawOrderStatus) error {
	return repository.DB(c).
		Model(&mall.MallDrawOrder{}).
		Where("id = ?", id).
		Update("status", status).Error
}

// UpdateStatusCAS 条件推进状态：WHERE id=? AND status=from → SET status=to。
//
// 单条 UPDATE 内完成「校验 + 写入」，靠 DB 行锁串行化，因此并发下只有一个
// 调用者能拿到 RowsAffected = 1；先读后写的两步写法做不到这一点。
func (r *gormDrawOrderRepository) UpdateStatusCAS(c *gin.Context, id int64, from, to mall.MallDrawOrderStatus) (int64, error) {
	res := repository.DB(c).
		Model(&mall.MallDrawOrder{}).
		Where("id = ? AND status = ?", id, from).
		Update("status", to)
	return res.RowsAffected, res.Error
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

// ListBoxDrawnOrders 拉某商品全部「drawn 且未被批次账单结算」订单。
//
// NOT EXISTS 判 mall_settlement_bill_items.draw_order_id（UK 索引 O(1)），
// 与周期结算的 ListEligibleOrders 同模式；批次账单与周期结算两表互斥使用，
// 同一订单不会两边重复结算。id 升序对应下单时间序，保证账单明细顺序稳定。
func (r *gormDrawOrderRepository) ListBoxDrawnOrders(c *gin.Context, boxID int64) ([]mall.MallDrawOrder, error) {
	rows := make([]mall.MallDrawOrder, 0)
	err := repository.DB(c).
		Where("blind_box_id = ? AND status = ?", boxID, mall.OrderStatusDrawn).
		Where("NOT EXISTS (SELECT 1 FROM mall_settlement_bill_items bi WHERE bi.draw_order_id = mall_draw_orders.id)").
		Order("id ASC").
		Find(&rows).Error
	return rows, err
}

// CountDrawnByBoxIDs 按 box id 集合批量统计 drawn 订单数。
//
// 单表 GROUP BY（无 join）；map 缺 key = 该 box 无 drawn 订单（0 单）。
func (r *gormDrawOrderRepository) CountDrawnByBoxIDs(c *gin.Context, boxIDs []int64) (map[int64]int64, error) {
	out := make(map[int64]int64)
	if len(boxIDs) == 0 {
		return out, nil
	}
	type countRow struct {
		BlindBoxID int64
		Cnt        int64
	}
	rows := make([]countRow, 0)
	if err := repository.DB(c).Model(&mall.MallDrawOrder{}).
		Select("blind_box_id, COUNT(*) AS cnt").
		Where("blind_box_id IN ? AND status = ?", boxIDs, mall.OrderStatusDrawn).
		Group("blind_box_id").
		Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		out[row.BlindBoxID] = row.Cnt
	}
	return out, nil
}

// 编译期断言。
var _ DrawOrderRepository = (*gormDrawOrderRepository)(nil)