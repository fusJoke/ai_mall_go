package admin

import (
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"ai-go-mall/internal/model/mall"
	"ai-go-mall/internal/repository"
	mallRepo "ai-go-mall/internal/repository/mall"
)

// =============================================================================
// helpers
// =============================================================================

func newAdminBlindBoxCtx() *gin.Context {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", "/api/v1/admin/blindboxes", nil)
	return c
}

// =============================================================================
// Mock BlindBoxRepository
// =============================================================================

type adminBlindBoxRepo struct {
	listFunc           func(c *gin.Context, opts repository.ListOptions) ([]mall.MallBlindBox, int64, error)
	getByIDFunc        func(c *gin.Context, id int64) (*mall.MallBlindBox, error)
	toggleStatusFunc   func(c *gin.Context, id int64, status mall.MallBlindBoxStatus) error
	toggleOnSaleFunc   func(c *gin.Context, id int64, onSale bool) error
	toggleFeaturedFunc func(c *gin.Context, id int64, featured bool) error

	toggleStatusCalls    int
	lastToggleStatusID   int64
	lastToggleStatusVal  mall.MallBlindBoxStatus
	toggleOnSaleCalls    int
	lastToggleOnSaleID   int64
	lastToggleOnSaleVal  bool
	toggleFeaturedCalls  int
	lastToggleFeaturedID int64
	lastToggleFeatured   bool
}

func (m *adminBlindBoxRepo) GetByID(c *gin.Context, id int64) (*mall.MallBlindBox, error) {
	if m.getByIDFunc != nil {
		return m.getByIDFunc(c, id)
	}
	return nil, gorm.ErrRecordNotFound
}
func (m *adminBlindBoxRepo) List(c *gin.Context, opts repository.ListOptions) ([]mall.MallBlindBox, int64, error) {
	if m.listFunc != nil {
		return m.listFunc(c, opts)
	}
	return nil, 0, nil
}
func (m *adminBlindBoxRepo) ToggleStatus(c *gin.Context, id int64, status mall.MallBlindBoxStatus) error {
	m.toggleStatusCalls++
	m.lastToggleStatusID = id
	m.lastToggleStatusVal = status
	if m.toggleStatusFunc != nil {
		return m.toggleStatusFunc(c, id, status)
	}
	return nil
}
func (m *adminBlindBoxRepo) ToggleOnSale(c *gin.Context, id int64, onSale bool) error {
	m.toggleOnSaleCalls++
	m.lastToggleOnSaleID = id
	m.lastToggleOnSaleVal = onSale
	if m.toggleOnSaleFunc != nil {
		return m.toggleOnSaleFunc(c, id, onSale)
	}
	return nil
}
func (m *adminBlindBoxRepo) ToggleFeatured(c *gin.Context, id int64, featured bool) error {
	m.toggleFeaturedCalls++
	m.lastToggleFeaturedID = id
	m.lastToggleFeatured = featured
	if m.toggleFeaturedFunc != nil {
		return m.toggleFeaturedFunc(c, id, featured)
	}
	return nil
}
func (m *adminBlindBoxRepo) ListBySupplier(c *gin.Context, supplierID int64, opts repository.ListOptions) ([]mall.MallBlindBox, int64, error) {
	return nil, 0, nil
}
func (m *adminBlindBoxRepo) ListFeaturedOnSale(c *gin.Context, opts repository.ListOptions) ([]mall.MallBlindBox, int64, error) {
	return nil, 0, nil
}
func (m *adminBlindBoxRepo) ListActiveOnSaleBySupplierIDs(c *gin.Context, supplierIDs []int64, opts repository.ListOptions) ([]mall.MallBlindBox, int64, error) {
	return nil, 0, nil
}
func (m *adminBlindBoxRepo) ListByIDs(c *gin.Context, ids []int64) ([]mall.MallBlindBox, error) {
	return nil, nil
}
func (m *adminBlindBoxRepo) Create(c *gin.Context, e *mall.MallBlindBox) error { return nil }
func (m *adminBlindBoxRepo) Update(c *gin.Context, e *mall.MallBlindBox) error { return nil }
func (m *adminBlindBoxRepo) Delete(c *gin.Context, id int64) error             { return nil }

var _ mallRepo.BlindBoxRepository = (*adminBlindBoxRepo)(nil)

// =============================================================================
// tests
// =============================================================================

func newAdminBlindBoxSvc(repo *adminBlindBoxRepo) BlindBoxService {
	return NewBlindBoxService(BlindBoxServiceDeps{BlindBoxRepo: repo})
}

// =============================================================================
// List
// =============================================================================

func TestAdminBlindBoxList_DelegatesToRepo(t *testing.T) {
	repo := &adminBlindBoxRepo{
		listFunc: func(c *gin.Context, opts repository.ListOptions) ([]mall.MallBlindBox, int64, error) {
			if opts.Page != 1 || opts.PageSize != 20 {
				t.Errorf("opts = %+v, want Page=1 PageSize=20", opts)
			}
			return []mall.MallBlindBox{
				{ID: 1, Name: "Panini Prizm", Status: mall.StatusActive, OnSale: true, IsFeatured: true},
				{ID: 2, Name: "Topps Chrome", Status: mall.StatusDisabled, OnSale: false},
			}, 2, nil
		},
	}
	svc := newAdminBlindBoxSvc(repo)
	items, total, err := svc.List(newAdminBlindBoxCtx(), repository.ListOptions{Page: 1, PageSize: 20})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if total != 2 || len(items) != 2 {
		t.Errorf("got (%d, %d), want (2, 2)", total, len(items))
	}
}

func TestAdminBlindBoxList_RepoErrorPropagates(t *testing.T) {
	wantErr := errors.New("db: timeout")
	repo := &adminBlindBoxRepo{
		listFunc: func(c *gin.Context, opts repository.ListOptions) ([]mall.MallBlindBox, int64, error) {
			return nil, 0, wantErr
		},
	}
	svc := newAdminBlindBoxSvc(repo)
	_, _, err := svc.List(newAdminBlindBoxCtx(), repository.ListOptions{Page: 1, PageSize: 10})
	if !errors.Is(err, wantErr) {
		t.Errorf("err = %v, want %v", err, wantErr)
	}
}

// =============================================================================
// ToggleStatus
// =============================================================================

func TestAdminBlindBoxToggleStatus_InvalidID(t *testing.T) {
	svc := newAdminBlindBoxSvc(&adminBlindBoxRepo{})
	err := svc.ToggleStatus(newAdminBlindBoxCtx(), 0)
	if err == nil {
		t.Errorf("err = nil, want error")
	}
}

func TestAdminBlindBoxToggleStatus_NotFound(t *testing.T) {
	repo := &adminBlindBoxRepo{
		getByIDFunc: func(c *gin.Context, id int64) (*mall.MallBlindBox, error) {
			return nil, gorm.ErrRecordNotFound
		},
	}
	svc := newAdminBlindBoxSvc(repo)
	err := svc.ToggleStatus(newAdminBlindBoxCtx(), 99)
	if !errors.Is(err, ErrBlindBoxNotFound) {
		t.Errorf("err = %v, want ErrBlindBoxNotFound", err)
	}
	if repo.toggleStatusCalls != 0 {
		t.Errorf("ToggleStatus called %d times, want 0 (not found)", repo.toggleStatusCalls)
	}
}

func TestAdminBlindBoxToggleStatus_ActiveToDisabled(t *testing.T) {
	repo := &adminBlindBoxRepo{
		getByIDFunc: func(c *gin.Context, id int64) (*mall.MallBlindBox, error) {
			return &mall.MallBlindBox{ID: id, Status: mall.StatusActive}, nil
		},
	}
	svc := newAdminBlindBoxSvc(repo)
	if err := svc.ToggleStatus(newAdminBlindBoxCtx(), 1); err != nil {
		t.Fatalf("ToggleStatus: %v", err)
	}
	if repo.toggleStatusCalls != 1 {
		t.Errorf("ToggleStatus called %d times, want 1", repo.toggleStatusCalls)
	}
	if repo.lastToggleStatusVal != mall.StatusDisabled {
		t.Errorf("new status = %q, want disabled (active→disabled)", repo.lastToggleStatusVal)
	}
}

func TestAdminBlindBoxToggleStatus_DisabledToActive(t *testing.T) {
	repo := &adminBlindBoxRepo{
		getByIDFunc: func(c *gin.Context, id int64) (*mall.MallBlindBox, error) {
			return &mall.MallBlindBox{ID: id, Status: mall.StatusDisabled}, nil
		},
	}
	svc := newAdminBlindBoxSvc(repo)
	if err := svc.ToggleStatus(newAdminBlindBoxCtx(), 1); err != nil {
		t.Fatalf("ToggleStatus: %v", err)
	}
	if repo.lastToggleStatusVal != mall.StatusActive {
		t.Errorf("new status = %q, want active (disabled→active)", repo.lastToggleStatusVal)
	}
}

func TestAdminBlindBoxToggleStatus_GetByIDErrorPropagates(t *testing.T) {
	wantErr := errors.New("db: connection lost")
	repo := &adminBlindBoxRepo{
		getByIDFunc: func(c *gin.Context, id int64) (*mall.MallBlindBox, error) {
			return nil, wantErr
		},
	}
	svc := newAdminBlindBoxSvc(repo)
	err := svc.ToggleStatus(newAdminBlindBoxCtx(), 1)
	if !errors.Is(err, wantErr) {
		t.Errorf("err = %v, want %v", err, wantErr)
	}
	if repo.toggleStatusCalls != 0 {
		t.Errorf("ToggleStatus called %d times, want 0 (get failed)", repo.toggleStatusCalls)
	}
}

func TestAdminBlindBoxToggleStatus_ToggleErrorPropagates(t *testing.T) {
	wantErr := errors.New("repo: update fail")
	repo := &adminBlindBoxRepo{
		getByIDFunc: func(c *gin.Context, id int64) (*mall.MallBlindBox, error) {
			return &mall.MallBlindBox{ID: id, Status: mall.StatusActive}, nil
		},
		toggleStatusFunc: func(c *gin.Context, id int64, s mall.MallBlindBoxStatus) error {
			return wantErr
		},
	}
	svc := newAdminBlindBoxSvc(repo)
	err := svc.ToggleStatus(newAdminBlindBoxCtx(), 1)
	if !errors.Is(err, wantErr) {
		t.Errorf("err = %v, want %v", err, wantErr)
	}
}

// =============================================================================
// ToggleOnSale
// =============================================================================

func TestAdminBlindBoxToggleOnSale_InvalidID(t *testing.T) {
	svc := newAdminBlindBoxSvc(&adminBlindBoxRepo{})
	err := svc.ToggleOnSale(newAdminBlindBoxCtx(), 0)
	if err == nil {
		t.Errorf("err = nil, want error")
	}
}

func TestAdminBlindBoxToggleOnSale_NotFound(t *testing.T) {
	repo := &adminBlindBoxRepo{
		getByIDFunc: func(c *gin.Context, id int64) (*mall.MallBlindBox, error) {
			return nil, gorm.ErrRecordNotFound
		},
	}
	svc := newAdminBlindBoxSvc(repo)
	err := svc.ToggleOnSale(newAdminBlindBoxCtx(), 99)
	if !errors.Is(err, ErrBlindBoxNotFound) {
		t.Errorf("err = %v, want ErrBlindBoxNotFound", err)
	}
}

func TestAdminBlindBoxToggleOnSale_FalseToTrue(t *testing.T) {
	repo := &adminBlindBoxRepo{
		getByIDFunc: func(c *gin.Context, id int64) (*mall.MallBlindBox, error) {
			return &mall.MallBlindBox{ID: id, OnSale: false}, nil
		},
	}
	svc := newAdminBlindBoxSvc(repo)
	if err := svc.ToggleOnSale(newAdminBlindBoxCtx(), 1); err != nil {
		t.Fatalf("ToggleOnSale: %v", err)
	}
	if repo.toggleOnSaleCalls != 1 {
		t.Errorf("ToggleOnSale called %d times, want 1", repo.toggleOnSaleCalls)
	}
	if !repo.lastToggleOnSaleVal {
		t.Errorf("new on_sale = false, want true (false→true)")
	}
}

func TestAdminBlindBoxToggleOnSale_TrueToFalse(t *testing.T) {
	repo := &adminBlindBoxRepo{
		getByIDFunc: func(c *gin.Context, id int64) (*mall.MallBlindBox, error) {
			return &mall.MallBlindBox{ID: id, OnSale: true}, nil
		},
	}
	svc := newAdminBlindBoxSvc(repo)
	if err := svc.ToggleOnSale(newAdminBlindBoxCtx(), 1); err != nil {
		t.Fatalf("ToggleOnSale: %v", err)
	}
	if repo.lastToggleOnSaleVal {
		t.Errorf("new on_sale = true, want false (true→false)")
	}
}

func TestAdminBlindBoxToggleOnSale_ToggleErrorPropagates(t *testing.T) {
	wantErr := errors.New("repo: update fail")
	repo := &adminBlindBoxRepo{
		getByIDFunc: func(c *gin.Context, id int64) (*mall.MallBlindBox, error) {
			return &mall.MallBlindBox{ID: id, OnSale: true}, nil
		},
		toggleOnSaleFunc: func(c *gin.Context, id int64, onSale bool) error {
			return wantErr
		},
	}
	svc := newAdminBlindBoxSvc(repo)
	err := svc.ToggleOnSale(newAdminBlindBoxCtx(), 1)
	if !errors.Is(err, wantErr) {
		t.Errorf("err = %v, want %v", err, wantErr)
	}
}

// =============================================================================
// ToggleFeatured
// =============================================================================

func TestAdminBlindBoxToggleFeatured_InvalidID(t *testing.T) {
	svc := newAdminBlindBoxSvc(&adminBlindBoxRepo{})
	err := svc.ToggleFeatured(newAdminBlindBoxCtx(), 0)
	if err == nil {
		t.Errorf("err = nil, want error")
	}
}

func TestAdminBlindBoxToggleFeatured_NotFound(t *testing.T) {
	repo := &adminBlindBoxRepo{
		getByIDFunc: func(c *gin.Context, id int64) (*mall.MallBlindBox, error) {
			return nil, gorm.ErrRecordNotFound
		},
	}
	svc := newAdminBlindBoxSvc(repo)
	err := svc.ToggleFeatured(newAdminBlindBoxCtx(), 99)
	if !errors.Is(err, ErrBlindBoxNotFound) {
		t.Errorf("err = %v, want ErrBlindBoxNotFound", err)
	}
}

func TestAdminBlindBoxToggleFeatured_FalseToTrue(t *testing.T) {
	repo := &adminBlindBoxRepo{
		getByIDFunc: func(c *gin.Context, id int64) (*mall.MallBlindBox, error) {
			return &mall.MallBlindBox{ID: id, IsFeatured: false}, nil
		},
	}
	svc := newAdminBlindBoxSvc(repo)
	if err := svc.ToggleFeatured(newAdminBlindBoxCtx(), 1); err != nil {
		t.Fatalf("ToggleFeatured: %v", err)
	}
	if repo.toggleFeaturedCalls != 1 {
		t.Errorf("ToggleFeatured called %d times, want 1", repo.toggleFeaturedCalls)
	}
	if !repo.lastToggleFeatured {
		t.Errorf("new is_featured = false, want true (false→true)")
	}
}

func TestAdminBlindBoxToggleFeatured_TrueToFalse(t *testing.T) {
	repo := &adminBlindBoxRepo{
		getByIDFunc: func(c *gin.Context, id int64) (*mall.MallBlindBox, error) {
			return &mall.MallBlindBox{ID: id, IsFeatured: true}, nil
		},
	}
	svc := newAdminBlindBoxSvc(repo)
	if err := svc.ToggleFeatured(newAdminBlindBoxCtx(), 1); err != nil {
		t.Fatalf("ToggleFeatured: %v", err)
	}
	if repo.lastToggleFeatured {
		t.Errorf("new is_featured = true, want false (true→false)")
	}
}

func TestAdminBlindBoxToggleFeatured_ToggleErrorPropagates(t *testing.T) {
	wantErr := errors.New("repo: update fail")
	repo := &adminBlindBoxRepo{
		getByIDFunc: func(c *gin.Context, id int64) (*mall.MallBlindBox, error) {
			return &mall.MallBlindBox{ID: id, IsFeatured: false}, nil
		},
		toggleFeaturedFunc: func(c *gin.Context, id int64, featured bool) error {
			return wantErr
		},
	}
	svc := newAdminBlindBoxSvc(repo)
	err := svc.ToggleFeatured(newAdminBlindBoxCtx(), 1)
	if !errors.Is(err, wantErr) {
		t.Errorf("err = %v, want %v", err, wantErr)
	}
}
