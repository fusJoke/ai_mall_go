package supplier

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	captchaInfra "ai-go-mall/internal/infra/captcha"
	"ai-go-mall/internal/model/mall"
	supplierSvc "ai-go-mall/internal/service/supplier"
)

// mockAuthService 实现 supplierSvc.AuthService。
type mockAuthService struct {
	loginFunc  func(c *gin.Context, username, password, captchaKey string, points []captchaInfra.Point, remember bool) (*mall.MallSupplierUser, string, error)
	logoutFunc func(ctx context.Context, rawToken string) error

	loginCalls  int
	logoutCalls int
	lastLogout  string
}

func (m *mockAuthService) Login(c *gin.Context, username, password, captchaKey string, points []captchaInfra.Point, remember bool) (*mall.MallSupplierUser, string, error) {
	m.loginCalls++
	if m.loginFunc != nil {
		return m.loginFunc(c, username, password, captchaKey, points, remember)
	}
	return &mall.MallSupplierUser{ID: 1, SupplierID: 10, Username: username}, "mock-token", nil
}

func (m *mockAuthService) Logout(ctx context.Context, rawToken string) error {
	m.logoutCalls++
	m.lastLogout = rawToken
	if m.logoutFunc != nil {
		return m.logoutFunc(ctx, rawToken)
	}
	return nil
}

var _ supplierSvc.AuthService = (*mockAuthService)(nil)

func doSupplierPost(t *testing.T, path string, h gin.HandlerFunc, body any, authz string) (int, map[string]any) {
	t.Helper()
	var req *http.Request
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		req = httptest.NewRequest(http.MethodPost, path, bytes.NewReader(b))
		req.Header.Set("Content-Type", "application/json")
	} else {
		req = httptest.NewRequest(http.MethodPost, path, nil)
	}
	if authz != "" {
		req.Header.Set("Authorization", authz)
	}
	w := httptest.NewRecorder()

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST(path, h)
	r.ServeHTTP(w, req)

	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	return w.Code, resp
}

func TestSupplierLogin_HappyPath(t *testing.T) {
	svc := &mockAuthService{
		loginFunc: func(c *gin.Context, username, password, key string, points []captchaInfra.Point, remember bool) (*mall.MallSupplierUser, string, error) {
			if username != "panini_admin" || password != "secret" {
				t.Errorf("credentials = %s/%s", username, password)
			}
			if len(points) != 1 {
				t.Errorf("points = %d, want 1", len(points))
			}
			return &mall.MallSupplierUser{ID: 3, SupplierID: 10, Username: username}, "sup-token", nil
		},
	}
	h := NewAuthHandler(svc)

	code, resp := doSupplierPost(t, "/supplier/login", h.Login, map[string]any{
		"username":    "panini_admin",
		"password":    "secret",
		"captcha_key": "cap-1",
		"points":      []map[string]int{{"x": 5, "y": 6}},
		"remember":    false,
	}, "")

	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%v", code, resp)
	}
	if resp["token"] != "sup-token" {
		t.Errorf("token = %v", resp["token"])
	}
	user, ok := resp["user"].(map[string]any)
	if !ok {
		t.Fatalf("user missing: %v", resp["user"])
	}
	if user["id"] != float64(3) || user["supplier_id"] != float64(10) || user["username"] != "panini_admin" {
		t.Errorf("user = %v", user)
	}
	// 密码哈希绝不能出现在响应里。
	if _, leaked := user["password"]; leaked {
		t.Errorf("password hash leaked in response")
	}
}

func TestSupplierLogin_MissingCaptcha_400(t *testing.T) {
	svc := &mockAuthService{}
	h := NewAuthHandler(svc)

	code, _ := doSupplierPost(t, "/supplier/login", h.Login, map[string]any{
		"username": "panini_admin",
		"password": "secret",
		"points":   []map[string]int{{"x": 0, "y": 0}},
	}, "")
	if code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", code)
	}
	if svc.loginCalls != 0 {
		t.Errorf("Login should NOT be called")
	}
}

func TestSupplierLogin_ErrorMapping(t *testing.T) {
	cases := []struct {
		name     string
		err      error
		wantCode int
		wantStr  string
	}{
		{"invalid_credentials", supplierSvc.ErrInvalidCredentials, http.StatusUnauthorized, "login.invalid_credentials"},
		{"invalid_captcha", supplierSvc.ErrInvalidCaptcha, http.StatusUnauthorized, "login.invalid_captcha"},
		{"account_disabled", supplierSvc.ErrAccountDisabled, http.StatusForbidden, "login.account_disabled"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := &mockAuthService{
				loginFunc: func(c *gin.Context, username, password, key string, points []captchaInfra.Point, remember bool) (*mall.MallSupplierUser, string, error) {
					return nil, "", tc.err
				},
			}
			h := NewAuthHandler(svc)
			code, body := doSupplierPost(t, "/supplier/login", h.Login, map[string]any{
				"username":    "panini_admin",
				"password":    "secret",
				"captcha_key": "cap",
				"points":      []map[string]int{{"x": 0, "y": 0}},
			}, "")
			if code != tc.wantCode {
				t.Errorf("status = %d, want %d", code, tc.wantCode)
			}
			if body["code"] != tc.wantStr {
				t.Errorf("code = %v, want %v", body["code"], tc.wantStr)
			}
		})
	}
}

func TestSupplierLogout_HappyPath(t *testing.T) {
	svc := &mockAuthService{}
	h := NewAuthHandler(svc)

	code, body := doSupplierPost(t, "/supplier/logout", h.Logout, nil, "Bearer sup-token")
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200", code)
	}
	if body["code"] != "logout.ok" {
		t.Errorf("code = %v", body["code"])
	}
	if svc.lastLogout != "sup-token" {
		t.Errorf("lastLogout = %q", svc.lastLogout)
	}
}

func TestSupplierLogout_MissingHeader_Idempotent(t *testing.T) {
	svc := &mockAuthService{}
	h := NewAuthHandler(svc)

	code, _ := doSupplierPost(t, "/supplier/logout", h.Logout, nil, "")
	if code != http.StatusOK {
		t.Errorf("status = %d, want 200", code)
	}
	if svc.logoutCalls != 0 {
		t.Errorf("Logout should NOT be called without header")
	}
}
