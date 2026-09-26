// Package captcha 提供点选验证码的生成、校验与过期清理能力。
//
// 公开 API：
//   - Manager.CreateClick  生成一道点选验证码
//   - Manager.VerifyClick  校验用户点击
//   - GetManager / Reset    懒初始化单例
//
// 设计：
//   - 单一存储后端（MySQL），不抽象 Driver 接口；Manager 直接持 Repository。
//   - 元素类型支持：中文文字 / 英文大写字母 / ICON（ICON 用文件名作为元素名）。
//   - 中文文字字符集来自 captcha.yaml（中文字符集）；英文大写字母由代码生成。
//   - 懒清理：CreateClick / VerifyClick 入口触发一次 DeleteExpired(now)。
package captcha

import "errors"

// Sentinel errors，调用方通过 errors.Is 区分业务分支。
var (
	// ErrNotFound captcha 不存在（key 错误或已过期清理）。
	ErrNotFound = errors.New("captcha: not found")

	// ErrExpired captcha 已过期（ExpiresAt 早于当前时间）。
	ErrExpired = errors.New("captcha: expired")

	// ErrMismatch 用户答案与正确答案不匹配（顺序错乱、元素不符）。
	ErrMismatch = errors.New("captcha: click mismatch")

	// ErrInvalidElem 配置的元素类型不在支持列表内。
	ErrInvalidElem = errors.New("captcha: invalid element type")

	// ErrInvalidInput 入参非法（key 为空、答案为空、长度不符等）。
	ErrInvalidInput = errors.New("captcha: invalid input")

	// ErrInternal 内部错误（仓库调用失败、图像编码失败等）。
	ErrInternal = errors.New("captcha: internal error")
)
