package mall

import (
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

// MallSettlementBillStatus 批次账单状态枚举。
// 状态机规则由 internal/domain/state 约束（openspec: add-batch-settlement）：
//
//	generated → confirmed → paid
//	generated → rejected → generated（admin 重核，version+1）
//	confirmed → failed → confirmed（打款失败修复重试）
type MallSettlementBillStatus string

// 批次账单状态枚举值。
const (
	BillStatusGenerated MallSettlementBillStatus = "generated" // 售罄自动生成·待供应商确认
	BillStatusConfirmed MallSettlementBillStatus = "confirmed" // 供应商已确认·待打款
	BillStatusRejected  MallSettlementBillStatus = "rejected"  // 供应商驳回（附原因）
	BillStatusFailed    MallSettlementBillStatus = "failed"    // 打款失败（可修复重试）
	BillStatusPaid      MallSettlementBillStatus = "paid"      // 已打款（终态）
)

// MallSettlementBill 批次结算账单主表（1 商品 = 1 账单，openspec: add-batch-settlement）。
//
// 关键设计（商品即批次）：
//   - blind_box_id UK：一商品一账单；商品生命周期 sold_out 时自动生成，
//     账单 paid 与商品 lifecycle=settled 同事务落库。
//   - supplier_id 冗余：账单中心/管理页免 join。
//   - commission_rate 快照：供应商后续调费率不影响历史账单。
//   - version：供应商驳回 → admin 重新核算 → version+1 回 generated，金额可修。
//
// 资金流（与周期结算同模式，设计 D13/D14）：
//   - 抽卡 → supplier.balance += price（现有）
//   - MarkPaid → supplier.balance -= payout_amount（复用现有逻辑）
type MallSettlementBill struct {
	// ID 账单主键。
	ID int64 `json:"id" gorm:"comment:ID;primaryKey;autoIncrement"`

	// BlindBoxID 商品 ID（UK，1 商品 1 账单）。
	BlindBoxID int64 `json:"blind_box_id" gorm:"comment:商品ID(1商品1账单);not null;uniqueIndex:idx_mall_settlement_bills_blind_box"`

	// SupplierID 供应商 ID（冗余，免 join）。
	SupplierID int64 `json:"supplier_id" gorm:"comment:供应商ID(冗余);not null;uniqueIndex:idx_mall_settlement_bills_supplier_status,priority:1"`

	// TotalAmount 本批总 GMV（元）= 该商品全部 drawn 订单金额之和。
	TotalAmount float64 `json:"total_amount" gorm:"comment:本批总GMV(元);type:decimal(14,2);not null;default:0"`

	// CommissionRate 手续费率快照（4 位小数，例 0.1000 = 10%）。
	CommissionRate float64 `json:"commission_rate" gorm:"comment:本单手续费率(快照);type:decimal(5,4);not null"`

	// CommissionAmount 手续费（元）。
	CommissionAmount float64 `json:"commission_amount" gorm:"comment:手续费(元);type:decimal(14,2);not null;default:0"`

	// PayoutAmount 应打款额（元）= total_amount - commission_amount。
	PayoutAmount float64 `json:"payout_amount" gorm:"comment:应打款额(元);type:decimal(14,2);not null;default:0"`

	// Status 状态：MallSettlementBillStatus*。
	Status MallSettlementBillStatus `json:"status" gorm:"comment:状态(generated/confirmed/rejected/failed/paid);size:16;default:'generated';not null;uniqueIndex:idx_mall_settlement_bills_supplier_status,priority:2;index:idx_mall_settlement_bills_status_created,priority:1"`

	// RejectReason 驳回原因（供应商驳回时必填）。
	RejectReason string `json:"reject_reason" gorm:"comment:驳回原因(供应商驳回时必填);size:500;not null;default:''"`

	// Version 重核版本号（rejected 重核后 +1）。
	Version int `json:"version" gorm:"comment:重核版本号(rejected重核后+1);not null;default:1"`

	// ConfirmedAt 供应商确认时间；nil 表示未确认。
	ConfirmedAt *time.Time `json:"confirmed_at" gorm:"comment:供应商确认时间"`

	// PaidAt 打款时间；nil 表示未打款。
	PaidAt *time.Time `json:"paid_at" gorm:"comment:打款时间"`

	// PaidBy 打款人（admin.id）；nil 表示未打款。
	PaidBy *int64 `json:"paid_by" gorm:"comment:打款人"`

	// Remark 备注（admin 重核说明等）。
	Remark string `json:"remark" gorm:"comment:备注(admin重核说明等);size:500"`

	// UpdatedAt 更新时间（GORM 自动维护）。
	UpdatedAt time.Time `json:"updated_at" gorm:"comment:更新时间;not null;autoUpdateTime;index:idx_mall_settlement_bills_supplier_status,priority:3"`

	// CreatedAt 创建时间（GORM 自动维护）。
	CreatedAt time.Time `json:"created_at" gorm:"comment:创建时间;not null;autoCreateTime;index:idx_mall_settlement_bills_status_created,priority:2;index:idx_mall_settlement_bills_supplier_status,priority:4"`

	// DeletedAt 软删除标记（GORM 自动维护；Valid=true 即已删除）。
	DeletedAt gorm.DeletedAt `json:"deleted_at" gorm:"comment:删除时间;index:idx_mall_settlement_bills_deleted_at"`
}

// TableName 让 GORM 把 "mall_settlement_bills" 交给 namer 处理。
func (MallSettlementBill) TableName(namer schema.Namer) string {
	return namer.TableName("mall_settlement_bills")
}

// MallSettlementBillItem 批次账单明细（一订单一行，金额快照）。
//
// 关键设计：
//   - draw_order_id UK：一订单只可被结算一次（与周期结算 items 同规则；
//     两表并存、按商品/按周期互斥使用，同一订单不会两边重复结算）。
//   - 金额三字段均为快照：账单生成时的金额落表，不随订单后续变更追溯。
type MallSettlementBillItem struct {
	// ID 明细主键。
	ID int64 `json:"id" gorm:"comment:ID;primaryKey;autoIncrement"`

	// BillID 所属账单 ID。
	BillID int64 `json:"bill_id" gorm:"comment:账单ID;not null;index:idx_mall_settlement_bill_items_bill"`

	// DrawOrderID 抽卡订单 ID（UK）。
	DrawOrderID int64 `json:"draw_order_id" gorm:"comment:抽卡订单ID;not null;uniqueIndex:idx_mall_settlement_bill_items_draw_order"`

	// Amount 订单金额（元）。
	Amount float64 `json:"amount" gorm:"comment:订单金额(元);type:decimal(10,2);not null;default:0"`

	// CommissionAmount 本订单手续费（元）。
	CommissionAmount float64 `json:"commission_amount" gorm:"comment:手续费(元);type:decimal(10,2);not null;default:0"`

	// PayoutAmount 本订单应打款额（元）= amount - commission_amount。
	PayoutAmount float64 `json:"payout_amount" gorm:"comment:应打款额(元);type:decimal(10,2);not null;default:0"`

	// UpdatedAt 更新时间（GORM 自动维护）。
	UpdatedAt time.Time `json:"updated_at" gorm:"comment:更新时间;not null;autoUpdateTime"`

	// CreatedAt 创建时间（GORM 自动维护）。
	CreatedAt time.Time `json:"created_at" gorm:"comment:创建时间;not null;autoCreateTime"`

	// DeletedAt 软删除标记（GORM 自动维护；Valid=true 即已删除）。
	DeletedAt gorm.DeletedAt `json:"deleted_at" gorm:"comment:删除时间;index:idx_mall_settlement_bill_items_deleted_at"`
}

// TableName 让 GORM 把 "mall_settlement_bill_items" 交给 namer 处理。
func (MallSettlementBillItem) TableName(namer schema.Namer) string {
	return namer.TableName("mall_settlement_bill_items")
}
