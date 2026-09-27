package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"ai-go-mall/internal/infra/token"
	"ai-go-mall/internal/model"
)

// withCheckToken 在测试期间替换 checkToken，结束时还原（不依赖 t.Cleanup，
// 让调用方控制 hook 生命周期，方便子用例叠覆盖）。
func withCheckToken(t *testing.T, fn func(ctx context.Context, rawToken string) (*model.Token, error)) {
	t.Helper()
	prev := checkToken
	checkToken = fn
	t.Cleanup(func() { checkToken = prev })
}

// newAuthTestEngine 拼一个最小 gin 引擎：AdminAuth + 一个「命中」handler，
// 命中 handler 用 200 + {"reached": true} 标记「中间件放行」。
func newAuthTestEngine() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/protected", AdminAuth(), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"reached": true})
	})
	return r
}

// runReq 跑一次请求，返回 (status, decoded body)。
func runReq(t *testing.T, r *gin.Engine, method, target, authz string) (int, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(method, target, nil)
	if authz != "" {
		req.Header.Set("Authorization", authz)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var body map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	return w.Code, body
}

// --- 解析层测试（不依赖 checkToken） ---

func TestAdminAuth_MissingHeader(t *testing.T) {
	r := newAuthTestEngine()
	withCheckToken(t, func(context.Context, string) (*model.Token, error) {
		t.Fatalf("checkToken should NOT be called when Authorization is missing")
		return nil, nil
	})

	code, body := runReq(t, r, "GET", "/protected", "")
	if code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", code)
	}
	if body["code"] != "auth.missing_token" {
		t.Errorf("body.code = %v, want auth.missing_token", body["code"])
	}
}

func TestAdminAuth_NonBearerScheme(t *testing.T) {
	r := newAuthTestEngine()
	withCheckToken(t, func(context.Context, string) (*model.Token, error) {
		t.Fatalf("checkToken should NOT be called for non-Bearer scheme")
		return nil, nil
	})

	code, body := runReq(t, r, "GET", "/protected", "Token abc.def")
	if code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", code)
	}
	if body["code"] != "auth.missing_token" {
		t.Errorf("body.code = %v, want auth.missing_token", body["code"])
	}
}

func TestAdminAuth_EmptyBearerToken(t *testing.T) {
	r := newAuthTestEngine()
	withCheckToken(t, func(context.Context, string) (*model.Token, error) {
		t.Fatalf("checkToken should NOT be called for empty bearer token")
		return nil, nil
	})

	code, body := runReq(t, r, "GET", "/protected", "Bearer    ")
	if code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", code)
	}
	if body["code"] != "auth.missing_token" {
		t.Errorf("body.code = %v, want auth.missing_token", body["code"])
	}
}

func TestAdminAuth_BearerWithOnlyPrefix(t *testing.T) {
	r := newAuthTestEngine()
	withCheckToken(t, func(context.Context, string) (*model.Token, error) {
		t.Fatalf("checkToken should NOT be called for bearer without token body")
		return nil, nil
	})

	// "Bearer " (尾随一个空格) → 截取后空串
	code, body := runReq(t, r, "GET", "/protected", "Bearer ")
	if code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", code)
	}
	if body["code"] != "auth.missing_token" {
		t.Errorf("body.code = %v, want auth.missing_token", body["code"])
	}
}

// --- 校验层测试（覆盖 checkToken 各分支） ---

func TestAdminAuth_ValidToken_PassesThrough(t *testing.T) {
	r := newAuthTestEngine()
	withCheckToken(t, func(ctx context.Context, rawToken string) (*model.Token, error) {
		if rawToken != "valid-token" {
			t.Errorf("checkToken got rawToken = %q, want %q", rawToken, "valid-token")
		}
		return &model.Token{UserID: 1, Type: "admin"}, nil
	})

	code, body := runReq(t, r, "GET", "/protected", "Bearer valid-token")
	if code != http.StatusOK {
		t.Errorf("status = %d, want 200", code)
	}
	if body["reached"] != true {
		t.Errorf("body.reached = %v, want true (handler should be reached)", body["reached"])
	}
}

func TestAdminAuth_ExpiredToken_401(t *testing.T) {
	r := newAuthTestEngine()
	withCheckToken(t, func(context.Context, string) (*model.Token, error) {
		return nil, token.ErrTokenExpired
	})

	code, body := runReq(t, r, "GET", "/protected", "Bearer expired-token")
	if code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", code)
	}
	if body["code"] != "auth.token_expired" {
		t.Errorf("body.code = %v, want auth.token_expired", body["code"])
	}
}

func TestAdminAuth_NotFoundToken_401(t *testing.T) {
	r := newAuthTestEngine()
	withCheckToken(t, func(context.Context, string) (*model.Token, error) {
		return nil, token.ErrTokenNotFound
	})

	code, body := runReq(t, r, "GET", "/protected", "Bearer ghost-token")
	if code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", code)
	}
	if body["code"] != "auth.token_not_found" {
		t.Errorf("body.code = %v, want auth.token_not_found", body["code"])
	}
}

func TestAdminAuth_OtherError_401(t *testing.T) {
	r := newAuthTestEngine()
	withCheckToken(t, func(context.Context, string) (*model.Token, error) {
		return nil, errGeneric // 见文件末尾定义
	})

	code, body := runReq(t, r, "GET", "/protected", "Bearer any")
	if code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", code)
	}
	if body["code"] != "auth.invalid_token" {
		t.Errorf("body.code = %v, want auth.invalid_token", body["code"])
	}
}

func TestAdminAuth_TokenManagerUnavailable_500(t *testing.T) {
	r := newAuthTestEngine()
	withCheckToken(t, func(context.Context, string) (*model.Token, error) {
		return nil, errTokenManagerUnavailable
	})

	code, body := runReq(t, r, "GET", "/protected", "Bearer any")
	if code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", code)
	}
	if body["code"] != "auth.internal" {
		t.Errorf("body.code = %v, want auth.internal", body["code"])
	}
}

// --- L2: Bearer scheme 大小写不敏感（RFC 6750）---

func TestAdminAuth_LowercaseBearer_PassesThrough(t *testing.T) {
	r := newAuthTestEngine()
	withCheckToken(t, func(ctx context.Context, rawToken string) (*model.Token, error) {
		if rawToken != "valid-token" {
			t.Errorf("checkToken got rawToken = %q, want %q (lowercase bearer should still parse)", rawToken, "valid-token")
		}
		return &model.Token{UserID: 1, Type: "admin"}, nil
	})

	code, body := runReq(t, r, "GET", "/protected", "bearer valid-token")
	if code != http.StatusOK {
		t.Errorf("status = %d, want 200", code)
	}
	if body["reached"] != true {
		t.Errorf("body.reached = %v, want true (handler should be reached)", body["reached"])
	}
}

func TestAdminAuth_MixedCaseBearer_PassesThrough(t *testing.T) {
	r := newAuthTestEngine()
	withCheckToken(t, func(context.Context, string) (*model.Token, error) {
		return &model.Token{UserID: 1, Type: "admin"}, nil
	})

	// 各种大小写 + 可选尾部空格：都应被识别为合法 Bearer scheme。
	for _, scheme := range []string{"Bearer ", "bearer ", "BEARER ", "BeArEr ", "bEaReR "} {
		t.Run(scheme, func(t *testing.T) {
			authz := scheme + "valid-token"
			code, body := runReq(t, r, "GET", "/protected", authz)
			if code != http.StatusOK {
				t.Errorf("status = %d, want 200 (scheme=%q)", code, scheme)
			}
			if body["reached"] != true {
				t.Errorf("body.reached = %v, want true (scheme=%q)", body["reached"], scheme)
			}
		})
	}
}

// --- N1: token 类型校验（仅允许 type=admin）---

func TestAdminAuth_TokenTypeMismatch_401(t *testing.T) {
	r := newAuthTestEngine()
	withCheckToken(t, func(context.Context, string) (*model.Token, error) {
		// 合法 token 但 type 是 user —— 必须被中间件拒掉。
		return &model.Token{UserID: 1, Type: "user"}, nil
	})

	code, body := runReq(t, r, "GET", "/protected", "Bearer any")
	if code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", code)
	}
	if body["code"] != "auth.token_type_mismatch" {
		t.Errorf("body.code = %v, want auth.token_type_mismatch", body["code"])
	}
}

func TestAdminAuth_TokenTypeAdmin_PassesThrough(t *testing.T) {
	r := newAuthTestEngine()
	withCheckToken(t, func(context.Context, string) (*model.Token, error) {
		return &model.Token{UserID: 1, Type: "admin"}, nil
	})

	code, body := runReq(t, r, "GET", "/protected", "Bearer any")
	if code != http.StatusOK {
		t.Errorf("status = %d, want 200 (type=admin should pass)", code)
	}
	if body["reached"] != true {
		t.Errorf("body.reached = %v, want true", body["reached"])
	}
}

// --- L1: 错误响应不回显 err.Error() 内部细节 ---

func TestAdminAuth_DoesNotLeakErrMessage(t *testing.T) {
	// 验证 401 响应体的 message 字段是固定文案，不会把 err.Error() 透传给客户端。
	cases := []struct {
		name      string
		scheme    string
		errFromCK error
	}{
		{"not_found", "Bearer x", token.ErrTokenNotFound},
		{"expired", "Bearer x", token.ErrTokenExpired},
		{"generic", "Bearer x", errGeneric},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := newAuthTestEngine()
			withCheckToken(t, func(context.Context, string) (*model.Token, error) {
				return nil, tc.errFromCK
			})

			code, body := runReq(t, r, "GET", "/protected", tc.scheme)
			if code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401", code)
			}
			msg, _ := body["message"].(string)
			if msg == "" {
				t.Fatal("body.message missing")
			}
			// 必须包含 err.Error() 子串才会判失败 —— 这里反过来：固定文案应不与 err 文本重叠。
			if msg == tc.errFromCK.Error() {
				t.Errorf("body.message leaks driver text: %q == %q", msg, tc.errFromCK.Error())
			}
		})
	}
}

// errGeneric 是测试用的「非哨兵」错误，验证默认 401 auth.invalid_token 兜底分支。
var errGeneric = stringError("some unexpected failure")

type stringError string

func (e stringError) Error() string { return string(e) }