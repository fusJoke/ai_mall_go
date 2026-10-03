// factory.go — ChannelFactory：按渠道名创建 Channel 的注册表 + 进程级装配。
//
// 设计（D18）：
//   - drivers 注册表集中管理 driver 构造函数；新增 driver（wechat / alipay /
//     真实银行 API）只需在此追加一行 + 新加 driver 文件，业务代码不动。
//   - Init() 读 config.Get().Channel，逐个校验 payment / bank / sms 三渠道
//     名能成功 Create（fail fast：配置写错启动即报错，不留到第一次业务调用）。
//   - 业务代码通过 Get() 拿 ChannelFactory；未 Init 时 Get() 返回 nil，
//     业务侧按 nil 跳过（与 mq.Get() 同约定）。
package channel

import (
	"fmt"

	"ai-go-mall/internal/infra/config"
)

// ChannelFactory 按 name 创建 Channel（name = "payment" / "bank" / "sms" 的 driver 名）。
type ChannelFactory interface {
	Create(name string) (Channel, error)
}

// drivers 是 driver 构造函数注册表。
//
// key 即 config/channel.yaml 里 channel.payment / bank / sms 的取值域；
// 未来 wechat / alipay 落地时在此追加。
var drivers = map[string]func() Channel{
	"mock": NewMockChannel,
	"bank": NewBankChannel,
}

// channelFactory 是 ChannelFactory 的默认实现（无状态）。
type channelFactory struct{}

// Create 按名字实例化 driver；未知名返回 ErrUnknownChannel。
func (channelFactory) Create(name string) (Channel, error) {
	newDriver, ok := drivers[name]
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrUnknownChannel, name)
	}
	return newDriver(), nil
}

// factory 是 Init 缓存的 ChannelFactory 单例。Init 未调用或失败时为 nil。
var factory ChannelFactory

// Init 校验 config.Get().Channel 里的三个渠道名均可实例化（fail fast），
// 通过后缓存 channelFactory 单例。
//
// 同一进程重复调用是 no-op；如需强制重载，先调 Reset。
// 注意：依赖 config.Get()，必须先调 config.Init()。
func Init() error {
	if factory != nil {
		return nil
	}

	cfg := config.Get().Channel
	f := channelFactory{}
	// 逐个 Create 一遍：driver 构造在 MVP 是纯内存操作，代价可忽略；
	// 换来的是"配置名写错 → 启动失败"而不是"第一笔支付时才炸"。
	for name, field := range map[string]string{
		cfg.Payment: "payment",
		cfg.Bank:    "bank",
		cfg.SMS:     "sms",
	} {
		if _, err := f.Create(name); err != nil {
			return fmt.Errorf("channel init: %s driver invalid: %w", field, err)
		}
	}

	factory = f
	return nil
}

// Get 返回已初始化的 ChannelFactory。Init 未调用过或失败时返回 nil。
func Get() ChannelFactory {
	return factory
}

// Reset 清空 ChannelFactory 缓存。专供测试使用。
func Reset() {
	factory = nil
}
