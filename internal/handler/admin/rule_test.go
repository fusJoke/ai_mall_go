package admin

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"ai-go-mall/internal/model"
	"ai-go-mall/internal/repository"
	adminSvc "ai-go-mall/internal/service/admin"
)

// =============================================================================
// stubRuleSvc —— adminSvc.RuleService 接口的可编程实现。
//
// 仿 manager_test.go 的 stubSvc 风格；stub 而不是 mock，让
// service.CRUDService[model.AdminRule] 的 List 签名严格匹配，编译期一次过。
// =============================================================================

type stubRuleSvc struct {
	createFn       func(c *gin.Context, entity *model.AdminRule) error
	updateFn       func(c *gin.Context, entity *model.AdminRule) error
	getByIDFn      func(c *gin.Context, id int64) (*model.AdminRule, error)
	deleteFn       func(c *gin.Context, id int64) error
	listFn         func(c *gin.Context, opts repository.ListOptions) ([]model.AdminRule, int64, error)
	validatePIDFn  func(c *gin.Context, id, newPID uint) error
	toggleStatusFn func(c *gin.Context, id uint) error
	batchDeleteFn  func(c *gin.Context, ids []uint) (int, int, error)
	listAllFn      func(c *gin.Context) ([]model.AdminRule, error)
}

func (s *stubRuleSvc) Create(c *gin.Context, e *model.AdminRule) error {
	if s.createFn != nil {
		return s.createFn(c, e)
	}
	return nil
}
func (s *stubRuleSvc) Update(c *gin.Context, e *model.AdminRule) error {
	if s.updateFn != nil {
		return s.updateFn(c, e)
	}
	return nil
}
func (s *stubRuleSvc) GetByID(c *gin.Context, id int64) (*model.AdminRule, error) {
	if s.getByIDFn != nil {
		return s.getByIDFn(c, id)
	}
	return nil, gorm.ErrRecordNotFound
}
func (s *stubRuleSvc) Delete(c *gin.Context, id int64) error {
	if s.deleteFn != nil {
		return s.deleteFn(c, id)
	}
	return nil
}
func (s *stubRuleSvc) List(c *gin.Context, opts repository.ListOptions) ([]model.AdminRule, int64, error) {
	if s.listFn != nil {
		return s.listFn(c, opts)
	}
	return nil, 0, nil
}
func (s *stubRuleSvc) ValidatePID(c *gin.Context, id, newPID uint) error {
	if s.validatePIDFn != nil {
		return s.validatePIDFn(c, id, newPID)
	}
	return nil
}
func (s *stubRuleSvc) ToggleStatus(c *gin.Context, id uint) error {
	if s.toggleStatusFn != nil {
		return s.toggleStatusFn(c, id)
	}
	return nil
}
func (s *stubRuleSvc) BatchDelete(c *gin.Context, ids []uint) (int, int, error) {
	if s.batchDeleteFn != nil {
		return s.batchDeleteFn(c, ids)
	}
	return 0, 0, nil
}
func (s *stubRuleSvc) ListAll(c *gin.Context) ([]model.AdminRule, error) {
	if s.listAllFn != nil {
		return s.listAllFn(c)
	}
	return nil, nil
}

var _ adminSvc.RuleService = (*stubRuleSvc)(nil)

// --- helpers ---

func newRuleCtx(t *testing.T) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	return c, w
}

func postRuleJSON(c *gin.Context, path string, body any) {
	c.Request = httptest.NewRequest(http.MethodPost, path, bytes.NewReader(mustJSON(body)))
	c.Request.Header.Set("Content-Type", "application/json")
}

// --- ListAll ---

// TestRuleHandler_ListAll_ReturnsArray 验证：mock svc 返 3 条 → 响应 JSON 是 array。
func TestRuleHandler_ListAll_ReturnsArray(t *testing.T) {
	svc := &stubRuleSvc{
		listAllFn: func(c *gin.Context) ([]model.AdminRule, error) {
			return []model.AdminRule{
				{ID: 1, Title: "A"},
				{ID: 2, Title: "B"},
				{ID: 3, Title: "C"},
			}, nil
		},
	}
	c, w := newRuleCtx(t)
	c.Request = httptest.NewRequest(http.MethodGet, "/admin/rule/all", nil)

	h := NewRuleHandler(svc)
	h.ListAll(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	body := w.Body.Bytes()
	// 必须解析成 JSON 数组
	var arr []model.AdminRule
	if err := json.Unmarshal(body, &arr); err != nil {
		t.Fatalf("body is not JSON array: %v, body=%s", err, body)
	}
	if len(arr) != 3 {
		t.Errorf("len(arr) = %d, want 3", len(arr))
	}
}

// TestRuleHandler_ListAll_InternalMapping 验证：svc 返错 → 500。
func TestRuleHandler_ListAll_InternalMapping(t *testing.T) {
	svc := &stubRuleSvc{
		listAllFn: func(c *gin.Context) ([]model.AdminRule, error) {
			return nil, errors.New("db down")
		},
	}
	c, w := newRuleCtx(t)
	c.Request = httptest.NewRequest(http.MethodGet, "/admin/rule/all", nil)

	h := NewRuleHandler(svc)
	h.ListAll(c)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
	if !strings.Contains(w.Body.String(), "rule.list_all.internal") {
		t.Errorf("body = %s, want rule.list_all.internal", w.Body.String())
	}
}

// --- ToggleStatus ---

func TestRuleHandler_ToggleStatus_NotFoundMapping(t *testing.T) {
	svc := &stubRuleSvc{
		toggleStatusFn: func(c *gin.Context, id uint) error { return gorm.ErrRecordNotFound },
	}
	c, w := newRuleCtx(t)
	postRuleJSON(c, "/admin/rule/toggle-status", ruleToggleStatusRequest{ID: 99})

	h := NewRuleHandler(svc)
	h.ToggleStatus(c)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
	if !strings.Contains(w.Body.String(), "rule.toggle_status.not_found") {
		t.Errorf("body = %s, want not_found code", w.Body.String())
	}
}

func TestRuleHandler_ToggleStatus_InternalMapping(t *testing.T) {
	svc := &stubRuleSvc{
		toggleStatusFn: func(c *gin.Context, id uint) error { return errors.New("db down") },
	}
	c, w := newRuleCtx(t)
	postRuleJSON(c, "/admin/rule/toggle-status", ruleToggleStatusRequest{ID: 7})

	h := NewRuleHandler(svc)
	h.ToggleStatus(c)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
	body := w.Body.String()
	if strings.Contains(body, "db down") {
		t.Errorf("body leaks driver detail: %s", body)
	}
	if !strings.Contains(body, "rule.toggle_status.internal") {
		t.Errorf("body = %s, want internal code", body)
	}
}

// --- BatchDelete ---

// TestRuleHandler_BatchDelete_ResponseShape 验证：service 返回 (1, 2, nil) 时
// 响应 JSON 是 {"deleted":1, "skipped":2}。
func TestRuleHandler_BatchDelete_ResponseShape(t *testing.T) {
	svc := &stubRuleSvc{
		batchDeleteFn: func(c *gin.Context, ids []uint) (int, int, error) {
			return 1, 2, nil
		},
	}
	c, w := newRuleCtx(t)
	postRuleJSON(c, "/admin/rule/batch-delete", ruleBatchDeleteRequest{IDs: []uint{1, 2, 3}})

	h := NewRuleHandler(svc)
	h.BatchDelete(c)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
	var resp struct {
		Deleted int `json:"deleted"`
		Skipped int `json:"skipped"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if resp.Deleted != 1 || resp.Skipped != 2 {
		t.Errorf("resp = %+v, want {deleted:1, skipped:2}", resp)
	}
}

func TestRuleHandler_BatchDelete_InvalidInput(t *testing.T) {
	svc := &stubRuleSvc{} // batchDeleteFn 不调 → svc 不会被触达
	c, w := newRuleCtx(t)
	c.Request = httptest.NewRequest(http.MethodPost, "/admin/rule/batch-delete",
		bytes.NewReader([]byte(`{not-json}`)))
	c.Request.Header.Set("Content-Type", "application/json")

	h := NewRuleHandler(svc)
	h.BatchDelete(c)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

// --- Create / EditPost PID 错误映射 ---

func TestRuleHandler_Create_PIDCycleMapping(t *testing.T) {
	svc := &stubRuleSvc{
		createFn: func(c *gin.Context, e *model.AdminRule) error { return adminSvc.ErrPIDCycle },
	}
	c, w := newRuleCtx(t)
	postRuleJSON(c, "/admin/rule/create", model.AdminRule{Title: "X", Pid: 7})

	h := NewRuleHandler(svc)
	h.Create(c)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
	if !strings.Contains(w.Body.String(), "rule.create.pid_cycle") {
		t.Errorf("body = %s, want rule.create.pid_cycle", w.Body.String())
	}
}

func TestRuleHandler_Create_PIDNotFoundMapping(t *testing.T) {
	svc := &stubRuleSvc{
		createFn: func(c *gin.Context, e *model.AdminRule) error { return adminSvc.ErrPIDNotFound },
	}
	c, w := newRuleCtx(t)
	postRuleJSON(c, "/admin/rule/create", model.AdminRule{Title: "X", Pid: 9999})

	h := NewRuleHandler(svc)
	h.Create(c)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
	if !strings.Contains(w.Body.String(), "rule.create.pid_not_found") {
		t.Errorf("body = %s, want rule.create.pid_not_found", w.Body.String())
	}
}

func TestRuleHandler_EditPost_PIDCycleMapping(t *testing.T) {
	svc := &stubRuleSvc{
		updateFn: func(c *gin.Context, e *model.AdminRule) error { return adminSvc.ErrPIDCycle },
	}
	c, w := newRuleCtx(t)
	postRuleJSON(c, "/admin/rule/edit", model.AdminRule{ID: 7, Title: "X", Pid: 12})

	h := NewRuleHandler(svc)
	h.EditPost(c)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
	if !strings.Contains(w.Body.String(), "rule.edit.pid_cycle") {
		t.Errorf("body = %s, want rule.edit.pid_cycle", w.Body.String())
	}
}

func TestRuleHandler_EditPost_PIDNotFoundMapping(t *testing.T) {
	svc := &stubRuleSvc{
		updateFn: func(c *gin.Context, e *model.AdminRule) error { return adminSvc.ErrPIDNotFound },
	}
	c, w := newRuleCtx(t)
	postRuleJSON(c, "/admin/rule/edit", model.AdminRule{ID: 7, Title: "X", Pid: 9999})

	h := NewRuleHandler(svc)
	h.EditPost(c)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
	if !strings.Contains(w.Body.String(), "rule.edit.pid_not_found") {
		t.Errorf("body = %s, want rule.edit.pid_not_found", w.Body.String())
	}
}

// --- Delete has-children 映射 ---

func TestRuleHandler_Delete_HasChildrenMapping(t *testing.T) {
	svc := &stubRuleSvc{
		deleteFn: func(c *gin.Context, id int64) error { return adminSvc.ErrHasChildren },
	}
	c, w := newRuleCtx(t)
	c.Request = httptest.NewRequest(http.MethodPost, "/admin/rule/delete?id=7", nil)

	h := NewRuleHandler(svc)
	h.Delete(c)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
	if !strings.Contains(w.Body.String(), "rule.delete.has_children") {
		t.Errorf("body = %s, want rule.delete.has_children", w.Body.String())
	}
}

func TestRuleHandler_Delete_HappyPath(t *testing.T) {
	var gotID int64
	svc := &stubRuleSvc{
		deleteFn: func(c *gin.Context, id int64) error { gotID = id; return nil },
	}
	c, w := newRuleCtx(t)
	c.Request = httptest.NewRequest(http.MethodPost, "/admin/rule/delete?id=42", nil)

	h := NewRuleHandler(svc)
	h.Delete(c)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
	if gotID != 42 {
		t.Errorf("svc.Delete id = %d, want 42", gotID)
	}
}
