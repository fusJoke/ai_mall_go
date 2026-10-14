package log

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"
)

func TestParseLevel(t *testing.T) {
	cases := map[string]slog.Level{
		"debug": slog.LevelDebug,
		"info":  slog.LevelInfo,
		"warn":  slog.LevelWarn,
		"error": slog.LevelError,
		// 空值 / 大小写混杂 / 未知值：一律回落 info（Init 不 fail 的兜底）。
		"":      slog.LevelInfo,
		"INFO":  slog.LevelInfo,
		"Debug": slog.LevelDebug,
		"bogus": slog.LevelInfo,
	}
	for in, want := range cases {
		if got := parseLevel(in); got != want {
			t.Errorf("parseLevel(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestFromCtxFallsBackToDefault(t *testing.T) {
	// 空 ctx、nil ctx 都不得 panic，且回落全局 logger。
	if FromCtx(context.Background()) == nil {
		t.Fatal("FromCtx(空 ctx) 应回落全局 logger，不得为 nil")
	}
	if FromCtx(nil) == nil {
		t.Fatal("FromCtx(nil) 应回落全局 logger，不得为 nil")
	}
}

func TestIntoFromCtxRoundTrip(t *testing.T) {
	custom := slog.New(slog.NewTextHandler(io.Discard, nil))
	ctx := IntoCtx(context.Background(), custom)
	if got := FromCtx(ctx); got != custom {
		t.Fatal("FromCtx 应原样取回 IntoCtx 注入的 logger")
	}
}

func TestComponentPrefixesField(t *testing.T) {
	var buf bytes.Buffer
	prev := L()
	SetForTest(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() { SetForTest(prev) })

	Component("hotspot").Warn("reload failed", "err", "x")
	out := buf.String()
	// slog TextHandler 对含空格的值会加引号（msg="reload failed"），
	// 故只断言裸值是否出现，不锁引号格式。
	if !strings.Contains(out, "component=hotspot") || !strings.Contains(out, "reload failed") {
		t.Errorf("组件日志应带 component=hotspot 前缀，got:\n%s", out)
	}
}
