// Package captcha 是点选验证码的 service 层薄包装。
//
// 设计要点：
//   - 该层只负责把 infra/captcha.Manager 的能力翻译成「业务领域语言」：
//     暴露给 handler 的入参/返回值已在 service 层定型；
//     infra 层的 sentinel error 已在 service 层归类（透传）。
//   - 这里只暴露「HTTP 预检」链路：CreateClick + VerifyClick(precheck)。
//     「消费型」VerifyClick 不暴露在这里 —— 登录链路 service/admin 直接
//     持有 infra/captcha.Manager 调 deleteOnSuccess=true 的版本（详见 service/admin）。
package captcha

import (
	"context"

	infra "ai-go-mall/internal/infra/captcha"
)

// Service 是点选验证码业务接口。
type Service interface {
	// CreateClick 生成一道点选验证码并返回 key + elements + base64 图像。
	CreateClick(ctx context.Context) (*ClickCaptcha, error)
	// VerifyClick 对客户端提交的坐标做精度比对（预检语义：deleteOnSuccess=false）。
	VerifyClick(ctx context.Context, req *VerifyReq) error
}

// ClickCaptcha 是 service 层对外的「点选验证码」结构，字段名稳定，
// 直接用于 handler 序列化（JSON tag 保持 snake_case）。
type ClickCaptcha struct {
	Key      string   `json:"key"`
	Elements []string `json:"elements"`
	Image    string   `json:"image"`
	Width    int      `json:"width"`
	Height   int      `json:"height"`
}

// VerifyReq 是 service 层对外的 verify 入参形态；JSON 与 handler 入参共用。
type VerifyReq = infra.VerifyReq

// service 是 Service 的默认实现，薄包装 infra.Manager。
type service struct {
	mgr *infra.Manager
}

// NewService 接收 infra/captcha.Manager，返回 Service。
func NewService(mgr *infra.Manager) Service {
	return &service{mgr: mgr}
}

// CreateClick 透传 infra.Manager.CreateClick，并把 infra 类型映射到 service 层类型。
func (s *service) CreateClick(ctx context.Context) (*ClickCaptcha, error) {
	got, err := s.mgr.CreateClick(ctx)
	if err != nil {
		return nil, err
	}
	return &ClickCaptcha{
		Key:      got.Key,
		Elements: got.Elements,
		Image:    got.Image,
		Width:    got.Width,
		Height:   got.Height,
	}, nil
}

// VerifyClick 透传 infra.Manager.VerifyClick，固定 deleteOnSuccess=false（预检语义）。
func (s *service) VerifyClick(ctx context.Context, req *VerifyReq) error {
	return s.mgr.VerifyClick(ctx, req, false)
}
