package user

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	captchaInfra "ai-go-mall/internal/infra/captcha"
	"ai-go-mall/internal/model"
	"ai-go-mall/internal/repository"
	userSvc "ai-go-mall/internal/service/user"
)

// =============================================================================
// helpers
// =============================================================================

func newAuthCtx() *gin.Context {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", "/user/login", nil)
	return c
}

// doLogin 构造一个 POST /user/login 请求，跑一遍 AuthHandler.Login。
func doLogin(t *testing.T, h *AuthHandler, body any) (int, map[string]any) {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	req := httptest.NewRequest("POST", "/user/login", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/user/login", h.Login)
	r.ServeHTTP(w, req)

	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	return w.Code, resp
}

// doLogout 构造一个 POST /user/logout 请求（带/不带 Authorization），
// 跑一遍 AuthHandler.Logout。
func doLogout(t *testing.T, h *AuthHandler, authz string) (int, map[string]any) {
	t.Helper()
	req := httptest.NewRequest("POST", "/user/logout", nil)
	if authz != "" {
		req.Header.Set("Authorization", authz)
	}
	w := httptest.NewRecorder()

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/user/logout", h.Logout)
	r.ServeHTTP(w, req)

	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	return w.Code, resp
}

// =============================================================================
// mockUserService
// =============================================================================
//
// 完整实现 user.Service 接口以满足 NewAuthHandler 签名（实际只用 Login/Logout）。
type mockUserService struct {
	loginFunc  func(c *gin.Context, username, password, captchaKey string, points []captchaInfra.Point, remember bool) (*model.User, string, error)
	logoutFunc func(ctx context.Context, rawToken string) error

	// 记录调用次数
	loginCalls  int
	logoutCalls int
	lastLogout  string
}

func (m *mockUserService) Login(c *gin.Context, username, password, captchaKey string, points []captchaInfra.Point, remember bool) (*model.User, string, error) {
	m.loginCalls++
	if m.loginFunc != nil {
		return m.loginFunc(c, username, password, captchaKey, points, remember)
	}
	return &model.User{ID: 1, Username: username}, "mock-token", nil
}

func (m *mockUserService) Logout(ctx context.Context, rawToken string) error {
	m.logoutCalls++
	m.lastLogout = rawToken
	if m.logoutFunc != nil {
		return m.logoutFunc(ctx, rawToken)
	}
	return nil
}

// CRUDService 桩（满足 interface）
func (m *mockUserService) Create(c *gin.Context, e *model.User) error { return nil }
func (m *mockUserService) List(c *gin.Context, opts repository.ListOptions) ([]model.User, int64, error) {
	return nil, 0, nil
}
func (m *mockUserService) GetByID(c *gin.Context, id int64) (*model.User, error) {
	return nil, gorm.ErrRecordNotFound
}
func (m *mockUserService) Update(c *gin.Context, e *model.User) error { return nil }
func (m *mockUserService) Delete(c *gin.Context, id int64) error      { return nil }

var _ userSvc.Service = (*mockUserService)(nil)

// =============================================================================
// Login tests
// =============================================================================

func TestUserAuthLogin_HappyPath(t *testing.T) {
	svc := &mockUserService{
		loginFunc: func(c *gin.Context, username, password, key string, points []captchaInfra.Point, remember bool) (*model.User, string, error) {
			if username != "alice" || password != "secret123" {
				t.Errorf("unexpected credentials: %s/%s", username, password)
			}
			if !remember {
				t.Errorf("remember = false, want true")
			}
			if len(points) != 2 {
				t.Errorf("points = %d, want 2", len(points))
			}
			return &model.User{
				ID:       42,
				Username: "alice",
				Nickname: "Alice",
				Status:   1,
				Balance:  1234.56,
			}, "mock-token-xyz", nil
		},
	}
	h := NewAuthHandler(svc)

	body := map[string]any{
		"username":    "alice",
		"password":    "secret123",
		"captcha_key": "cap-abc",
		"points":      []map[string]int{{"x": 10, "y": 20}, {"x": 30, "y": 40}},
		"remember":    true,
	}
	code, resp := doLogin(t, h, body)

	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200", code)
	}
	if svc.loginCalls != 1 {
		t.Errorf("Login called %d times, want 1", svc.loginCalls)
	}
	if resp["token"] != "mock-token-xyz" {
		t.Errorf("token = %v, want mock-token-xyz", resp["token"])
	}
	user, ok := resp["user"].(map[string]any)
	if !ok {
		t.Fatalf("user missing or wrong type: %T", resp["user"])
	}
	if user["id"] != float64(42) {
		t.Errorf("user.id = %v, want 42", user["id"])
	}
	if user["username"] != "alice" {
		t.Errorf("user.username = %v, want alice", user["username"])
	}
	if user["balance"] != "1234.56" {
		t.Errorf("user.balance = %v, want 1234.56 (formatted)", user["balance"])
	}
}

func TestUserAuthLogin_InvalidInput_MissingUsername(t *testing.T) {
	svc := &mockUserService{}
	h := NewAuthHandler(svc)

	code, _ := doLogin(t, h, map[string]any{
		"password":    "secret",
		"captcha_key": "cap",
		"points":      []map[string]int{{"x": 0, "y": 0}},
	})

	if code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", code)
	}
	if svc.loginCalls != 0 {
		t.Errorf("Login should NOT be called on validation failure")
	}
}

func TestUserAuthLogin_InvalidInput_MissingCaptcha(t *testing.T) {
	svc := &mockUserService{}
	h := NewAuthHandler(svc)

	code, _ := doLogin(t, h, map[string]any{
		"username": "alice",
		"password": "secret",
		"points":   []map[string]int{{"x": 0, "y": 0}},
	})

	if code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", code)
	}
	if svc.loginCalls != 0 {
		t.Errorf("Login should NOT be called on validation failure")
	}
}

func TestUserAuthLogin_InvalidCredentials_401(t *testing.T) {
	svc := &mockUserService{
		loginFunc: func(c *gin.Context, username, password, key string, points []captchaInfra.Point, remember bool) (*model.User, string, error) {
			return nil, "", userSvc.ErrInvalidCredentials
		},
	}
	h := NewAuthHandler(svc)

	code, body := doLogin(t, h, map[string]any{
		"username":    "alice",
		"password":    "wrong",
		"captcha_key": "cap",
		"points":      []map[string]int{{"x": 0, "y": 0}},
	})

	if code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", code)
	}
	if body["code"] != "login.invalid_credentials" {
		t.Errorf("body.code = %v, want login.invalid_credentials", body["code"])
	}
}

func TestUserAuthLogin_InvalidCaptcha_401(t *testing.T) {
	svc := &mockUserService{
		loginFunc: func(c *gin.Context, username, password, key string, points []captchaInfra.Point, remember bool) (*model.User, string, error) {
			return nil, "", userSvc.ErrInvalidCaptcha
		},
	}
	h := NewAuthHandler(svc)

	code, body := doLogin(t, h, map[string]any{
		"username":    "alice",
		"password":    "secret",
		"captcha_key": "bad-key",
		"points":      []map[string]int{{"x": 0, "y": 0}},
	})

	if code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", code)
	}
	if body["code"] != "login.invalid_captcha" {
		t.Errorf("body.code = %v, want login.invalid_captcha", body["code"])
	}
}

func TestUserAuthLogin_AccountDisabled_403(t *testing.T) {
	svc := &mockUserService{
		loginFunc: func(c *gin.Context, username, password, key string, points []captchaInfra.Point, remember bool) (*model.User, string, error) {
			return nil, "", userSvc.ErrAccountDisabled
		},
	}
	h := NewAuthHandler(svc)

	code, body := doLogin(t, h, map[string]any{
		"username":    "alice",
		"password":    "secret",
		"captcha_key": "cap",
		"points":      []map[string]int{{"x": 0, "y": 0}},
	})

	if code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", code)
	}
	if body["code"] != "login.account_disabled" {
		t.Errorf("body.code = %v, want login.account_disabled", body["code"])
	}
}

func TestUserAuthLogin_InternalError_500(t *testing.T) {
	svc := &mockUserService{
		loginFunc: func(c *gin.Context, username, password, key string, points []captchaInfra.Point, remember bool) (*model.User, string, error) {
			return nil, "", errors.New("db: connection refused")
		},
	}
	h := NewAuthHandler(svc)

	code, body := doLogin(t, h, map[string]any{
		"username":    "alice",
		"password":    "secret",
		"captcha_key": "cap",
		"points":      []map[string]int{{"x": 0, "y": 0}},
	})

	if code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", code)
	}
	if body["code"] != "login.internal" {
		t.Errorf("body.code = %v, want login.internal", body["code"])
	}
}

// =============================================================================
// Logout tests
// =============================================================================

func TestUserAuthLogout_HappyPath(t *testing.T) {
	svc := &mockUserService{}
	h := NewAuthHandler(svc)

	code, body := doLogout(t, h, "Bearer raw-token-xyz")

	if code != http.StatusOK {
		t.Errorf("status = %d, want 200", code)
	}
	if body["code"] != "logout.ok" {
		t.Errorf("body.code = %v, want logout.ok", body["code"])
	}
	if svc.logoutCalls != 1 {
		t.Errorf("Logout called %d times, want 1", svc.logoutCalls)
	}
	if svc.lastLogout != "raw-token-xyz" {
		t.Errorf("lastLogout = %q, want raw-token-xyz", svc.lastLogout)
	}
}

func TestUserAuthLogout_MissingHeader_Idempotent(t *testing.T) {
	svc := &mockUserService{}
	h := NewAuthHandler(svc)

	code, body := doLogout(t, h, "")

	if code != http.StatusOK {
		t.Errorf("status = %d, want 200 (idempotent)", code)
	}
	if body["code"] != "logout.ok" {
		t.Errorf("body.code = %v, want logout.ok", body["code"])
	}
	if svc.logoutCalls != 0 {
		t.Errorf("Logout should NOT be called when no header")
	}
}

func TestUserAuthLogout_NonBearerScheme_Idempotent(t *testing.T) {
	svc := &mockUserService{}
	h := NewAuthHandler(svc)

	code, body := doLogout(t, h, "Token abc.def")

	if code != http.StatusOK {
		t.Errorf("status = %d, want 200 (idempotent)", code)
	}
	if svc.logoutCalls != 0 {
		t.Errorf("Logout should NOT be called for non-Bearer scheme")
	}
	_ = body
}

func TestUserAuthLogout_LowercaseBearer_PassesThrough(t *testing.T) {
	svc := &mockUserService{}
	h := NewAuthHandler(svc)

	code, _ := doLogout(t, h, "bearer raw-token")

	if code != http.StatusOK {
		t.Errorf("status = %d, want 200", code)
	}
	if svc.lastLogout != "raw-token" {
		t.Errorf("lastLogout = %q, want raw-token", svc.lastLogout)
	}
}

func TestUserAuthLogout_EmptyBearer_Idempotent(t *testing.T) {
	svc := &mockUserService{}
	h := NewAuthHandler(svc)

	code, _ := doLogout(t, h, "Bearer ")

	if code != http.StatusOK {
		t.Errorf("status = %d, want 200 (idempotent)", code)
	}
	if svc.logoutCalls != 0 {
		t.Errorf("Logout should NOT be called for empty bearer")
	}
}

func TestUserAuthLogout_ServiceError_500(t *testing.T) {
	svc := &mockUserService{
		logoutFunc: func(ctx context.Context, rawToken string) error {
			return errors.New("db: connection refused")
		},
	}
	h := NewAuthHandler(svc)

	code, body := doLogout(t, h, "Bearer raw-token")

	if code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", code)
	}
	if body["code"] != "logout.internal" {
		t.Errorf("body.code = %v, want logout.internal", body["code"])
	}
}

// =============================================================================
// extractBearerToken unit tests (验证 RFC 6750 大小写不敏感)
// =============================================================================

func TestExtractBearerToken_Variants(t *testing.T) {
	cases := []struct {
		name  string
		authz string
		want  string
	}{
		{"standard", "Bearer abc.def", "abc.def"},
		{"lowercase", "bearer abc.def", "abc.def"},
		{"uppercase", "BEARER abc.def", "abc.def"},
		{"mixed", "BeArEr abc.def", "abc.def"},
		{"with_spaces", "Bearer  abc.def  ", "abc.def"},
		{"empty_after", "Bearer ", ""},
		{"non_bearer", "Token abc.def", ""},
		{"empty", "", ""},
		{"too_short", "Bear", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := extractBearerToken(tc.authz)
			if got != tc.want {
				t.Errorf("extractBearerToken(%q) = %q, want %q", tc.authz, got, tc.want)
			}
		})
	}
}
