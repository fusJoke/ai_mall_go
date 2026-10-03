// Package mall — bill.go 实现批次账单的核心业务（中立包，user / admin 两侧共用）。
//
// 业务规则（openspec: add-batch-settlement，商品即批次 v2 设计）：
//   - GenerateForBox：聚合某商品全部 drawn 订单 → 按 commission_rate 快照算
//     commission / payout → 建账单 + 明细。幂等：已有账单直接返回。
//     只允许在抽卡事务尾部（售罄检测通过、lifecycle 已 CAS 为 sold_out）调用，
//     因此本方法不自开事务——传入 txCtx 即与外层抽卡事务共用同一连接。
//   - MarkPaid：状态机校验（confirmed → paid 唯一路径）→ 复合事务
//     [账单置 paid + 扣 supplier.balance + 商品置 settled]，任一步失败整体回滚。
//   - Regenerate：admin 重核（rejected → generated，金额整体快照更新 + version+1）。
//
// 手续费率口径（与 admin.settlement 同源，design D13）：
// supplier.commission_rate ?? config.mall.default_commission_rate；
// 费率在生成账单时快照落表，供应商后续调费率不影响历史账单。
//
// 金额精度：与 admin.settlement 同口径——分笔 round（每订单 2 位小数）后求和，
// 避免「整体 round vs 分笔 round」的 1 分误差。
package mall

import (
	"errors"
	"math"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"ai-go-mall/internal/domain/state"
	"ai-go-mall/internal/infra/config"
	"ai-go-mall/internal/model/mall"
	"ai-go-mall/internal/repository"
	mallRepo "ai-go-mall/internal/repository/mall"
	supplierRepo "ai-go-mall/internal/repository/supplier"
)

// 业务 sentinel error。
//
// 设计：service 层 re-export repo 包的同名 var，handler 统一从 service 包识别错误；
// ErrBillStatusConflict / ErrBoxNotSoldOut / ErrNoOrdersForBill / ErrBoxLifecycleConflict 是 service 层独有。
var (
	// ErrBillNotFound 账单不存在 / 已软删（→ 404）。
	ErrBillNotFound = mallRepo.ErrBillNotFound

	// ErrBillConflict 同商品已存在账单（UK blind_box_id 冲突 → 409；幂等竞态兜底）。
	ErrBillConflict = mallRepo.ErrBillConflict

	// ErrInsufficientSupplierBalance supplier.balance < payout_amount（MarkPaid → 422）。
	ErrInsufficientSupplierBalance = mallRepo.ErrInsufficientSupplierBalance

	// ErrBillStatusConflict CAS 未命中：账单状态被并发改变 / 与预校验时不一致（→ 409）。
	ErrBillStatusConflict = errors.New("mall.bill: bill status conflict (concurrent update or stale read)")

	// ErrBoxNotSoldOut 商品生命周期不是 sold_out/settled（账单只能对售罄商品生成）。
	ErrBoxNotSoldOut = errors.New("mall.bill: box is not sold out (bill generation only allowed after sold out)")

	// ErrNoOrdersForBill 售罄但聚合到 0 笔 drawn 订单（脏数据，拒绝生成空账单）。
	ErrNoOrdersForBill = errors.New("mall.bill: no drawn orders for sold out box (dirty data)")

	// ErrBoxLifecycleConflict MarkPaid 时商品生命周期 CAS 未命中（期望 sold_out，脏数据防御）。
	ErrBoxLifecycleConflict = errors.New("mall.bill: box lifecycle conflict (want sold_out)")
)

// BillService 批次账单核心业务接口（user / admin 共用，中立包）。
type BillService interface {
	// GenerateForBox 幂等生成某商品的批次账单。
	//
	// 前置：商品已售罄（lifecycle ∈ {sold_out, settled}）；已有账单 → 直接返回。
	// 必须在抽卡事务内调用（c 为 txCtx），账单落库与抽卡事务原子提交。
	// 并发生成的极端情况由 UK blind_box_id 兜底 → ErrBillConflict。
	GenerateForBox(c *gin.Context, boxID int64) (*mall.MallSettlementBill, error)

	// MarkPaid admin 打款：状态机校验后复合事务
	// [bill confirmed→paid CAS + supplier.balance -= payout + box sold_out→settled]。
	//
	// 错误：ErrBillNotFound / state.ErrInvalidStateTransition（未确认就打款等）/
	// ErrBillStatusConflict（并发抢先）/ ErrInsufficientSupplierBalance / ErrBoxLifecycleConflict。
	MarkPaid(c *gin.Context, billID int64, adminID int64) error

	// Regenerate admin 重核：rejected → generated，金额整体快照更新 + version+1。
	//
	// 错误：ErrBillNotFound / state.ErrInvalidStateTransition / ErrBillStatusConflict。
	Regenerate(c *gin.Context, billID int64, amounts mallRepo.BillRegenAmounts, remark string) error

	// FinalizeSoldOut 抽卡事务尾部的售罄落定（openspec: add-batch-settlement 步骤 7.7）。
	//
	// 前置：调用方（draw.go 普通抽卡 / 秒杀事务）已判定池内总库存为 0
	// （SubtotalStockByPoolID == 0），且当前处于抽卡事务内（c 为 txCtx）。
	// 步骤：
	//  1. 商品 lifecycle selling→sold_out CAS（0 行 = 重复触发 / 状态已推进，幂等继续）；
	//  2. GenerateForBox 幂等建账单（已有账单直接返回）。
	// 任一步失败 → 抽卡事务整体回滚（账单与订单原子——用户不会为「没有账单的抽卡」付款）。
	FinalizeSoldOut(c *gin.Context, boxID int64) error

	// ListAdmin admin 账单列表：全量分页，可按 status / supplier 筛（零值不筛）。
	ListAdmin(c *gin.Context, status mall.MallSettlementBillStatus, supplierID int64, opts repository.ListOptions) (items []mall.MallSettlementBill, total int64, err error)

	// Detail 账单 + 明细（admin 无归属概念，直达）。
	//
	// 账单不存在 → ErrBillNotFound。
	Detail(c *gin.Context, billID int64) (*mall.MallSettlementBill, []mall.MallSettlementBillItem, error)
}

// BillServiceDeps 注入 BillService 所需的依赖。
type BillServiceDeps struct {
	BillRepo     mallRepo.SettlementBillRepository
	BoxRepo      mallRepo.BlindBoxRepository
	SupplierRepo supplierRepo.SupplierRepository
	OrderRepo    mallRepo.DrawOrderRepository
}

// baseBillService 是 BillService 的默认实现。
type baseBillService struct {
	bill     mallRepo.SettlementBillRepository
	box      mallRepo.BlindBoxRepository
	supplier supplierRepo.SupplierRepository
	order    mallRepo.DrawOrderRepository
}

// NewBillService 接收依赖，返回 BillService 接口。
func NewBillService(d BillServiceDeps) BillService {
	return &baseBillService{
		bill:     d.BillRepo,
		box:      d.BoxRepo,
		supplier: d.SupplierRepo,
		order:    d.OrderRepo,
	}
}

// GenerateForBox 实现见 BillService 注释。
func (s *baseBillService) GenerateForBox(c *gin.Context, boxID int64) (*mall.MallSettlementBill, error) {
	if boxID <= 0 {
		return nil, errors.New("mall.bill: invalid box id")
	}

	// 幂等：已有账单直接返回（抽卡事务重放 / 补偿场景安全）。
	if existing, err := s.bill.GetByBoxID(c, boxID); err == nil {
		return existing, nil
	} else if !errors.Is(err, mallRepo.ErrBillNotFound) {
		return nil, err
	}

	// 商品必须存在，且只能对售罄商品生成账单。
	box, err := s.box.GetByID(c, boxID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("mall.bill: box not found")
		}
		return nil, err
	}
	if box.Lifecycle == mall.LifecycleSelling {
		return nil, ErrBoxNotSoldOut
	}

	// 费率快照（supplier 优先，NULL 走 config 默认）。
	rate, err := s.resolveRate(c, box.SupplierID)
	if err != nil {
		return nil, err
	}

	// 聚合全部「drawn 且未被结算」订单。
	orders, err := s.order.ListBoxDrawnOrders(c, boxID)
	if err != nil {
		return nil, err
	}
	if len(orders) == 0 {
		return nil, ErrNoOrdersForBill
	}

	total, commission, payout := computeBillTotals(orders, rate)
	items := make([]mall.MallSettlementBillItem, 0, len(orders))
	for _, o := range orders {
		// 单订单按 rate 快照算（分笔 round，与 admin.settlement 同口径）。
		orderCommission := roundHalfUp(o.Price*rate, 2)
		orderPayout := roundHalfUp(o.Price-orderCommission, 2)
		items = append(items, mall.MallSettlementBillItem{
			DrawOrderID:      o.ID,
			Amount:           roundHalfUp(o.Price, 2),
			CommissionAmount: orderCommission,
			PayoutAmount:     orderPayout,
		})
	}

	bill := &mall.MallSettlementBill{
		BlindBoxID:       boxID,
		SupplierID:       box.SupplierID,
		TotalAmount:      total,
		CommissionRate:   rate,
		CommissionAmount: commission,
		PayoutAmount:     payout,
		Status:           mall.BillStatusGenerated,
	}
	if err := s.bill.CreateWithItems(c, bill, items); err != nil {
		// ErrBillConflict（并发售罄的极端竞态）原样上抛，调用方按幂等处理。
		return nil, err
	}
	return bill, nil
}

// MarkPaid 实现见 BillService 注释。
func (s *baseBillService) MarkPaid(c *gin.Context, billID int64, adminID int64) error {
	if adminID <= 0 {
		return errors.New("mall.bill: invalid admin id")
	}
	if billID <= 0 {
		return ErrBillNotFound
	}

	// 预校验：账单存在 + 状态机允许 confirmed → paid。
	bill, err := s.bill.GetByID(c, billID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrBillNotFound
		}
		return err
	}
	currentState, ok := mapBillStatusToState(bill.Status)
	if !ok {
		return ErrBillNotFound
	}
	if _, err := state.Transition(currentState, state.BillStates.Paid, &state.Context{Entity: bill}); err != nil {
		return err
	}

	// 复合事务：任一步失败整体回滚（含 paid CAS 本身）。
	return repository.RunInTx(c, func(txCtx *gin.Context) error {
		// 1. 账单置 paid（CAS confirmed → paid，0 行 = 并发抢先）。
		rows, err := s.bill.MarkPaid(txCtx, billID, adminID)
		if err != nil {
			return err
		}
		if rows == 0 {
			return ErrBillStatusConflict
		}

		// 2. 扣供应商余额（条件扣减防超额；payout=0 跳过）。
		if bill.PayoutAmount > 0 {
			rows, err := s.bill.DeductSupplierBalance(txCtx, bill.SupplierID, bill.PayoutAmount)
			if err != nil {
				return err
			}
			if rows == 0 {
				return ErrInsufficientSupplierBalance
			}
		}

		// 3. 商品生命周期落定（sold_out → settled；0 行 = 脏数据，保守拒绝）。
		rows, err = s.bill.SetBoxLifecycle(txCtx, bill.BlindBoxID, mall.LifecycleSoldOut, mall.LifecycleSettled)
		if err != nil {
			return err
		}
		if rows == 0 {
			return ErrBoxLifecycleConflict
		}
		return nil
	})
}

// Regenerate 实现见 BillService 注释。
func (s *baseBillService) Regenerate(c *gin.Context, billID int64, amounts mallRepo.BillRegenAmounts, remark string) error {
	if billID <= 0 {
		return ErrBillNotFound
	}

	bill, err := s.bill.GetByID(c, billID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrBillNotFound
		}
		return err
	}

	// 状态机校验：只有 rejected 可以回 generated（重核只允许发生在驳回态）。
	currentState, ok := mapBillStatusToState(bill.Status)
	if !ok {
		return ErrBillNotFound
	}
	if _, err := state.Transition(currentState, state.BillStates.Generated, &state.Context{Entity: bill}); err != nil {
		return err
	}

	rows, err := s.bill.Regenerate(c, billID, amounts, remark)
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrBillStatusConflict
	}
	return nil
}

// FinalizeSoldOut 实现见 BillService 注释。
func (s *baseBillService) FinalizeSoldOut(c *gin.Context, boxID int64) error {
	if boxID <= 0 {
		return errors.New("mall.bill: invalid box id")
	}
	// 1. CAS 落定售罄。0 行 = 重复触发（并发抢先后重放 / 补偿重跑），幂等继续。
	if _, err := s.bill.SetBoxLifecycle(c, boxID, mall.LifecycleSelling, mall.LifecycleSoldOut); err != nil {
		return err
	}
	// 2. 幂等建账单（已有账单 → GenerateForBox 直接返回）。
	if _, err := s.GenerateForBox(c, boxID); err != nil {
		return err
	}
	return nil
}

// resolveRate 解析手续费率（supplier.commission_rate ?? config 默认）。
func (s *baseBillService) resolveRate(c *gin.Context, supplierID int64) (float64, error) {
	if supplierID <= 0 {
		return 0, errors.New("mall.bill: invalid supplier id")
	}
	sup, err := s.supplier.GetByID(c, supplierID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, errors.New("mall.bill: supplier not found")
		}
		return 0, err
	}
	if sup.CommissionRate != nil {
		return *sup.CommissionRate, nil
	}
	return config.Get().Mall.DefaultCommissionRate, nil
}

// mapBillStatusToState 把 mall.MallSettlementBillStatus 映射到 state.BillState。
//
// 未识别状态 → return false，调用方保守按 ErrBillNotFound 处理，避免脏数据进入资金操作。
func mapBillStatusToState(s mall.MallSettlementBillStatus) (state.BillState, bool) {
	switch s {
	case mall.BillStatusGenerated:
		return state.BillStates.Generated, true
	case mall.BillStatusConfirmed:
		return state.BillStates.Confirmed, true
	case mall.BillStatusRejected:
		return state.BillStates.Rejected, true
	case mall.BillStatusFailed:
		return state.BillStates.Failed, true
	case mall.BillStatusPaid:
		return state.BillStates.Paid, true
	}
	return state.BillState{}, false
}

// computeBillTotals 汇总订单金额 + 佣金 + 打款额（与 admin.settlement 同口径）。
//
// 算法：每订单 price * rate 分笔 round（2 位小数）后求和，避免累积浮点误差。
func computeBillTotals(orders []mall.MallDrawOrder, rate float64) (total, commission, payout float64) {
	for _, o := range orders {
		p := roundHalfUp(o.Price, 2)
		cm := roundHalfUp(o.Price*rate, 2)
		po := roundHalfUp(p-cm, 2)
		total += p
		commission += cm
		payout += po
	}
	total = roundHalfUp(total, 2)
	commission = roundHalfUp(commission, 2)
	payout = roundHalfUp(payout, 2)
	return
}

// roundHalfUp 四舍五入到 N 位小数（half-away-from-zero，财务"四舍五入"语义）。
// 与 admin.settlement 同实现；中立包不 import admin 包，故此处独立维护。
func roundHalfUp(v float64, decimals int) float64 {
	shift := math.Pow(10, float64(decimals))
	if v >= 0 {
		return math.Floor(v*shift+0.5) / shift
	}
	return math.Ceil(v*shift-0.5) / shift
}

// ListAdmin 实现见 BillService 注释（透传 repo，status/supplierID 零值不筛）。
func (s *baseBillService) ListAdmin(c *gin.Context, status mall.MallSettlementBillStatus, supplierID int64, opts repository.ListOptions) ([]mall.MallSettlementBill, int64, error) {
	return s.bill.ListAdmin(c, status, supplierID, opts)
}

// Detail 实现见 BillService 注释。
func (s *baseBillService) Detail(c *gin.Context, billID int64) (*mall.MallSettlementBill, []mall.MallSettlementBillItem, error) {
	if billID <= 0 {
		return nil, nil, ErrBillNotFound
	}
	bill, items, err := s.bill.GetByIDWithItems(c, billID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, ErrBillNotFound
		}
		return nil, nil, err
	}
	return bill, items, nil
}

// 编译期断言。
var _ BillService = (*baseBillService)(nil)
