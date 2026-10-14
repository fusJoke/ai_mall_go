package log

// GORM 日志统一接入 slog：
//   - 级别策略在 Trace 内自主决定；gorm logger.LogMode 不做二次开关
//    （级别由 slog Handler 统一管）。
//   - gorm.ErrRecordNotFound 静默：它常是业务正常分支（"查无此行"），
//     打 Error 会刷屏；要排查时把级别调到 debug 即可见。
//   - ctx 贯穿：GORM 把执行时的 ctx 传进 Trace，配合请求中间件
//     （middleware.RequestLogger），每条 SQL 日志自动带 trace_id。

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// GormLogger 实现 gormlogger.Interface；无状态，全局共用一个实例。
type GormLogger struct {
	// SlowThreshold 慢查询阈值，超过打 Warn；0 = 不启用。
	SlowThreshold time.Duration
}

// 编译期断言：确保实现 GORM logger 接口。
var _ gormlogger.Interface = (*GormLogger)(nil)

// NewGormLogger slow 传 0 关闭慢查询告警，常规业务建议 200ms。
func NewGormLogger(slow time.Duration) *GormLogger {
	return &GormLogger{SlowThreshold: slow}
}

// LogMode GORM 用它切级别（如 stmt.Debug()）。级别已由 slog Handler 管，原样返回。
func (l *GormLogger) LogMode(gormlogger.LogLevel) gormlogger.Interface { return l }

func (l *GormLogger) Info(ctx context.Context, msg string, data ...any) {
	FromCtx(ctx).Info(fmt.Sprintf(msg, data...))
}

func (l *GormLogger) Warn(ctx context.Context, msg string, data ...any) {
	FromCtx(ctx).Warn(fmt.Sprintf(msg, data...))
}

func (l *GormLogger) Error(ctx context.Context, msg string, data ...any) {
	FromCtx(ctx).Error(fmt.Sprintf(msg, data...))
}

func (l *GormLogger) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	elapsed := time.Since(begin)
	sqlStr, rows := fc()

	switch {
	case err != nil && !errors.Is(err, gorm.ErrRecordNotFound):
		FromCtx(ctx).Error("gorm error",
			"err", err.Error(),
			"sql", sqlStr,
			"rows", rows,
			"elapsed_ms", elapsed.Milliseconds(),
		)
	case l.SlowThreshold > 0 && elapsed > l.SlowThreshold:
		FromCtx(ctx).Warn("gorm slow query",
			"sql", sqlStr,
			"rows", rows,
			"elapsed_ms", elapsed.Milliseconds(),
			"threshold_ms", l.SlowThreshold.Milliseconds(),
		)
	default:
		// Debug 级打全量 SQL；生产 Info 级自然静默，排查时调级即可。
		FromCtx(ctx).Debug("gorm query",
			"sql", sqlStr,
			"rows", rows,
			"elapsed_ms", elapsed.Milliseconds(),
		)
	}
}
