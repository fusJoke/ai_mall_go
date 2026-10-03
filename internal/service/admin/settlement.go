// Package admin — settlement.go 实现 admin 对结算单的管理（列表 / 预览 / 生成 / mark_paid）。
//
// 业务规则（spec Requirement "Admin settlement management" + design D13 / D14）：
//   - List：拉全量结算单（按 ID DESC），admin 管理页用，分页参数由 handler 兜底。
//   - Detail：拿 settlement + 全部 items，admin 详情页 / 行展开。
//   - Preview：不写库，列某 supplier + 周期内可结算订单 + 算金额，供 admin 决策参考。
//   - Generate：列 eligible orders → 算金额 → INSERT settlement + items；
//     UK 冲突（supplier + period 已存在）→ ErrSettlementConflict；
//     无可结算订单 → ErrNoOrdersToSettle。
//   - MarkPaid：调 repo.MarkPaid 事务（标 paid + 扣 supplier balance）；
//     已 paid → ErrSettlementAlreadyPaid；余额不够 → ErrInsufficientSupplierBalance。
//
// 手续费率口径（design D13）：supplier.commission_rate ?? config.mall.default_commission_rate；
// commission_rate 在生成结算单时**快照**到本单 record，避免供应商后续调整 commission_rate 影响历史单。
//
// 金额精度：单价 / 佣金 / 打款额统一保留 2 位小数（math.Round + 四舍五入），避免 long-term 浮点累积误差。
package admin

import (
	"context"
	"errors"
	"log"
	"math"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"ai-go-mall/internal/domain/state"
	"ai-go-mall/internal/infra/channel"
	"ai-go-mall/internal/infra/config"
	"ai-go-mall/internal/model/mall"
	"ai-go-mall/internal/repository"
	mallRepo "ai-go-mall/internal/repository/mall"
	supplierRepo "ai-go-mall/internal/repository/supplier"
)

// 业务 sentinel error。
//
// 设计：service 层直接 re-export repo 包的同名 var，避免 handler 既 import repo 又 import service
// 才能识别错误（统一从 service 包暴露）。ErrNoOrdersToSettle 是 service 层独有，repo 没有。
var (
	// ErrNoOrdersToSettle 周期内没有可结算订单（Generate 时碰到）。
	ErrNoOrdersToSettle = errors.New("admin.settlement: no orders to settle")

	// ErrSettlementConflict 同 supplier + 同期 已存在结算单（UK 冲突 → handler 映射 409）。
	ErrSettlementConflict = mallRepo.ErrSettlementConflict

	// ErrSettlementAlreadyPaid 二次 mark_paid（→ 409）。
	ErrSettlementAlreadyPaid = mallRepo.ErrSettlementAlreadyPaid

	// ErrInsufficientSupplierBalance supplier.balance < payout_amount（MarkPaid → 422）。
	ErrInsufficientSupplierBalance = mallRepo.ErrInsufficientSupplierBalance

	// ErrSettlementNotFound settlement 不存在 / 已软删。
	ErrSettlementNotFound = mallRepo.ErrSettlementNotFound

	// ErrInvalidPeriod period 非法（end <= start）。
	ErrInvalidPeriod = errors.New("admin.settlement: invalid period (end must be > start)")

	// ErrSupplierNotFound 供应商不存在 / 已软删。
	ErrSupplierNotFound = errors.New("admin.settlement: supplier not found")
)

// PreviewSettlement 预览返回结构（handler 序列化为 JSON）。
type PreviewSettlement struct {
	// SupplierID 供应商 ID。
	SupplierID int64 `json:"supplier_id"`

	// SupplierName 供应商名称（方便前端预览页直接显示）。
	SupplierName string `json:"supplier_name"`

	// PeriodStart 周期起始。
	PeriodStart time.Time `json:"period_start"`

	// PeriodEnd 周期结束（不含）。
	PeriodEnd time.Time `json:"period_end"`

	// CommissionRate 适用手续费率（4 位小数）。
	CommissionRate float64 `json:"commission_rate"`

	// OrderCount 周期内可结算订单数。
	OrderCount int `json:"order_count"`

	// TotalAmount 周期总 GMV。
	TotalAmount float64 `json:"total_amount"`

	// CommissionAmount 平台应收手续费。
	CommissionAmount float64 `json:"commission_amount"`

	// PayoutAmount 应打款额 = TotalAmount - CommissionAmount。
	PayoutAmount float64 `json:"payout_amount"`
}

// SettlementService admin 视角的结算单管理接口。
type SettlementService interface {
	// List 拉全量结算单（按 ID DESC）。
	List(c *gin.Context, opts repository.ListOptions) (items []mall.MallSettlement, total int64, err error)

	// Detail 拿 settlement + 全部 items。
	//
	// settlement 不存在 → ErrSettlementNotFound。
	Detail(c *gin.Context, id int64) (*mall.MallSettlement, []mall.MallSettlementItem, error)

	// Preview 不写库，列某 supplier + 周期内可结算订单 + 算金额。
	//
	// 校验：supplier 必须存在；periodStart < periodEnd。
	// 注意：Preview 不检查 UK 冲突，admin 在 Generate 时才知道是否真的冲突。
	Preview(c *gin.Context, supplierID int64, periodStart, periodEnd time.Time) (*PreviewSettlement, error)

	// Generate 事务：列 eligible orders + 算金额 + INSERT settlement + items。
	//
	// 校验：
	//   - supplier 必须存在 → ErrSupplierNotFound；
	//   - period 必须合法 → ErrInvalidPeriod；
	//   - 无可结算订单 → ErrNoOrdersToSettle；
	//   - UK (supplier + period) 冲突 → ErrSettlementConflict。
	Generate(c *gin.Context, supplierID int64, periodStart, periodEnd time.Time) (*mall.MallSettlement, error)

	// MarkPaid 把结算单标 paid（事务内扣 payout_amount）。
	//
	// 校验：adminID 必须 > 0；settlementID 必须 > 0 且存在。
	// 错误：ErrSettlementNotFound / ErrSettlementAlreadyPaid / ErrInsufficientSupplierBalance。
	MarkPaid(c *gin.Context, settlementID int64, adminID int64) error
}

// SettlementServiceDeps 注入 SettlementService 所需的依赖。
type SettlementServiceDeps struct {
	SettlementRepo mallRepo.SettlementRepository
	SupplierRepo   supplierRepo.SupplierRepository
	// DrawOrderRepo 仅供未来扩展（如按订单维度反向查 settlement）；当前未用，预留。
	DrawOrderRepo mallRepo.DrawOrderRepository

	// Channels 渠道工厂（D18）：MarkPaid 成功后经 "bank" 渠道记录打款。
	// MVP 是 mock/占位 driver（仅记录）；nil 时跳过（兼容旧单测 / 缺依赖装配）。
	Channels channel.ChannelFactory
}

// baseSettlementService 是 SettlementService 的默认实现。
type baseSettlementService struct {
	repo     mallRepo.SettlementRepository
	supplier supplierRepo.SupplierRepository
	order    mallRepo.DrawOrderRepository
	channels channel.ChannelFactory
}

// NewSettlementService 接收依赖，返回 SettlementService 接口。
func NewSettlementService(d SettlementServiceDeps) SettlementService {
	return &baseSettlementService{
		repo:     d.SettlementRepo,
		supplier: d.SupplierRepo,
		order:    d.DrawOrderRepo,
		channels: d.Channels,
	}
}

// List 实现见 SettlementService 注释。
//
// Page / PageSize 由 handler 校验；service 层只负责透传到 repo。
func (s *baseSettlementService) List(c *gin.Context, opts repository.ListOptions) ([]mall.MallSettlement, int64, error) {
	return s.repo.List(c, opts)
}

// Detail 实现见 SettlementService 注释。
func (s *baseSettlementService) Detail(c *gin.Context, id int64) (*mall.MallSettlement, []mall.MallSettlementItem, error) {
	if id <= 0 {
		return nil, nil, ErrSettlementNotFound
	}
	settlement, items, err := s.repo.GetByIDWithItems(c, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, ErrSettlementNotFound
		}
		return nil, nil, err
	}
	return settlement, items, nil
}

// Preview 实现见 SettlementService 注释。
func (s *baseSettlementService) Preview(c *gin.Context, supplierID int64, periodStart, periodEnd time.Time) (*PreviewSettlement, error) {
	sup, rate, err := s.loadSupplierAndRate(c, supplierID)
	if err != nil {
		return nil, err
	}
	if !periodEnd.After(periodStart) {
		return nil, ErrInvalidPeriod
	}

	orders, err := s.repo.ListEligibleOrders(c, supplierID, periodStart, periodEnd)
	if err != nil {
		return nil, err
	}

	total, commission, payout := computeSettlementTotals(orders, rate)
	return &PreviewSettlement{
		SupplierID:       supplierID,
		SupplierName:     sup.Name,
		PeriodStart:      periodStart,
		PeriodEnd:        periodEnd,
		CommissionRate:   rate,
		OrderCount:       len(orders),
		TotalAmount:      total,
		CommissionAmount: commission,
		PayoutAmount:     payout,
	}, nil
}

// Generate 实现见 SettlementService 注释。
func (s *baseSettlementService) Generate(c *gin.Context, supplierID int64, periodStart, periodEnd time.Time) (*mall.MallSettlement, error) {
	_, rate, err := s.loadSupplierAndRate(c, supplierID)
	if err != nil {
		return nil, err
	}
	if !periodEnd.After(periodStart) {
		return nil, ErrInvalidPeriod
	}

	orders, err := s.repo.ListEligibleOrders(c, supplierID, periodStart, periodEnd)
	if err != nil {
		return nil, err
	}
	if len(orders) == 0 {
		return nil, ErrNoOrdersToSettle
	}

	total, commission, payout := computeSettlementTotals(orders, rate)
	items := make([]mall.MallSettlementItem, 0, len(orders))
	for _, o := range orders {
		// 单订单按 supplier rate 算（一致口径，避免 sum 单笔后再 round 导致 1 分误差）。
		orderCommission := roundHalfUp(o.Price*rate, 2)
		orderPayout := roundHalfUp(o.Price-orderCommission, 2)
		items = append(items, mall.MallSettlementItem{
			DrawOrderID:      o.ID,
			Amount:           roundHalfUp(o.Price, 2),
			CommissionAmount: orderCommission,
			PayoutAmount:     orderPayout,
		})
	}

	settlement := &mall.MallSettlement{
		SupplierID:       supplierID,
		PeriodStart:      periodStart,
		PeriodEnd:        periodEnd,
		TotalAmount:      total,
		CommissionRate:   rate,
		CommissionAmount: commission,
		PayoutAmount:     payout,
		Status:           mall.SettlementStatusPending,
	}
	if err := s.repo.Generate(c, settlement, items); err != nil {
		// repo 已识别 UK 冲突 → ErrSettlementConflict，原样上抛。
		return nil, err
	}

	// 状态机推进 pending → processing（Generate 完成后立即生效）。
	// 校验失败（编程错误）→ abort；正常路径上 pending → processing 永远合法。
	if _, err := state.Transition(state.SettlementStates.Pending, state.SettlementStates.Processing, &state.Context{Entity: settlement}); err != nil {
		return nil, err
	}
	if err := s.repo.UpdateStatus(c, settlement.ID, mall.SettlementStatusProcessing); err != nil {
		return nil, err
	}
	settlement.Status = mall.SettlementStatusProcessing
	return settlement, nil
}

// MarkPaid 实现见 SettlementService 注释。
func (s *baseSettlementService) MarkPaid(c *gin.Context, settlementID int64, adminID int64) error {
	if adminID <= 0 {
		return errors.New("admin.settlement: invalid admin id")
	}
	if settlementID <= 0 {
		return ErrSettlementNotFound
	}

	// 先查 settlement 拿 PayoutAmount + 当前 status；MarkPaid 事务需要这两个值。
	existing, err := s.repo.GetByID(c, settlementID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrSettlementNotFound
		}
		return err
	}
	if existing == nil {
		return ErrSettlementNotFound
	}

	// 状态机校验：processing → paid 是唯一合法路径。
	//   pending  → paid 拒绝（必须先经 Generate 走到 processing）；
	//   paid     → paid 拒绝（已打款，二次 mark_paid）；
	//   failed   → paid 拒绝（终态）。
	// 任何非法转换 → ErrInvalidStateTransition（handler 映射 HTTP 409）。
	currentState, ok := mapSettlementStatusToState(existing.Status)
	if !ok {
		return ErrSettlementNotFound
	}
	if _, err := state.Transition(currentState, state.SettlementStates.Paid, &state.Context{Entity: existing}); err != nil {
		return err
	}

	if err := s.repo.MarkPaid(c, settlementID, adminID, existing.PayoutAmount); err != nil {
		return err
	}

	// 渠道层记录打款（D18；MarkPaid 事务已 COMMIT，失败仅 warn，不回滚业务）。
	// MVP 的 bank driver 是 mock 占位；未来接入真实银行代发时此处即打款调用点。
	s.recordPayout(c.Request.Context(), existing, adminID)
	return nil
}

// recordPayout 经渠道工厂记录一笔打款（fire-and-forget）。
//
// 幂等：MarkPaid 事务本身幂等（二次调用被状态机拒绝），渠道调用只发生在
// MarkPaid 成功之后，同一结算单不会重复记录。
func (s *baseSettlementService) recordPayout(ctx context.Context, settlement *mall.MallSettlement, adminID int64) {
	if s.channels == nil || settlement == nil {
		return
	}
	ch, err := s.channels.Create("bank")
	if err != nil {
		log.Printf("admin.settlement: create bank channel failed: %v", err)
		return
	}
	if _, err := ch.Payout(ctx, &channel.PayoutRequest{
		SupplierID:   settlement.SupplierID,
		SettlementID: settlement.ID,
		Amount:       settlement.PayoutAmount,
		Remark:       "settlement mark_paid by admin",
	}); err != nil {
		log.Printf("admin.settlement: payout channel record failed (settlement %d): %v", settlement.ID, err)
	}
}

// mapSettlementStatusToState 把 mall.MallSettlementStatus 映射到 state.SettlementState。
//
// 未识别状态（如未来扩展加入的新状态值未同步到状态机）→ return false，
// 由调用方决定如何处理（保守起见当 ErrSettlementNotFound 返回，避免脏数据被打款）。
func mapSettlementStatusToState(s mall.MallSettlementStatus) (state.SettlementState, bool) {
	switch s {
	case mall.SettlementStatusPending:
		return state.SettlementStates.Pending, true
	case mall.SettlementStatusProcessing:
		return state.SettlementStates.Processing, true
	case mall.SettlementStatusPaid:
		return state.SettlementStates.Paid, true
	case mall.SettlementStatusFailed:
		return state.SettlementStates.Failed, true
	}
	return state.SettlementState{}, false
}

// loadSupplierAndRate 加载 supplier 并解析 commission_rate（supplier 优先，NULL 走 config 默认）。
//
// 返回的 rate 是 dereference 后的 float64，避免调用方还要 nil 检查。
func (s *baseSettlementService) loadSupplierAndRate(c *gin.Context, supplierID int64) (*mall.MallSupplier, float64, error) {
	if supplierID <= 0 {
		return nil, 0, ErrSupplierNotFound
	}
	sup, err := s.supplier.GetByID(c, supplierID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, 0, ErrSupplierNotFound
		}
		return nil, 0, err
	}
	if sup == nil {
		return nil, 0, ErrSupplierNotFound
	}
	if sup.CommissionRate != nil {
		return sup, *sup.CommissionRate, nil
	}
	defaultRate := config.Get().Mall.DefaultCommissionRate
	return sup, defaultRate, nil
}

// computeSettlementTotals 汇总周期内订单的金额 + 佣金 + 打款额。
//
// 算法：每订单按 price * rate 算 commission（round to 2 decimals）；
// sum 单价得 totalAmount，sum commission 得 commissionAmount，payout = total - commission。
// 这样 totalAmount=∑price 与 commissionAmount=∑commission 都各自 round，避免「整体 round vs 分笔 round」差异。
func computeSettlementTotals(orders []mall.MallDrawOrder, rate float64) (total, commission, payout float64) {
	for _, o := range orders {
		p := roundHalfUp(o.Price, 2)
		c := roundHalfUp(o.Price*rate, 2)
		po := roundHalfUp(p-c, 2)
		total += p
		commission += c
		payout += po
	}
	total = roundHalfUp(total, 2)
	commission = roundHalfUp(commission, 2)
	payout = roundHalfUp(payout, 2)
	return
}

// roundHalfUp 四舍五入到 N 位小数（half-away-from-zero）。
//
// math.Round 默认 half-to-even（银行家舍入），业务场景下用 half-up 更直观
// （财务人员熟悉的"四舍五入"语义）；用 Floor + 0.5 实现，decimals ≤ 6 时精度足够。
func roundHalfUp(v float64, decimals int) float64 {
	shift := math.Pow(10, float64(decimals))
	if v >= 0 {
		return math.Floor(v*shift+0.5) / shift
	}
	return math.Ceil(v*shift-0.5) / shift
}

// 编译期断言。
var _ SettlementService = (*baseSettlementService)(nil)
