package admin

import (
	"errors"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"ai-go-mall/internal/model"
)

// --- helpers ---

// adminContextKey 与 internal/middleware/auth.go 保持一致。
// 测试里直接把当前 admin 塞进 context，模拟 AdminAuth 已生效。
const adminContextKey = "admin.current"

// newCtxWithSelf 返回一个挂了 self admin 的 gin.Context。
func newCtxWithSelf(t *testing.T, selfID int64) *gin.Context {
	t.Helper()
	c := newTestContext()
	c.Set(adminContextKey, &model.Admin{ID: selfID})
	return c
}

// --- Create / Update hook 测试 ---

// TestCreate_HashesPassword 验证：baseService.Create 在调用 repo.Create 之前
// 把 Password 字段跑一遍 bcrypt —— 传入的明文密码被替换为 $2 开头哈希。
func TestCreate_HashesPassword(t *testing.T) {
	repo := &mockRepo{}
	svc := newService(repo, &mockIssuer{}, &mockCaptcha{})

	plain := "S3cretPwd!"
	entity := &model.Admin{Username: "alice", Password: plain}
	if err := svc.Create(newTestContext(), entity); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if entity.Password == plain {
		t.Errorf("Create did not hash password: still plaintext")
	}
	if !strings.HasPrefix(entity.Password, "$2") {
		t.Errorf("hashed password = %q, want bcrypt prefix $2", entity.Password)
	}
}

// TestUpdate_PreservesPasswordWhenEmpty 验证：Update 时 Password == "" 不会被
// 错误地"哈希空串再写" —— service 必须把它当成"不改密码"短路返回。
func TestUpdate_PreservesPasswordWhenEmpty(t *testing.T) {
	repo := &mockRepo{}
	svc := newService(repo, &mockIssuer{}, &mockCaptcha{})

	entity := &model.Admin{ID: 7, Username: "alice", Password: ""}
	if err := svc.Update(newTestContext(), entity); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if entity.Password != "" {
		t.Errorf("Update with empty Password should leave it empty, got %q", entity.Password)
	}
}

// TestCreate_RejectsShortPassword 验证：长度 < MinPasswordLength 的密码直接被
// service 层挡掉，repo.Create 不被调用。
//
// mockRepo.Create 是 no-op 默认实现，本身不会 panic；这里通过验证返回的
// ErrPasswordTooShort 即可推断「Create 链路在到达 repo 前就被拦下」。
func TestCreate_RejectsShortPassword(t *testing.T) {
	repo := &mockRepo{}
	svc := newService(repo, &mockIssuer{}, &mockCaptcha{})

	entity := &model.Admin{Username: "alice", Password: "short"}
	if err := svc.Create(newTestContext(), entity); !errors.Is(err, ErrPasswordTooShort) {
		t.Errorf("Create = %v, want ErrPasswordTooShort", err)
	}
}

// --- ChangePassword ---

// TestChangePassword_RevokesTokens 验证：ChangePassword 成功路径会触发
// repo.UpdatePassword + tm.Clear，参数正确。
func TestChangePassword_RevokesTokens(t *testing.T) {
	repo := &mockRepo{}
	iss := &mockIssuer{}
	svc := newService(repo, iss, &mockCaptcha{})

	if err := svc.ChangePassword(newTestContext(), 7, "BrandNew!2026"); err != nil {
		t.Fatalf("ChangePassword: %v", err)
	}
	if repo.updatePwdCalls != 1 {
		t.Errorf("repo.UpdatePassword called %d times, want 1", repo.updatePwdCalls)
	}
	if repo.lastUpdatePwdID != 7 {
		t.Errorf("repo.UpdatePassword id = %d, want 7", repo.lastUpdatePwdID)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(repo.lastUpdatePwdHash), []byte("BrandNew!2026")); err != nil {
		t.Errorf("stored hash does not match new password: %v", err)
	}
	if iss.clearCalls != 1 {
		t.Errorf("issuer.Clear called %d times, want 1", iss.clearCalls)
	}
	if iss.lastClear.UserID != 7 {
		t.Errorf("issuer.Clear UserID = %d, want 7", iss.lastClear.UserID)
	}
	if iss.lastClear.TokenType != tokenTypeAdmin {
		t.Errorf("issuer.Clear TokenType = %q, want %q", iss.lastClear.TokenType, tokenTypeAdmin)
	}
}

// TestChangePassword_TooShort 验证：长度 < MinPasswordLength → ErrPasswordTooShort；
// repo.UpdatePassword 与 issuer.Clear 都不被调用。
func TestChangePassword_TooShort(t *testing.T) {
	repo := &mockRepo{}
	iss := &mockIssuer{}
	svc := newService(repo, iss, &mockCaptcha{})

	if err := svc.ChangePassword(newTestContext(), 7, "short"); !errors.Is(err, ErrPasswordTooShort) {
		t.Errorf("ChangePassword = %v, want ErrPasswordTooShort", err)
	}
	if repo.updatePwdCalls != 0 {
		t.Errorf("repo.UpdatePassword called %d times on short pwd, want 0", repo.updatePwdCalls)
	}
	if iss.clearCalls != 0 {
		t.Errorf("issuer.Clear called %d times on short pwd, want 0", iss.clearCalls)
	}
}

// TestChangePassword_RepoError 验证：repo.UpdatePassword 失败时 service 透传。
func TestChangePassword_RepoError(t *testing.T) {
	wantErr := errors.New("db down")
	repo := &mockRepo{
		updatePasswordFunc: func(c *gin.Context, id uint, hashed string) error {
			return wantErr
		},
	}
	iss := &mockIssuer{}
	svc := newService(repo, iss, &mockCaptcha{})

	if err := svc.ChangePassword(newTestContext(), 7, "BrandNew!2026"); !errors.Is(err, wantErr) {
		t.Errorf("ChangePassword = %v, want %v", err, wantErr)
	}
	if iss.clearCalls != 0 {
		t.Errorf("issuer.Clear should not be called when repo failed, got %d", iss.clearCalls)
	}
}

// --- ToggleStatus ---

// TestToggleStatus_SelfProtection 验证：self id == 目标 id → ErrSelfProtection；
// repo 不被调用，token 不被清。
func TestToggleStatus_SelfProtection(t *testing.T) {
	repo := &mockRepo{}
	iss := &mockIssuer{}
	svc := newService(repo, iss, &mockCaptcha{})

	c := newCtxWithSelf(t, 1)
	if err := svc.ToggleStatus(c, 1); !errors.Is(err, ErrSelfProtection) {
		t.Errorf("ToggleStatus = %v, want ErrSelfProtection", err)
	}
	if repo.getByIDCalls != 0 {
		t.Errorf("repo.GetByID called %d times on self-protection, want 0", repo.getByIDCalls)
	}
	if repo.updateStatusCalls != 0 {
		t.Errorf("repo.UpdateStatus called %d times on self-protection, want 0", repo.updateStatusCalls)
	}
	if iss.clearCalls != 0 {
		t.Errorf("issuer.Clear called %d times on self-protection, want 0", iss.clearCalls)
	}
}

// TestToggleStatus_1ToZeroRevokes 验证：1→0 切到禁用时 UpdateStatus(0) + Clear 都被调。
func TestToggleStatus_1ToZeroRevokes(t *testing.T) {
	repo := &mockRepo{
		getByIDFunc: func(c *gin.Context, id int64) (*model.Admin, error) {
			return &model.Admin{ID: 7, Status: 1}, nil
		},
	}
	iss := &mockIssuer{}
	svc := newService(repo, iss, &mockCaptcha{})

	c := newCtxWithSelf(t, 1)
	if err := svc.ToggleStatus(c, 7); err != nil {
		t.Fatalf("ToggleStatus: %v", err)
	}
	if repo.updateStatusCalls != 1 {
		t.Errorf("repo.UpdateStatus called %d times, want 1", repo.updateStatusCalls)
	}
	if repo.lastUpdateStatus != 0 {
		t.Errorf("UpdateStatus arg = %d, want 0 (1→0)", repo.lastUpdateStatus)
	}
	if iss.clearCalls != 1 {
		t.Errorf("issuer.Clear called %d times on 1→0, want 1", iss.clearCalls)
	}
	if iss.lastClear.UserID != 7 {
		t.Errorf("issuer.Clear UserID = %d, want 7", iss.lastClear.UserID)
	}
}

// TestToggleStatus_0ToOneDoesNotRevoke 验证：0→1 启用时 UpdateStatus(1) 被调，
// Clear 不被调（admin 解禁用户不是为了逼 ta 下线）。
func TestToggleStatus_0ToOneDoesNotRevoke(t *testing.T) {
	repo := &mockRepo{
		getByIDFunc: func(c *gin.Context, id int64) (*model.Admin, error) {
			return &model.Admin{ID: 7, Status: 0}, nil
		},
	}
	iss := &mockIssuer{}
	svc := newService(repo, iss, &mockCaptcha{})

	c := newCtxWithSelf(t, 1)
	if err := svc.ToggleStatus(c, 7); err != nil {
		t.Fatalf("ToggleStatus: %v", err)
	}
	if repo.updateStatusCalls != 1 {
		t.Errorf("repo.UpdateStatus called %d times, want 1", repo.updateStatusCalls)
	}
	if repo.lastUpdateStatus != 1 {
		t.Errorf("UpdateStatus arg = %d, want 1 (0→1)", repo.lastUpdateStatus)
	}
	if iss.clearCalls != 0 {
		t.Errorf("issuer.Clear called %d times on 0→1, want 0", iss.clearCalls)
	}
}

// TestToggleStatus_NotFound 验证：repo.GetByID 返回 (nil, nil) 时 service 返回
// gorm.ErrRecordNotFound —— handler 把它映射成 404。
func TestToggleStatus_NotFound(t *testing.T) {
	repo := &mockRepo{
		getByIDFunc: func(c *gin.Context, id int64) (*model.Admin, error) {
			return nil, nil
		},
	}
	svc := newService(repo, &mockIssuer{}, &mockCaptcha{})

	c := newCtxWithSelf(t, 1)
	if err := svc.ToggleStatus(c, 99); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Errorf("ToggleStatus = %v, want gorm.ErrRecordNotFound", err)
	}
	if repo.updateStatusCalls != 0 {
		t.Errorf("repo.UpdateStatus called %d times on not-found, want 0", repo.updateStatusCalls)
	}
}

// --- Unlock ---

// TestUnlock_CallsResetLoginFailure 验证：Unlock 直接转发到 repo.ResetLoginFailure，
// 不动 Password / 不清 token。
func TestUnlock_CallsResetLoginFailure(t *testing.T) {
	repo := &mockRepo{}
	iss := &mockIssuer{}
	svc := newService(repo, iss, &mockCaptcha{})

	if err := svc.Unlock(newTestContext(), 7); err != nil {
		t.Fatalf("Unlock: %v", err)
	}
	if repo.resetCalls != 1 {
		t.Errorf("repo.ResetLoginFailure called %d times, want 1", repo.resetCalls)
	}
	if repo.lastResetID != 7 {
		t.Errorf("repo.ResetLoginFailure id = %d, want 7", repo.lastResetID)
	}
	if iss.clearCalls != 0 {
		t.Errorf("issuer.Clear should NOT be called on Unlock, got %d", iss.clearCalls)
	}
}

// TestUnlock_RepoError 验证：repo 错误透传。
func TestUnlock_RepoError(t *testing.T) {
	wantErr := errors.New("db down")
	repo := &mockRepo{
		resetLoginFailureFunc: func(c *gin.Context, id uint) error { return wantErr },
	}
	svc := newService(repo, &mockIssuer{}, &mockCaptcha{})

	if err := svc.Unlock(newTestContext(), 7); !errors.Is(err, wantErr) {
		t.Errorf("Unlock = %v, want %v", err, wantErr)
	}
}

// --- BatchDelete ---

// TestBatchDelete_FiltersSelf 验证：ids=[1,2,3] self=1 → repo.DeleteBatch([2,3])，
// 返回 (2, 1, nil)。
func TestBatchDelete_FiltersSelf(t *testing.T) {
	repo := &mockRepo{}
	svc := newService(repo, &mockIssuer{}, &mockCaptcha{})

	c := newCtxWithSelf(t, 1)
	deleted, skipped, err := svc.BatchDelete(c, []uint{1, 2, 3})
	if err != nil {
		t.Fatalf("BatchDelete: %v", err)
	}
	if deleted != 2 {
		t.Errorf("deleted = %d, want 2", deleted)
	}
	if skipped != 1 {
		t.Errorf("skipped = %d, want 1", skipped)
	}
	if repo.deleteBatchCalls != 1 {
		t.Errorf("repo.DeleteBatch called %d times, want 1", repo.deleteBatchCalls)
	}
	if len(repo.lastDeleteBatch) != 2 || repo.lastDeleteBatch[0] != 2 || repo.lastDeleteBatch[1] != 3 {
		t.Errorf("repo.DeleteBatch ids = %v, want [2, 3]", repo.lastDeleteBatch)
	}
}

// TestBatchDelete_Empty 验证：空 ids → (0, 0, nil)，repo.DeleteBatch 不被调。
func TestBatchDelete_Empty(t *testing.T) {
	repo := &mockRepo{}
	svc := newService(repo, &mockIssuer{}, &mockCaptcha{})

	c := newCtxWithSelf(t, 1)
	deleted, skipped, err := svc.BatchDelete(c, []uint{})
	if err != nil {
		t.Fatalf("BatchDelete: %v", err)
	}
	if deleted != 0 || skipped != 0 {
		t.Errorf("got (%d, %d), want (0, 0)", deleted, skipped)
	}
	if repo.deleteBatchCalls != 0 {
		t.Errorf("repo.DeleteBatch called %d times on empty, want 0", repo.deleteBatchCalls)
	}
}

// TestBatchDelete_RepoError 验证：repo 错误透传，且 skipped 仍记录到调用方。
func TestBatchDelete_RepoError(t *testing.T) {
	wantErr := errors.New("db down")
	repo := &mockRepo{
		deleteBatchFunc: func(c *gin.Context, ids []uint) error { return wantErr },
	}
	svc := newService(repo, &mockIssuer{}, &mockCaptcha{})

	c := newCtxWithSelf(t, 1)
	deleted, skipped, err := svc.BatchDelete(c, []uint{1, 2, 3})
	if !errors.Is(err, wantErr) {
		t.Errorf("BatchDelete err = %v, want %v", err, wantErr)
	}
	if deleted != 0 || skipped != 1 {
		t.Errorf("got (%d, %d) on repo error, want (0, 1)", deleted, skipped)
	}
}
