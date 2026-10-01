package admin

import (
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"ai-go-mall/internal/model"
	"ai-go-mall/internal/repository"
	adminRepo "ai-go-mall/internal/repository/admin"
)

// =============================================================================
// mockRuleRepo —— RuleRepository 全接口实现，覆盖 add-admin-rule-management
// 新增的 9 个写方法 + 既有 4 个只读方法。
//
// 与 permission_test.go 的 mockRuleRepository 同名但不共享：permission 路径
// 只关心只读方法；本测试需要写方法的字段化控制，因此独立一份。
// =============================================================================

type mockRuleRepo struct {
	// 既有字段（permission 风格沿用）。
	rules     map[uint]*model.AdminRule
	activeIDs []uint

	// 新增字段：PID 校验 / 写路径专用。
	childrenOf map[uint]bool // id → 是否有未软删子节点

	// 调用计数器 + 末次参数（用于断言调用次数与入参）。
	createCalls        int
	lastCreateRule     *model.AdminRule
	updateCalls        int
	lastUpdateRule     *model.AdminRule
	deleteCalls        int
	lastDeleteID       uint
	deleteBatchCalls   int
	lastDeleteBatch    []uint
	updateStatusCalls  int
	lastUpdateStatusID uint
	lastUpdateStatus   int8

	// 错误注入：模拟 Repository 失败。
	createErr       error
	updateErr       error
	deleteErr       error
	deleteBatchErr  error
	updateStatusErr error
	getPIDErr       error
	hasChildrenErr  error
	getByIDErr      error
	listErr         error
}

func (m *mockRuleRepo) ListByIDs(ids []uint) ([]model.AdminRule, error) {
	out := make([]model.AdminRule, 0, len(ids))
	for _, id := range ids {
		if r, ok := m.rules[id]; ok {
			out = append(out, *r)
		}
	}
	return out, nil
}

func (m *mockRuleRepo) ExistsByIDsAndName(_ []uint, _ string) (bool, error) {
	return false, nil
}

func (m *mockRuleRepo) ListActiveIDs() ([]uint, error) {
	out := make([]uint, len(m.activeIDs))
	copy(out, m.activeIDs)
	return out, nil
}

func (m *mockRuleRepo) ListActiveAsMenu() ([]model.AdminRule, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	out := make([]model.AdminRule, 0, len(m.activeIDs))
	for _, id := range m.activeIDs {
		if r, ok := m.rules[id]; ok {
			out = append(out, *r)
		}
	}
	return out, nil
}

func (m *mockRuleRepo) Create(_ *gin.Context, rule *model.AdminRule) error {
	m.createCalls++
	m.lastCreateRule = rule
	if m.createErr != nil {
		return m.createErr
	}
	return nil
}

func (m *mockRuleRepo) Update(_ *gin.Context, rule *model.AdminRule) error {
	m.updateCalls++
	m.lastUpdateRule = rule
	if m.updateErr != nil {
		return m.updateErr
	}
	return nil
}

func (m *mockRuleRepo) Delete(_ *gin.Context, id uint) error {
	m.deleteCalls++
	m.lastDeleteID = id
	if m.deleteErr != nil {
		return m.deleteErr
	}
	return nil
}

func (m *mockRuleRepo) DeleteBatch(_ *gin.Context, ids []uint) error {
	m.deleteBatchCalls++
	m.lastDeleteBatch = ids
	if m.deleteBatchErr != nil {
		return m.deleteBatchErr
	}
	return nil
}

func (m *mockRuleRepo) UpdateStatus(_ *gin.Context, id uint, status int8) error {
	m.updateStatusCalls++
	m.lastUpdateStatusID = id
	m.lastUpdateStatus = status
	if m.updateStatusErr != nil {
		return m.updateStatusErr
	}
	return nil
}

func (m *mockRuleRepo) GetPID(_ *gin.Context, id uint) (uint, error) {
	if m.getPIDErr != nil {
		return 0, m.getPIDErr
	}
	if r, ok := m.rules[id]; ok {
		return r.Pid, nil
	}
	return 0, gorm.ErrRecordNotFound
}

func (m *mockRuleRepo) HasChildren(_ *gin.Context, id uint) (bool, error) {
	if m.hasChildrenErr != nil {
		return false, m.hasChildrenErr
	}
	return m.childrenOf[id], nil
}

func (m *mockRuleRepo) GetByID(_ *gin.Context, id uint) (*model.AdminRule, error) {
	if m.getByIDErr != nil {
		return nil, m.getByIDErr
	}
	if r, ok := m.rules[id]; ok {
		return r, nil
	}
	return nil, gorm.ErrRecordNotFound
}

func (m *mockRuleRepo) List(_ *gin.Context, _ repository.ListOptions) ([]model.AdminRule, int64, error) {
	return nil, 0, nil
}

// 编译期断言：mockRuleRepo 必须实现 adminRepo.RuleRepository。
var _ adminRepo.RuleRepository = (*mockRuleRepo)(nil)

// =============================================================================
// helpers
// =============================================================================

func newRuleCtx() *gin.Context {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/admin/rule/create", nil)
	return c
}

// =============================================================================
// ValidatePID
// =============================================================================

func TestValidatePID_ZeroIsAlwaysValid(t *testing.T) {
	repo := &mockRuleRepo{}
	svc := NewRuleService(repo)

	if err := svc.ValidatePID(newRuleCtx(), 7, 0); err != nil {
		t.Errorf("ValidatePID(7, 0) = %v, want nil", err)
	}
	if err := svc.ValidatePID(newRuleCtx(), 0, 0); err != nil {
		t.Errorf("ValidatePID(0, 0) = %v, want nil", err)
	}
}

func TestValidatePID_SelfCycle(t *testing.T) {
	repo := &mockRuleRepo{}
	svc := NewRuleService(repo)

	if err := svc.ValidatePID(newRuleCtx(), 7, 7); !errors.Is(err, ErrPIDCycle) {
		t.Errorf("ValidatePID(7, 7) = %v, want ErrPIDCycle", err)
	}
}

func TestValidatePID_DescendantCycle(t *testing.T) {
	// 7 → 5 → 12（即 newPID=12 是 id=7 的孙子），把 7 的 pid 设为 12 即成环。
	repo := &mockRuleRepo{
		rules: map[uint]*model.AdminRule{
			5:  {ID: 5, Pid: 7},
			12: {ID: 12, Pid: 5},
		},
	}
	svc := NewRuleService(repo)

	if err := svc.ValidatePID(newRuleCtx(), 7, 12); !errors.Is(err, ErrPIDCycle) {
		t.Errorf("ValidatePID(7, 12) = %v, want ErrPIDCycle", err)
	}
}

func TestValidatePID_PIDNotFound(t *testing.T) {
	// newPID=99 在 mock map 中不存在 → 第一跳 GetPID 返 ErrRecordNotFound。
	repo := &mockRuleRepo{
		rules: map[uint]*model.AdminRule{},
	}
	svc := NewRuleService(repo)

	if err := svc.ValidatePID(newRuleCtx(), 7, 99); !errors.Is(err, ErrPIDNotFound) {
		t.Errorf("ValidatePID(7, 99) = %v, want ErrPIDNotFound", err)
	}
}

func TestValidatePID_ChainBreaksAtNotFound(t *testing.T) {
	// newPID=8 → 8.pid=88；88 不存在 → 视为合法终止点。
	repo := &mockRuleRepo{
		rules: map[uint]*model.AdminRule{
			8: {ID: 8, Pid: 88},
			// 88 missing
		},
	}
	svc := NewRuleService(repo)

	if err := svc.ValidatePID(newRuleCtx(), 7, 8); err != nil {
		t.Errorf("ValidatePID(7, 8) = %v, want nil (chain break)", err)
	}
}

func TestValidatePID_ExistingDataCycleDoesNotLoop(t *testing.T) {
	// 现存数据已有环：8.pid=9, 9.pid=8；visited 守护应防止无限循环。
	repo := &mockRuleRepo{
		rules: map[uint]*model.AdminRule{
			8: {ID: 8, Pid: 9},
			9: {ID: 9, Pid: 8},
		},
	}
	svc := NewRuleService(repo)

	// 把 7 的 pid 设为 8：8 的链是 8→9→8→... 永远命中不到 7，应返回 nil。
	if err := svc.ValidatePID(newRuleCtx(), 7, 8); err != nil {
		t.Errorf("ValidatePID(7, 8) = %v, want nil (visited break)", err)
	}
}

// =============================================================================
// Create / Update
// =============================================================================

func TestCreate_HappyPath(t *testing.T) {
	repo := &mockRuleRepo{}
	svc := NewRuleService(repo)

	entity := &model.AdminRule{Title: "X", Name: "x", Pid: 0}
	if err := svc.Create(newRuleCtx(), entity); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if repo.createCalls != 1 {
		t.Errorf("repo.Create called %d times, want 1", repo.createCalls)
	}
}

func TestCreate_PIDNotFound(t *testing.T) {
	repo := &mockRuleRepo{}
	svc := NewRuleService(repo)

	entity := &model.AdminRule{Title: "X", Name: "x", Pid: 9999}
	if err := svc.Create(newRuleCtx(), entity); !errors.Is(err, ErrPIDNotFound) {
		t.Errorf("Create = %v, want ErrPIDNotFound", err)
	}
	if repo.createCalls != 0 {
		t.Errorf("repo.Create called %d times on bad PID, want 0", repo.createCalls)
	}
}

func TestUpdate_PIDCycleDescendant(t *testing.T) {
	repo := &mockRuleRepo{
		rules: map[uint]*model.AdminRule{
			5:  {ID: 5, Pid: 7},
			12: {ID: 12, Pid: 5},
		},
	}
	svc := NewRuleService(repo)

	entity := &model.AdminRule{ID: 7, Title: "X", Pid: 12}
	if err := svc.Update(newRuleCtx(), entity); !errors.Is(err, ErrPIDCycle) {
		t.Errorf("Update = %v, want ErrPIDCycle", err)
	}
	if repo.updateCalls != 0 {
		t.Errorf("repo.Update called %d times on cycle, want 0", repo.updateCalls)
	}
}

func TestUpdate_HappyPath(t *testing.T) {
	repo := &mockRuleRepo{
		rules: map[uint]*model.AdminRule{
			3: {ID: 3, Pid: 0}, // 顶级
		},
	}
	svc := NewRuleService(repo)

	entity := &model.AdminRule{ID: 7, Title: "X", Pid: 3}
	if err := svc.Update(newRuleCtx(), entity); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if repo.updateCalls != 1 {
		t.Errorf("repo.Update called %d times, want 1", repo.updateCalls)
	}
}

// =============================================================================
// Delete / BatchDelete
// =============================================================================

func TestDelete_HasChildren(t *testing.T) {
	repo := &mockRuleRepo{
		childrenOf: map[uint]bool{7: true},
	}
	svc := NewRuleService(repo)

	if err := svc.Delete(newRuleCtx(), 7); !errors.Is(err, ErrHasChildren) {
		t.Errorf("Delete = %v, want ErrHasChildren", err)
	}
	if repo.deleteCalls != 0 {
		t.Errorf("repo.Delete called %d times on has-children, want 0", repo.deleteCalls)
	}
}

func TestDelete_HappyPath(t *testing.T) {
	repo := &mockRuleRepo{
		childrenOf: map[uint]bool{7: false},
	}
	svc := NewRuleService(repo)

	if err := svc.Delete(newRuleCtx(), 7); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if repo.deleteCalls != 1 {
		t.Errorf("repo.Delete called %d times, want 1", repo.deleteCalls)
	}
}

func TestBatchDelete_EmptyShortCircuit(t *testing.T) {
	repo := &mockRuleRepo{}
	svc := NewRuleService(repo)

	deleted, skipped, err := svc.BatchDelete(newRuleCtx(), []uint{})
	if err != nil {
		t.Fatalf("BatchDelete: %v", err)
	}
	if deleted != 0 || skipped != 0 {
		t.Errorf("got (%d, %d), want (0, 0)", deleted, skipped)
	}
}

func TestBatchDelete_MixedSkipReasons(t *testing.T) {
	// id=1 不存在、id=2 存在且无子、id=3 有子 → deleted=1, skipped=2。
	repo := &mockRuleRepo{
		rules: map[uint]*model.AdminRule{
			2: {ID: 2, Pid: 0}, // 存在且无子
			3: {ID: 3, Pid: 0}, // 存在但有子
		},
		childrenOf: map[uint]bool{3: true},
	}
	svc := NewRuleService(repo)

	deleted, skipped, err := svc.BatchDelete(newRuleCtx(), []uint{1, 2, 3})
	if err != nil {
		t.Fatalf("BatchDelete: %v", err)
	}
	if deleted != 1 || skipped != 2 {
		t.Errorf("got (%d, %d), want (1, 2)", deleted, skipped)
	}
}

// =============================================================================
// ToggleStatus
// =============================================================================

func TestToggleStatus_1To0(t *testing.T) {
	repo := &mockRuleRepo{
		rules: map[uint]*model.AdminRule{
			7: {ID: 7, Status: 1},
		},
	}
	svc := NewRuleService(repo)

	if err := svc.ToggleStatus(newRuleCtx(), 7); err != nil {
		t.Fatalf("ToggleStatus: %v", err)
	}
	if repo.updateStatusCalls != 1 {
		t.Errorf("repo.UpdateStatus called %d times, want 1", repo.updateStatusCalls)
	}
	if repo.lastUpdateStatus != 0 {
		t.Errorf("UpdateStatus arg = %d, want 0 (1→0)", repo.lastUpdateStatus)
	}
}

func TestToggleStatus_0To1(t *testing.T) {
	repo := &mockRuleRepo{
		rules: map[uint]*model.AdminRule{
			7: {ID: 7, Status: 0},
		},
	}
	svc := NewRuleService(repo)

	if err := svc.ToggleStatus(newRuleCtx(), 7); err != nil {
		t.Fatalf("ToggleStatus: %v", err)
	}
	if repo.lastUpdateStatus != 1 {
		t.Errorf("UpdateStatus arg = %d, want 1 (0→1)", repo.lastUpdateStatus)
	}
}

func TestRuleToggleStatus_NotFound(t *testing.T) {
	repo := &mockRuleRepo{
		rules: map[uint]*model.AdminRule{},
	}
	svc := NewRuleService(repo)

	if err := svc.ToggleStatus(newRuleCtx(), 99); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Errorf("ToggleStatus = %v, want gorm.ErrRecordNotFound", err)
	}
}

// =============================================================================
// ListAll
// =============================================================================

func TestListAll_ReturnsActiveMenu(t *testing.T) {
	repo := &mockRuleRepo{
		rules: map[uint]*model.AdminRule{
			1: {ID: 1, Title: "A"},
			2: {ID: 2, Title: "B"},
		},
		activeIDs: []uint{1, 2},
	}
	svc := NewRuleService(repo)

	got, err := svc.ListAll(newRuleCtx())
	if err != nil {
		t.Fatalf("ListAll: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("len(got) = %d, want 2", len(got))
	}
}
