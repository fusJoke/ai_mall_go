package draw_check

import (
	"errors"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"ai-go-mall/internal/domain/chain"
	"ai-go-mall/internal/model/mall"
	"ai-go-mall/internal/repository"
	mallRepo "ai-go-mall/internal/repository/mall"
	supplierRepo "ai-go-mall/internal/repository/supplier"
)

// =============================================================================
// stubs
// =============================================================================

type stubBlindBoxRepo struct {
	getByIDFunc func(c *gin.Context, id int64) (*mall.MallBlindBox, error)
}

func (s *stubBlindBoxRepo) GetByID(c *gin.Context, id int64) (*mall.MallBlindBox, error) {
	if s.getByIDFunc != nil {
		return s.getByIDFunc(c, id)
	}
	return nil, gorm.ErrRecordNotFound
}

func (s *stubBlindBoxRepo) ListBySupplier(c *gin.Context, sid int64, opts repository.ListOptions) ([]mall.MallBlindBox, int64, error) {
	return nil, 0, nil
}
func (s *stubBlindBoxRepo) ListFeaturedOnSale(c *gin.Context, opts repository.ListOptions) ([]mall.MallBlindBox, int64, error) {
	return nil, 0, nil
}
func (s *stubBlindBoxRepo) ListActiveOnSaleBySupplierIDs(c *gin.Context, ids []int64, opts repository.ListOptions) ([]mall.MallBlindBox, int64, error) {
	return nil, 0, nil
}
func (s *stubBlindBoxRepo) ListByIDs(c *gin.Context, ids []int64) ([]mall.MallBlindBox, error) {
	return nil, nil
}
func (s *stubBlindBoxRepo) Create(c *gin.Context, e *mall.MallBlindBox) error { return nil }
func (s *stubBlindBoxRepo) Update(c *gin.Context, e *mall.MallBlindBox) error { return nil }
func (s *stubBlindBoxRepo) Delete(c *gin.Context, id int64) error             { return nil }
func (s *stubBlindBoxRepo) List(c *gin.Context, opts repository.ListOptions) ([]mall.MallBlindBox, int64, error) {
	return nil, 0, nil
}
func (s *stubBlindBoxRepo) Get(c *gin.Context, id int64) (*mall.MallBlindBox, error) {
	return nil, nil
}
func (s *stubBlindBoxRepo) ToggleStatus(c *gin.Context, id int64, st mall.MallBlindBoxStatus) error {
	return nil
}
func (s *stubBlindBoxRepo) ToggleOnSale(c *gin.Context, id int64, onSale bool) error { return nil }
func (s *stubBlindBoxRepo) ToggleFeatured(c *gin.Context, id int64, f bool) error    { return nil }

var _ mallRepo.BlindBoxRepository = (*stubBlindBoxRepo)(nil)

type stubSupplierRepo struct {
	getByIDFunc func(c *gin.Context, id int64) (*mall.MallSupplier, error)
}

func (s *stubSupplierRepo) GetByID(c *gin.Context, id int64) (*mall.MallSupplier, error) {
	if s.getByIDFunc != nil {
		return s.getByIDFunc(c, id)
	}
	return nil, gorm.ErrRecordNotFound
}

func (s *stubSupplierRepo) ListByIDs(c *gin.Context, ids []int64) ([]mall.MallSupplier, error) {
	return nil, nil
}
func (s *stubSupplierRepo) Create(c *gin.Context, e *mall.MallSupplier) error { return nil }
func (s *stubSupplierRepo) Update(c *gin.Context, e *mall.MallSupplier) error { return nil }
func (s *stubSupplierRepo) Delete(c *gin.Context, id int64) error             { return nil }
func (s *stubSupplierRepo) List(c *gin.Context, opts repository.ListOptions) ([]mall.MallSupplier, int64, error) {
	return nil, 0, nil
}
func (s *stubSupplierRepo) Get(c *gin.Context, id int64) (*mall.MallSupplier, error) {
	return nil, nil
}
func (s *stubSupplierRepo) ToggleStatus(c *gin.Context, id int64, st mall.MallBlindBoxStatus) error {
	return nil
}
func (s *stubSupplierRepo) ToggleFeatured(c *gin.Context, id int64, f bool) error { return nil }
func (s *stubSupplierRepo) ListAll(c *gin.Context, opts repository.ListOptions) ([]mall.MallSupplier, int64, error) {
	return nil, 0, nil
}
func (s *stubSupplierRepo) ListWithFilter(c *gin.Context, opts supplierRepo.SupplierListOptions) ([]mall.MallSupplier, int64, error) {
	return nil, 0, nil
}
func (s *stubSupplierRepo) UpdateBalanceAndSales(c *gin.Context, id int64, amount float64) error {
	return nil
}

var _ supplierRepo.SupplierRepository = (*stubSupplierRepo)(nil)

// =============================================================================
// 单测
// =============================================================================

func TestBlindBoxBuyableCheck_Pass(t *testing.T) {
	bb := &stubBlindBoxRepo{
		getByIDFunc: func(c *gin.Context, id int64) (*mall.MallBlindBox, error) {
			return &mall.MallBlindBox{
				ID: id, SupplierID: 7,
				Status: mall.StatusActive, OnSale: true,
			}, nil
		},
	}
	sup := &stubSupplierRepo{
		getByIDFunc: func(c *gin.Context, id int64) (*mall.MallSupplier, error) {
			return &mall.MallSupplier{ID: id, Status: mall.StatusActive}, nil
		},
	}
	check := NewBlindBoxBuyableCheck(bb, sup)

	err := check.Validate(newCtx(), &chain.DrawInput{
		Source: chain.SourceNormal, UserID: 1, BlindBoxID: 10, Now: time.Now(),
	})
	if err != nil {
		t.Fatalf("expected pass, got err = %v", err)
	}
}

func TestBlindBoxBuyableCheck_RejectBlindBoxNotFound(t *testing.T) {
	bb := &stubBlindBoxRepo{
		getByIDFunc: func(c *gin.Context, id int64) (*mall.MallBlindBox, error) {
			return nil, gorm.ErrRecordNotFound
		},
	}
	sup := &stubSupplierRepo{}
	check := NewBlindBoxBuyableCheck(bb, sup)

	err := check.Validate(newCtx(), &chain.DrawInput{
		Source: chain.SourceNormal, UserID: 1, BlindBoxID: 10, Now: time.Now(),
	})
	if !errors.Is(err, chain.ErrBlindBoxNotAvailable) {
		t.Fatalf("err = %v, want chain.ErrBlindBoxNotAvailable", err)
	}
}

func TestBlindBoxBuyableCheck_RejectBlindBoxDisabled(t *testing.T) {
	bb := &stubBlindBoxRepo{
		getByIDFunc: func(c *gin.Context, id int64) (*mall.MallBlindBox, error) {
			return &mall.MallBlindBox{ID: id, Status: mall.StatusDisabled, OnSale: true}, nil
		},
	}
	check := NewBlindBoxBuyableCheck(bb, &stubSupplierRepo{})

	err := check.Validate(newCtx(), &chain.DrawInput{
		Source: chain.SourceNormal, UserID: 1, BlindBoxID: 10, Now: time.Now(),
	})
	if !errors.Is(err, chain.ErrBlindBoxNotAvailable) {
		t.Fatalf("err = %v, want chain.ErrBlindBoxNotAvailable", err)
	}
}

func TestBlindBoxBuyableCheck_RejectNotOnSale(t *testing.T) {
	bb := &stubBlindBoxRepo{
		getByIDFunc: func(c *gin.Context, id int64) (*mall.MallBlindBox, error) {
			return &mall.MallBlindBox{ID: id, Status: mall.StatusActive, OnSale: false}, nil
		},
	}
	check := NewBlindBoxBuyableCheck(bb, &stubSupplierRepo{})

	err := check.Validate(newCtx(), &chain.DrawInput{
		Source: chain.SourceNormal, UserID: 1, BlindBoxID: 10, Now: time.Now(),
	})
	if !errors.Is(err, chain.ErrBlindBoxNotAvailable) {
		t.Fatalf("err = %v, want chain.ErrBlindBoxNotAvailable", err)
	}
}

func TestBlindBoxBuyableCheck_RejectSupplierDisabled(t *testing.T) {
	bb := &stubBlindBoxRepo{
		getByIDFunc: func(c *gin.Context, id int64) (*mall.MallBlindBox, error) {
			return &mall.MallBlindBox{ID: id, SupplierID: 7, Status: mall.StatusActive, OnSale: true}, nil
		},
	}
	sup := &stubSupplierRepo{
		getByIDFunc: func(c *gin.Context, id int64) (*mall.MallSupplier, error) {
			return &mall.MallSupplier{ID: id, Status: mall.StatusDisabled}, nil
		},
	}
	check := NewBlindBoxBuyableCheck(bb, sup)

	err := check.Validate(newCtx(), &chain.DrawInput{
		Source: chain.SourceNormal, UserID: 1, BlindBoxID: 10, Now: time.Now(),
	})
	if !errors.Is(err, chain.ErrBlindBoxNotAvailable) {
		t.Fatalf("err = %v, want chain.ErrBlindBoxNotAvailable", err)
	}
}

func TestBlindBoxBuyableCheck_RejectBlindBoxIDZero(t *testing.T) {
	check := NewBlindBoxBuyableCheck(&stubBlindBoxRepo{}, &stubSupplierRepo{})

	err := check.Validate(newCtx(), &chain.DrawInput{
		Source: chain.SourceNormal, UserID: 1, BlindBoxID: 0, Now: time.Now(),
	})
	if !errors.Is(err, chain.ErrBlindBoxNotAvailable) {
		t.Fatalf("err = %v, want chain.ErrBlindBoxNotAvailable", err)
	}
}

func TestBlindBoxBuyableCheck_PropagatesSupplierDBError(t *testing.T) {
	bb := &stubBlindBoxRepo{
		getByIDFunc: func(c *gin.Context, id int64) (*mall.MallBlindBox, error) {
			return &mall.MallBlindBox{ID: id, SupplierID: 7, Status: mall.StatusActive, OnSale: true}, nil
		},
	}
	dbErr := errors.New("db down")
	sup := &stubSupplierRepo{
		getByIDFunc: func(c *gin.Context, id int64) (*mall.MallSupplier, error) {
			return nil, dbErr
		},
	}
	check := NewBlindBoxBuyableCheck(bb, sup)

	err := check.Validate(newCtx(), &chain.DrawInput{
		Source: chain.SourceNormal, UserID: 1, BlindBoxID: 10, Now: time.Now(),
	})
	if !errors.Is(err, dbErr) {
		t.Fatalf("err = %v, want wrap dbErr", err)
	}
}

func TestBlindBoxBuyableCheck_Name(t *testing.T) {
	check := NewBlindBoxBuyableCheck(&stubBlindBoxRepo{}, &stubSupplierRepo{})
	if check.Name() != "blindbox_buyable" {
		t.Errorf("Name() = %q, want %q", check.Name(), "blindbox_buyable")
	}
}
