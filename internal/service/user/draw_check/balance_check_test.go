package draw_check

import (
	"errors"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"ai-go-mall/internal/domain/chain"
	"ai-go-mall/internal/model"
	"ai-go-mall/internal/model/mall"
	"ai-go-mall/internal/repository"
	mallRepo "ai-go-mall/internal/repository/mall"
)

// =============================================================================
// helpers（复用 stubs）
// =============================================================================

type stubPromoRepo struct {
	findActiveFunc func(c *gin.Context, bbID int64, now time.Time) (*mall.MallPromotion, error)
}

func (s *stubPromoRepo) FindActiveByBlindBoxID(c *gin.Context, bbID int64, now time.Time) (*mall.MallPromotion, error) {
	if s.findActiveFunc != nil {
		return s.findActiveFunc(c, bbID, now)
	}
	return nil, nil
}
func (s *stubPromoRepo) Create(c *gin.Context, e *mall.MallPromotion) error { return nil }
func (s *stubPromoRepo) Update(c *gin.Context, e *mall.MallPromotion) error { return nil }
func (s *stubPromoRepo) Delete(c *gin.Context, id int64) error              { return nil }
func (s *stubPromoRepo) List(c *gin.Context, opts repository.ListOptions) ([]mall.MallPromotion, int64, error) {
	return nil, 0, nil
}
func (s *stubPromoRepo) Get(c *gin.Context, id int64) (*mall.MallPromotion, error) {
	return nil, nil
}
func (s *stubPromoRepo) GetByID(c *gin.Context, id int64) (*mall.MallPromotion, error) {
	return nil, nil
}
func (s *stubPromoRepo) ListBySupplier(c *gin.Context, sid int64, opts repository.ListOptions) ([]mall.MallPromotion, int64, error) {
	return nil, 0, nil
}
func (s *stubPromoRepo) ListTimeOverlap(c *gin.Context, bbID int64, start, end time.Time) ([]mall.MallPromotion, error) {
	return nil, nil
}
func (s *stubPromoRepo) ToggleStatus(c *gin.Context, id int64, st mall.MallBlindBoxStatus) error {
	return nil
}
func (s *stubPromoRepo) Toggle(c *gin.Context, id int64) error { return nil }

// =============================================================================
// 单测：normal source（活动价优先 / 回退原价）
// =============================================================================

func TestBalanceCheck_Normal_NoPromo_Pass(t *testing.T) {
	users := &stubUserRepo{
		getByIDFunc: func(c *gin.Context, id int64) (*model.User, error) {
			return &model.User{ID: id, Balance: 1000, Status: 1}, nil
		},
	}
	bb := &stubBlindBoxRepo{
		getByIDFunc: func(c *gin.Context, id int64) (*mall.MallBlindBox, error) {
			return &mall.MallBlindBox{ID: id, Price: 100, Status: mall.StatusActive, OnSale: true}, nil
		},
	}
	promo := &stubPromoRepo{
		findActiveFunc: func(c *gin.Context, bbID int64, now time.Time) (*mall.MallPromotion, error) {
			return nil, nil
		},
	}
	check := NewBalanceCheck(users, bb, promo, &stubSeckillRepo{})

	err := check.Validate(newCtx(), &chain.DrawInput{
		Source: chain.SourceNormal, UserID: 1, BlindBoxID: 10, Now: time.Now(),
	})
	if err != nil {
		t.Fatalf("expected pass, got err = %v", err)
	}
}

func TestBalanceCheck_Normal_WithPromo_Pass(t *testing.T) {
	users := &stubUserRepo{
		getByIDFunc: func(c *gin.Context, id int64) (*model.User, error) {
			return &model.User{ID: id, Balance: 100, Status: 1}, nil
		},
	}
	bb := &stubBlindBoxRepo{
		getByIDFunc: func(c *gin.Context, id int64) (*mall.MallBlindBox, error) {
			return &mall.MallBlindBox{ID: id, Price: 200}, nil // 原价 200
		},
	}
	promo := &stubPromoRepo{
		findActiveFunc: func(c *gin.Context, bbID int64, now time.Time) (*mall.MallPromotion, error) {
			return &mall.MallPromotion{PromoPrice: 50, Status: mall.StatusActive}, nil // 活动价 50
		},
	}
	check := NewBalanceCheck(users, bb, promo, &stubSeckillRepo{})

	// balance=100 >= promo=50 → pass
	err := check.Validate(newCtx(), &chain.DrawInput{
		Source: chain.SourceNormal, UserID: 1, BlindBoxID: 10, Now: time.Now(),
	})
	if err != nil {
		t.Fatalf("expected pass with promo price, got err = %v", err)
	}
}

func TestBalanceCheck_Normal_InsufficientBalance(t *testing.T) {
	users := &stubUserRepo{
		getByIDFunc: func(c *gin.Context, id int64) (*model.User, error) {
			return &model.User{ID: id, Balance: 10, Status: 1}, nil
		},
	}
	bb := &stubBlindBoxRepo{
		getByIDFunc: func(c *gin.Context, id int64) (*mall.MallBlindBox, error) {
			return &mall.MallBlindBox{ID: id, Price: 100}, nil
		},
	}
	promo := &stubPromoRepo{}
	check := NewBalanceCheck(users, bb, promo, &stubSeckillRepo{})

	err := check.Validate(newCtx(), &chain.DrawInput{
		Source: chain.SourceNormal, UserID: 1, BlindBoxID: 10, Now: time.Now(),
	})
	if !errors.Is(err, chain.ErrInsufficientBalance) {
		t.Fatalf("err = %v, want chain.ErrInsufficientBalance", err)
	}
}

func TestBalanceCheck_Normal_BlindBoxNotFound_Reject(t *testing.T) {
	// 拿不到价格 → 保守拒绝（视为余额不足）。
	users := &stubUserRepo{
		getByIDFunc: func(c *gin.Context, id int64) (*model.User, error) {
			return &model.User{ID: id, Balance: 1000}, nil
		},
	}
	bb := &stubBlindBoxRepo{
		getByIDFunc: func(c *gin.Context, id int64) (*mall.MallBlindBox, error) {
			return nil, gorm.ErrRecordNotFound
		},
	}
	check := NewBalanceCheck(users, bb, &stubPromoRepo{}, &stubSeckillRepo{})

	err := check.Validate(newCtx(), &chain.DrawInput{
		Source: chain.SourceNormal, UserID: 1, BlindBoxID: 10, Now: time.Now(),
	})
	if !errors.Is(err, chain.ErrInsufficientBalance) {
		t.Fatalf("err = %v, want chain.ErrInsufficientBalance (price unknown → conservative reject)", err)
	}
}

// =============================================================================
// 单测：seckill source（seckill_price）
// =============================================================================

func TestBalanceCheck_Seckill_Pass(t *testing.T) {
	users := &stubUserRepo{
		getByIDFunc: func(c *gin.Context, id int64) (*model.User, error) {
			return &model.User{ID: id, Balance: 100}, nil
		},
	}
	sec := &stubSeckillRepo{
		getByIDFunc: func(c *gin.Context, id int64) (*mall.MallSeckillActivity, error) {
			return &mall.MallSeckillActivity{ID: id, SeckillPrice: 50}, nil
		},
	}
	check := NewBalanceCheck(users, &stubBlindBoxRepo{}, &stubPromoRepo{}, sec)

	err := check.Validate(newCtx(), &chain.DrawInput{
		Source: chain.SourceSeckill, UserID: 1, SeckillID: 7, Now: time.Now(),
	})
	if err != nil {
		t.Fatalf("expected pass, got err = %v", err)
	}
}

func TestBalanceCheck_Seckill_InsufficientBalance(t *testing.T) {
	users := &stubUserRepo{
		getByIDFunc: func(c *gin.Context, id int64) (*model.User, error) {
			return &model.User{ID: id, Balance: 10}, nil
		},
	}
	sec := &stubSeckillRepo{
		getByIDFunc: func(c *gin.Context, id int64) (*mall.MallSeckillActivity, error) {
			return &mall.MallSeckillActivity{ID: id, SeckillPrice: 50}, nil
		},
	}
	check := NewBalanceCheck(users, &stubBlindBoxRepo{}, &stubPromoRepo{}, sec)

	err := check.Validate(newCtx(), &chain.DrawInput{
		Source: chain.SourceSeckill, UserID: 1, SeckillID: 7, Now: time.Now(),
	})
	if !errors.Is(err, chain.ErrInsufficientBalance) {
		t.Fatalf("err = %v, want chain.ErrInsufficientBalance", err)
	}
}

func TestBalanceCheck_Seckill_NotFound_Reject(t *testing.T) {
	users := &stubUserRepo{
		getByIDFunc: func(c *gin.Context, id int64) (*model.User, error) {
			return &model.User{ID: id, Balance: 1000}, nil
		},
	}
	sec := &stubSeckillRepo{
		getByIDFunc: func(c *gin.Context, id int64) (*mall.MallSeckillActivity, error) {
			return nil, gorm.ErrRecordNotFound
		},
	}
	check := NewBalanceCheck(users, &stubBlindBoxRepo{}, &stubPromoRepo{}, sec)

	err := check.Validate(newCtx(), &chain.DrawInput{
		Source: chain.SourceSeckill, UserID: 1, SeckillID: 7, Now: time.Now(),
	})
	if !errors.Is(err, chain.ErrInsufficientBalance) {
		t.Fatalf("err = %v, want chain.ErrInsufficientBalance (seckill price unknown → reject)", err)
	}
}

// =============================================================================
// 其他
// =============================================================================

func TestBalanceCheck_RejectUserNotFound(t *testing.T) {
	users := &stubUserRepo{
		getByIDFunc: func(c *gin.Context, id int64) (*model.User, error) {
			return nil, gorm.ErrRecordNotFound
		},
	}
	check := NewBalanceCheck(users, &stubBlindBoxRepo{}, &stubPromoRepo{}, &stubSeckillRepo{})

	err := check.Validate(newCtx(), &chain.DrawInput{
		Source: chain.SourceNormal, UserID: 1, BlindBoxID: 10, Now: time.Now(),
	})
	if !errors.Is(err, chain.ErrInsufficientBalance) {
		t.Fatalf("err = %v, want chain.ErrInsufficientBalance", err)
	}
}

func TestBalanceCheck_Name(t *testing.T) {
	check := NewBalanceCheck(&stubUserRepo{}, &stubBlindBoxRepo{}, &stubPromoRepo{}, &stubSeckillRepo{})
	if check.Name() != "balance" {
		t.Errorf("Name() = %q, want %q", check.Name(), "balance")
	}
}

// 编译期引用：避免 mallRepo / chain 包 unused 警告。
var (
	_ mallRepo.PromotionRepository = (*stubPromoRepo)(nil)
	_                              = chain.SourceNormal
)
