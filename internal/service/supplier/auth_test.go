package supplier

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
	"ai-go-mall/internal/model/mall"
	"ai-go-mall/internal/repository"
	supplierRepo "ai-go-mall/internal/repository/supplier"
)

// =============================================================================
// mock SupplierUserRepository
// =============================================================================

// mockSupplierUserRepo 是 supplierRepo.SupplierUserRepository 的最小可编程实现。
//
// Login 路径触发 GetByUsername + UpdateLoginSuccess；禁止触发通用 Update（防 password 二次哈希）。
// 其余 CRUD 方法给 no-op 默认实现，保证编译期接口满足。
type mockSupplierUserRepo struct {
	getByUsernameFunc      func(c *gin.Context, username string) (*mall.MallSupplierUser, error)
	getBySupplierIDFunc    func(c *gin.Context, sid int64) (*mall.MallSupplierUser, error)
	updateFunc             func(c *gin.Context, entity *mall.MallSupplierUser) error
	updateLoginSuccessFunc func(c *gin.Context, id int64, ip string, at time.Time) error
	updateStatusFunc       func(c *gin.Context, id int64, status int8) error

	getByUsernameCalls      int
	updateCalls             int
	lastUpdate              *mall.MallSupplierUser
	updateLoginSuccessCalls int
	lastLoginSuccessID      int64
	lastLoginSuccessIP      string
	lastLoginSuccessAt      time.Time
	updateStatusCalls       int
	lastUpdateStatusID      int64
	lastUpdateStatus        int8
}

func (m *mockSupplierUserRepo) GetByUsername(c *gin.Context, username string) (*mall.MallSupplierUser, error) {
	m.getByUsernameCalls++
	if m.getByUsernameFunc != nil {
		return m.getByUsernameFunc(c, username)
	}
	return nil, gorm.ErrRecordNotFound
}
func (m *mockSupplierUserRepo) GetBySupplierID(c *gin.Context, sid int64) (*mall.MallSupplierUser, error) {
	if m.getBySupplierIDFunc != nil {
		return m.getBySupplierIDFunc(c, sid)
	}
	return nil, gorm.ErrRecordNotFound
}
func (m *mockSupplierUserRepo) GetByID(c *gin.Context, id int64) (*mall.MallSupplierUser, error) {
	return nil, gorm.ErrRecordNotFound
}
func (m *mockSupplierUserRepo) Create(c *gin.Context, e *mall.MallSupplierUser) error { return nil }
func (m *mockSupplierUserRepo) List(c *gin.Context, opts repository.ListOptions) ([]mall.MallSupplierUser, int64, error) {
	return nil, 0, nil
}
func (m *mockSupplierUserRepo) Update(c *gin.Context, entity *mall.MallSupplierUser) error {
	m.updateCalls++
	m.lastUpdate = entity
	if m.updateFunc != nil {
		return m.updateFunc(c, entity)
	}
	return nil
}
func (m *mockSupplierUserRepo) Delete(c *gin.Context, id int64) error { return nil }

func (m *mockSupplierUserRepo) UpdateLoginSuccess(c *gin.Context, id int64, ip string, at time.Time) error {
	m.updateLoginSuccessCalls++
	m.lastLoginSuccessID = id
	m.lastLoginSuccessIP = ip
	m.lastLoginSuccessAt = at
	if m.updateLoginSuccessFunc != nil {
		return m.updateLoginSuccessFunc(c, id, ip, at)
	}
	return nil
}
func (m *mockSupplierUserRepo) UpdateStatus(c *gin.Context, id int64, status int8) error {
	m.updateStatusCalls++
	m.lastUpdateStatusID = id
	m.lastUpdateStatus = status
	if m.updateStatusFunc != nil {
		return m.updateStatusFunc(c, id, status)
	}
	return nil
}

var _ supplierRepo.SupplierUserRepository = (*mockSupplierUserRepo)(nil)

// =============================================================================
// mock tokenIssuer
// =============================================================================

type mockIssuer struct {
	createFunc func(ctx context.Context, t *model.Token) error
	deleteFunc func(ctx context.Context, rawToken string) error
	clearFunc  func(ctx context.Context, userID int64, tokenType string) error

	createCalls int
	lastToken   *model.Token
	deleteCalls int
	lastDelete  string
	clearCalls  int
	lastClear   struct {
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

var _ tokenIssuer = (*mockIssuer)(nil)

// =============================================================================
// mock captchaVerifier
// =============================================================================

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

var _ captchaVerifier = (*mockCaptcha)(nil)

// =============================================================================
// helpers
// =============================================================================

const (
	testUsername = "supplier-1"
	testPassword = "s3cret-pwd"
)

// testPoints 是 login 测试用的默认 captcha points（任意 2 个点）。
var testPoints = []captchaInfra.Point{{X: 100, Y: 80}, {X: 200, Y: 120}}

func newTestSupplierUser(t *testing.T) *mall.MallSupplierUser {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(testPassword), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("bcrypt.GenerateFromPassword: %v", err)
	}
	return &mall.MallSupplierUser{
		ID:         7,
		SupplierID: 100,
		Username:   testUsername,
		Password:   string(hash),
		Status:     1,
	}
}

func newTestContext() *gin.Context {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/supplier/login", nil)
	return c
}

func newAuthSvc(repo *mockSupplierUserRepo, iss *mockIssuer, captcha *mockCaptcha) AuthService {
	if captcha == nil {
		captcha = &mockCaptcha{}
	}
	// 默认注入「主体启用」的状态读取；主体禁用场景见 TestLogin_SupplierMainDisabled。
	sup := &mockSupplierStatusReader{supplier: &mall.MallSupplier{ID: 100, Status: mall.StatusActive}}
	return NewAuthService(repo, sup, iss, captcha)
}

// =============================================================================
// 测试用例
// =============================================================================

func TestLogin_CaptchaFail(t *testing.T) {
	repo := &mockSupplierUserRepo{}
	iss := &mockIssuer{}
	captcha := &mockCaptcha{
		verifyFunc: func(ctx context.Context, req *captchaInfra.VerifyReq, deleteOnSuccess bool) error {
			return errors.New("captcha: points mismatch")
		},
	}
	svc := newAuthSvc(repo, iss, captcha)

	_, tok, err := svc.Login(newTestContext(), testUsername, testPassword, "cap-key", testPoints, false)
	if !errors.Is(err, ErrInvalidCaptcha) {
		t.Errorf("err = %v, want ErrInvalidCaptcha", err)
	}
	if tok != "" {
		t.Errorf("token = %q, want empty", tok)
	}
	// captcha 失败后不应进入 GetByUsername 分支（防脚本试探）。
	if repo.getByUsernameCalls != 0 {
		t.Errorf("GetByUsername called %d times after captcha fail, want 0", repo.getByUsernameCalls)
	}
	if iss.createCalls != 0 {
		t.Errorf("issuer.Create called %d times after captcha fail, want 0", iss.createCalls)
	}
	// captcha 的 consume=true 语义已通过 mock 字段校验（lastDeleteOnSuccess）。
	if !captcha.lastDeleteOnSuccess {
		t.Errorf("captcha deleteOnSuccess = false, want true (consume 语义)")
	}
}

func TestLogin_UserNotFound(t *testing.T) {
	repo := &mockSupplierUserRepo{
		getByUsernameFunc: func(c *gin.Context, username string) (*mall.MallSupplierUser, error) {
			return nil, gorm.ErrRecordNotFound
		},
	}
	iss := &mockIssuer{}
	svc := newAuthSvc(repo, iss, nil)

	_, tok, err := svc.Login(newTestContext(), testUsername, testPassword, "cap-key", testPoints, false)
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Errorf("err = %v, want ErrInvalidCredentials", err)
	}
	if tok != "" {
		t.Errorf("token = %q, want empty", tok)
	}
	if iss.createCalls != 0 {
		t.Errorf("issuer.Create called %d times, want 0", iss.createCalls)
	}
}

func TestLogin_GetByUsernameErrorPropagates(t *testing.T) {
	wantErr := errors.New("repo: db connection lost")
	repo := &mockSupplierUserRepo{
		getByUsernameFunc: func(c *gin.Context, username string) (*mall.MallSupplierUser, error) {
			return nil, wantErr
		},
	}
	svc := newAuthSvc(repo, &mockIssuer{}, nil)

	_, _, err := svc.Login(newTestContext(), testUsername, testPassword, "cap-key", testPoints, false)
	if !errors.Is(err, wantErr) {
		t.Errorf("err = %v, want %v", err, wantErr)
	}
}

func TestLogin_WrongPassword(t *testing.T) {
	u := newTestSupplierUser(t)
	repo := &mockSupplierUserRepo{
		getByUsernameFunc: func(c *gin.Context, username string) (*mall.MallSupplierUser, error) {
			return u, nil
		},
	}
	iss := &mockIssuer{}
	svc := newAuthSvc(repo, iss, nil)

	_, tok, err := svc.Login(newTestContext(), testUsername, "wrong-password", "cap-key", testPoints, false)
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Errorf("err = %v, want ErrInvalidCredentials", err)
	}
	if tok != "" {
		t.Errorf("token = %q, want empty", tok)
	}
	// 密码错误时不应更新 LoginSuccess / 创建 token。
	if repo.updateLoginSuccessCalls != 0 {
		t.Errorf("UpdateLoginSuccess called %d times on wrong pwd, want 0", repo.updateLoginSuccessCalls)
	}
	if iss.createCalls != 0 {
		t.Errorf("issuer.Create called %d times on wrong pwd, want 0", iss.createCalls)
	}
	// 与 user auth 不同：supplier auth 不做递增锁定，所以也不会触发 UpdateStatus。
	if repo.updateStatusCalls != 0 {
		t.Errorf("UpdateStatus called %d times on wrong pwd, want 0 (no progressive lockout)", repo.updateStatusCalls)
	}
	if iss.clearCalls != 0 {
		t.Errorf("issuer.Clear called %d times on wrong pwd, want 0 (no progressive lockout)", iss.clearCalls)
	}
}

func TestLogin_AccountDisabled(t *testing.T) {
	u := newTestSupplierUser(t)
	u.Status = 0 // 禁用
	repo := &mockSupplierUserRepo{
		getByUsernameFunc: func(c *gin.Context, username string) (*mall.MallSupplierUser, error) {
			return u, nil
		},
	}
	iss := &mockIssuer{}
	svc := newAuthSvc(repo, iss, nil)

	_, tok, err := svc.Login(newTestContext(), testUsername, testPassword, "cap-key", testPoints, false)
	if !errors.Is(err, ErrAccountDisabled) {
		t.Errorf("err = %v, want ErrAccountDisabled", err)
	}
	if tok != "" {
		t.Errorf("token = %q, want empty", tok)
	}
	if iss.createCalls != 0 {
		t.Errorf("issuer.Create called %d times, want 0", iss.createCalls)
	}
}

func TestLogin_Success_ShortTTL(t *testing.T) {
	u := newTestSupplierUser(t)
	repo := &mockSupplierUserRepo{
		getByUsernameFunc: func(c *gin.Context, username string) (*mall.MallSupplierUser, error) {
			return u, nil
		},
	}
	iss := &mockIssuer{}
	svc := newAuthSvc(repo, iss, nil)

	gotU, tok, err := svc.Login(newTestContext(), testUsername, testPassword, "cap-key", testPoints, false)
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if gotU == nil || gotU.ID != 7 {
		t.Errorf("user = %+v, want ID=7", gotU)
	}
	if tok == "" {
		t.Errorf("token = empty, want non-empty")
	}
	if iss.createCalls != 1 {
		t.Errorf("issuer.Create called %d times, want 1", iss.createCalls)
	}
	// token 元数据校验。
	tk := iss.lastToken
	if tk == nil {
		t.Fatalf("lastToken = nil")
	}
	if tk.Type != tokenTypeSupplier {
		t.Errorf("token.Type = %q, want %q", tk.Type, tokenTypeSupplier)
	}
	if tk.UserID != 7 {
		t.Errorf("token.UserID = %d, want 7", tk.UserID)
	}
	if tk.Token != tok {
		t.Errorf("token.Token = %q, want %q", tk.Token, tok)
	}
	// TTL 应 ≈ TokenTTLShort（3 天）：time.Until(expires_at) 应在 TokenTTLShort ± 1s 内。
	ttl := time.Until(tk.ExpiresAt)
	if ttl < TokenTTLShort-time.Second || ttl > TokenTTLShort+time.Second {
		t.Errorf("TTL = %s, want ~%s", ttl, TokenTTLShort)
	}
	if ttl <= 0 {
		t.Errorf("ExpiresAt not in future, %s", tk.ExpiresAt)
	}

	// 副作用：UpdateLoginSuccess 应被调用一次，IP / Time 已记录。
	if repo.updateLoginSuccessCalls != 1 {
		t.Errorf("UpdateLoginSuccess called %d times, want 1", repo.updateLoginSuccessCalls)
	}
	if repo.lastLoginSuccessID != 7 {
		t.Errorf("UpdateLoginSuccess id = %d, want 7", repo.lastLoginSuccessID)
	}
	if repo.lastLoginSuccessAt.IsZero() {
		t.Errorf("UpdateLoginSuccess at not set")
	}
	// 禁止走通用 Update（防 password 二次哈希）。
	if repo.updateCalls != 0 {
		t.Errorf("repo.Update called %d times, want 0 (must use UpdateLoginSuccess)", repo.updateCalls)
	}
}

func TestLogin_Success_RememberMe_LongTTL(t *testing.T) {
	u := newTestSupplierUser(t)
	repo := &mockSupplierUserRepo{
		getByUsernameFunc: func(c *gin.Context, username string) (*mall.MallSupplierUser, error) {
			return u, nil
		},
	}
	iss := &mockIssuer{}
	svc := newAuthSvc(repo, iss, nil)

	_, tok, err := svc.Login(newTestContext(), testUsername, testPassword, "cap-key", testPoints, true)
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if tok == "" {
		t.Errorf("token = empty, want non-empty")
	}
	if iss.createCalls != 1 {
		t.Errorf("issuer.Create called %d times, want 1", iss.createCalls)
	}
	// TTL 应 ≈ TokenTTLRemember（30 天）。
	if iss.lastToken == nil {
		t.Fatalf("lastToken = nil")
	}
	expectedExpiry := time.Now().Add(TokenTTLRemember)
	actualExpiry := iss.lastToken.ExpiresAt
	if actualExpiry.Before(expectedExpiry.Add(-time.Minute)) ||
		actualExpiry.After(expectedExpiry.Add(time.Minute)) {
		t.Errorf("ExpiresAt = %s, want ~%s (TokenTTLRemember)", actualExpiry, expectedExpiry)
	}
}

func TestLogin_UpdateLoginSuccessErrorPropagates(t *testing.T) {
	u := newTestSupplierUser(t)
	wantErr := errors.New("repo: login success write failed")
	repo := &mockSupplierUserRepo{
		getByUsernameFunc: func(c *gin.Context, username string) (*mall.MallSupplierUser, error) {
			return u, nil
		},
		updateLoginSuccessFunc: func(c *gin.Context, id int64, ip string, at time.Time) error {
			return wantErr
		},
	}
	svc := newAuthSvc(repo, &mockIssuer{}, nil)

	_, tok, err := svc.Login(newTestContext(), testUsername, testPassword, "cap-key", testPoints, false)
	if !errors.Is(err, wantErr) {
		t.Errorf("err = %v, want %v", err, wantErr)
	}
	if tok != "" {
		t.Errorf("token = %q, want empty (login success write failed)", tok)
	}
}

func TestLogin_TokenCreateErrorPropagates(t *testing.T) {
	u := newTestSupplierUser(t)
	wantErr := errors.New("token: db insert failed")
	repo := &mockSupplierUserRepo{
		getByUsernameFunc: func(c *gin.Context, username string) (*mall.MallSupplierUser, error) {
			return u, nil
		},
	}
	iss := &mockIssuer{
		createFunc: func(ctx context.Context, t *model.Token) error {
			return wantErr
		},
	}
	svc := newAuthSvc(repo, iss, nil)

	_, _, err := svc.Login(newTestContext(), testUsername, testPassword, "cap-key", testPoints, false)
	if !errors.Is(err, wantErr) {
		t.Errorf("err = %v, want %v", err, wantErr)
	}
	// token 创建失败时，UpdateLoginSuccess 已落库 —— 这是预期的非原子行为。
	if repo.updateLoginSuccessCalls != 1 {
		t.Errorf("UpdateLoginSuccess called %d times, want 1 (pre-token side-effect)", repo.updateLoginSuccessCalls)
	}
}

func TestLogout_DelegatesToIssuer(t *testing.T) {
	iss := &mockIssuer{}
	svc := newAuthSvc(&mockSupplierUserRepo{}, iss, nil)
	if err := svc.Logout(context.Background(), "raw-token-xyz"); err != nil {
		t.Fatalf("Logout: %v", err)
	}
	if iss.deleteCalls != 1 {
		t.Errorf("issuer.Delete called %d times, want 1", iss.deleteCalls)
	}
	if iss.lastDelete != "raw-token-xyz" {
		t.Errorf("lastDelete = %q, want raw-token-xyz", iss.lastDelete)
	}
}

// mockSupplierStatusReader 是 supplierStatusReader 的测试桩（final review
// Important #3：登录必须校验供应商主体状态，被禁主体 → ErrAccountDisabled）。
type mockSupplierStatusReader struct {
	supplier *mall.MallSupplier
	err      error

	getByIDCalls int
}

func (m *mockSupplierStatusReader) GetByID(c *gin.Context, id int64) (*mall.MallSupplier, error) {
	m.getByIDCalls++
	if m.err != nil {
		return nil, m.err
	}
	return m.supplier, nil
}

// TestLogin_SupplierMainDisabled 账号本身启用，但所属供应商主体被 admin 禁用
// → 登录必须拒绝（spec Scenario "Disabled supplier cannot login"）。
func TestLogin_SupplierMainDisabled(t *testing.T) {
	u := newTestSupplierUser(t) // u.Status = 1（账号启用）
	repo := &mockSupplierUserRepo{
		getByUsernameFunc: func(c *gin.Context, username string) (*mall.MallSupplierUser, error) {
			return u, nil
		},
	}
	iss := &mockIssuer{}
	sup := &mockSupplierStatusReader{
		supplier: &mall.MallSupplier{ID: 100, Status: mall.StatusDisabled},
	}
	svc := NewAuthService(repo, sup, iss, &mockCaptcha{})

	_, tok, err := svc.Login(newTestContext(), testUsername, testPassword, "cap-key", testPoints, false)
	if !errors.Is(err, ErrAccountDisabled) {
		t.Errorf("err = %v, want ErrAccountDisabled", err)
	}
	if tok != "" {
		t.Errorf("token = %q, want empty", tok)
	}
	if iss.createCalls != 0 {
		t.Errorf("issuer.Create called %d times, want 0", iss.createCalls)
	}
}

// TestLogin_SupplierMainMissing 主体不存在（脏数据）也必须拒绝登录。
func TestLogin_SupplierMainMissing(t *testing.T) {
	u := newTestSupplierUser(t)
	repo := &mockSupplierUserRepo{
		getByUsernameFunc: func(c *gin.Context, username string) (*mall.MallSupplierUser, error) {
			return u, nil
		},
	}
	iss := &mockIssuer{}
	sup := &mockSupplierStatusReader{
		supplier: nil, // GetByID 找不到 → (nil, nil)
	}
	svc := NewAuthService(repo, sup, iss, &mockCaptcha{})

	_, _, err := svc.Login(newTestContext(), testUsername, testPassword, "cap-key", testPoints, false)
	if !errors.Is(err, ErrAccountDisabled) {
		t.Errorf("err = %v, want ErrAccountDisabled", err)
	}
}
