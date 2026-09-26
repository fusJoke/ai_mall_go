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
// createFunc 字段化：测试可控制 Create 的行为（成功 / 失败 / 记录入参）。
type mockIssuer struct {
	createFunc func(ctx context.Context, t *model.Token) error

	createCalls int
	lastToken   *model.Token
}

func (m *mockIssuer) Create(ctx context.Context, t *model.Token) error {
	m.createCalls++
	m.lastToken = t
	if m.createFunc != nil {
		return m.createFunc(ctx, t)
	}
	return nil
}

// 编译期断言：mockIssuer 必须实现 tokenIssuer。
var _ tokenIssuer = (*mockIssuer)(nil)

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

// newService 拼装测试用的 Service：mockRepo + mockIssuer。
func newService(repo *mockRepo, iss *mockIssuer) Service {
	return NewService(repo, iss)
}

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

	svc := newService(repo, iss)
	adm, tok, err := svc.Login(newTestContext(), testUsername, testPassword, false)

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

	svc := newService(repo, iss)
	gotAdm, tok, err := svc.Login(newTestContext(), testUsername, "wrong-password", false)

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

func TestLogin_AccountDisabled(t *testing.T) {
	adm := newTestAdmin(t)
	adm.Status = 0 // 禁用
	repo := &mockRepo{
		getByUsernameFunc: func(c *gin.Context, username string) (*model.Admin, error) {
			return adm, nil
		},
	}
	iss := &mockIssuer{}

	svc := newService(repo, iss)
	gotAdm, tok, err := svc.Login(newTestContext(), testUsername, testPassword, false)

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

	svc := newService(repo, iss)
	before := time.Now()
	gotAdm, rawToken, err := svc.Login(newTestContext(), testUsername, testPassword, false)
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

	svc := newService(repo, iss)
	before := time.Now()
	_, _, err := svc.Login(newTestContext(), testUsername, testPassword, true)
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

	svc := newService(repo, iss)
	gotAdm, tok, err := svc.Login(newTestContext(), testUsername, testPassword, false)

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