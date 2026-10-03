// Package channel 提供第三方渠道（支付 / 银行打款 / 短信）的统一抽象与多驱动实现。
//
// 设计（mall MVP 设计 D18）：
//   - Channel 接口：Pay / Payout / Notify 三个操作覆盖业务用例。
//   - driver 与包同目录（mock.go / bank.go / 未来 wechat.go / alipay.go），
//     文件名即 driver 名；drivers 注册表见 factory.go。
//   - ChannelFactory 按 name 创建 driver，业务代码不感知具体实现。
//   - Init() 读 config.Get().Channel 校验三个渠道名可实例化（fail fast）。
//
// 业务动机：
//   - MVP 抽卡"模拟支付"、结算"手动 mark_paid"、短信未启用 → 全走 mock；
//   - 未来对接真实支付（微信 / 支付宝）、银行打款 API、短信下发时，
//     仅新增 driver + 改 config/channel.yaml，业务代码不动。
//
// MVP 不做的事（D18）：
//   - 真实支付（依然余额模拟扣款）/ 真实银行 API（依然手动 mark_paid）/ 真实短信。
package channel

import (
	"context"
	"errors"
	"time"
)

// ErrUnknownChannel 未知渠道名（factory.Create 的 name 不在 drivers 注册表）。
//
// 业务侧处理：500 + 错误日志（spec Requirement "Third-party channel adapter and factory"）。
var ErrUnknownChannel = errors.New("channel: unknown channel name")

// PayRequest 是支付请求（未来真实支付的收银参数；MVP 由 mock 记录）。
type PayRequest struct {
	// OrderNo 业务订单号（mall_draw_orders.order_no）。
	OrderNo string

	// UserID 支付用户 ID。
	UserID int64

	// Amount 支付金额（元）。
	Amount float64

	// Subject 商品描述（渠道账单展示用）。
	Subject string
}

// PayResult 是支付结果。
type PayResult struct {
	// Status 渠道状态："success" / "pending" / "failed"（由 driver 定义取值）。
	Status string

	// TradeNo 渠道流水号（对账用；mock 为 mock-pay-{uuid}）。
	TradeNo string

	// PaidAt 渠道完成时间。
	PaidAt time.Time
}

// PayoutRequest 是打款（出账）请求（结算 mark_paid 的未来银行代发参数）。
type PayoutRequest struct {
	// SupplierID 收款供应商 ID。
	SupplierID int64

	// SettlementID 关联结算单 ID（对账溯源）。
	SettlementID int64

	// Amount 打款金额（元）。
	Amount float64

	// Remark 备注（银行回单展示）。
	Remark string
}

// PayoutResult 是打款结果。
type PayoutResult struct {
	// Status 渠道状态："success" / "pending" / "failed"。
	Status string

	// PayoutNo 渠道打款流水号（mock 为 mock-payout-{uuid}）。
	PayoutNo string

	// PaidAt 渠道完成时间。
	PaidAt time.Time
}

// NotifyRequest 是渠道回调 / 通知请求（未来支付回调处理；MVP 由 mock 记录）。
type NotifyRequest struct {
	// Topic 通知主题（如 "payment.callback"）。
	Topic string

	// Payload 通知正文（driver 自行解析）。
	Payload []byte
}

// Channel 是第三方渠道的对外门面接口。
//
// 三个操作的语义：
//   - Pay：收钱（用户支付订单）；
//   - Payout：出钱（平台向供应商打款）；
//   - Notify：下发通知 / 回调（短信、webhook 等）。
//
// 实现 要求：driver 必须幂等可重试（同一 OrderNo / SettlementID 重复调用
// 不产生重复扣款），真实 driver 落地时以渠道流水号做幂等键。
type Channel interface {
	// Pay 发起支付。
	Pay(ctx context.Context, req *PayRequest) (*PayResult, error)

	// Payout 发起打款。
	Payout(ctx context.Context, req *PayoutRequest) (*PayoutResult, error)

	// Notify 下发通知。
	Notify(ctx context.Context, req *NotifyRequest) error
}
