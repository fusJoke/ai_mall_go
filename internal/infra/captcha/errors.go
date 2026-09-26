// Package captcha 提供点选验证码的生成、校验与过期清理能力。
//
// 公开 API：
//   - Manager.CreateClick  生成一道点选验证码
//   - Manager.VerifyClick  校验用户点击
//   - Init / Get / Reset    单例管理（与 token 包对齐）
//
// 设计：
//   - 单一存储后端（MySQL），不抽象 Driver 接口；Manager 直接持 Repository。
//   - 元素类型支持：中文文字 / 英文大写字母 / ICON（ICON 用文件名作为元素名）。
//   - 中文文字 / 英文大写字母 由代码随机生成；ICON 从嵌入资产读取。
//   - 过期清理：Init 启动后台 goroutine，Reset 关闭。
//
// 启动顺序：cmd/serve/main.go 在 database.Init() 之后调用 captcha.Init()。
package captcha

import "errors"

// Sentinel errors，调用方通过 errors.Is 区分业务分支。
var (
	// ErrNotFound captcha 不存在（key 错误或已过期清理）。
	ErrNotFound = errors.New("captcha: not found")

	// ErrExpired captcha 已过期（ExpiresAt 早于当前时间）。
	ErrExpired = errors.New("captcha: expired")

	// ErrMismatch 用户点击与正确答案不匹配（坐标越界、顺序错乱、元素不符）。
	ErrMismatch = errors.New("captcha: click mismatch")

	// ErrInvalidElement 配置的元素类型不在支持列表内。
	ErrInvalidElement = errors.New("captcha: invalid element type")

	// ErrInvalidInput 入参非法（key 为空、点击数为 0、宽度高度非正等）。
	ErrInvalidInput = errors.New("captcha: invalid input")

	// ErrInternal 内部错误（仓库调用失败、图像编码失败等）。
	ErrInternal = errors.New("captcha: internal error")
)
