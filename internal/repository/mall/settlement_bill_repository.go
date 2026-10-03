package mall

import (
	"errors"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"ai-go-mall/internal/model/mall"
	"ai-go-mall/internal/repository"
)

// 业务 sentinel error（service 层向上传递，handler 层映射 HTTP 码）。
var (
	// ErrBillNotFound 账单不存在 / 已软删 → handler 映射 404。
	ErrBillNotFound = errors.New("bill: not found")

	// ErrBillConflict 同一商品已存在账单（UK blind_box_id 冲突）→ handler 映射 409。
	// GenerateForBox 幂等的兜底：lifecycle CAS（selling→sold_out）已保证只有一个
	// 抽卡事务能走到生成步骤，此处 UK 再挡一层极端并发。
	ErrBillConflict = errors.New("bill: conflict (box already has a bill)")
)

// BillRegenAmounts admin 重核后的金额快照（Regenerate 入参）。
//
// 四字段整体落库：重核语义是「整单重新核算」，不接受只改部分金额，
// 避免 total 与 commission/payout 组合出自相矛盾的账单。
type BillRegenAmounts struct {
	// TotalAmount 本批总 GMV（元）。
	TotalAmount float64

	// CommissionRate 手续费率快照（重核可一并修正费率）。
	CommissionRate float64

	// CommissionAmount 手续费（元）。
	CommissionAmount float64

	// PayoutAmount 应打款额（元）= TotalAmount - CommissionAmount。
	PayoutAmount float64
}

// SettlementBillRepository 定义 mall_settlement_bills / mall_settlement_bill_items
// 两表的批次账单数据访问接口（openspec: add-batch-settlement）。
//
// 设计原则：
//   - 通用 CRUD 由 CRUDRepository[mall.MallSettlementBill] 提供；
//   - CreateWithItems 不自开事务：账单生成发生在「抽卡事务尾部」（draw.go 7.7
//     售罄检测），调用方传入 txCtx 后 repository.DB(txCtx) 自动复用同一事务
//     连接；若在事务外调用，两条 INSERT 不保证原子——仅限手工补账场景；
//   - UpdateStatus / MarkPaid / Regenerate / SetBoxLifecycle 均为单条 CAS UPDATE，
//     返回 RowsAffected（0 = 状态不满足 / 并发抢先），复合事务（如 paid + 扣
//     balance + 商品置 settled）由 service 层组合这三个原子操作；
//   - DeductSupplierBalance / SetBoxLifecycle 是结算场景专用的跨表原子操作
//     （mall_suppliers / mall_blind_boxes），先例见 settlement_repository.MarkPaid。
type SettlementBillRepository interface {
	repository.CRUDRepository[mall.MallSettlementBill]

	// CreateWithItems INSERT 账单 + 批量 INSERT 明细。
	//
	// bill.ID 在 INSERT 后由 GORM 自动赋值；items.BillID 由本方法回填。
	// UK blind_box_id 冲突 → ErrBillConflict。
	CreateWithItems(c *gin.Context, bill *mall.MallSettlementBill, items []mall.MallSettlementBillItem) error

	// GetByIDWithItems 主键查账单 + 明细（供应商/admin 详情页）。
	// 账单不存在 → ErrBillNotFound。
	GetByIDWithItems(c *gin.Context, id int64) (*mall.MallSettlementBill, []mall.MallSettlementBillItem, error)

	// GetByBoxID 按商品查账单（1 商品 1 账单）。
	// GenerateForBox 幂等前置检查用；不存在 → ErrBillNotFound。
	GetByBoxID(c *gin.Context, boxID int64) (*mall.MallSettlementBill, error)

	// ListBySupplier 供应商账单中心：按 supplier_id 分页，status 为空则不筛。
	ListBySupplier(c *gin.Context, supplierID int64, status mall.MallSettlementBillStatus, opts repository.ListOptions) ([]mall.MallSettlementBill, int64, error)

	// ListAdmin admin 账单管理：status / supplierID 均为可选筛选（零值 = 不筛）。
	ListAdmin(c *gin.Context, status mall.MallSettlementBillStatus, supplierID int64, opts repository.ListOptions) ([]mall.MallSettlementBill, int64, error)

	// UpdateStatus CAS 推进状态：WHERE id=? AND status=from → SET status=to。
	//
	// extra 追加写入列（列名 → 值，如 reject_reason），nil = 纯状态推进；
	// 列名由 service 层受控传入，不得透传外部输入的 key。
	// 返回 RowsAffected：0 = 状态不是 from（并发抢先 / 非法流转），由调用方裁决。
	UpdateStatus(c *gin.Context, id int64, from, to mall.MallSettlementBillStatus, extra map[string]any) (int64, error)

	// Regenerate admin 重核：金额整体更新 + version+1 + 状态回 generated，
	// CAS WHERE status='rejected'（重核只允许发生在驳回态）。
	// 返回 RowsAffected：0 = 账单不处于 rejected（或不存在）。
	Regenerate(c *gin.Context, id int64, amounts BillRegenAmounts, remark string) (int64, error)

	// MarkPaid 单表 CAS：confirmed → paid，同时写 paid_at / paid_by。
	// 返回 RowsAffected：0 = 状态不是 confirmed（已打款 / 并发抢先）。
	// 扣 supplier.balance 与商品置 settled 由 service 层在同一事务内组合调用。
	MarkPaid(c *gin.Context, id int64, adminID int64) (int64, error)

	// DeductSupplierBalance 条件扣减供应商余额：WHERE id=? AND balance >= ?。
	// 返回 RowsAffected：0 = 余额不足（业务错误映射留给 service 层，
	// 这里只报告行数，保持原子操作语义纯粹）。
	DeductSupplierBalance(c *gin.Context, supplierID int64, amount float64) (int64, error)

	// SetBoxLifecycle 商品生命周期 CAS：WHERE id=? AND lifecycle=from → SET lifecycle=to。
	// MarkPaid 后 sold_out → settled 用；返回 RowsAffected：0 = 生命周期不是 from。
	SetBoxLifecycle(c *gin.Context, boxID int64, from, to mall.MallBlindBoxLifecycle) (int64, error)
}

// gormSettlementBillRepository 是 SettlementBillRepository 的 *gorm.DB 实现。
type gormSettlementBillRepository struct {
	repository.CRUDRepository[mall.MallSettlementBill]
}

// NewSettlementBillRepository 返回 SettlementBillRepository 接口。
func NewSettlementBillRepository() SettlementBillRepository {
	return &gormSettlementBillRepository{
		CRUDRepository: repository.NewBaseRepository[mall.MallSettlementBill](),
	}
}

// CreateWithItems INSERT 账单 + 批量 INSERT 明细（不开事务，事务边界见接口注释）。
//
// UK blind_box_id 冲突 → ErrBillConflict；items.BillID 回填 bill.ID。
func (r *gormSettlementBillRepository) CreateWithItems(c *gin.Context, bill *mall.MallSettlementBill, items []mall.MallSettlementBillItem) error {
	if bill == nil {
		return errors.New("bill: nil bill")
	}
	if err := repository.DB(c).Create(bill).Error; err != nil {
		if isDuplicateKeyErr(err) {
			return ErrBillConflict
		}
		return err
	}
	if len(items) == 0 {
		return nil
	}
	// 回填 items.BillID = 新生成的 bill.ID。
	for i := range items {
		items[i].BillID = bill.ID
	}
	// items 整批 INSERT，mall_settlement_bill_items UK (draw_order_id) 兜底防「同订单两次结算」。
	return repository.DB(c).Create(&items).Error
}

// GetByIDWithItems 主键查账单 + 明细。
func (r *gormSettlementBillRepository) GetByIDWithItems(c *gin.Context, id int64) (*mall.MallSettlementBill, []mall.MallSettlementBillItem, error) {
	var b mall.MallSettlementBill
	if err := repository.DB(c).First(&b, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, ErrBillNotFound
		}
		return nil, nil, err
	}
	items, err := r.listItemsByBillID(c, b.ID)
	if err != nil {
		return nil, nil, err
	}
	return &b, items, nil
}

// GetByBoxID 按商品查账单（UK blind_box_id 保证至多一行）。
func (r *gormSettlementBillRepository) GetByBoxID(c *gin.Context, boxID int64) (*mall.MallSettlementBill, error) {
	var b mall.MallSettlementBill
	if err := repository.DB(c).Where("blind_box_id = ?", boxID).First(&b).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrBillNotFound
		}
		return nil, err
	}
	return &b, nil
}

// listItemsByBillID 拉某账单明细（id 升序，对应订单时间序）。
func (r *gormSettlementBillRepository) listItemsByBillID(c *gin.Context, billID int64) ([]mall.MallSettlementBillItem, error) {
	items := make([]mall.MallSettlementBillItem, 0)
	err := repository.DB(c).
		Where("bill_id = ?", billID).
		Order("id ASC").
		Find(&items).Error
	return items, err
}

// ListBySupplier 供应商账单中心分页查。
func (r *gormSettlementBillRepository) ListBySupplier(c *gin.Context, supplierID int64, status mall.MallSettlementBillStatus, opts repository.ListOptions) ([]mall.MallSettlementBill, int64, error) {
	return r.listFiltered(c, func(db *gorm.DB) *gorm.DB {
		return db.Where("supplier_id = ?", supplierID)
	}, status, opts)
}

// ListAdmin admin 账单管理分页查（supplierID 零值 = 不筛供应商）。
func (r *gormSettlementBillRepository) ListAdmin(c *gin.Context, status mall.MallSettlementBillStatus, supplierID int64, opts repository.ListOptions) ([]mall.MallSettlementBill, int64, error) {
	return r.listFiltered(c, func(db *gorm.DB) *gorm.DB {
		if supplierID > 0 {
			db = db.Where("supplier_id = ?", supplierID)
		}
		return db
	}, status, opts)
}

// listFiltered 共用分页查询：conds 拼 scoping 条件，status 非空时追加筛选。
//
// total 用不带 Limit/Offset 的 Count 单独取；total=0 短路不发 Find。
func (r *gormSettlementBillRepository) listFiltered(c *gin.Context, conds func(*gorm.DB) *gorm.DB, status mall.MallSettlementBillStatus, opts repository.ListOptions) ([]mall.MallSettlementBill, int64, error) {
	db := conds(repository.DB(c).Model(&mall.MallSettlementBill{}))
	if status != "" {
		db = db.Where("status = ?", status)
	}

	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	items := make([]mall.MallSettlementBill, 0)
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

// UpdateStatus CAS 推进状态 + 可选追加列。
//
// extra 列名由 service 层受控传入（当前仅 reject_reason）。
func (r *gormSettlementBillRepository) UpdateStatus(c *gin.Context, id int64, from, to mall.MallSettlementBillStatus, extra map[string]any) (int64, error) {
	updates := map[string]any{"status": to}
	for k, v := range extra {
		updates[k] = v
	}
	res := repository.DB(c).
		Model(&mall.MallSettlementBill{}).
		Where("id = ? AND status = ?", id, from).
		Updates(updates)
	return res.RowsAffected, res.Error
}

// Regenerate admin 重核：金额整体更新 + version+1 + 回 generated（CAS rejected）。
//
// reject_reason 同步清空（重核通过后驳回原因失效）；remark 记录重核说明。
func (r *gormSettlementBillRepository) Regenerate(c *gin.Context, id int64, amounts BillRegenAmounts, remark string) (int64, error) {
	if amounts.TotalAmount < 0 || amounts.CommissionAmount < 0 || amounts.PayoutAmount < 0 {
		return 0, errors.New("bill: regen amounts must be non-negative")
	}
	res := repository.DB(c).
		Model(&mall.MallSettlementBill{}).
		Where("id = ? AND status = ?", id, mall.BillStatusRejected).
		Updates(map[string]any{
			"total_amount":      amounts.TotalAmount,
			"commission_rate":   amounts.CommissionRate,
			"commission_amount": amounts.CommissionAmount,
			"payout_amount":     amounts.PayoutAmount,
			"version":           gorm.Expr("version + 1"),
			"status":            mall.BillStatusGenerated,
			"reject_reason":     "",
			"remark":            remark,
		})
	return res.RowsAffected, res.Error
}

// MarkPaid 单表 CAS：confirmed → paid + paid_at / paid_by。
func (r *gormSettlementBillRepository) MarkPaid(c *gin.Context, id int64, adminID int64) (int64, error) {
	if adminID <= 0 {
		return 0, errors.New("bill: admin id required")
	}
	now := time.Now()
	res := repository.DB(c).
		Model(&mall.MallSettlementBill{}).
		Where("id = ? AND status = ?", id, mall.BillStatusConfirmed).
		Updates(map[string]any{
			"status":  mall.BillStatusPaid,
			"paid_at": &now,
			"paid_by": adminID,
		})
	return res.RowsAffected, res.Error
}

// DeductSupplierBalance 条件扣减供应商余额（WHERE balance >= ? 防超额）。
func (r *gormSettlementBillRepository) DeductSupplierBalance(c *gin.Context, supplierID int64, amount float64) (int64, error) {
	if amount < 0 {
		return 0, errors.New("bill: deduct amount must be non-negative")
	}
	res := repository.DB(c).
		Model(&mall.MallSupplier{}).
		Where("id = ? AND balance >= ?", supplierID, amount).
		Update("balance", gorm.Expr("balance - ?", amount))
	return res.RowsAffected, res.Error
}

// SetBoxLifecycle 商品生命周期 CAS（结算场景专用，操作 mall_blind_boxes 表）。
func (r *gormSettlementBillRepository) SetBoxLifecycle(c *gin.Context, boxID int64, from, to mall.MallBlindBoxLifecycle) (int64, error) {
	res := repository.DB(c).
		Model(&mall.MallBlindBox{}).
		Where("id = ? AND lifecycle = ?", boxID, from).
		Update("lifecycle", to)
	return res.RowsAffected, res.Error
}

// 编译期断言。
var _ SettlementBillRepository = (*gormSettlementBillRepository)(nil)
