// mock.go — Mock 渠道实现（MVP 默认 driver）。
//
// 行为（D18）：所有操作返回成功并记录调用日志 —— 既给本地开发当"模拟支付"，
// 也给单测当可编程 stub（通过 Calls() / ResetCalls() 断言业务确实走了渠道层）。
//
// 线程安全：调用记录用 mutex 保护（业务侧可能在 goroutine 里调用）。
package channel

import (
	"context"
	"strconv"
	"sync"
	"time"

	"github.com/google/uuid"
)

// MockCall 是 mock driver 记录的一次调用。
type MockCall struct {
	// Op 操作名："pay" / "payout" / "notify"。
	Op string

	// Ref 业务引用号：pay = OrderNo，payout = SettlementID 的字符串，notify = Topic。
	Ref string

	// Amount 金额（notify 恒为 0）。
	Amount float64

	// At 调用时刻。
	At time.Time
}

// MockChannel 是 Channel 的 mock 实现。
type MockChannel struct {
	mu    sync.Mutex
	calls []MockCall
	now   func() time.Time // 可注入时钟（单测断言用）；nil 走 time.Now
}

// NewMockChannel 返回 *MockChannel（drivers 注册表入口）。
func NewMockChannel() Channel {
	return &MockChannel{}
}

// Pay 记录一次支付调用并返回成功结果。
func (m *MockChannel) Pay(ctx context.Context, req *PayRequest) (*PayResult, error) {
	if req == nil {
		req = &PayRequest{}
	}
	m.record("pay", req.OrderNo, req.Amount)
	return &PayResult{
		Status:  "success",
		TradeNo: "mock-pay-" + newMockUUID(),
		PaidAt:  m.nowFn(),
	}, nil
}

// Payout 记录一次打款调用并返回成功结果。
func (m *MockChannel) Payout(ctx context.Context, req *PayoutRequest) (*PayoutResult, error) {
	if req == nil {
		req = &PayoutRequest{}
	}
	m.record("payout", "settlement:"+strconv.FormatInt(req.SettlementID, 10), req.Amount)
	return &PayoutResult{
		Status:   "success",
		PayoutNo: "mock-payout-" + newMockUUID(),
		PaidAt:   m.nowFn(),
	}, nil
}

// Notify 记录一次通知调用并返回成功。
func (m *MockChannel) Notify(ctx context.Context, req *NotifyRequest) error {
	if req == nil {
		req = &NotifyRequest{}
	}
	m.record("notify", req.Topic, 0)
	return nil
}

// Calls 返回调用记录的副本（单测断言用）。
func (m *MockChannel) Calls() []MockCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]MockCall, len(m.calls))
	copy(out, m.calls)
	return out
}

// ResetCalls 清空调用记录（单测隔离用）。
func (m *MockChannel) ResetCalls() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = nil
}

// record 追加一条调用记录。
func (m *MockChannel) record(op, ref string, amount float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, MockCall{Op: op, Ref: ref, Amount: amount, At: m.nowFn()})
}

func (m *MockChannel) nowFn() time.Time {
	if m.now != nil {
		return m.now()
	}
	return time.Now()
}

// newMockUUID 生成短流水号（仅 mock 展示用，非业务唯一键）。
func newMockUUID() string {
	id, err := uuid.NewRandom()
	if err != nil {
		return time.Now().Format("20060102150405")
	}
	return id.String()
}
