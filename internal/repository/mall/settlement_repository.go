package mall

import (
	"errors"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-sql-driver/mysql"
	"gorm.io/gorm"

	"ai-go-mall/internal/model/mall"
	"ai-go-mall/internal/repository"
)

// 业务 sentinel error（service 层向上传递，handler 层映射 HTTP 码）。
var (
	// ErrSettlementNotFound settlement 不存在 / 已软删。
	ErrSettlementNotFound = errors.New("settlement: not found")

	// ErrSettlementAlreadyPaid 已 mark_paid（再次 mark_paid 时返回 → handler 映射 409）。
	ErrSettlementAlreadyPaid = errors.New("settlement: already paid")

	// ErrSettlementConflict 同一 supplier + 同期已存在 settlement 单（UK 冲突 → 409）。
	ErrSettlementConflict = errors.New("settlement: conflict (supplier + period already settled)")

	// ErrInsufficientSupplierBalance supplier.balance < payout_amount（MarkPaid 时返回 → handler 映射 422）。
	ErrInsufficientSupplierBalance = errors.New("settlement: insufficient supplier balance")
)

// SettlementRepository 定义 mall_settlements / mall_settlement_items /
// mall_platform_ledger 三表的复合数据访问接口。
//
// 设计原则：
//   - 通用 CRUD 由 CRUDRepository[mall.MallSettlement] 提供；
//   - Generate / MarkPaid 是单事务复合写操作，repository 自己包 RunInTx：
//     service 层只调一个方法即可拿到「settlement + items」或「settlement + supplier balance」
//     的原子结果，无需 service 层介入事务；
//   - IncrementLedger 提供 platform ledger counter 的 ON DUPLICATE 累加（抽卡事务步骤 7.6 用）。
type SettlementRepository interface {
	repository.CRUDRepository[mall.MallSettlement]

	// GetByID 按主键查单行。
	GetByID(c *gin.Context, id int64) (*mall.MallSettlement, error)

	// GetByIDWithItems 主键查 settlement + items 列表。
	GetByIDWithItems(c *gin.Context, id int64) (*mall.MallSettlement, []mall.MallSettlementItem, error)

	// ListBySupplier 按 supplier_id 分页查 settlement（admin 结算管理页 / 供应商查看）。
	ListBySupplier(c *gin.Context, supplierID int64, opts repository.ListOptions) (items []mall.MallSettlement, total int64, err error)

	// ListItemsBySettlementID 拉某 settlement 的明细（详情 / 行展开）。
	ListItemsBySettlementID(c *gin.Context, settlementID int64) ([]mall.MallSettlementItem, error)

	// ListEligibleOrders 列某 supplier 周期内「drawn 且未被结算」订单。
	//
	// SQL 语义：`status = drawn AND created_at ∈ [start, end) AND NOT EXISTS settlement_items`。
	// NOT EXISTS 优于 NOT IN，避免 IN 子查询在大量订单下性能劣化。
	// Generate 事务前置条件筛选用它。
	ListEligibleOrders(c *gin.Context, supplierID int64, periodStart, periodEnd time.Time) ([]mall.MallDrawOrder, error)

	// Generate 单事务内 INSERT settlement + 批量 INSERT items。
	//
	// 入参 settlement 会被 GORM 自动赋值 ID（service 层事后可读 settlement.ID）；
	// items 不需要预填 SettlementID，repository 会用 settlement.ID 回填。
	// 冲突：mall_settlements UK (supplier_id, period_start, period_end) → ErrSettlementConflict。
	Generate(c *gin.Context, settlement *mall.MallSettlement, items []mall.MallSettlementItem) error

	// UpdateStatus 仅写 Status 列（结算单状态机迁移专用）。
	//
	// 与 MarkPaid 不同：UpdateStatus 不做 CAS 检查、不写 paid_at/paid_by，
	// 单纯 SET status=?。状态合法性由调用方通过 internal/domain/state.Transition 校验。
	// 当前用于 Generate 后推进 pending → processing。
	UpdateStatus(c *gin.Context, id int64, status mall.MallSettlementStatus) error

	// MarkPaid 单事务内：
	//   1. SELECT settlement WHERE id = ? FOR UPDATE → 校验 status ∈ {pending, processing}
	//   2. UPDATE settlement SET status='paid', paid_at=NOW(), paid_by=?
	//   3. UPDATE supplier SET balance = balance - ? WHERE id = ? AND balance >= ?
	//      影响行 = 0 → ErrInsufficientSupplierBalance（事务回滚）
	//
	// status 已经是 paid → ErrSettlementAlreadyPaid。
	MarkPaid(c *gin.Context, settlementID int64, adminID int64, payoutAmount float64) error

	// IncrementLedger 累加 platform ledger counter（抽卡事务步骤 7.6）。
	//
	// SQL：`INSERT INTO mall_platform_ledger (counter_key, amount) VALUES (?, ?)
	//        ON DUPLICATE KEY UPDATE amount = amount + VALUES(amount)`。
	//
	// counterKey 取值见 design D14：total_revenue / total_commission。
	IncrementLedger(c *gin.Context, counterKey string, delta float64) error
}

// gormSettlementRepository 是 SettlementRepository 的 *gorm.DB 实现。
type gormSettlementRepository struct {
	repository.CRUDRepository[mall.MallSettlement]
}

// NewSettlementRepository 返回 SettlementRepository 接口。
func NewSettlementRepository() SettlementRepository {
	return &gormSettlementRepository{
		CRUDRepository: repository.NewBaseRepository[mall.MallSettlement](),
	}
}

// GetByID 按主键查单行。
func (r *gormSettlementRepository) GetByID(c *gin.Context, id int64) (*mall.MallSettlement, error) {
	var s mall.MallSettlement
	if err := repository.DB(c).First(&s, id).Error; err != nil {
		return nil, err
	}
	return &s, nil
}

// GetByIDWithItems 主键查 settlement + items 列表。
//
// settlement 不存在时 GetByID 返回 gorm.ErrRecordNotFound，调用方按 gorm 错误
// 识别 404；items 查询失败同样向上传递。
func (r *gormSettlementRepository) GetByIDWithItems(c *gin.Context, id int64) (*mall.MallSettlement, []mall.MallSettlementItem, error) {
	s, err := r.GetByID(c, id)
	if err != nil {
		return nil, nil, err
	}
	items, err := r.ListItemsBySettlementID(c, id)
	if err != nil {
		return nil, nil, err
	}
	return s, items, nil
}

// ListBySupplier 按 supplier_id 分页查。
func (r *gormSettlementRepository) ListBySupplier(c *gin.Context, supplierID int64, opts repository.ListOptions) ([]mall.MallSettlement, int64, error) {
	db := repository.DB(c).Model(&mall.MallSettlement{}).Where("supplier_id = ?", supplierID)

	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	items := make([]mall.MallSettlement, 0)
	if total == 0 {
		return items, total, nil
	}

	offset := (opts.Page - 1) * opts.PageSize
	if err := db.Offset(offset).Limit(opts.PageSize).
		Order("id DESC").
		Find(&items).Error; err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

// ListItemsBySettlementID 拉某 settlement 的明细。
func (r *gormSettlementRepository) ListItemsBySettlementID(c *gin.Context, settlementID int64) ([]mall.MallSettlementItem, error) {
	items := make([]mall.MallSettlementItem, 0)
	err := repository.DB(c).
		Where("settlement_id = ?", settlementID).
		Order("id ASC").
		Find(&items).Error
	return items, err
}

// ListEligibleOrders 列某 supplier 周期内「drawn 且未结算」订单。
//
// NOT EXISTS 子查询：(SELECT 1 FROM mall_settlement_items si WHERE si.draw_order_id = mall_draw_orders.id)
// 命中 UK idx_mall_settlement_items_draw_order，性能 O(1) 判断。
func (r *gormSettlementRepository) ListEligibleOrders(c *gin.Context, supplierID int64, periodStart, periodEnd time.Time) ([]mall.MallDrawOrder, error) {
	rows := make([]mall.MallDrawOrder, 0)
	err := repository.DB(c).
		Where("supplier_id = ? AND status = ? AND created_at >= ? AND created_at < ?",
			supplierID, mall.OrderStatusDrawn, periodStart, periodEnd).
		Where("NOT EXISTS (SELECT 1 FROM mall_settlement_items si WHERE si.draw_order_id = mall_draw_orders.id)").
		Order("created_at ASC").
		Find(&rows).Error
	return rows, err
}

// Generate 单事务内 INSERT settlement + 批量 INSERT items。
//
// UK (supplier_id, period_start, period_end) 冲突 → ErrSettlementConflict（避免 service 层重复判空）。
// settlement.ID 在 INSERT 后由 GORM 自动赋值；items.SettlementID 在 repository 内回填。
func (r *gormSettlementRepository) Generate(c *gin.Context, settlement *mall.MallSettlement, items []mall.MallSettlementItem) error {
	if settlement == nil {
		return errors.New("settlement: nil settlement")
	}

	return repository.RunInTx(c, func(txCtx *gin.Context) error {
		if err := repository.DB(txCtx).Create(settlement).Error; err != nil {
			if isDuplicateKeyErr(err) {
				return ErrSettlementConflict
			}
			return err
		}
		if len(items) == 0 {
			return nil
		}
		// 回填 items.SettlementID = 新生成的 settlement.ID。
		for i := range items {
			items[i].SettlementID = settlement.ID
		}
		// items.Insert 用 mall_settlement_items 的 UK (draw_order_id) 兜底二次保护，避免「同订单两次结算」。
		return repository.DB(txCtx).Create(&items).Error
	})
}

// UpdateStatus 仅写 Status 列。
//
// 与 MarkPaid 不同：UpdateStatus 不做 CAS 检查、不写 paid_at/paid_by，
// 单纯 SET status=?。状态合法性由调用方通过 internal/domain/state.Transition 校验。
func (r *gormSettlementRepository) UpdateStatus(c *gin.Context, id int64, status mall.MallSettlementStatus) error {
	return repository.DB(c).
		Model(&mall.MallSettlement{}).
		Where("id = ?", id).
		Update("status", status).Error
}

// MarkPaid 单事务内结算单置 paid + 扣 supplier balance。
//
// 步骤：
//  1. SELECT settlement WHERE id = ? FOR UPDATE → 校验 status；
//  2. UPDATE settlement：status='paid', paid_at=NOW(), paid_by=?；
//  3. UPDATE supplier：balance = balance - ? WHERE balance >= ? → 0 行 = ErrInsufficientSupplierBalance。
func (r *gormSettlementRepository) MarkPaid(c *gin.Context, settlementID int64, adminID int64, payoutAmount float64) error {
	if payoutAmount < 0 {
		return errors.New("settlement: payout amount must be non-negative")
	}
	if adminID <= 0 {
		return errors.New("settlement: admin id required")
	}

	return repository.RunInTx(c, func(txCtx *gin.Context) error {
		// 1. SELECT settlement（无 FOR UPDATE：GORM Find 默认无锁；并发 mark_paid
		//   防御靠「UPDATE WHERE status IN (pending, processing)」+ RowsAffected=0 失败回滚）。
		var s mall.MallSettlement
		if err := repository.DB(txCtx).First(&s, settlementID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrSettlementNotFound
			}
			return err
		}
		if s.Status == mall.SettlementStatusPaid {
			return ErrSettlementAlreadyPaid
		}
		if s.Status != mall.SettlementStatusPending && s.Status != mall.SettlementStatusProcessing {
			return ErrSettlementNotFound
		}

		// 2. UPDATE settlement。
		now := time.Now()
		res := repository.DB(txCtx).
			Model(&mall.MallSettlement{}).
			Where("id = ? AND status IN ?", settlementID, []string{string(mall.SettlementStatusPending), string(mall.SettlementStatusProcessing)}).
			Updates(map[string]any{
				"status":  mall.SettlementStatusPaid,
				"paid_at": &now,
				"paid_by": adminID,
			})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			// 并发 mark_paid（已经被别人先标 paid 了）。
			return ErrSettlementAlreadyPaid
		}

		// 3. UPDATE supplier balance（条件更新防超额）。
		if payoutAmount > 0 {
			res2 := repository.DB(txCtx).
				Model(&mall.MallSupplier{}).
				Where("id = ? AND balance >= ?", s.SupplierID, payoutAmount).
				Update("balance", gorm.Expr("balance - ?", payoutAmount))
			if res2.Error != nil {
				return res2.Error
			}
			if res2.RowsAffected == 0 {
				return ErrInsufficientSupplierBalance
			}
		}
		return nil
	})
}

// IncrementLedger 累加 platform ledger counter。
//
// 用 ON DUPLICATE KEY UPDATE amount = amount + VALUES(amount) 实现 counter 单调增。
// counterKey 为空 → 报错（编程错误，避免误覆盖总行）。
func (r *gormSettlementRepository) IncrementLedger(c *gin.Context, counterKey string, delta float64) error {
	if counterKey == "" {
		return errors.New("settlement: empty counter_key")
	}
	return repository.DB(c).
		Exec(
			"INSERT INTO mall_platform_ledger (counter_key, amount) VALUES (?, ?) "+
				"ON DUPLICATE KEY UPDATE amount = amount + VALUES(amount)",
			counterKey, delta,
		).
		Error
}

// isDuplicateKeyErr 判断 MySQL 唯一索引冲突错误（Error Number 1062）。
//
// 用于识别 mall_settlements (supplier_id, period_start, period_end) UK 冲突 → ErrSettlementConflict。
func isDuplicateKeyErr(err error) bool {
	if err == nil {
		return false
	}
	var me *mysql.MySQLError
	if errors.As(err, &me) {
		return me.Number == 1062
	}
	return false
}

// 编译期断言。
var _ SettlementRepository = (*gormSettlementRepository)(nil)