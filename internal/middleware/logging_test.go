package middleware

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"ai-go-mall/internal/infra/log"
)

// setup 注入写入 buf 的测试 logger 并恢复全局状态（与 config.SetForTest 同惯例）。
func setup(t *testing.T) *bytes.Buffer {
	t.Helper()
	gin.SetMode(gin.TestMode)
	var buf bytes.Buffer
	prev := log.L()
	log.SetForTest(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() { log.SetForTest(prev) })
	return &buf
}

func TestRequestLoggerEchoesRequestID(t *testing.T) {
	buf := setup(t)

	r := gin.New()
	r.Use(RequestLogger())
	r.GET("/ping", func(c *gin.Context) { c.String(http.StatusOK, "ok") })

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.Header.Set(HeaderRequestID, "trace-abc")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if got := rec.Header().Get(HeaderRequestID); got != "trace-abc" {
		t.Errorf("X-Request-ID = %q, want echo %q", got, "trace-abc")
	}
	out := buf.String()
	for _, want := range []string{"trace_id=trace-abc", "path=/ping", "status=200", "latency_ms="} {
		if !strings.Contains(out, want) {
			t.Errorf("访问日志缺少 %q:\n%s", want, out)
		}
	}
}

func TestRequestLoggerGeneratesRequestID(t *testing.T) {
	buf := setup(t)

	r := gin.New()
	r.Use(RequestLogger())
	r.GET("/ping", func(c *gin.Context) { c.String(http.StatusOK, "ok") })

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/ping", nil))

	tid := rec.Header().Get(HeaderRequestID)
	if tid == "" {
		t.Fatal("响应头缺少 X-Request-ID")
	}
	if !strings.Contains(buf.String(), "trace_id="+tid) {
		t.Errorf("日志 trace_id 应与响应头一致:\n%s", buf.String())
	}
}

func TestRequestLoggerLevelsByStatus(t *testing.T) {
	buf := setup(t)

	r := gin.New()
	r.Use(RequestLogger())
	r.GET("/not-found", func(c *gin.Context) { c.String(http.StatusNotFound, "nf") })
	r.GET("/boom", func(c *gin.Context) { c.String(http.StatusInternalServerError, "oops") })

	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/not-found", nil))
	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/boom", nil))

	out := buf.String()
	if !strings.Contains(out, "level=WARN") || !strings.Contains(out, "path=/not-found") {
		t.Errorf("4xx 应打 WARN:\n%s", out)
	}
	if !strings.Contains(out, "level=ERROR") || !strings.Contains(out, "path=/boom") {
		t.Errorf("5xx 应打 ERROR:\n%s", out)
	}
}

func TestRecoveryReturnsUnified500(t *testing.T) {
	buf := setup(t)

	r := gin.New()
	r.Use(RequestLogger(), Recovery())
	r.GET("/boom", func(c *gin.Context) { panic("boom") })

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/boom", nil))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "internal.panic") {
		t.Errorf("响应体应含 internal.panic: %s", rec.Body.String())
	}
	out := buf.String()
	for _, want := range []string{"panic recovered", "goroutine", "path=/boom"} {
		if !strings.Contains(out, want) {
			t.Errorf("panic 日志缺少 %q:\n%s", want, out)
		}
	}
}
