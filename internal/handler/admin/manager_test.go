package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	captchaInfra "ai-go-mall/internal/infra/captcha"
	"ai-go-mall/internal/model"
	"ai-go-mall/internal/repository"
	adminSvc "ai-go-mall/internal/service/admin"
)

// stubSvc 是 adminSvc.Service 接口的可编程实现。
//
// 仅在测试需要时覆写对应方法；其它方法保持 panic —— 调用到不该调的就
// 算测试错。stub 而不是 mock，让 service.CRUDService[model.Admin] 的
// List(opts repository.ListOptions) 签名严格匹配，编译期一次过。
type stubSvc struct {
	createFn         func(c *gin.Context, entity *model.Admin) error
	updateFn         func(c *gin.Context, entity *model.Admin) error
	getByIDFn        func(c *gin.Context, id int64) (*model.Admin, error)
	deleteFn         func(c *gin.Context, id int64) error
	listFn           func(c *gin.Context, opts repository.ListOptions) ([]model.Admin, int64, error)
	changePasswordFn func(c *gin.Context, id uint, newPassword string) error
	toggleStatusFn   func(c *gin.Context, id uint) error
	unlockFn         func(c *gin.Context, id uint) error
	batchDeleteFn    func(c *gin.Context, ids []uint) (int, int, error)
}

func (s *stubSvc) Create(c *gin.Context, e *model.Admin) error {
	if s.createFn != nil {
		return s.createFn(c, e)
	}
	return nil
}
func (s *stubSvc) Update(c *gin.Context, e *model.Admin) error {
	if s.updateFn != nil {
		return s.updateFn(c, e)
	}
	return nil
}
func (s *stubSvc) GetByID(c *gin.Context, id int64) (*model.Admin, error) {
	if s.getByIDFn != nil {
		return s.getByIDFn(c, id)
	}
	return nil, gorm.ErrRecordNotFound
}
func (s *stubSvc) Delete(c *gin.Context, id int64) error {
	if s.deleteFn != nil {
		return s.deleteFn(c, id)
	}
	return nil
}
func (s *stubSvc) List(c *gin.Context, opts repository.ListOptions) ([]model.Admin, int64, error) {
	if s.listFn != nil {
		return s.listFn(c, opts)
	}
	return nil, 0, nil
}
func (*stubSvc) Login(*gin.Context, string, string, string, []captchaInfra.Point, bool) (*model.Admin, string, error) {
	panic("Login not stubbed")
}
func (*stubSvc) Logout(_ context.Context, _ string) error { panic("Logout not stubbed") }
func (s *stubSvc) ChangePassword(c *gin.Context, id uint, newPassword string) error {
	if s.changePasswordFn != nil {
		return s.changePasswordFn(c, id, newPassword)
	}
	return nil
}
func (s *stubSvc) ToggleStatus(c *gin.Context, id uint) error {
	if s.toggleStatusFn != nil {
		return s.toggleStatusFn(c, id)
	}
	return nil
}
func (s *stubSvc) Unlock(c *gin.Context, id uint) error {
	if s.unlockFn != nil {
		return s.unlockFn(c, id)
	}
	return nil
}
func (s *stubSvc) BatchDelete(c *gin.Context, ids []uint) (int, int, error) {
	if s.batchDeleteFn != nil {
		return s.batchDeleteFn(c, ids)
	}
	return len(ids), 0, nil
}

var _ adminSvc.Service = (*stubSvc)(nil)

// --- helpers ---

// newCtx 返回 (gin.Context, *httptest.ResponseRecorder) —— 与 init_test.go 一致。
func newCtx(t *testing.T, selfID int64) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	if selfID > 0 {
		c.Set("admin.current", &model.Admin{ID: selfID, Status: 1})
	}
	return c, w
}

func postJSON(c *gin.Context, path string, body any) {
	c.Request = httptest.NewRequest(http.MethodPost, path, bytes.NewReader(mustJSON(body)))
	c.Request.Header.Set("Content-Type", "application/json")
}

func mustJSON(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}

// --- List 脱敏 ---

// TestHandler_List_StripsPassword 验证：service 返回 items 含 Password 字段，
// handler.List 序列化前转 adminInfo，输出 JSON 不含 password。
func TestHandler_List_StripsPassword(t *testing.T) {
	svc := &stubSvc{
		listFn: func(c *gin.Context, opts repository.ListOptions) ([]model.Admin, int64, error) {
			return []model.Admin{{
				ID:       1,
				Username: "alice",
				Password: "$2a$10$must-not-leak",
			}}, 1, nil
		},
	}
	c, w := newCtx(t, 0)
	c.Request = httptest.NewRequest(http.MethodGet, "/admin/admin/list", nil)

	h := NewHandler(svc)
	h.List(c)

	body := w.Body.String()
	if strings.Contains(body, "$2a$") || strings.Contains(strings.ToLower(body), "password") {
		t.Errorf("List body leaked password: %s", body)
	}
	if !strings.Contains(body, "alice") {
		t.Errorf("List body should contain username, got %s", body)
	}
}

// --- ChangePassword ---

func TestChangePassword_ServiceCalled(t *testing.T) {
	var gotID uint
	var gotPwd string
	svc := &stubSvc{
		changePasswordFn: func(c *gin.Context, id uint, newPassword string) error {
			gotID = id
			gotPwd = newPassword
			return nil
		},
	}
	c, w := newCtx(t, 0)
	postJSON(c, "/admin/admin/change-password", changePasswordRequest{ID: 7, NewPassword: "NewPwd!2026"})

	h := NewHandler(svc)
	h.ChangePassword(c)

	if gotID != 7 || gotPwd != "NewPwd!2026" {
		t.Errorf("svc got (%d, %q), want (7, %q)", gotID, gotPwd, "NewPwd!2026")
	}
	if !strings.Contains(w.Body.String(), "admin.change_password.ok") {
		t.Errorf("body = %s, want admin.change_password.ok", w.Body.String())
	}
}

func TestChangePassword_TooShortMapping(t *testing.T) {
	svc := &stubSvc{
		changePasswordFn: func(c *gin.Context, id uint, newPassword string) error {
			return adminSvc.ErrPasswordTooShort
		},
	}
	c, w := newCtx(t, 0)
	postJSON(c, "/admin/admin/change-password", changePasswordRequest{ID: 7, NewPassword: "short"})

	h := NewHandler(svc)
	h.ChangePassword(c)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
	if !strings.Contains(w.Body.String(), "admin.change_password.password_too_short") {
		t.Errorf("body = %s, want password_too_short code", w.Body.String())
	}
}

func TestChangePassword_InternalMapping(t *testing.T) {
	svc := &stubSvc{
		changePasswordFn: func(c *gin.Context, id uint, newPassword string) error {
			return errors.New("db down (host=db.internal)")
		},
	}
	c, w := newCtx(t, 0)
	postJSON(c, "/admin/admin/change-password", changePasswordRequest{ID: 7, NewPassword: "BrandNew!2026"})

	h := NewHandler(svc)
	h.ChangePassword(c)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
	body := w.Body.String()
	if strings.Contains(body, "db.internal") {
		t.Errorf("body leaks driver detail: %s", body)
	}
	if !strings.Contains(body, "admin.change_password.internal") {
		t.Errorf("body = %s, want internal code", body)
	}
}

// --- ToggleStatus ---

func TestToggleStatus_SelfProtectionMapping(t *testing.T) {
	svc := &stubSvc{
		toggleStatusFn: func(c *gin.Context, id uint) error { return adminSvc.ErrSelfProtection },
	}
	c, w := newCtx(t, 1) // self=1
	postJSON(c, "/admin/admin/toggle-status", toggleStatusRequest{ID: 1})

	h := NewHandler(svc)
	h.ToggleStatus(c)

	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", w.Code)
	}
	if !strings.Contains(w.Body.String(), "admin.toggle_status.self_protection") {
		t.Errorf("body = %s, want self_protection code", w.Body.String())
	}
}

func TestToggleStatus_NotFoundMapping(t *testing.T) {
	svc := &stubSvc{
		toggleStatusFn: func(c *gin.Context, id uint) error { return gorm.ErrRecordNotFound },
	}
	c, w := newCtx(t, 1)
	postJSON(c, "/admin/admin/toggle-status", toggleStatusRequest{ID: 99})

	h := NewHandler(svc)
	h.ToggleStatus(c)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

// --- Unlock ---

func TestUnlock_Ok(t *testing.T) {
	var gotID uint
	svc := &stubSvc{
		unlockFn: func(c *gin.Context, id uint) error { gotID = id; return nil },
	}
	c, w := newCtx(t, 1)
	postJSON(c, "/admin/admin/unlock", unlockRequest{ID: 42})

	h := NewHandler(svc)
	h.Unlock(c)

	if gotID != 42 {
		t.Errorf("svc got id = %d, want 42", gotID)
	}
	if !strings.Contains(w.Body.String(), "admin.unlock.ok") {
		t.Errorf("body = %s, want unlock.ok", w.Body.String())
	}
}

// --- BatchDelete ---

// TestBatchDelete_ResponseShape 验证：service 返回 (2, 1, nil) 时响应 JSON 是
// {"deleted":2,"skipped_self":1}。
func TestBatchDelete_ResponseShape(t *testing.T) {
	svc := &stubSvc{
		batchDeleteFn: func(c *gin.Context, ids []uint) (int, int, error) {
			return 2, 1, nil
		},
	}
	c, w := newCtx(t, 1)
	postJSON(c, "/admin/admin/batch-delete", batchDeleteRequest{IDs: []uint{1, 2, 3}})

	h := NewHandler(svc)
	h.BatchDelete(c)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
	var resp struct {
		Deleted     int `json:"deleted"`
		SkippedSelf int `json:"skipped_self"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if resp.Deleted != 2 || resp.SkippedSelf != 1 {
		t.Errorf("resp = %+v, want {deleted:2, skipped_self:1}", resp)
	}
}

// --- Delete self-protection ---

func TestDelete_SelfProtection(t *testing.T) {
	svc := &stubSvc{
		deleteFn: func(c *gin.Context, id int64) error {
			t.Errorf("svc.Delete should NOT be called for self-protection")
			return nil
		},
	}
	c, w := newCtx(t, 1) // self=1
	c.Request = httptest.NewRequest(http.MethodPost, "/admin/admin/delete?id=1", nil)

	h := NewHandler(svc)
	h.Delete(c)

	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", w.Code)
	}
	if !strings.Contains(w.Body.String(), "admin.delete.self_protection") {
		t.Errorf("body = %s, want self_protection code", w.Body.String())
	}
}

func TestDelete_OtherAdmin(t *testing.T) {
	var gotID int64
	svc := &stubSvc{
		deleteFn: func(c *gin.Context, id int64) error { gotID = id; return nil },
	}
	c, w := newCtx(t, 1)
	c.Request = httptest.NewRequest(http.MethodPost, "/admin/admin/delete?id=42", nil)

	h := NewHandler(svc)
	h.Delete(c)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
	if gotID != 42 {
		t.Errorf("svc.Delete got id = %d, want 42", gotID)
	}
}

// --- Create password stripping ---

func TestCreate_StripsPassword(t *testing.T) {
	svc := &stubSvc{
		createFn: func(c *gin.Context, e *model.Admin) error {
			e.ID = 100
			// 模拟 service.Create 已把 Password 哈希成 "$2a$10$hashed..."
			e.Password = "$2a$10$hashedxxxxxxxxxxxxxxxxx"
			return nil
		},
	}
	c, w := newCtx(t, 0)
	postJSON(c, "/admin/admin/create", model.Admin{Username: "alice", Password: "plain"})

	h := NewHandler(svc)
	h.Create(c)

	body := w.Body.String()
	if strings.Contains(body, "$2a$") || strings.Contains(strings.ToLower(body), "password") {
		t.Errorf("Create body leaked password: %s", body)
	}
	if !strings.Contains(body, "alice") {
		t.Errorf("Create body should contain username, got %s", body)
	}
}
