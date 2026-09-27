package admin

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
	adminRepo "ai-go-mall/internal/repository/admin"
	"ai-go-mall/internal/repository"
)

// --- mock adminRepo.Repository ---

// mockRepo 是 adminRepo.Repository 的最小可编程实现。
//
// Login 路径只触发 GetByUsername 与 Update；其余 CRUD 方法给 no-op 默认实现，
// 保证编译期 adminRepo.Repository 接口满足（接口里其它方法也要在）。
type mockRepo struct {
	getByUsernameFunc func(c *gin.Context, username string) (*model.Admin, error)
	updateFunc        func(c *gin.Context, entity *model.Admin) error

	updateCalls int
	lastUpdate  *model.Admin
}

func (m *mockRepo) GetByUsername(c *gin.Context, username string) (*model.Admin, error) {
	if m.getByUsernameFunc != nil {
		return m.getByUsernameFunc(c, username)
	}
	return nil, gorm.ErrRecordNotFound
}

func (m *mockRepo) Update(c *gin.Context, entity *model.Admin) error {
	m.updateCalls++
	m.lastUpdate = entity
	if m.updateFunc != nil {
		return m.updateFunc(c, entity)
	}
	return nil
}

// 其余 CRUDRepository 方法：no-op 默认实现，仅占位让接口实现完整。
func (m *mockRepo) Create(c *gin.Context, entity *model.Admin) error   { return nil }
func (m *mockRepo) List(c *gin.Context, opts repository.ListOptions) ([]model.Admin, int64, error) {
	return nil, 0, nil
}
func (m *mockRepo) GetByID(c *gin.Context, id int64) (*model.Admin, error) { return nil, nil }
func (m *mockRepo) Delete(c *gin.Context, id int64) error                  { return nil }

// 编译期断言：mockRepo 必须实现 adminRepo.Repository。
var _ adminRepo.Repository = (*mockRepo)(nil)

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

	verifyCalls        int
	lastReq            *captchaInfra.VerifyReq
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

// newTestAdmin 构造一个 *model.Admin，Password 字段是 testPassword 的 bcrypt 哈希。
func newTestAdmin(t *testing.T) *model.Admin {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(testPassword), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("bcrypt.GenerateFromPassword: %v", err)
	}
	return &model.Admin{
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
	c.Request = httptest.NewRequest("POST", "/admin/login", nil)
	return c
}

// --- 测试用例 ---

func TestLogin_UserNotFound(t *testing.T) {
	repo := &mockRepo{
		getByUsernameFunc: func(c *gin.Context, username string) (*model.Admin, error) {
			return nil, gorm.ErrRecordNotFound
		},
	}
	iss := &mockIssuer{}
	captcha := &mockCaptcha{}

	svc := newService(repo, iss, captcha)
	adm, tok, err := svc.Login(newTestContext(), testUsername, testPassword, "cap-key", testPoints, false)

	if !errors.Is(err, ErrInvalidCredentials) {
		t.Errorf("Login = %v, want ErrInvalidCredentials", err)
	}
	if adm != nil || tok != "" {
		t.Errorf("Login returned non-zero admin/token on invalid creds, got (%+v, %q)", adm, tok)
	}
	if iss.createCalls != 0 {
		t.Errorf("issuer.Create called %d times on invalid creds, want 0", iss.createCalls)
	}
	if repo.updateCalls != 0 {
		t.Errorf("repo.Update called %d times on invalid creds, want 0", repo.updateCalls)
	}
}

func TestLogin_WrongPassword(t *testing.T) {
	adm := newTestAdmin(t)
	repo := &mockRepo{
		getByUsernameFunc: func(c *gin.Context, username string) (*model.Admin, error) {
			return adm, nil
		},
	}
	iss := &mockIssuer{}
	captcha := &mockCaptcha{}

	svc := newService(repo, iss, captcha)
	gotAdm, tok, err := svc.Login(newTestContext(), testUsername, "wrong-password", "cap-key", testPoints, false)

	if !errors.Is(err, ErrInvalidCredentials) {
		t.Errorf("Login = %v, want ErrInvalidCredentials", err)
	}
	if gotAdm != nil || tok != "" {
		t.Errorf("Login returned non-zero on wrong password, got (%+v, %q)", gotAdm, tok)
	}
	if iss.createCalls != 0 {
		t.Errorf("issuer.Create called %d times on wrong password, want 0", iss.createCalls)
	}
	// 失败计数递增：Update 应被调一次，且 LoginFailure == 1
	if repo.updateCalls != 1 {
		t.Errorf("repo.Update called %d times, want 1", repo.updateCalls)
	}
	if repo.lastUpdate == nil || repo.lastUpdate.LoginFailure != 1 {
		t.Errorf("repo.Update should set LoginFailure=1, got %+v", repo.lastUpdate)
	}
}

// TestLogin_WrongPassword_LocksAtThreshold 验证：密码错误且累计 LoginFailure 已到
// 阈值（4 → 5）时，admin.Status 被置 0 触发账号锁定；Update 落库带 LoginFailure=5 / Status=0；
// 且 tm.Clear 被同步调用一次以吊销该 admin 既有 token。
//
// 与 TestLogin_WrongPassword 互补：后者从 0 起步，本测试从「再错一次就锁」起步，
// 一起覆盖阈值以下 / 阈值触发的两种边界。
func TestLogin_WrongPassword_LocksAtThreshold(t *testing.T) {
	adm := newTestAdmin(t)
	// 让首次失败直接命中阈值：起始计数 4，加 1 后 = MaxLoginFailure → 锁定。
	adm.LoginFailure = MaxLoginFailure - 1
	repo := &mockRepo{
		getByUsernameFunc: func(c *gin.Context, username string) (*model.Admin, error) {
			return adm, nil
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
	if repo.updateCalls != 1 {
		t.Fatalf("repo.Update called %d times, want 1", repo.updateCalls)
	}
	if repo.lastUpdate == nil {
		t.Fatal("repo.lastUpdate is nil after Update")
	}
	if repo.lastUpdate.LoginFailure != MaxLoginFailure {
		t.Errorf("repo.lastUpdate.LoginFailure = %d, want %d", repo.lastUpdate.LoginFailure, MaxLoginFailure)
	}
	if repo.lastUpdate.Status != 0 {
		t.Errorf("repo.lastUpdate.Status = %d, want 0 (locked)", repo.lastUpdate.Status)
	}
	// N2: 锁定同步触发 token 吊销。
	if iss.clearCalls != 1 {
		t.Errorf("issuer.Clear called %d times on lockout, want 1", iss.clearCalls)
	}
	if iss.lastClear.UserID != adm.ID {
		t.Errorf("issuer.Clear userID = %d, want %d", iss.lastClear.UserID, adm.ID)
	}
	if iss.lastClear.TokenType != tokenTypeAdmin {
		t.Errorf("issuer.Clear tokenType = %q, want %q", iss.lastClear.TokenType, tokenTypeAdmin)
	}
}

// TestLogin_WrongPassword_BelowThreshold_NoClear 验证：密码错误但未达阈值时
// tm.Clear 不被调用 —— Clear 只在「首次跨过阈值」的锁定事件触发，不在每次错密码时都跑。
//
// 防止回归：之前有一次错误实现是「每次错密码都 Clear」，会把正常用户踢下线。
func TestLogin_WrongPassword_BelowThreshold_NoClear(t *testing.T) {
	adm := newTestAdmin(t)
	adm.LoginFailure = 0 // 远低于阈值
	repo := &mockRepo{
		getByUsernameFunc: func(c *gin.Context, username string) (*model.Admin, error) {
			return adm, nil
		},
	}
	iss := &mockIssuer{}
	captcha := &mockCaptcha{}

	svc := newService(repo, iss, captcha)
	_, _, err := svc.Login(newTestContext(), testUsername, "wrong-password", "cap-key", testPoints, false)

	if !errors.Is(err, ErrInvalidCredentials) {
		t.Errorf("Login = %v, want ErrInvalidCredentials", err)
	}
	if iss.clearCalls != 0 {
		t.Errorf("issuer.Clear called %d times below threshold, want 0", iss.clearCalls)
	}
	if repo.lastUpdate == nil || repo.lastUpdate.Status != 1 {
		t.Errorf("repo.Update should keep Status=1 below threshold, got %+v", repo.lastUpdate)
	}
}

// TestLogin_LockedAccount_RejectsCorrectPassword 验证：被锁定（Status=0）的账号即便
// 密码正确也走状态分支返回 ErrAccountDisabled，不会签发 token。
//
// 这条路径在「锁定被触发后，正主来登录」场景下生效；现有的 TestLogin_AccountDisabled
// 也是这条路径，只是测试构造方式不同（手动设 Status=0），这里专门从锁定结果回头测。
func TestLogin_LockedAccount_RejectsCorrectPassword(t *testing.T) {
	adm := newTestAdmin(t)
	adm.Status = 0
	repo := &mockRepo{
		getByUsernameFunc: func(c *gin.Context, username string) (*model.Admin, error) {
			return adm, nil
		},
	}
	iss := &mockIssuer{}
	captcha := &mockCaptcha{}

	svc := newService(repo, iss, captcha)
	_, tok, err := svc.Login(newTestContext(), testUsername, testPassword, "cap-key", testPoints, false)

	if !errors.Is(err, ErrAccountDisabled) {
		t.Errorf("Login = %v, want ErrAccountDisabled", err)
	}
	if tok != "" {
		t.Errorf("Login returned token on locked account, got %q", tok)
	}
	if iss.createCalls != 0 {
		t.Errorf("issuer.Create called %d times on locked account, want 0", iss.createCalls)
	}
}

// TestLogin_Success_ResetsLoginFailure 验证：登录成功会把 LoginFailure 清零（不只更新 last-login 字段）。
//
// 之前 TestLogin_Success_* 只检查 token 签发，没断言 LoginFailure 复位行为；
// 现在锁定阈值生效后这个回归点必须显式覆盖，避免「LoginFailure 越攒越多导致误锁」。
func TestLogin_Success_ResetsLoginFailure(t *testing.T) {
	adm := newTestAdmin(t)
	adm.LoginFailure = 3 // 模拟「之前错了几次但没锁」的状态
	repo := &mockRepo{
		getByUsernameFunc: func(c *gin.Context, username string) (*model.Admin, error) {
			return adm, nil
		},
	}
	iss := &mockIssuer{}
	captcha := &mockCaptcha{}

	svc := newService(repo, iss, captcha)
	_, _, err := svc.Login(newTestContext(), testUsername, testPassword, "cap-key", testPoints, false)
	if err != nil {
		t.Fatalf("Login = %v, want nil", err)
	}
	if repo.updateCalls < 1 {
		t.Fatalf("repo.Update called %d times, want >=1", repo.updateCalls)
	}
	if repo.lastUpdate == nil {
		t.Fatal("repo.lastUpdate is nil after Update")
	}
	if repo.lastUpdate.LoginFailure != 0 {
		t.Errorf("repo.lastUpdate.LoginFailure = %d, want 0 (reset on success)", repo.lastUpdate.LoginFailure)
	}
	if repo.lastUpdate.Status != 1 {
		t.Errorf("repo.lastUpdate.Status = %d, want 1 (unchanged on success)", repo.lastUpdate.Status)
	}
}

func TestLogin_AccountDisabled(t *testing.T) {
	adm := newTestAdmin(t)
	adm.Status = 0 // 禁用
	repo := &mockRepo{
		getByUsernameFunc: func(c *gin.Context, username string) (*model.Admin, error) {
			return adm, nil
		},
	}
	iss := &mockIssuer{}
	captcha := &mockCaptcha{}

	svc := newService(repo, iss, captcha)
	gotAdm, tok, err := svc.Login(newTestContext(), testUsername, testPassword, "cap-key", testPoints, false)

	if !errors.Is(err, ErrAccountDisabled) {
		t.Errorf("Login = %v, want ErrAccountDisabled", err)
	}
	if gotAdm != nil || tok != "" {
		t.Errorf("Login returned non-zero on disabled, got (%+v, %q)", gotAdm, tok)
	}
	if iss.createCalls != 0 {
		t.Errorf("issuer.Create called %d times on disabled, want 0", iss.createCalls)
	}
}

func TestLogin_Success_NoRemember_3DayTTL(t *testing.T) {
	adm := newTestAdmin(t)
	repo := &mockRepo{
		getByUsernameFunc: func(c *gin.Context, username string) (*model.Admin, error) {
			return adm, nil
		},
	}
	iss := &mockIssuer{}
	captcha := &mockCaptcha{}

	svc := newService(repo, iss, captcha)
	before := time.Now()
	gotAdm, rawToken, err := svc.Login(newTestContext(), testUsername, testPassword, "cap-key", testPoints, false)
	after := time.Now()

	if err != nil {
		t.Fatalf("Login = %v, want nil", err)
	}
	if gotAdm == nil || rawToken == "" {
		t.Fatalf("Login returned zero on success, got (%+v, %q)", gotAdm, rawToken)
	}
	if gotAdm.ID != adm.ID {
		t.Errorf("Login returned admin.ID=%d, want %d", gotAdm.ID, adm.ID)
	}
	if iss.createCalls != 1 {
		t.Errorf("issuer.Create called %d times, want 1", iss.createCalls)
	}
	if iss.lastToken == nil {
		t.Fatal("issuer.lastToken is nil after Create")
	}
	if iss.lastToken.Type != tokenTypeAdmin {
		t.Errorf("token.Type = %q, want %q", iss.lastToken.Type, tokenTypeAdmin)
	}
	if iss.lastToken.UserID != adm.ID {
		t.Errorf("token.UserID = %d, want %d", iss.lastToken.UserID, adm.ID)
	}
	if iss.lastToken.Token != rawToken {
		t.Errorf("token.Token = %q, want %q (service 返回的明文)", iss.lastToken.Token, rawToken)
	}

	// ExpiresAt 应在 before+3d 与 after+3d 之间（容差边界）
	wantMin := before.Add(TokenTTLShort)
	wantMax := after.Add(TokenTTLShort)
	if iss.lastToken.ExpiresAt.Before(wantMin) || iss.lastToken.ExpiresAt.After(wantMax.Add(time.Second)) {
		t.Errorf("token.ExpiresAt = %v, want in [%v, %v]", iss.lastToken.ExpiresAt, wantMin, wantMax)
	}
}

func TestLogin_Success_Remember_30DayTTL(t *testing.T) {
	adm := newTestAdmin(t)
	repo := &mockRepo{
		getByUsernameFunc: func(c *gin.Context, username string) (*model.Admin, error) {
			return adm, nil
		},
	}
	iss := &mockIssuer{}
	captcha := &mockCaptcha{}

	svc := newService(repo, iss, captcha)
	before := time.Now()
	_, _, err := svc.Login(newTestContext(), testUsername, testPassword, "cap-key", testPoints, true)
	after := time.Now()

	if err != nil {
		t.Fatalf("Login = %v, want nil", err)
	}
	if iss.createCalls != 1 {
		t.Errorf("issuer.Create called %d times, want 1", iss.createCalls)
	}

	wantMin := before.Add(TokenTTLRemember)
	wantMax := after.Add(TokenTTLRemember)
	if iss.lastToken.ExpiresAt.Before(wantMin) || iss.lastToken.ExpiresAt.After(wantMax.Add(time.Second)) {
		t.Errorf("token.ExpiresAt = %v, want in [%v, %v] (remember=true, ~30d)", iss.lastToken.ExpiresAt, wantMin, wantMax)
	}
}

func TestLogin_TokenCreateFails(t *testing.T) {
	adm := newTestAdmin(t)
	repo := &mockRepo{
		getByUsernameFunc: func(c *gin.Context, username string) (*model.Admin, error) {
			return adm, nil
		},
	}
	wantErr := errors.New("token store down")
	iss := &mockIssuer{
		createFunc: func(ctx context.Context, t *model.Token) error {
			return wantErr
		},
	}
	captcha := &mockCaptcha{}

	svc := newService(repo, iss, captcha)
	gotAdm, tok, err := svc.Login(newTestContext(), testUsername, testPassword, "cap-key", testPoints, false)

	if !errors.Is(err, wantErr) {
		t.Errorf("Login = %v, want %v", err, wantErr)
	}
	if gotAdm != nil || tok != "" {
		t.Errorf("Login returned non-zero on token.Create failure, got (%+v, %q)", gotAdm, tok)
	}
	// admin Update 已经被调一次（写 LastLoginAt 等），这是已知折中
	if repo.updateCalls != 1 {
		t.Errorf("repo.Update called %d times, want 1", repo.updateCalls)
	}
}

// TestLogin_CaptchaFailed_ShortCircuits 验证 captcha 二次校验失败时立即返回 ErrInvalidCaptcha，
// 不进入密码分支（GetByUsername / bcrypt 都不会被触发）。
func TestLogin_CaptchaFailed_ShortCircuits(t *testing.T) {
	wantErr := errors.New("captcha mismatch")
	captcha := &mockCaptcha{
		verifyFunc: func(ctx context.Context, req *captchaInfra.VerifyReq, deleteOnSuccess bool) error {
			return wantErr
		},
	}
	repo := &mockRepo{
		getByUsernameFunc: func(c *gin.Context, username string) (*model.Admin, error) {
			t.Errorf("repo.GetByUsername should NOT be called when captcha fails")
			return nil, gorm.ErrRecordNotFound
		},
	}
	iss := &mockIssuer{}

	svc := newService(repo, iss, captcha)
	adm, tok, err := svc.Login(newTestContext(), testUsername, testPassword, "cap-key", testPoints, false)

	if !errors.Is(err, ErrInvalidCaptcha) {
		t.Errorf("Login = %v, want ErrInvalidCaptcha", err)
	}
	if adm != nil || tok != "" {
		t.Errorf("Login returned non-zero on captcha fail, got (%+v, %q)", adm, tok)
	}
	if iss.createCalls != 0 {
		t.Errorf("issuer.Create called %d times on captcha fail, want 0", iss.createCalls)
	}
	if captcha.verifyCalls != 1 {
		t.Errorf("captcha.Verify called %d times, want 1", captcha.verifyCalls)
	}
	if !captcha.lastDeleteOnSuccess {
		t.Errorf("captcha.Verify should be consume-mode (deleteOnSuccess=true), got false")
	}
}

// TestLogin_CaptchaPassed_ProceedsToPassword 验证 captcha 通过后正常走完密码 + token 签发。
func TestLogin_CaptchaPassed_ProceedsToPassword(t *testing.T) {
	adm := newTestAdmin(t)
	repo := &mockRepo{
		getByUsernameFunc: func(c *gin.Context, username string) (*model.Admin, error) {
			return adm, nil
		},
	}
	iss := &mockIssuer{}
	captcha := &mockCaptcha{}

	svc := newService(repo, iss, captcha)
	_, _, err := svc.Login(newTestContext(), testUsername, testPassword, "cap-key", testPoints, false)

	if err != nil {
		t.Fatalf("Login = %v, want nil", err)
	}
	if captcha.verifyCalls != 1 {
		t.Errorf("captcha.Verify called %d times, want 1", captcha.verifyCalls)
	}
	if iss.createCalls != 1 {
		t.Errorf("issuer.Create called %d times, want 1", iss.createCalls)
	}
}

// TestLogin_EmptyCaptchaKey_StillGoesThroughCaptcha 验证空 captchaKey 时仍走 captcha 校验；
// mockCaptcha 默认通过，验证后续进入密码分支。
func TestLogin_EmptyCaptchaKey_StillGoesThroughCaptcha(t *testing.T) {
	adm := newTestAdmin(t)
	repo := &mockRepo{
		getByUsernameFunc: func(c *gin.Context, username string) (*model.Admin, error) {
			return adm, nil
		},
	}
	iss := &mockIssuer{}
	captcha := &mockCaptcha{
		verifyFunc: func(ctx context.Context, req *captchaInfra.VerifyReq, deleteOnSuccess bool) error {
			// 空 key 应被 infra 层挡掉；这里用 mock 模拟成功，验证链路流转。
			if req.Key != "" {
				t.Errorf("captcha.Verify req.Key = %q, want empty", req.Key)
			}
			return nil
		},
	}

	svc := newService(repo, iss, captcha)
	_, _, err := svc.Login(newTestContext(), testUsername, testPassword, "", testPoints, false)
	if err != nil {
		t.Fatalf("Login = %v, want nil", err)
	}
}

// --- Logout tests ---

// TestLogout_Success 验证：service.Logout 把 rawToken 透传给 tokenIssuer.Delete；
// mock 记录到调用次数与入参，service 自身不产生错误。
func TestLogout_Success(t *testing.T) {
	repo := &mockRepo{}
	iss := &mockIssuer{}
	captcha := &mockCaptcha{}

	svc := newService(repo, iss, captcha)
	if err := svc.Logout(context.Background(), "abc.def"); err != nil {
		t.Fatalf("Logout = %v, want nil", err)
	}
	if iss.deleteCalls != 1 {
		t.Errorf("issuer.Delete called %d times, want 1", iss.deleteCalls)
	}
	if iss.lastDelete != "abc.def" {
		t.Errorf("issuer.Delete got rawToken = %q, want %q", iss.lastDelete, "abc.def")
	}
}

// TestLogout_DeletePropagatesError 验证：tokenIssuer.Delete 失败时 service.Logout
// 原样透传（errors.Is 命中），不吞错也不包额外层。
func TestLogout_DeletePropagatesError(t *testing.T) {
	wantErr := errors.New("token store down")
	repo := &mockRepo{}
	iss := &mockIssuer{
		deleteFunc: func(ctx context.Context, rawToken string) error {
			return wantErr
		},
	}
	captcha := &mockCaptcha{}

	svc := newService(repo, iss, captcha)
	err := svc.Logout(context.Background(), "any-token")
	if !errors.Is(err, wantErr) {
		t.Errorf("Logout = %v, want %v", err, wantErr)
	}
	if iss.deleteCalls != 1 {
		t.Errorf("issuer.Delete called %d times, want 1", iss.deleteCalls)
	}
}

// TestLogout_NoRepoAccess 验证：Logout 流程不触达 admin repo ——
// 只走 tokenIssuer，repo.Update / GetByUsername 等计数应保持 0。
func TestLogout_NoRepoAccess(t *testing.T) {
	repo := &mockRepo{
		getByUsernameFunc: func(c *gin.Context, username string) (*model.Admin, error) {
			t.Errorf("repo.GetByUsername should NOT be called during logout")
			return nil, gorm.ErrRecordNotFound
		},
		updateFunc: func(c *gin.Context, entity *model.Admin) error {
			t.Errorf("repo.Update should NOT be called during logout")
			return nil
		},
	}
	iss := &mockIssuer{}
	captcha := &mockCaptcha{}

	svc := newService(repo, iss, captcha)
	if err := svc.Logout(context.Background(), "any-token"); err != nil {
		t.Fatalf("Logout = %v, want nil", err)
	}
	if repo.updateCalls != 0 {
		t.Errorf("repo.Update called %d times during logout, want 0", repo.updateCalls)
	}
	if iss.deleteCalls != 1 {
		t.Errorf("issuer.Delete called %d times, want 1", iss.deleteCalls)
	}
}