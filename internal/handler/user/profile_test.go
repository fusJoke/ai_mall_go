package user

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	captchaInfra "ai-go-mall/internal/infra/captcha"
	"ai-go-mall/internal/model"
)

// =============================================================================
// helpers
// =============================================================================

// doProfile 构造 GET /user/profile 请求并跑一遍 AuthHandler.Profile。
//
// userID > 0 时经 withUser 往 context 注入当前用户（等价于 UserAuth 中间件
// 已完成鉴权的状态）；userID == 0 时不注入，模拟「token 合法但用户已不存在」。
func doProfile(t *testing.T, h *AuthHandler, userID int64) (int, map[string]any) {
	t.Helper()

	req := httptest.NewRequest("GET", "/user/profile", nil)
	w := httptest.NewRecorder()

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/user/profile", withUser(userID), h.Profile)
	r.ServeHTTP(w, req)

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v (body=%s)", err, w.Body.String())
	}
	return w.Code, resp
}

// =============================================================================
// Profile tests
// =============================================================================

// TestProfile_HappyPath 覆盖 spec「Scenario: 查询余额」。
//
// 断言：200 + 最新余额 + 敏感字段不外泄。
func TestProfile_HappyPath(t *testing.T) {
	svc := &mockUserService{
		getByIDFunc: func(_ *gin.Context, id int64) (*model.User, error) {
			return &model.User{
				ID:       id,
				Username: "alice",
				Nickname: "Alice",
				Avatar:   "https://cdn.example.com/a.png",
				Mobile:   ptr("13800001111"),
				Email:    ptr("alice@example.com"),
				// 刻意用 4 位小数：验证 handler 走 fmtBalance 的 "%.2f"，
				// 而不是把 float64 原样塞进 JSON。
				Balance:     1005.6789,
				Status:      1,
				LastLoginIp: "127.0.0.1",
			}, nil
		},
	}
	h := NewAuthHandler(svc)

	code, resp := doProfile(t, h, 42)
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (resp=%v)", code, resp)
	}

	if got := resp["id"]; got != float64(42) {
		t.Errorf("id = %v, want 42", got)
	}
	if got := resp["username"]; got != "alice" {
		t.Errorf("username = %v, want alice", got)
	}
	if got := resp["nickname"]; got != "Alice" {
		t.Errorf("nickname = %v, want Alice", got)
	}
	if got := resp["mobile"]; got != "13800001111" {
		t.Errorf("mobile = %v, want 13800001111", got)
	}

	// balance 必须是**字符串**且为 2 位小数。
	balance, ok := resp["balance"].(string)
	if !ok {
		t.Fatalf("balance type = %T, want string (value=%v)", resp["balance"], resp["balance"])
	}
	if balance != "1005.68" {
		t.Errorf("balance = %q, want %q", balance, "1005.68")
	}

	// spec: MUST NOT 返回 password / login_failure / deleted_at 等内部字段。
	for _, forbidden := range []string{"password", "login_failure", "deleted_at", "created_at", "updated_at"} {
		if _, exists := resp[forbidden]; exists {
			t.Errorf("response MUST NOT contain %q, got %v", forbidden, resp[forbidden])
		}
	}

	if svc.getByIDCalls != 1 {
		t.Errorf("GetByID calls = %d, want 1", svc.getByIDCalls)
	}
	if svc.lastGetByID != 42 {
		t.Errorf("GetByID id = %d, want 42（须取自 context 中的当前用户）", svc.lastGetByID)
	}
}

// TestProfile_UnauthorizedWhenNoContextUser 覆盖防御性兜底：
// token 合法但用户在库中已不存在（中间件 lookupUser 返回 nil 且不 abort）。
func TestProfile_UnauthorizedWhenNoContextUser(t *testing.T) {
	svc := &mockUserService{
		getByIDFunc: func(_ *gin.Context, _ int64) (*model.User, error) {
			t.Error("GetByID MUST NOT be called when current user is absent")
			return nil, nil
		},
	}
	h := NewAuthHandler(svc)

	code, resp := doProfile(t, h, 0)
	if code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 (resp=%v)", code, resp)
	}
	if got := resp["code"]; got != "profile.unauthorized" {
		t.Errorf("code = %v, want profile.unauthorized", got)
	}
	if svc.getByIDCalls != 0 {
		t.Errorf("GetByID calls = %d, want 0", svc.getByIDCalls)
	}
}

// TestProfile_InternalOnRepoError 覆盖 spec 列出的 500 user.internal。
//
// 这条路径是「handler 仍走 service 读一次库」而非直接用 context 实体的核心理由：
// 让 spec 声明的失败码真实可达。
func TestProfile_InternalOnRepoError(t *testing.T) {
	svc := &mockUserService{
		getByIDFunc: func(_ *gin.Context, _ int64) (*model.User, error) {
			return nil, errors.New("boom: connection refused")
		},
	}
	h := NewAuthHandler(svc)

	code, resp := doProfile(t, h, 42)
	if code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500 (resp=%v)", code, resp)
	}
	if got := resp["code"]; got != "user.internal" {
		t.Errorf("code = %v, want user.internal", got)
	}
	// 错误细节 MUST NOT 外泄。
	if msg, _ := resp["message"].(string); msg == "" || msg == "boom: connection refused" {
		t.Errorf("message = %q, 期望固定文案而非驱动层 error", msg)
	}
}

// TestProfile_InternalOnNilUser 覆盖「repo 返回 (nil, nil) 但 context 有用户」
// 的竞态分支（查得到 token 的 uid，却读不回该行）。
func TestProfile_InternalOnNilUser(t *testing.T) {
	svc := &mockUserService{
		getByIDFunc: func(_ *gin.Context, _ int64) (*model.User, error) {
			return nil, nil
		},
	}
	h := NewAuthHandler(svc)

	code, resp := doProfile(t, h, 42)
	if code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500 (resp=%v)", code, resp)
	}
	if got := resp["code"]; got != "user.internal" {
		t.Errorf("code = %v, want user.internal", got)
	}
}

// TestProfile_StructMirrorsLoginUser 锁定「与登录响应 user 对象同构」这一契约。
//
// 用同一份 model.User 分别跑 Login 与 Profile，比较两个响应里的 user 对象
// 字段集合与取值 —— 只要有人给其中一侧单独加字段，本测试立即失败。
func TestProfile_StructMirrorsLoginUser(t *testing.T) {
	u := &model.User{
		ID:          7,
		Username:    "bob",
		Nickname:    "Bob",
		Avatar:      "https://cdn.example.com/b.png",
		Mobile:      ptr("13900002222"),
		Email:       ptr("bob@example.com"),
		Balance:     88.5,
		Status:      1,
		LastLoginIp: "10.0.0.1",
	}

	svc := &mockUserService{
		loginFunc: func(_ *gin.Context, _, _, _ string, _ []captchaInfra.Point, _ bool) (*model.User, string, error) {
			return u, "tok", nil
		},
		getByIDFunc: func(_ *gin.Context, _ int64) (*model.User, error) {
			return u, nil
		},
	}
	h := NewAuthHandler(svc)

	_, profileResp := doProfile(t, h, 7)

	loginCode, loginResp := doLogin(t, h, map[string]any{
		"username":    "bob",
		"password":    "secret123",
		"captcha_key": "cap-abc",
		"points":      []map[string]int{{"x": 1, "y": 2}},
	})
	if loginCode != http.StatusOK {
		t.Fatalf("login status = %d, want 200 (resp=%v)", loginCode, loginResp)
	}

	loginUser, ok := loginResp["user"].(map[string]any)
	if !ok {
		t.Fatalf("login response 缺 user 对象: %v", loginResp)
	}

	if got, want := sortedKeys(profileResp), sortedKeys(loginUser); !equalStrings(got, want) {
		t.Errorf("Profile 字段集合 = %v, 登录 user 字段集合 = %v；两者 MUST 同构", got, want)
	}
	for k, want := range loginUser {
		if got := profileResp[k]; got != want {
			t.Errorf("字段 %q: profile = %v, login = %v；取值 MUST 一致", k, got, want)
		}
	}
}

// =============================================================================
// 小工具
// =============================================================================

func ptr[T any](v T) *T { return &v }

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	// 简易插入排序：字段数 < 20，无需引 sort 包。
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && keys[j] < keys[j-1]; j-- {
			keys[j], keys[j-1] = keys[j-1], keys[j]
		}
	}
	return keys
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
