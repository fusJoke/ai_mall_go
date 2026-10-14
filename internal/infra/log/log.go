// Package log 全局日志入口：业务代码统一面向标准库 log/slog，
// 不直接依赖任何第三方日志库。
//
// 设计要点：
//   - slog 是接口层，编码引擎（Handler）可插拔：起步用标准库 JSONHandler，
//     需要极限性能时一行换成 zerolog/zap 的 slog Handler，调用点零改动。
//   - 请求级字段（trace_id 等）经 context 传递：入口中间件派生带字段的
//     子 logger 写入 request context，业务层 FromCtx 取出即用。
//   - 文件输出交给 lumberjack 按大小轮转 + 压缩归档，进程内无额外 goroutine。
//   - 启动期 fail fast 仍用标准库 log.Fatal（os.Exit 跳过 defer，语义同 PHP die）；
//     运行期一律 slog，禁止 os.Exit。
package log

import (
	"context"
	"io"
	"log/slog"
	"os"
	"strings"

	"github.com/google/uuid"
	"gopkg.in/natefinch/lumberjack.v2"
)

// Config 日志初始化参数，由各入口 main 从 config.Get().Log 装配后传入；
// 本包不读 viper，与 infra 其他包保持一致边界。
type Config struct {
	// Mode: "console"（dev，人读文本，写 stderr）| "json"（prod，写 File 或 stderr）
	Mode string
	// Level: "debug" | "info" | "warn" | "error"，默认 info
	Level string
	// File: Mode=json 时的输出文件；空 = stderr
	File string
	// 轮转参数（仅 File 非空时生效），零值走默认
	MaxSizeMB  int  // 单文件上限 MB，默认 100
	MaxBackups int  // 保留旧文件数，默认 7
	MaxAgeDays int  // 保留天数，默认 28
	Compress   bool // 归档是否 gzip
}

// defaultLogger 在 Init 之前也可用（写 stderr 文本，行为同标准库 log）。
var defaultLogger = slog.New(slog.NewTextHandler(os.Stderr, nil))

// Init 初始化全局 logger；各入口 main 在 config.Init 之后第一时间调用。
// 本函数自身不产生错误（配置非法项一律回落默认值），故无返回值。
func Init(cfg Config) {
	opts := &slog.HandlerOptions{Level: parseLevel(cfg.Level)}

	var w io.Writer = os.Stderr
	if cfg.Mode == "json" {
		if cfg.File != "" {
			w = &lumberjack.Logger{
				Filename:   cfg.File,
				MaxSize:    orDefault(cfg.MaxSizeMB, 100),
				MaxBackups: orDefault(cfg.MaxBackups, 7),
				MaxAge:     orDefault(cfg.MaxAgeDays, 28),
				Compress:   cfg.Compress,
				LocalTime:  true,
			}
		}
		defaultLogger = slog.New(slog.NewJSONHandler(w, opts))
		return
	}
	defaultLogger = slog.New(slog.NewTextHandler(w, opts))
}

// L 返回全局 logger；infra/service 组件内请优先用 Component 预置组件名。
func L() *slog.Logger { return defaultLogger }

// SetForTest 注入测试用 logger。仅供测试代码使用（与 config.SetForTest 同惯例）；
// 生产代码不应调用。
func SetForTest(l *slog.Logger) { defaultLogger = l }

// Component 预置 component 字段的组件 logger，供包级使用：
//
//	var lg = log.Component("hotspot")
func Component(name string) *slog.Logger { return defaultLogger.With("component", name) }

// NewTraceID 生成请求追踪 ID（复用项目已有 google/uuid 依赖）。
func NewTraceID() string { return uuid.NewString() }

// ctxKey 私有类型，避免与其他包的 context key 冲突。
type ctxKey struct{}

// IntoCtx 把请求级 logger 写入 context。
func IntoCtx(ctx context.Context, l *slog.Logger) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, ctxKey{}, l)
}

// FromCtx 取请求级 logger；无则回落全局。业务层取日志器一律经它，
// trace_id 等请求字段自动携带。
func FromCtx(ctx context.Context) *slog.Logger {
	if ctx != nil {
		if l, ok := ctx.Value(ctxKey{}).(*slog.Logger); ok && l != nil {
			return l
		}
	}
	return defaultLogger
}

// parseLevel 解析级别字符串；空值与未知值一律回落 info（Init 不 fail 的兜底）。
func parseLevel(s string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func orDefault(v, d int) int {
	if v <= 0 {
		return d
	}
	return v
}
