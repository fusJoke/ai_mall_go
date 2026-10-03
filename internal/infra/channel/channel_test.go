// Package channel — factory_test.go 覆盖工厂注册表与 mock / bank driver 行为
// （任务 13.7：Create 正确返回 driver；未知 name 返回 ErrUnknownChannel）。
package channel

import (
	"context"
	"errors"
	"testing"

	"ai-go-mall/internal/infra/config"
)

func TestFactory_Create_KnownDrivers(t *testing.T) {
	f := channelFactory{}

	cases := []struct {
		name  string
		check func(Channel) error
	}{
		{"mock", func(c Channel) error {
			if _, ok := c.(*MockChannel); !ok {
				return errors.New("want *MockChannel")
			}
			return nil
		}},
		{"bank", func(c Channel) error {
			if _, ok := c.(*BankChannel); !ok {
				return errors.New("want *BankChannel")
			}
			return nil
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := f.Create(tc.name)
			if err != nil {
				t.Fatalf("Create(%q): %v", tc.name, err)
			}
			if err := tc.check(got); err != nil {
				t.Errorf("Create(%q) wrong type: %v", tc.name, err)
			}
		})
	}
}

func TestFactory_Create_UnknownName(t *testing.T) {
	f := channelFactory{}

	got, err := f.Create("wechat")
	if got != nil {
		t.Errorf("got = %v, want nil", got)
	}
	if !errors.Is(err, ErrUnknownChannel) {
		t.Errorf("err = %v, want ErrUnknownChannel", err)
	}
	// 空名同样拒绝。
	if _, err := f.Create(""); !errors.Is(err, ErrUnknownChannel) {
		t.Errorf("empty name err = %v, want ErrUnknownChannel", err)
	}
}

func TestFactory_Init_UnknownConfigName(t *testing.T) {
	// 直接调 channelFactory 校验逻辑的等价路径：注册表外的名字必须被拒。
	// （channel.Init 依赖 config.Get() 全局态，这里只测 Create 语义。）
	f := channelFactory{}
	for _, name := range []string{"sms"} {
		if _, err := f.Create(name); !errors.Is(err, ErrUnknownChannel) {
			t.Errorf("Create(%q) err = %v, want ErrUnknownChannel", name, err)
		}
	}
}

func TestInit_AllMockConfig_OK(t *testing.T) {
	Reset()
	defer Reset()
	config.SetForTest(&config.Config{
		Channel: config.ChannelConfig{Payment: "mock", Bank: "bank", SMS: "mock"},
	})

	if err := Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	f := Get()
	if f == nil {
		t.Fatal("Get() after Init = nil, want factory")
	}
	// 装配后业务可按配置名拿到 driver。
	for _, name := range []string{"mock", "bank"} {
		if _, err := f.Create(name); err != nil {
			t.Errorf("Create(%q): %v", name, err)
		}
	}

	// 幂等：重复 Init 是 no-op。
	if err := Init(); err != nil {
		t.Errorf("second Init: %v", err)
	}
}

func TestInit_UnknownName_FailFast(t *testing.T) {
	Reset()
	defer Reset()
	config.SetForTest(&config.Config{
		Channel: config.ChannelConfig{Payment: "no-such-driver", Bank: "mock", SMS: "mock"},
	})

	if err := Init(); err == nil {
		t.Fatal("Init with unknown driver = nil err, want error")
	}
	if Get() != nil {
		t.Error("Get() after failed Init = non-nil, want nil")
	}
}

func TestMockChannel_RecordsCalls(t *testing.T) {
	m := &MockChannel{}
	ctx := context.Background()

	if _, err := m.Pay(ctx, &PayRequest{OrderNo: "NO1", UserID: 42, Amount: 79}); err != nil {
		t.Fatalf("Pay: %v", err)
	}
	if _, err := m.Payout(ctx, &PayoutRequest{SupplierID: 3, SettlementID: 9, Amount: 100}); err != nil {
		t.Fatalf("Payout: %v", err)
	}
	if err := m.Notify(ctx, &NotifyRequest{Topic: "payment.callback"}); err != nil {
		t.Fatalf("Notify: %v", err)
	}

	calls := m.Calls()
	if len(calls) != 3 {
		t.Fatalf("len(calls) = %d, want 3", len(calls))
	}
	if calls[0].Op != "pay" || calls[0].Ref != "NO1" || calls[0].Amount != 79 {
		t.Errorf("calls[0] = %+v", calls[0])
	}
	if calls[1].Op != "payout" || calls[1].Ref != "settlement:9" || calls[1].Amount != 100 {
		t.Errorf("calls[1] = %+v", calls[1])
	}
	if calls[2].Op != "notify" || calls[2].Ref != "payment.callback" {
		t.Errorf("calls[2] = %+v", calls[2])
	}

	m.ResetCalls()
	if len(m.Calls()) != 0 {
		t.Errorf("ResetCalls did not clear")
	}
}

func TestMockChannel_SuccessResults(t *testing.T) {
	m := &MockChannel{}
	ctx := context.Background()

	pay, err := m.Pay(ctx, &PayRequest{OrderNo: "NO2"})
	if err != nil || pay.Status != "success" || pay.TradeNo == "" {
		t.Errorf("Pay = %+v, err = %v; want success + trade no", pay, err)
	}
	out, err := m.Payout(ctx, &PayoutRequest{SettlementID: 1})
	if err != nil || out.Status != "success" || out.PayoutNo == "" {
		t.Errorf("Payout = %+v, err = %v; want success + payout no", out, err)
	}
}

func TestBankChannel_DelegatesToMock(t *testing.T) {
	b := NewBankChannel()
	bc, ok := b.(*BankChannel)
	if !ok {
		t.Fatalf("NewBankChannel returned %T", b)
	}
	ctx := context.Background()

	if _, err := b.Payout(ctx, &PayoutRequest{SettlementID: 7, Amount: 88}); err != nil {
		t.Fatalf("Payout: %v", err)
	}
	calls := bc.Calls()
	if len(calls) != 1 || calls[0].Op != "payout" || calls[0].Amount != 88 {
		t.Errorf("calls = %+v", calls)
	}
}
