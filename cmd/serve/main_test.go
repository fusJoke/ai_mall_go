package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPingRoute(t *testing.T) {
	h := newRouter("test-server", false)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/ping", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if got, want := body["message"], "pong"; got != want {
		t.Errorf("message = %v, want %v", got, want)
	}
	if got, want := body["server"], "test-server"; got != want {
		t.Errorf("server = %v, want %v", got, want)
	}
	if _, ok := body["time"]; !ok {
		t.Errorf("response missing \"time\" field: %v", body)
	}
}

func TestHealthzRoute(t *testing.T) {
	h := newRouter("test-server", false)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got, want := rec.Body.String(), "ok"; got != want {
		t.Errorf("body = %q, want %q", got, want)
	}
}

func TestUnknownRouteReturns404(t *testing.T) {
	h := newRouter("test-server", false)

	req := httptest.NewRequest(http.MethodGet, "/does-not-exist", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d (404)", rec.Code, http.StatusNotFound)
	}
}

// TestSwaggerRouteMounted 验证 newRouter 按 devMode 正确挂载或不挂载 /swagger/*any。
//
// 走真实的 newRouter 而非自己构造 gin.Engine，这样如果 newRouter 哪天忘记挂载，
// 断言会直接失败。dev=true 子测试断言 spec/UI 可达；
// dev=false 子测试断言 /swagger/*any 全部 404（路由根本没注册）。
func TestSwaggerRouteMounted(t *testing.T) {
	cases := []struct {
		name        string
		devMode     bool
		path        string
		wantStatus  int
		wantContain string // 仅 status==200 时断言
	}{
		{
			name:        "dev=true 时 /swagger/doc.json 返回 OpenAPI 2.0 spec",
			devMode:     true,
			path:        "/swagger/doc.json",
			wantStatus:  http.StatusOK,
			wantContain: `"swagger": "2.0"`,
		},
		{
			name:        "dev=true 时 /swagger/index.html 返回 swagger UI",
			devMode:     true,
			path:        "/swagger/index.html",
			wantStatus:  http.StatusOK,
			wantContain: "<html",
		},
		{
			name:       "dev=false 时 /swagger/doc.json 路由不存在（404）",
			devMode:    false,
			path:       "/swagger/doc.json",
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "dev=false 时 /swagger/index.html 路由不存在（404）",
			devMode:    false,
			path:       "/swagger/index.html",
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newRouter("test-server", tc.devMode)

			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			if rec.Code != tc.wantStatus {
				body, _ := io.ReadAll(rec.Body)
				t.Fatalf("status: want %d got %d, body=%s", tc.wantStatus, rec.Code, string(body))
			}
			if tc.wantContain != "" && !strings.Contains(rec.Body.String(), tc.wantContain) {
				snippet := rec.Body.String()
				if len(snippet) > 500 {
					snippet = snippet[:500]
				}
				t.Fatalf("body 不包含 %q\n实际前 500 字节: %s", tc.wantContain, snippet)
			}
		})
	}
}
