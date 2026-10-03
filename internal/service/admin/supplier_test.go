package admin

import (
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"ai-go-mall/internal/model/mall"
	"ai-go-mall/internal/repository"
	supplierRepo "ai-go-mall/internal/repository/supplier"
)

// =============================================================================
// helpers
// =============================================================================

func newAdminSupplierCtx() *gin.Context {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", "/api/v1/admin/suppliers", nil)
	return c
}

// =============================================================================
// Mock SupplierRepository
// =============================================================================

type adminSupplierRepo struct {
	getByIDFunc        func(c *gin.Context, id int64) (*mall.MallSupplier, error)
	listWithFilterFunc func(c *gin.Context, opts supplierRepo.SupplierListOptions) ([]mall.MallSupplier, int64, error)
	toggleStatusFunc   func(c *gin.Context, id int64, status mall.MallBlindBoxStatus) error
	toggleFeaturedFunc func(c *gin.Context, id int64, featured bool) error
	updateBalanceFunc  func(c *gin.Context, id int64, amount float64) error
	listByIDsFunc      func(c *gin.Context, ids []int64) ([]mall.MallSupplier, error)

	toggleStatusCalls    int
	lastToggleStatusID   int64
	lastToggleStatusVal  mall.MallBlindBoxStatus
	toggleFeaturedCalls  int
	lastToggleFeaturedID int64
	lastToggleFeatured   bool
}

func (m *adminSupplierRepo) GetByID(c *gin.Context, id int64) (*mall.MallSupplier, error) {
	if m.getByIDFunc != nil {
		return m.getByIDFunc(c, id)
	}
	return nil, gorm.ErrRecordNotFound
}
func (m *adminSupplierRepo) ListByIDs(c *gin.Context, ids []int64) ([]mall.MallSupplier, error) {
	if m.listByIDsFunc != nil {
		return m.listByIDsFunc(c, ids)
	}
	return nil, nil
}
func (m *adminSupplierRepo) ListWithFilter(c *gin.Context, opts supplierRepo.SupplierListOptions) ([]mall.MallSupplier, int64, error) {
	if m.listWithFilterFunc != nil {
		return m.listWithFilterFunc(c, opts)
	}
	return nil, 0, nil
}
func (m *adminSupplierRepo) ToggleStatus(c *gin.Context, id int64, status mall.MallBlindBoxStatus) error {
	m.toggleStatusCalls++
	m.lastToggleStatusID = id
	m.lastToggleStatusVal = status
	if m.toggleStatusFunc != nil {
		return m.toggleStatusFunc(c, id, status)
	}
	return nil
}
func (m *adminSupplierRepo) ToggleFeatured(c *gin.Context, id int64, featured bool) error {
	m.toggleFeaturedCalls++
	m.lastToggleFeaturedID = id
	m.lastToggleFeatured = featured
	if m.toggleFeaturedFunc != nil {
		return m.toggleFeaturedFunc(c, id, featured)
	}
	return nil
}
func (m *adminSupplierRepo) UpdateBalanceAndSales(c *gin.Context, id int64, amount float64) error {
	if m.updateBalanceFunc != nil {
		return m.updateBalanceFunc(c, id, amount)
	}
	return nil
}
func (m *adminSupplierRepo) Create(c *gin.Context, e *mall.MallSupplier) error { return nil }
func (m *adminSupplierRepo) List(c *gin.Context, opts repository.ListOptions) ([]mall.MallSupplier, int64, error) {
	return nil, 0, nil
}
func (m *adminSupplierRepo) Update(c *gin.Context, e *mall.MallSupplier) error { return nil }
func (m *adminSupplierRepo) Delete(c *gin.Context, id int64) error             { return nil }

var _ supplierRepo.SupplierRepository = (*adminSupplierRepo)(nil)

// =============================================================================
// tests
// =============================================================================

func newAdminSupplierSvc(repo *adminSupplierRepo) SupplierService {
	return NewSupplierService(SupplierServiceDeps{SupplierRepo: repo})
}

// =============================================================================
// List
// =============================================================================

func TestAdminSupplierList_DelegatesToRepo(t *testing.T) {
	repo := &adminSupplierRepo{
		listWithFilterFunc: func(c *gin.Context, opts supplierRepo.SupplierListOptions) ([]mall.MallSupplier, int64, error) {
			featured := true
			if opts.StatusFilter != "active" {
				t.Errorf("StatusFilter = %q, want 'active'", opts.StatusFilter)
			}
			if opts.IsFeaturedFilter == nil || *opts.IsFeaturedFilter != featured {
				t.Errorf("IsFeaturedFilter = %v, want &true", opts.IsFeaturedFilter)
			}
			if opts.NameKeyword != "panini" {
				t.Errorf("NameKeyword = %q, want 'panini'", opts.NameKeyword)
			}
			return []mall.MallSupplier{
				{ID: 1, Name: "Panini 旗舰店", Status: mall.StatusActive, IsFeatured: true},
			}, 1, nil
		},
	}
	svc := newAdminSupplierSvc(repo)
	featured := true
	items, total, err := svc.List(newAdminSupplierCtx(), supplierRepo.SupplierListOptions{
		Page: 1, PageSize: 10, StatusFilter: "active", IsFeaturedFilter: &featured, NameKeyword: "panini",
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if total != 1 || len(items) != 1 {
		t.Errorf("got (%d, %d), want (1, 1)", total, len(items))
	}
}

func TestAdminSupplierList_RepoErrorPropagates(t *testing.T) {
	wantErr := errors.New("db: timeout")
	repo := &adminSupplierRepo{
		listWithFilterFunc: func(c *gin.Context, opts supplierRepo.SupplierListOptions) ([]mall.MallSupplier, int64, error) {
			return nil, 0, wantErr
		},
	}
	svc := newAdminSupplierSvc(repo)
	_, _, err := svc.List(newAdminSupplierCtx(), supplierRepo.SupplierListOptions{Page: 1, PageSize: 10})
	if !errors.Is(err, wantErr) {
		t.Errorf("err = %v, want %v", err, wantErr)
	}
}

// =============================================================================
// ToggleStatus
// =============================================================================

func TestAdminSupplierToggleStatus_InvalidID(t *testing.T) {
	svc := newAdminSupplierSvc(&adminSupplierRepo{})
	err := svc.ToggleStatus(newAdminSupplierCtx(), 0)
	if err == nil {
		t.Errorf("err = nil, want error")
	}
}

func TestAdminSupplierToggleStatus_NotFound(t *testing.T) {
	repo := &adminSupplierRepo{
		getByIDFunc: func(c *gin.Context, id int64) (*mall.MallSupplier, error) {
			return nil, supplierRepo.ErrSupplierNotFound
		},
	}
	svc := newAdminSupplierSvc(repo)
	err := svc.ToggleStatus(newAdminSupplierCtx(), 99)
	if !errors.Is(err, supplierRepo.ErrSupplierNotFound) {
		t.Errorf("err = %v, want ErrSupplierNotFound", err)
	}
	if repo.toggleStatusCalls != 0 {
		t.Errorf("ToggleStatus called %d times, want 0 (not found)", repo.toggleStatusCalls)
	}
}

func TestAdminSupplierToggleStatus_ActiveToDisabled(t *testing.T) {
	repo := &adminSupplierRepo{
		getByIDFunc: func(c *gin.Context, id int64) (*mall.MallSupplier, error) {
			return &mall.MallSupplier{ID: id, Status: mall.StatusActive}, nil
		},
	}
	svc := newAdminSupplierSvc(repo)
	if err := svc.ToggleStatus(newAdminSupplierCtx(), 1); err != nil {
		t.Fatalf("ToggleStatus: %v", err)
	}
	if repo.toggleStatusCalls != 1 {
		t.Errorf("ToggleStatus called %d times, want 1", repo.toggleStatusCalls)
	}
	if repo.lastToggleStatusVal != mall.StatusDisabled {
		t.Errorf("new status = %q, want disabled (active→disabled)", repo.lastToggleStatusVal)
	}
}

func TestAdminSupplierToggleStatus_DisabledToActive(t *testing.T) {
	repo := &adminSupplierRepo{
		getByIDFunc: func(c *gin.Context, id int64) (*mall.MallSupplier, error) {
			return &mall.MallSupplier{ID: id, Status: mall.StatusDisabled}, nil
		},
	}
	svc := newAdminSupplierSvc(repo)
	if err := svc.ToggleStatus(newAdminSupplierCtx(), 1); err != nil {
		t.Fatalf("ToggleStatus: %v", err)
	}
	if repo.lastToggleStatusVal != mall.StatusActive {
		t.Errorf("new status = %q, want active (disabled→active)", repo.lastToggleStatusVal)
	}
}

func TestAdminSupplierToggleStatus_ToggleErrorPropagates(t *testing.T) {
	wantErr := errors.New("repo: update fail")
	repo := &adminSupplierRepo{
		getByIDFunc: func(c *gin.Context, id int64) (*mall.MallSupplier, error) {
			return &mall.MallSupplier{ID: id, Status: mall.StatusActive}, nil
		},
		toggleStatusFunc: func(c *gin.Context, id int64, s mall.MallBlindBoxStatus) error {
			return wantErr
		},
	}
	svc := newAdminSupplierSvc(repo)
	err := svc.ToggleStatus(newAdminSupplierCtx(), 1)
	if !errors.Is(err, wantErr) {
		t.Errorf("err = %v, want %v", err, wantErr)
	}
}

// =============================================================================
// ToggleFeatured
// =============================================================================

func TestAdminSupplierToggleFeatured_InvalidID(t *testing.T) {
	svc := newAdminSupplierSvc(&adminSupplierRepo{})
	err := svc.ToggleFeatured(newAdminSupplierCtx(), 0)
	if err == nil {
		t.Errorf("err = nil, want error")
	}
}

func TestAdminSupplierToggleFeatured_NotFound(t *testing.T) {
	repo := &adminSupplierRepo{
		getByIDFunc: func(c *gin.Context, id int64) (*mall.MallSupplier, error) {
			return nil, supplierRepo.ErrSupplierNotFound
		},
	}
	svc := newAdminSupplierSvc(repo)
	err := svc.ToggleFeatured(newAdminSupplierCtx(), 99)
	if !errors.Is(err, supplierRepo.ErrSupplierNotFound) {
		t.Errorf("err = %v, want ErrSupplierNotFound", err)
	}
}

func TestAdminSupplierToggleFeatured_FalseToTrue(t *testing.T) {
	repo := &adminSupplierRepo{
		getByIDFunc: func(c *gin.Context, id int64) (*mall.MallSupplier, error) {
			return &mall.MallSupplier{ID: id, IsFeatured: false}, nil
		},
	}
	svc := newAdminSupplierSvc(repo)
	if err := svc.ToggleFeatured(newAdminSupplierCtx(), 1); err != nil {
		t.Fatalf("ToggleFeatured: %v", err)
	}
	if repo.toggleFeaturedCalls != 1 {
		t.Errorf("ToggleFeatured called %d times, want 1", repo.toggleFeaturedCalls)
	}
	if !repo.lastToggleFeatured {
		t.Errorf("new featured = false, want true (false→true)")
	}
}

func TestAdminSupplierToggleFeatured_TrueToFalse(t *testing.T) {
	repo := &adminSupplierRepo{
		getByIDFunc: func(c *gin.Context, id int64) (*mall.MallSupplier, error) {
			return &mall.MallSupplier{ID: id, IsFeatured: true}, nil
		},
	}
	svc := newAdminSupplierSvc(repo)
	if err := svc.ToggleFeatured(newAdminSupplierCtx(), 1); err != nil {
		t.Fatalf("ToggleFeatured: %v", err)
	}
	if repo.lastToggleFeatured {
		t.Errorf("new featured = true, want false (true→false)")
	}
}

func TestAdminSupplierToggleFeatured_ToggleErrorPropagates(t *testing.T) {
	wantErr := errors.New("repo: update fail")
	repo := &adminSupplierRepo{
		getByIDFunc: func(c *gin.Context, id int64) (*mall.MallSupplier, error) {
			return &mall.MallSupplier{ID: id, IsFeatured: false}, nil
		},
		toggleFeaturedFunc: func(c *gin.Context, id int64, featured bool) error {
			return wantErr
		},
	}
	svc := newAdminSupplierSvc(repo)
	err := svc.ToggleFeatured(newAdminSupplierCtx(), 1)
	if !errors.Is(err, wantErr) {
		t.Errorf("err = %v, want %v", err, wantErr)
	}
}
