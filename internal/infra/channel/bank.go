// bank.go — 银行打款渠道占位实现（MVP = 委托 mock）。
//
// D18：真实银行代发 API（如银企直连 / 网商银行）MVP 不做，结算仍走
// admin 手动 mark_paid；本文件保留 driver 槽位，让 config/channel.yaml 的
// channel.bank 配置项从第一天起就是可解析、可启动的。
//
// TODO(真实银行 API)：
//  1. 接入银行开放平台 SDK（企业网银付款 / 代发工资接口）；
//  2. Payout 改为真实调用，幂等键用 SettlementID（重复打款防护）；
//  3. PayoutResult.Status 映射银行受理状态（success / pending / failed）；
//  4. Notify 处理银行异步回执（打款成功 / 退票）。
package channel

import "context"

// BankChannel 是银行打款渠道的占位实现：全部操作委托 MockChannel。
//
// 为什么不是 nil / panic：结算 service 的集成调用点（MarkPaid 后记 Payout）
// 对 "bank" 渠道真实可用；委托 mock 保证调用链路从第一天就跑通，
// 未来替换实现时业务代码零改动。
type BankChannel struct {
	mock *MockChannel
}

// NewBankChannel 返回 *BankChannel（drivers 注册表入口）。
func NewBankChannel() Channel {
	return &BankChannel{mock: &MockChannel{}}
}

// Pay 委托 mock（银行渠道不承担收单职责；保留接口完整性）。
func (b *BankChannel) Pay(ctx context.Context, req *PayRequest) (*PayResult, error) {
	return b.mock.Pay(ctx, req)
}

// Payout 委托 mock 并返回成功（MVP 手动 mark_paid，无真实打款）。
func (b *BankChannel) Payout(ctx context.Context, req *PayoutRequest) (*PayoutResult, error) {
	return b.mock.Payout(ctx, req)
}

// Notify 委托 mock。
func (b *BankChannel) Notify(ctx context.Context, req *NotifyRequest) error {
	return b.mock.Notify(ctx, req)
}

// Calls 透出内部 mock 的调用记录（单测断言 bank 渠道路径用）。
func (b *BankChannel) Calls() []MockCall {
	return b.mock.Calls()
}
