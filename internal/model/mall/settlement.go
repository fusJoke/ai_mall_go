package mall

import (
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

// MallSettlementStatus 结算单状态枚举。
// 状态机规则由 internal/domain/state 状态机约束（Phase 14）：
//
//	pending → processing → paid
//	       ↘ failed（任意阶段兜底）
type MallSettlementStatus string

// 结算单状态枚举值。
const (
	SettlementStatusPending    MallSettlementStatus = "pending"    // 待生成（admin 触发前）
	SettlementStatusProcessing MallSettlementStatus = "processing" // 已生成待打款
	SettlementStatusPaid       MallSettlementStatus = "paid"       // 已打款（终态）
	SettlementStatusFailed     MallSettlementStatus = "failed"     // 失败（终态）
)

// MallSettlement 结算单主表（按周期聚合供应商收入）。
//
// 关键设计：
//   - (supplier_id, period_start, period_end) UK：防「同供应商 + 同期」重复结算。
//   - commission_rate 快照：避免后续供应商调整 commission_rate 影响历史单。
//   - paid_at / paid_by：admin mark_paid 时记录（paid_by = admin.id）。
//
// 资金流（设计 D13 / D14）：
//
//	抽卡 → supplier.balance += price
//	Generate 结算单 → 算 commission + payout（不扣 supplier.balance）
//	MarkPaid → supplier.balance -= payout_amount
type MallSettlement struct {
	// ID 结算单主键。
	ID int64 `json:"id" gorm:"comment:ID;primaryKey;autoIncrement"`

	// SupplierID 供应商 ID。
	SupplierID int64 `json:"supplier_id" gorm:"comment:供应商ID;not null;uniqueIndex:idx_mall_settlements_supplier_period,priority:1"`

	// PeriodStart 结算周期起始时间。
	PeriodStart time.Time `json:"period_start" gorm:"comment:结算周期起始;not null;uniqueIndex:idx_mall_settlements_supplier_period,priority:2"`

	// PeriodEnd 结算周期结束时间。
	PeriodEnd time.Time `json:"period_end" gorm:"comment:结算周期结束;not null;uniqueIndex:idx_mall_settlements_supplier_period,priority:3"`

	// TotalAmount 周期总 GMV（元）。
	TotalAmount float64 `json:"total_amount" gorm:"comment:周期总GMV(元);type:decimal(14,2);not null;default:0"`

	// CommissionRate 本单手续费率快照（4 位小数，例 0.1000 = 10%）。
	CommissionRate float64 `json:"commission_rate" gorm:"comment:本单手续费率;type:decimal(5,4);not null"`

	// CommissionAmount 手续费（元）。
	CommissionAmount float64 `json:"commission_amount" gorm:"comment:手续费(元);type:decimal(14,2);not null;default:0"`

	// PayoutAmount 应打款额（元）= total_amount - commission_amount。
	PayoutAmount float64 `json:"payout_amount" gorm:"comment:应打款额(元);type:decimal(14,2);not null;default:0"`

	// Status 状态：SettlementStatus*。
	Status MallSettlementStatus `json:"status" gorm:"comment:状态(pending/processing/paid/failed);size:16;default:'pending';not null;index:idx_mall_settlements_status_created,priority:1"`

	// PaidAt 打款时间；nil 表示未打款。
	PaidAt *time.Time `json:"paid_at" gorm:"comment:打款时间"`

	// PaidBy 打款人（admin.id）；nil 表示未打款。
	PaidBy *int64 `json:"paid_by" gorm:"comment:打款人"`

	// Remark 备注。
	Remark string `json:"remark" gorm:"comment:备注;size:500"`

	// UpdatedAt 更新时间（GORM 自动维护）。
	UpdatedAt time.Time `json:"updated_at" gorm:"comment:更新时间;not null;autoUpdateTime"`

	// CreatedAt 创建时间（GORM 自动维护）。
	CreatedAt time.Time `json:"created_at" gorm:"comment:创建时间;not null;autoCreateTime;index:idx_mall_settlements_status_created,priority:2"`

	// DeletedAt 软删除标记（GORM 自动维护；Valid=true 即已删除）。
	DeletedAt gorm.DeletedAt `json:"deleted_at" gorm:"comment:删除时间;index"`
}

// TableName 让 GORM 把 "mall_settlements" 交给 namer 处理。
func (MallSettlement) TableName(namer schema.Namer) string {
	return namer.TableName("mall_settlements")
}

// MallSettlementItem 结算明细（一订单一行）。
//
// 关键设计：
//   - draw_order_id UK：一订单只可被结算一次。
//   - amount / commission_amount / payout_amount 均为快照：
//     结算单生成时的金额落表，不随订单后续变更追溯。
type MallSettlementItem struct {
	// ID 明细主键。
	ID int64 `json:"id" gorm:"comment:ID;primaryKey;autoIncrement"`

	// SettlementID 所属结算单 ID。
	SettlementID int64 `json:"settlement_id" gorm:"comment:结算单ID;not null;index:idx_mall_settlement_items_settlement"`

	// DrawOrderID 抽卡订单 ID（UK）。
	DrawOrderID int64 `json:"draw_order_id" gorm:"comment:抽卡订单ID;not null;uniqueIndex:idx_mall_settlement_items_draw_order"`

	// Amount 订单金额（元，decimal(10,2)）。
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
	DeletedAt gorm.DeletedAt `json:"deleted_at" gorm:"comment:删除时间;index"`
}

// TableName 让 GORM 把 "mall_settlement_items" 交给 namer 处理。
func (MallSettlementItem) TableName(namer schema.Namer) string {
	return namer.TableName("mall_settlement_items")
}

// MallPlatformLedger 平台收入台账（counter 聚合行）。
//
// 关键设计：
//   - counter_key 主键：每条 counter 占一行，用 ON DUPLICATE KEY UPDATE 累加，
//     O(1) 写入 / O(1) 读取（设计 D14）。
//   - 当前已落地的 counter：
//     total_revenue —— 累计抽卡 GMV
//     total_commission —— 累计平台手续费
//
// 字段顺序：业务字段在前、UpdatedAt → CreatedAt 收尾（无 DeletedAt —— counter 行不软删）。
type MallPlatformLedger struct {
	// CounterKey 计数器键（如 total_revenue / total_commission）。
	CounterKey string `json:"counter_key" gorm:"comment:计数器键;size:64;primaryKey"`

	// Amount 聚合金额（decimal(20,2)，上限远超业务范围）。
	Amount float64 `json:"amount" gorm:"comment:聚合金额;type:decimal(20,2);not null;default:0"`

	// UpdatedAt 更新时间（GORM 自动维护）。
	UpdatedAt time.Time `json:"updated_at" gorm:"comment:更新时间;not null;autoUpdateTime"`

	// CreatedAt 创建时间（GORM 自动维护）。
	CreatedAt time.Time `json:"created_at" gorm:"comment:创建时间;not null;autoCreateTime"`
}

// TableName 让 GORM 把 "mall_platform_ledger" 交给 namer 处理。
func (MallPlatformLedger) TableName(namer schema.Namer) string {
	return namer.TableName("mall_platform_ledger")
}
