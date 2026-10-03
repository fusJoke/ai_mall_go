// Package user — channel_hooks.go 把 D18 渠道工厂接入抽卡路径。
//
// 集成语义（任务 13.8）：
//   - 事务 COMMIT 后经 factory.Create("payment").Pay 记录一笔"模拟支付"；
//     MVP driver 是 mock（仅记录，D18），未来替换为真实支付时业务代码不动。
//   - 放在事务外：渠道调用是外部 IO，不进事务（避免拉长 tx 持锁时间）；
//   - 失败仅 warn：订单与余额已在事务内落库，渠道记录是旁路审计，不回滚业务；
//   - factory 为 nil 时跳过（兼容旧单测 / 未 Init 的装配场景）。
//
// 未来真实支付的接入点：Draw/DrawSeckill 事务之前先 Pay 收款（预付语义），
// 拿到 PayResult.TradeNo 落订单；届时把本 helper 的调用位置与入参一并迁移。
package user

import (
	"context"
	"log"

	"ai-go-mall/internal/infra/channel"
)

// recordChannelPayment 经渠道工厂记录一笔支付（fire-and-forget）。
//
// userID 单独传参（DrawResult 不含用户 ID）；金额 / 订单号取自事务结果，
// 保证渠道记录与订单一致。
func recordChannelPayment(f channel.ChannelFactory, ctx context.Context, userID int64, r *DrawResult, subject string) {
	if f == nil || r == nil {
		return
	}
	ch, err := f.Create("payment")
	if err != nil {
		log.Printf("user.draw: create payment channel failed: %v", err)
		return
	}
	if _, err := ch.Pay(ctx, &channel.PayRequest{
		OrderNo: r.OrderNo,
		UserID:  userID,
		Amount:  r.ActualPrice,
		Subject: subject,
	}); err != nil {
		log.Printf("user.draw: payment channel record failed (order %s): %v", r.OrderNo, err)
	}
}
