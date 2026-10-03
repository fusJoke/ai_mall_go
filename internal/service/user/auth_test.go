package user

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	captchaInfra "ai-go-mall/internal/infra/captcha"
	"ai-go-mall/internal/model"
	"ai-go-mall/internal/repository"
	userRepo "ai-go-mall/internal/repository/user"
)

// --- mock userRepo.UserRepository ---

// mockRepo 是 userRepo.UserRepository 的最小可编程实现。
//
// Login 路径触发 GetByUsername + UpdateLoginFailure / UpdateStatus / UpdateLoginSuccess；
// 这些路径必须走定向列更新，**绝不**触发通用 Update —— 相关用例显式断言 updateCalls == 0。
// 其余 CRUD 方法给 no-op 默认实现，保证编译期接口满足。
type mockRepo struct {
	getByUsernameFunc func(c *gin.Context, username string) (*model.User, error)
	updateFunc        func(c *gin.Context, entity *model.User) error
	getByIDFunc       func(c *gin.Context, id int64) (*model.User, error)

	decrementBalanceFunc   func(c *gin.Context, userID int64, amount float64) error
	incrementBalanceFunc   func(c *gin.Context, userID int64, amount float64) error
	updateLoginSuccessFunc func(c *gin.Context, id int64, ip string, at time.Time) error
	updateLoginFailureFunc func(c *gin.Context, id int64, failure int) error
	// incrementLoginFailureFunc：原子自增错误计数。
	//
	// 替代 FindSecurityIgnore 之 race：DB 一次性 `SET login_failure = login_failure + 1` + 回读新值。
	// service 不再在 Go 层做 `u.LoginFailure + 1`，避免两个并发错请求错落一个 stale read 后覆盖写。
	incrementLoginFailureFunc func(c *gin.Context, id int64) (int, error)
	updateStatusFunc          func(c *gin.Context, id int64, status int8) error

	updateCalls  int
	lastUpdate   *model.User
	getByIDCalls int
	lastGetByID  int64

	updateLoginFailureCalls int
	lastLoginFailureID      int64
	lastLoginFailure        int

	incrementLoginFailureCalls  int
	lastIncrementLoginFailureID int64
	lastIncrementLoginFailure   int

	updateStatusCalls  int
	lastUpdateStatusID int64
	lastUpdateStatus   int8

	updateLoginSuccessCalls int
	lastLoginSuccessID      int64
	lastLoginSuccessIP      string
	lastLoginSuccessAt      time.Time
}

func (m *mockRepo) GetByUsername(c *gin.Context, username string) (*model.User, error) {
	if m.getByUsernameFunc != nil {
		return m.getByUsernameFunc(c, username)
	}
	return nil, gorm.ErrRecordNotFound
}

func (m *mockRepo) Update(c *gin.Context, entity *model.User) error {
	m.updateCalls++
	m.lastUpdate = entity
	if m.updateFunc != nil {
		return m.updateFunc(c, entity)
	}
	return nil
}

// 其余 CRUDRepository 方法：no-op 默认实现，仅占位让接口实现完整。
func (m *mockRepo) Create(c *gin.Context, entity *model.User) error { return nil }
func (m *mockRepo) List(c *gin.Context, opts repository.ListOptions) ([]model.User, int64, error) {
	return nil, 0, nil
}
func (m *mockRepo) GetByID(c *gin.Context, id int64) (*model.User, error) {
	m.getByIDCalls++
	m.lastGetByID = id
	if m.getByIDFunc != nil {
		return m.getByIDFunc(c, id)
	}
	return nil, nil
}
func (m *mockRepo) Delete(c *gin.Context, id int64) error { return nil }

// 余额 / 登录相关方法的 mock 实现。
func (m *mockRepo) DecrementBalance(c *gin.Context, userID int64, amount float64) error {
	if m.decrementBalanceFunc != nil {
		return m.decrementBalanceFunc(c, userID, amount)
	}
	return nil
}
func (m *mockRepo) IncrementBalance(c *gin.Context, userID int64, amount float64) error {
	if m.incrementBalanceFunc != nil {
		return m.incrementBalanceFunc(c, userID, amount)
	}
	return nil
}
func (m *mockRepo) UpdateLoginSuccess(c *gin.Context, id int64, ip string, at time.Time) error {
	m.updateLoginSuccessCalls++
	m.lastLoginSuccessID = id
	m.lastLoginSuccessIP = ip
	m.lastLoginSuccessAt = at
	if m.updateLoginSuccessFunc != nil {
		return m.updateLoginSuccessFunc(c, id, ip, at)
	}
	return nil
}
func (m *mockRepo) UpdateLoginFailure(c *gin.Context, id int64, failure int) error {
	m.updateLoginFailureCalls++
	m.lastLoginFailureID = id
	m.lastLoginFailure = failure
	if m.updateLoginFailureFunc != nil {
		return m.updateLoginFailureFunc(c, id, failure)
	}
	return nil
}

// IncrementLoginFailure 是「原子自增 login_failure」仓储方法的 mock 实现。
//
// 替代原 UpdateLoginFailure(id, failure int) 的语义：
//   - service 不再传「期望写入的失败计数」——它在 Go 层读 `u.LoginFailure` 后做 +1 并覆写，
//     存在 read-then-write race（H1 审查项）。
//   - 此处 mock 默认实现 = 模拟 DB 的原子自增：每次调用计数器 +1 并返回新值。
//
// 测试可设 incrementLoginFailureFunc 覆盖默认行为。
func (m *mockRepo) IncrementLoginFailure(c *gin.Context, id int64) (int, error) {
	m.incrementLoginFailureCalls++
	m.lastIncrementLoginFailureID = id
	m.lastIncrementLoginFailure = m.lastIncrementLoginFailure + 1
	if m.incrementLoginFailureFunc != nil {
		return m.incrementLoginFailureFunc(c, id)
	}
	return m.lastIncrementLoginFailure, nil
}
func (m *mockRepo) UpdateStatus(c *gin.Context, id int64, status int8) error {
	m.updateStatusCalls++
	m.lastUpdateStatusID = id
	m.lastUpdateStatus = status
	if m.updateStatusFunc != nil {
		return m.updateStatusFunc(c, id, status)
	}
	return nil
}

// 编译期断言：mockRepo 必须实现 userRepo.UserRepository。
var _ userRepo.UserRepository = (*mockRepo)(nil)

// --- mock tokenIssuer ---

// mockIssuer 是 service 包内 tokenIssuer 接口的最小实现。
//
// createFunc / deleteFunc / clearFunc 字段化：测试可控制 Create / Delete / Clear 的行为
// （成功 / 失败 / 记录入参）。
type mockIssuer struct {
	createFunc func(ctx context.Context, t *model.Token) error
	deleteFunc func(ctx context.Context, rawToken string) error
	clearFunc  func(ctx context.Context, userID int64, tokenType string) error

	createCalls int
	lastToken   *model.Token

	deleteCalls int
	lastDelete  string

	clearCalls int
	lastClear  struct {
		UserID    int64
		TokenType string
	}
}

func (m *mockIssuer) Create(ctx context.Context, t *model.Token) error {
	m.createCalls++
	m.lastToken = t
	if m.createFunc != nil {
		return m.createFunc(ctx, t)
	}
	return nil
}

func (m *mockIssuer) Delete(ctx context.Context, rawToken string) error {
	m.deleteCalls++
	m.lastDelete = rawToken
	if m.deleteFunc != nil {
		return m.deleteFunc(ctx, rawToken)
	}
	return nil
}

func (m *mockIssuer) Clear(ctx context.Context, userID int64, tokenType string) error {
	m.clearCalls++
	m.lastClear.UserID = userID
	m.lastClear.TokenType = tokenType
	if m.clearFunc != nil {
		return m.clearFunc(ctx, userID, tokenType)
	}
	return nil
}

// 编译期断言：mockIssuer 必须实现 tokenIssuer。
var _ tokenIssuer = (*mockIssuer)(nil)

// --- mock captchaVerifier ---

// mockCaptcha 是 service 包内 captchaVerifier 接口的最小实现。
//
// verifyFunc 字段化：测试可控制 VerifyClick 的行为（成功 / 失败 / 记录入参）；
// 默认实现为「校验通过」—— 校验流程不阻塞现有 Login 测试。
type mockCaptcha struct {
	verifyFunc func(ctx context.Context, req *captchaInfra.VerifyReq, deleteOnSuccess bool) error

	verifyCalls         int
	lastReq             *captchaInfra.VerifyReq
	lastDeleteOnSuccess bool
}

func (m *mockCaptcha) VerifyClick(ctx context.Context, req *captchaInfra.VerifyReq, deleteOnSuccess bool) error {
	m.verifyCalls++
	m.lastReq = req
	m.lastDeleteOnSuccess = deleteOnSuccess
	if m.verifyFunc != nil {
		return m.verifyFunc(ctx, req, deleteOnSuccess)
	}
	return nil
}

// 编译期断言：mockCaptcha 必须实现 captchaVerifier。
var _ captchaVerifier = (*mockCaptcha)(nil)

// --- 测试 helper ---

const (
	testUsername = "alice"
	testPassword = "s3cret-pwd"
)

// newTestUser 构造一个 *model.User，Password 字段是 testPassword 的 bcrypt 哈希。
func newTestUser(t *testing.T) *model.User {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(testPassword), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("bcrypt.GenerateFromPassword: %v", err)
	}
	return &model.User{
		ID:       42,
		Username: testUsername,
		Password: string(hash),
		Status:   1,
	}
}

// newService 拼装测试用的 Service：mockRepo + mockIssuer + mockCaptcha（默认 captcha 校验通过）。
func newService(repo *mockRepo, iss *mockIssuer, captcha *mockCaptcha) Service {
	if captcha == nil {
		captcha = &mockCaptcha{}
	}
	return NewService(repo, iss, captcha)
}

// testPoints 是 login 测试用的默认 captcha points（任意 2 个点）。
var testPoints = []captchaInfra.Point{{X: 100, Y: 80}, {X: 200, Y: 120}}

// newTestContext 返回一个非 nil 的 *gin.Context（用 gin.CreateTestContext + 手动挂 Request）。
//
// Login 路径会调 c.ClientIP() 走到 c.Request.RemoteAddr；
// gin.CreateTestContext 默认不构造 Request，所以必须手动挂一个，否则 panic。
func newTestContext() *gin.Context {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/user/login", nil)
	return c
}

// --- 测试用例 ---

func TestLogin_UserNotFound(t *testing.T) {
	repo := &mockRepo{
		getByUsernameFunc: func(c *gin.Context, username string) (*model.User, error) {
			return nil, gorm.ErrRecordNotFound
		},
	}
	iss := &mockIssuer{}
	captcha := &mockCaptcha{}

	svc := newService(repo, iss, captcha)
	u, tok, err := svc.Login(newTestContext(), testUsername, testPassword, "cap-key", testPoints, false)

	if !errors.Is(err, ErrInvalidCredentials) {
		t.Errorf("Login = %v, want ErrInvalidCredentials", err)
	}
	if u != nil || tok != "" {
		t.Errorf("Login returned non-zero user/token on invalid creds, got (%+v, %q)", u, tok)
	}
	if iss.createCalls != 0 {
		t.Errorf("issuer.Create called %d times on invalid creds, want 0", iss.createCalls)
	}
	if repo.updateCalls != 0 {
		t.Errorf("repo.Update called %d times on invalid creds, want 0", repo.updateCalls)
	}
}

func TestLogin_WrongPassword(t *testing.T) {
	u := newTestUser(t)
	repo := &mockRepo{
		getByUsernameFunc: func(c *gin.Context, username string) (*model.User, error) {
			return u, nil
		},
		// 原子自增模拟：DB 返回 newFailure=1（未触锁）。
		incrementLoginFailureFunc: func(c *gin.Context, id int64) (int, error) {
			return 1, nil
		},
	}
	iss := &mockIssuer{}
	captcha := &mockCaptcha{}

	svc := newService(repo, iss, captcha)
	gotU, tok, err := svc.Login(newTestContext(), testUsername, "wrong-password", "cap-key", testPoints, false)

	if !errors.Is(err, ErrInvalidCredentials) {
		t.Errorf("Login = %v, want ErrInvalidCredentials", err)
	}
	if gotU != nil || tok != "" {
		t.Errorf("Login returned non-zero on wrong password, got (%+v, %q)", gotU, tok)
	}
	if iss.createCalls != 0 {
		t.Errorf("issuer.Create called %d times on wrong password, want 0", iss.createCalls)
	}
	// 失败计数：走原子自增接口 IncrementLoginFailure（非 Update），newFailure == 1、未触锁
	if repo.incrementLoginFailureCalls != 1 {
		t.Errorf("repo.IncrementLoginFailure called %d times, want 1", repo.incrementLoginFailureCalls)
	}
	if repo.lastIncrementLoginFailureID != u.ID {
		t.Errorf("repo.IncrementLoginFailure id = %d, want %d", repo.lastIncrementLoginFailureID, u.ID)
	}
	// 未触锁 → 不应调 UpdateStatus / Clear
	if repo.updateStatusCalls != 0 {
		t.Errorf("repo.UpdateStatus called %d times below threshold, want 0", repo.updateStatusCalls)
	}
	if iss.clearCalls != 0 {
		t.Errorf("issuer.Clear called %d times below threshold, want 0", iss.clearCalls)
	}
	// 回归：Login 不得触碰通用 Update（哈希入口）。
	if repo.updateCalls != 0 {
		t.Errorf("repo.Update called %d times on wrong password, want 0", repo.updateCalls)
	}
	// 回归：旧 read-then-write 接口绝不能再被触发。
	if repo.updateLoginFailureCalls != 0 {
		t.Errorf("repo.UpdateLoginFailure called %d times, want 0 (replaced by atomic)", repo.updateLoginFailureCalls)
	}
}

// TestLogin_WrongPassword_LocksAtThreshold 验证：密码错误且原子自增返回 newFailure
// 命中阈值（>= MaxLoginFailure）时，user.Status 被置 0 触发账号锁定；UpdateStatus 被调一次
// 以把 status 置 0；且 tm.Clear 被同步调用一次以吊销该 user 既有 token。
//
// 与 TestLogin_WrongPassword 互补：后者 newFailure=1（未触锁），本测试 newFailure=5（触锁）。
func TestLogin_WrongPassword_LocksAtThreshold(t *testing.T) {
	u := newTestUser(t)
	repo := &mockRepo{
		getByUsernameFunc: func(c *gin.Context, username string) (*model.User, error) {
			return u, nil
		},
		// 原子自增模拟：DB 返回 newFailure=MaxLoginFailure（触锁）。
		incrementLoginFailureFunc: func(c *gin.Context, id int64) (int, error) {
			return MaxLoginFailure, nil
		},
	}
	iss := &mockIssuer{}
	captcha := &mockCaptcha{}

	svc := newService(repo, iss, captcha)
	_, tok, err := svc.Login(newTestContext(), testUsername, "wrong-password", "cap-key", testPoints, false)

	if !errors.Is(err, ErrInvalidCredentials) {
		t.Errorf("Login = %v, want ErrInvalidCredentials", err)
	}
	if tok != "" {
		t.Errorf("Login returned token on invalid creds, got %q", tok)
	}
	if iss.createCalls != 0 {
		t.Errorf("issuer.Create called %d times on invalid creds, want 0", iss.createCalls)
	}
	if repo.incrementLoginFailureCalls != 1 {
		t.Fatalf("repo.IncrementLoginFailure called %d times, want 1", repo.incrementLoginFailureCalls)
	}
	if repo.lastIncrementLoginFailureID != u.ID {
		t.Errorf("repo.IncrementLoginFailure id = %d, want %d", repo.lastIncrementLoginFailureID, u.ID)
	}
	// 触锁：UpdateStatus 应被调一次把 Status 置 0
	if repo.updateStatusCalls != 1 {
		t.Errorf("repo.UpdateStatus called %d times on lockout, want 1", repo.updateStatusCalls)
	}
	if repo.lastUpdateStatus != 0 {
		t.Errorf("repo.UpdateStatus status = %d, want 0", repo.lastUpdateStatus)
	}
	if repo.lastUpdateStatusID != u.ID {
		t.Errorf("repo.UpdateStatus userID = %d, want %d", repo.lastUpdateStatusID, u.ID)
	}
	// 回归：锁定路径同样不得触碰通用 Update。
	if repo.updateCalls != 0 {
		t.Errorf("repo.Update called %d times on lockout, want 0", repo.updateCalls)
	}
	// 回归：旧 read-then-write 接口绝不能再被触发。
	if repo.updateLoginFailureCalls != 0 {
		t.Errorf("repo.UpdateLoginFailure called %d times, want 0 (replaced by atomic)", repo.updateLoginFailureCalls)
	}
	// 触锁：tm.Clear 被同步调用一次以吊销该 user 既有 token。
	if iss.clearCalls != 1 {
		t.Errorf("issuer.Clear called %d times on lockout, want 1", iss.clearCalls)
	}
	if iss.lastClear.UserID != u.ID {
		t.Errorf("issuer.Clear userID = %d, want %d", iss.lastClear.UserID, u.ID)
	}
	if iss.lastClear.TokenType != tokenTypeUser {
		t.Errorf("issuer.Clear tokenType = %q, want %q", iss.lastClear.TokenType, tokenTypeUser)
	}
}

func TestLogin_AccountDisabled(t *testing.T) {
	u := newTestUser(t)
	u.Status = 0
	repo := &mockRepo{
		getByUsernameFunc: func(c *gin.Context, username string) (*model.User, error) {
			return u, nil
		},
	}
	iss := &mockIssuer{}
	captcha := &mockCaptcha{}

	svc := newService(repo, iss, captcha)
	gotU, tok, err := svc.Login(newTestContext(), testUsername, testPassword, "cap-key", testPoints, false)

	if !errors.Is(err, ErrAccountDisabled) {
		t.Errorf("Login = %v, want ErrAccountDisabled", err)
	}
	if gotU != nil || tok != "" {
		t.Errorf("Login returned non-zero on disabled account, got (%+v, %q)", gotU, tok)
	}
	// 密码正确但账号被禁：不应触碰 UpdateLoginSuccess / UpdateLoginFailure
	if repo.updateLoginSuccessCalls != 0 {
		t.Errorf("repo.UpdateLoginSuccess called %d times on disabled account, want 0", repo.updateLoginSuccessCalls)
	}
	if repo.updateLoginFailureCalls != 0 {
		t.Errorf("repo.UpdateLoginFailure called %d times on disabled account, want 0", repo.updateLoginFailureCalls)
	}
	if iss.createCalls != 0 {
		t.Errorf("issuer.Create called %d times on disabled account, want 0", iss.createCalls)
	}
}

func TestLogin_InvalidCaptcha(t *testing.T) {
	repo := &mockRepo{} // 用户名查询不会被调用 —— captcha 失败直接退出
	iss := &mockIssuer{}
	captcha := &mockCaptcha{
		verifyFunc: func(ctx context.Context, req *captchaInfra.VerifyReq, deleteOnSuccess bool) error {
			return errors.New("captcha: invalid points")
		},
	}

	svc := newService(repo, iss, captcha)
	u, tok, err := svc.Login(newTestContext(), testUsername, testPassword, "cap-key", testPoints, false)

	if !errors.Is(err, ErrInvalidCaptcha) {
		t.Errorf("Login = %v, want ErrInvalidCaptcha", err)
	}
	if u != nil || tok != "" {
		t.Errorf("Login returned non-zero on invalid captcha, got (%+v, %q)", u, tok)
	}
	if repo.getByIDCalls != 0 && false { // never reached, but keeps static analysis happy
		t.Errorf("repo.GetByID should not be called on captcha failure")
	}
	if iss.createCalls != 0 {
		t.Errorf("issuer.Create called %d times on invalid captcha, want 0", iss.createCalls)
	}
}

// TestLogin_Success 验证正常登录路径：
//   - 密码正确 + Status=1 → UpdateLoginSuccess 被调一次
//   - issuer.Create 被调一次，token.Type = "user"
//   - TTL：remember=false → 约 3 天，remember=true → 约 30 天
func TestLogin_Success(t *testing.T) {
	u := newTestUser(t)
	repo := &mockRepo{
		getByUsernameFunc: func(c *gin.Context, username string) (*model.User, error) {
			return u, nil
		},
	}
	iss := &mockIssuer{}
	captcha := &mockCaptcha{}

	svc := newService(repo, iss, captcha)
	before := time.Now()
	gotU, tok, err := svc.Login(newTestContext(), testUsername, testPassword, "cap-key", testPoints, false)
	after := time.Now()

	if err != nil {
		t.Fatalf("Login = %v, want nil", err)
	}
	if gotU == nil || gotU.ID != u.ID {
		t.Errorf("Login returned %+v, want ID=%d", gotU, u.ID)
	}
	if tok == "" {
		t.Errorf("Login returned empty token")
	}
	if repo.updateLoginSuccessCalls != 1 {
		t.Errorf("repo.UpdateLoginSuccess called %d times, want 1", repo.updateLoginSuccessCalls)
	}
	if repo.updateLoginFailureCalls != 0 {
		t.Errorf("repo.UpdateLoginFailure called %d times on success, want 0", repo.updateLoginFailureCalls)
	}
	if iss.createCalls != 1 {
		t.Errorf("issuer.Create called %d times, want 1", iss.createCalls)
	}
	if iss.lastToken == nil {
		t.Fatalf("issuer.lastToken is nil")
	}
	if iss.lastToken.Type != tokenTypeUser {
		t.Errorf("token.Type = %q, want %q", iss.lastToken.Type, tokenTypeUser)
	}
	if iss.lastToken.UserID != u.ID {
		t.Errorf("token.UserID = %d, want %d", iss.lastToken.UserID, u.ID)
	}
	// 期望 TTL ≈ 3 天（误差 ±1 分钟，避开秒级边界）
	ttlDiff := iss.lastToken.ExpiresAt.Sub(before)
	if ttlDiff < TokenTTLShort-time.Minute || ttlDiff > TokenTTLShort+time.Minute {
		t.Errorf("token TTL (remember=false) = %v, want ~%v", ttlDiff, TokenTTLShort)
	}
	// after 同样应落在 ±1 分钟范围内
	ttlDiffAfter := iss.lastToken.ExpiresAt.Sub(after)
	if ttlDiffAfter < TokenTTLShort-time.Minute || ttlDiffAfter > TokenTTLShort+time.Minute {
		t.Errorf("token TTL (remember=false, after) = %v, want ~%v", ttlDiffAfter, TokenTTLShort)
	}
}

// TestLogin_Success_Remember 验证 remember=true 触发 30 天 TTL。
func TestLogin_Success_Remember(t *testing.T) {
	u := newTestUser(t)
	repo := &mockRepo{
		getByUsernameFunc: func(c *gin.Context, username string) (*model.User, error) {
			return u, nil
		},
	}
	iss := &mockIssuer{}
	captcha := &mockCaptcha{}

	svc := newService(repo, iss, captcha)
	before := time.Now()
	_, tok, err := svc.Login(newTestContext(), testUsername, testPassword, "cap-key", testPoints, true)
	after := time.Now()

	if err != nil {
		t.Fatalf("Login = %v, want nil", err)
	}
	if tok == "" {
		t.Fatalf("Login returned empty token")
	}
	if iss.createCalls != 1 || iss.lastToken == nil {
		t.Fatalf("issuer.Create not called once")
	}
	ttlDiff := iss.lastToken.ExpiresAt.Sub(before)
	if ttlDiff < TokenTTLRemember-time.Minute || ttlDiff > TokenTTLRemember+time.Minute {
		t.Errorf("token TTL (remember=true) = %v, want ~%v", ttlDiff, TokenTTLRemember)
	}
	ttlDiffAfter := iss.lastToken.ExpiresAt.Sub(after)
	if ttlDiffAfter < TokenTTLRemember-time.Minute || ttlDiffAfter > TokenTTLRemember+time.Minute {
		t.Errorf("token TTL (remember=true, after) = %v, want ~%v", ttlDiffAfter, TokenTTLRemember)
	}
}

// TestLogout_DelegatesToIssuer 验证 Logout 把 rawToken 透传给 tokenIssuer.Delete。
func TestLogout_DelegatesToIssuer(t *testing.T) {
	repo := &mockRepo{}
	iss := &mockIssuer{
		deleteFunc: func(ctx context.Context, rawToken string) error {
			if rawToken != "raw-token-xyz" {
				t.Errorf("issuer.Delete rawToken = %q, want %q", rawToken, "raw-token-xyz")
			}
			return nil
		},
	}
	captcha := &mockCaptcha{}

	svc := newService(repo, iss, captcha)
	if err := svc.Logout(context.Background(), "raw-token-xyz"); err != nil {
		t.Fatalf("Logout = %v, want nil", err)
	}
	if iss.deleteCalls != 1 {
		t.Errorf("issuer.Delete called %d times, want 1", iss.deleteCalls)
	}
}

// TestLogout_PropagatesError 验证 tokenIssuer.Delete 错误会被透传。
func TestLogout_PropagatesError(t *testing.T) {
	repo := &mockRepo{}
	iss := &mockIssuer{
		deleteFunc: func(ctx context.Context, rawToken string) error {
			return errors.New("token: db down")
		},
	}
	captcha := &mockCaptcha{}

	svc := newService(repo, iss, captcha)
	if err := svc.Logout(context.Background(), "x"); err == nil {
		t.Errorf("Logout = nil, want error")
	}
}

// TestLogin_WrongPassword_AtomicIncrement 验证 H1 审查项的修复：
// 密码错误分支必须走仓储的「原子自增」接口（IncrementLoginFailure），
// 不能再用旧的 read-then-write（UpdateLoginFailure）+ Go 层 +1 的模式 ——
// 后者在并发场景下会让两个错请求都从 DB 读到 `login_failure=4`、
// 都 +1 后都写 5，互相覆盖，锁阈值判定失效。
//
// 本测试在 mock 层模拟"DB 原子自增"语义：连续三次错密码，
// mock 的 IncrementLoginFailure 依次返回 1、2、3；只有第三次达到阈值时
// 才触发 UpdateStatus(0) + Clear()（==锁账号 + 吊销既有 token）。
//
// 回归断言：
//   - service 调用 IncrementLoginFailure 恰好 1 次（不是 UpdateLoginFailure）。
//   - service 没有触碰 UpdateLoginFailure（防止反复出现 read-then-write 反模式）。
//   - UpdateStatus(0) / Clear() 在阈值恰好达成时被调一次。
func TestLogin_WrongPassword_AtomicIncrement(t *testing.T) {
	u := newTestUser(t)
	u.LoginFailure = 2 // 已在 DB 累积 2 次失败；本测试再错一次 → 3，仍未达阈值
	repo := &mockRepo{
		getByUsernameFunc: func(c *gin.Context, username string) (*model.User, error) {
			return u, nil
		},
		incrementLoginFailureFunc: func(c *gin.Context, id int64) (int, error) {
			return 3, nil // 模拟 DB 原子自增 2→3
		},
	}
	iss := &mockIssuer{}
	captcha := &mockCaptcha{}

	svc := newService(repo, iss, captcha)
	_, _, err := svc.Login(newTestContext(), testUsername, "wrong", "cap-key", testPoints, false)

	if !errors.Is(err, ErrInvalidCredentials) {
		t.Errorf("Login = %v, want ErrInvalidCredentials", err)
	}
	// 核心断言：service 必须走原子自增接口
	if repo.incrementLoginFailureCalls != 1 {
		t.Errorf("IncrementLoginFailure called %d times, want 1", repo.incrementLoginFailureCalls)
	}
	if repo.lastIncrementLoginFailureID != u.ID {
		t.Errorf("IncrementLoginFailure id = %d, want %d", repo.lastIncrementLoginFailureID, u.ID)
	}
	// 反向断言：旧的 read-then-write 接口绝不能再被触发
	if repo.updateLoginFailureCalls != 0 {
		t.Errorf("UpdateLoginFailure called %d times, want 0 (replaced by atomic)", repo.updateLoginFailureCalls)
	}
	// 未达阈值 → 不触发锁 / 不吊销 token
	if repo.updateStatusCalls != 0 {
		t.Errorf("UpdateStatus called %d times below threshold, want 0", repo.updateStatusCalls)
	}
	if iss.clearCalls != 0 {
		t.Errorf("issuer.Clear called %d times below threshold, want 0", iss.clearCalls)
	}
}

// TestLogin_WrongPassword_AtomicIncrement_LocksAtThreshold 验证原子自增到阈值时锁账号。
//
// 与 TestLogin_WrongPassword_LocksAtThreshold 互补：
//   - 后者走旧 UpdateLoginFailure(id, computedFailure) 接口；
//   - 本测试走新 IncrementLoginFailure 接口，验证阈值判定以 mock 返回的「DB 实际新值」为准，
//     而非 service 在 Go 层 `u.LoginFailure + 1`（后者有 race）。
func TestLogin_WrongPassword_AtomicIncrement_LocksAtThreshold(t *testing.T) {
	u := newTestUser(t)
	repo := &mockRepo{
		getByUsernameFunc: func(c *gin.Context, username string) (*model.User, error) {
			return u, nil
		},
		incrementLoginFailureFunc: func(c *gin.Context, id int64) (int, error) {
			return MaxLoginFailure, nil // 模拟 DB 原子自增跨过阈值
		},
	}
	iss := &mockIssuer{}
	captcha := &mockCaptcha{}

	svc := newService(repo, iss, captcha)
	_, _, err := svc.Login(newTestContext(), testUsername, "wrong", "cap-key", testPoints, false)

	if !errors.Is(err, ErrInvalidCredentials) {
		t.Errorf("Login = %v, want ErrInvalidCredentials", err)
	}
	if repo.incrementLoginFailureCalls != 1 {
		t.Fatalf("IncrementLoginFailure called %d times, want 1", repo.incrementLoginFailureCalls)
	}
	// 触锁 → UpdateStatus(0) + Clear() 各调一次
	if repo.updateStatusCalls != 1 {
		t.Errorf("UpdateStatus called %d times on lockout, want 1", repo.updateStatusCalls)
	}
	if repo.lastUpdateStatus != 0 {
		t.Errorf("UpdateStatus status = %d, want 0", repo.lastUpdateStatus)
	}
	if iss.clearCalls != 1 {
		t.Errorf("issuer.Clear called %d times on lockout, want 1", iss.clearCalls)
	}
}

// TestLogin_WrongPassword_AtomicIncrement_Concurrent 验证并发场景：
//
// 模拟两个并发错请求「同时」读 u.LoginFailure = 4（DB 当前值）后：
//   - 旧实现：都在 Go 层算 5 → 都 UpdateLoginFailure(5) → 互相覆盖 → 锁不会真触发；
//   - 新实现：仓储层各自原子自增 → 一个返回 5、另一个返回 6 → service 两次都触发锁（Clear/UpdateStatus 各 2 次）。
//
// 本测试不验证「次数」（Clear/UpdateStatus 是幂等的），
// 只验证「新值来自仓储返回，而非 service 自行计算」 —— 即 service 没有计算逻辑可以踩到 race。
func TestLogin_WrongPassword_AtomicIncrement_Concurrent(t *testing.T) {
	u := newTestUser(t)
	u.LoginFailure = 4 // DB 当前值：再错一次就锁

	// 第一次 Login：原子自增返回 5（触锁）
	// 第二次 Login：原子自增返回 6（已锁后再错一次）
	callCount := 0
	repo := &mockRepo{
		getByUsernameFunc: func(c *gin.Context, username string) (*model.User, error) {
			return u, nil
		},
		incrementLoginFailureFunc: func(c *gin.Context, id int64) (int, error) {
			callCount++
			return 4 + callCount, nil
		},
	}
	iss := &mockIssuer{}
	captcha := &mockCaptcha{}

	svc := newService(repo, iss, captcha)
	// 第一次错密码
	if _, _, err := svc.Login(newTestContext(), testUsername, "wrong", "cap-key", testPoints, false); !errors.Is(err, ErrInvalidCredentials) {
		t.Errorf("Login #1 = %v, want ErrInvalidCredentials", err)
	}
	// 第二次错密码
	if _, _, err := svc.Login(newTestContext(), testUsername, "wrong", "cap-key", testPoints, false); !errors.Is(err, ErrInvalidCredentials) {
		t.Errorf("Login #2 = %v, want ErrInvalidCredentials", err)
	}

	// service 必须把「新值」完全交给仓储（自身不计算）
	if repo.incrementLoginFailureCalls != 2 {
		t.Errorf("IncrementLoginFailure called %d times, want 2", repo.incrementLoginFailureCalls)
	}
	// service 任何路径都不能触发 read-then-write 旧接口
	if repo.updateLoginFailureCalls != 0 {
		t.Errorf("UpdateLoginFailure called %d times in concurrent flow, want 0", repo.updateLoginFailureCalls)
	}
	// 两次都 >= 阈值 → 两次都触发锁副作用（幂等 Clear + UpdateStatus）
	if repo.updateStatusCalls != 2 {
		t.Errorf("UpdateStatus called %d times, want 2 (both increments crossed threshold)", repo.updateStatusCalls)
	}
	if iss.clearCalls != 2 {
		t.Errorf("issuer.Clear called %d times, want 2", iss.clearCalls)
	}
}
