// Package supplier — bill.go 实现 B 端供应商对自家批次账单的操作。
//
// 业务规则（openspec: add-batch-settlement）：
//   - List：仅返回 supplier_id=当前 supplier 的账单，可按 status 筛选（账单中心 tab）。
//   - Detail：账单 + 明细；ownership 自检（bill.SupplierID == supplierID，否则 403）。
//   - Confirm：generated → confirmed（供应商核对无误，等待 admin 打款）。
//   - Reject：generated → rejected，必填原因（金额有误等；admin 重核后回 generated）。
//
// 状态合法性由 internal/domain/state 状态机 fail-fast 校验；CAS UPDATE 由
// repository 落库，影响 0 行 = 并发抢先 → ErrBillStatusConflict（409）。
package supplier

import (
	"errors"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"ai-go-mall/internal/domain/state"
	"ai-go-mall/internal/model/mall"
	"ai-go-mall/internal/repository"
	mallRepo "ai-go-mall/internal/repository/mall"
	mallSvc "ai-go-mall/internal/service/mall"
)

// 业务错误。
//
// ErrBillNotFound / ErrBillStatusConflict re-export 自 service/mall（handler 统一
// 从本包识别错误）；ErrBillForbidden / ErrRejectReasonRequired 是本包独有。
var (
	// ErrBillNotFound 账单不存在 / 已软删（→ 404）。
	ErrBillNotFound = mallSvc.ErrBillNotFound

	// ErrBillForbidden 越权 —— 账单不属于当前 supplier（→ 403）。
	// 与 product 同策略：B 端是登录态，「越权」可识别，不模糊成 404。
	ErrBillForbidden = errors.New("supplier.bill: bill not owned by current supplier")

	// ErrBillStatusConflict CAS 未命中：账单状态被并发改变（→ 409）。
	ErrBillStatusConflict = mallSvc.ErrBillStatusConflict

	// ErrRejectReasonRequired 驳回原因必填（→ 422）。
	ErrRejectReasonRequired = errors.New("supplier.bill: reject reason is required")
)

// BillService 是 B 端供应商的批次账单接口。
type BillService interface {
	// List 拉当前 supplier 的账单（按 id DESC），status 为空则不筛（全部 tab）。
	List(c *gin.Context, supplierID int64, status mall.MallSettlementBillStatus, opts repository.ListOptions) (items []mall.MallSettlementBill, total int64, err error)

	// Detail 拿账单 + 明细；不属于当前 supplier → ErrBillForbidden。
	Detail(c *gin.Context, supplierID, billID int64) (*mall.MallSettlementBill, []mall.MallSettlementBillItem, error)

	// Confirm 确认账单（generated → confirmed）。
	//
	// 只允许对自家账单操作；非 generated 态 → state.ErrInvalidStateTransition；
	// 并发抢先（CAS 0 行）→ ErrBillStatusConflict。
	Confirm(c *gin.Context, supplierID, billID int64) error

	// Reject 驳回账单（generated → rejected），reason 必填（trim 后非空）。
	// 原因写入 reject_reason，admin 重核（Regenerate）后回 generated 并清空。
	Reject(c *gin.Context, supplierID, billID int64, reason string) error
}

// BillServiceDeps 注入 BillService 所需的依赖。
type BillServiceDeps struct {
	BillRepo mallRepo.SettlementBillRepository
}

// baseBillService 是 BillService 的默认实现。
type baseBillService struct {
	bill mallRepo.SettlementBillRepository
}

// NewBillService 接收依赖，返回 BillService 接口。
func NewBillService(d BillServiceDeps) BillService {
	return &baseBillService{bill: d.BillRepo}
}

// List 实现见 BillService 注释。
func (s *baseBillService) List(c *gin.Context, supplierID int64, status mall.MallSettlementBillStatus, opts repository.ListOptions) ([]mall.MallSettlementBill, int64, error) {
	if supplierID <= 0 {
		return nil, 0, ErrBillNotFound
	}
	return s.bill.ListBySupplier(c, supplierID, status, opts)
}

// Detail 实现见 BillService 注释。
func (s *baseBillService) Detail(c *gin.Context, supplierID, billID int64) (*mall.MallSettlementBill, []mall.MallSettlementBillItem, error) {
	if supplierID <= 0 || billID <= 0 {
		return nil, nil, ErrBillNotFound
	}
	bill, items, err := s.bill.GetByIDWithItems(c, billID)
	if err != nil {
		return nil, nil, err
	}
	if bill == nil || bill.SupplierID != supplierID {
		return nil, nil, ErrBillForbidden
	}
	return bill, items, nil
}

// Confirm 实现见 BillService 注释。
func (s *baseBillService) Confirm(c *gin.Context, supplierID, billID int64) error {
	bill, err := s.getOwnedBill(c, supplierID, billID)
	if err != nil {
		return err
	}
	// 状态机：以账单当前实际状态为起点，generated → confirmed 是唯一合法路径
	// （confirmed/rejected/failed/paid 一律拒绝，不重读库靠预校验挡并发）。
	current, ok := mapBillStatusToState(bill.Status)
	if !ok {
		return ErrBillNotFound
	}
	if _, err := state.Transition(current, state.BillStates.Confirmed, &state.Context{Entity: bill}); err != nil {
		return err
	}
	rows, err := s.bill.UpdateStatus(c, billID, mall.BillStatusGenerated, mall.BillStatusConfirmed, nil)
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrBillStatusConflict
	}
	return nil
}

// Reject 实现见 BillService 注释。
func (s *baseBillService) Reject(c *gin.Context, supplierID, billID int64, reason string) error {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return ErrRejectReasonRequired
	}
	bill, err := s.getOwnedBill(c, supplierID, billID)
	if err != nil {
		return err
	}
	// 状态机：以账单当前实际状态为起点，generated → rejected 是唯一合法路径。
	current, ok := mapBillStatusToState(bill.Status)
	if !ok {
		return ErrBillNotFound
	}
	if _, err := state.Transition(current, state.BillStates.Rejected, &state.Context{Entity: bill}); err != nil {
		return err
	}
	rows, err := s.bill.UpdateStatus(c, billID, mall.BillStatusGenerated, mall.BillStatusRejected,
		map[string]any{"reject_reason": reason})
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrBillStatusConflict
	}
	return nil
}

// getOwnedBill 读账单并校验归属；三态合一：
// 不存在 → ErrBillNotFound（404）；supplierID 不匹配 → ErrBillForbidden（403）。
func (s *baseBillService) getOwnedBill(c *gin.Context, supplierID, billID int64) (*mall.MallSettlementBill, error) {
	if supplierID <= 0 || billID <= 0 {
		return nil, ErrBillNotFound
	}
	bill, err := s.bill.GetByID(c, billID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrBillNotFound
		}
		return nil, err
	}
	if bill == nil || bill.SupplierID != supplierID {
		return nil, ErrBillForbidden
	}
	return bill, nil
}

// mapBillStatusToState 把 mall.MallSettlementBillStatus 映射到 state.BillState。
//
// 未识别状态 → return false，调用方保守按 ErrBillNotFound 处理。
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

// 编译期断言。
var _ BillService = (*baseBillService)(nil)
