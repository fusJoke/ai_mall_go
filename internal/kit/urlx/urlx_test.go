package urlx

import (
	"crypto/tls"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"ai-go-mall/internal/infra/config"
)

// withConfig 临时把 Config 注入到 config 单例，测试结束后通过 t.Cleanup 还原。
//
// 用 t.Cleanup 而非 defer —— defer 在 helper 函数返回时立刻执行，
// 会让注入的配置在测试体执行前就被还原；t.Cleanup 绑定到测试函数生命周期，
// 保证 helper 返回后注入仍生效到测试体结束。
//
// 模式与 internal/infra/upload/upload_test.go 一致。
func withConfig(t *testing.T, c config.Config) {
	t.Helper()
	prev := config.Get()
	if prev == nil {
		prev = &config.Config{}
	}
	t.Cleanup(func() { config.SetForTest(prev) })

	full := *prev
	full.Server = c.Server
	config.SetForTest(&full)
}

// newCtx 构造一个绑定到 targetURL 的 gin.Context，模拟 HTTP 请求上下文。
//
// targetURL 形如 "http://api.example.com:8080/uploads/x.jpg"，
// 用于设置 c.Request.Host + c.Request.URL。
func newCtx(targetURL string) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	// 直接用 httptest.NewRequest 构造，让 c.Request.Host 自动从 URL 推算。
	c.Request = httptest.NewRequest("GET", targetURL, nil)
	return c, w
}

func TestFullURL_EmptyResource(t *testing.T) {
	c, _ := newCtx("http://api.example.com:8080/x")
	if got := FullURL(c, ""); got != "" {
		t.Errorf("FullURL(c, '') = %q, want empty", got)
	}
}

func TestFullURL_Base64ReturnedAsIs(t *testing.T) {
	cases := []string{
		"data:image/png;base64,iVBORw0KGgo=",
		"data:application/pdf;base64,JVBERi0xLjQK",
	}
	for _, in := range cases {
		t.Run(in, func(t *testing.T) {
			c, _ := newCtx("http://api.example.com:8080/")
			got := FullURL(c, in)
			if got != in {
				t.Errorf("FullURL(c, %q) = %q, want %q", in, got, in)
			}
		})
	}
}

func TestFullURL_ProtocolReturnedAsIs(t *testing.T) {
	cases := []string{
		"https://cdn.example.com/x.jpg",
		"http://example.com/x.jpg",
		"https://example.com:8443/x.jpg",
	}
	for _, in := range cases {
		t.Run(in, func(t *testing.T) {
			c, _ := newCtx("http://api.example.com:8080/")
			got := FullURL(c, in)
			if got != in {
				t.Errorf("FullURL(c, %q) = %q, want %q", in, got, in)
			}
		})
	}
}

// TestFullURL_NoCDNUsesCurrentOrigin 验证未配 CDN 时走 c.Request.Host。
func TestFullURL_NoCDNUsesCurrentOrigin(t *testing.T) {
	withConfig(t, config.Config{Server: config.ServerConfig{}}) // CDNURL 空

	c, _ := newCtx("http://api.example.com:8080/")
	got := FullURL(c, "/uploads/avatar/x.jpg")
	want := "http://api.example.com:8080/uploads/avatar/x.jpg"
	if got != want {
		t.Errorf("FullURL = %q, want %q", got, want)
	}
}

// TestFullURL_HTTPSRequest 验证 TLS 请求被识别为 https。
func TestFullURL_HTTPSRequest(t *testing.T) {
	withConfig(t, config.Config{Server: config.ServerConfig{}})

	c, _ := newCtx("https://api.example.com/")
	// httptest.NewRequest("GET", "https://...") 不会自动设置 TLS；
	// 这里手工把 c.Request.TLS 置成非 nil，模拟 TLS 已终结在 gin 节点。
	c.Request.TLS = &tls.ConnectionState{}
	got := FullURL(c, "/uploads/x.jpg")
	want := "https://api.example.com/uploads/x.jpg"
	if got != want {
		t.Errorf("FullURL = %q, want %q", got, want)
	}
}

// TestFullURL_XForwardedProto 验证反向代理写入的 header 生效。
func TestFullURL_XForwardedProto(t *testing.T) {
	withConfig(t, config.Config{Server: config.ServerConfig{}})

	c, _ := newCtx("http://api.example.com/")
	c.Request.Header.Set("X-Forwarded-Proto", "https")
	got := FullURL(c, "/uploads/x.jpg")
	want := "https://api.example.com/uploads/x.jpg"
	if got != want {
		t.Errorf("FullURL = %q, want %q", got, want)
	}
}

// TestFullURL_CDNWithParams 验证 CDN 优先级最高，忽略当前请求 origin。
func TestFullURL_CDNWithParams(t *testing.T) {
	withConfig(t, config.Config{Server: config.ServerConfig{
		CDNURL:       "https://cdn.example.com",
		CDNURLParams: "format/heif",
	}})

	c, _ := newCtx("http://api.example.com:8080/")
	got := FullURL(c, "/uploads/x.jpg")
	want := "https://cdn.example.com/format/heif/uploads/x.jpg"
	if got != want {
		t.Errorf("FullURL = %q, want %q", got, want)
	}
	// 兜底断言：CDN 命中后绝不允许当前 origin 出现在结果里
	if strings.Contains(got, "api.example.com") {
		t.Errorf("CDN result %q should not contain current origin", got)
	}
}

// TestFullURL_CDNWithoutParams 验证 CDN params 为空时直接拼接。
func TestFullURL_CDNWithoutParams(t *testing.T) {
	withConfig(t, config.Config{Server: config.ServerConfig{
		CDNURL: "https://cdn.example.com",
	}})

	c, _ := newCtx("http://api.example.com:8080/")
	got := FullURL(c, "/uploads/x.jpg")
	want := "https://cdn.example.com/uploads/x.jpg"
	if got != want {
		t.Errorf("FullURL = %q, want %q", got, want)
	}
}
