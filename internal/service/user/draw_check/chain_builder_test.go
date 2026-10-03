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
)

// =============================================================================
// 端到端：Chain 装配结果 + 短路行为
//
// 依赖的 stubUserRepo / stubBlindBoxRepo / stubSupplierRepo / stubPromoRepo /
// stubSeckillRepo 都已在 sibling _test.go 文件里定义；本文件只补"装配 + 端到端"测试。
// =============================================================================

func TestNewDrawCheckChain_AssemblesWithoutPanic(t *testing.T) {
	// 用能跑通合法 input 的最小 stub，验证装配后 chain 不会 panic。
	users := &stubUserRepo{
		getByIDFunc: func(c *gin.Context, id int64) (*model.User, error) {
			return &model.User{ID: id, Balance: 1000, Status: 1}, nil
		},
	}
	ch := NewDrawCheckChain(CheckDeps{
		Users:     users,
		BlindBox:  &stubBlindBoxRepo{},
		Promotion: &stubPromoRepo{},
		Seckill:   &stubSeckillRepo{},
		Supplier:  &stubSupplierRepo{},
	})

	if err := ch.Validate(newCtx(), &chain.DrawInput{
		Source: chain.SourceNormal, UserID: 1, BlindBoxID: 1, Now: time.Now(),
	}); err != nil {
		t.Logf("chain.Validate returned err = %v (acceptable on default stubs)", err)
	}
}

func TestNewSeckillCheckChain_AssemblesWithoutPanic(t *testing.T) {
	// seckill 链路也用 [user_active, blindbox_buyable, time_window, balance]，
	// 区别只是调用方使用 Source=SourceSeckill 触发 time_window 的真实校验。
	ch := NewSeckillCheckChain(CheckDeps{
		Users:     &stubUserRepo{},
		BlindBox:  &stubBlindBoxRepo{},
		Promotion: &stubPromoRepo{},
		Seckill:   &stubSeckillRepo{},
		Supplier:  &stubSupplierRepo{},
	})
	if ch == nil {
		t.Fatal("SeckillCheckChain must not be nil")
	}
}

// =============================================================================
// 端到端：DrawChain 跑通合法 input
// =============================================================================

func TestDrawChain_HappyPath(t *testing.T) {
	users := &stubUserRepo{
		getByIDFunc: func(c *gin.Context, id int64) (*model.User, error) {
			return &model.User{ID: id, Balance: 1000, Status: 1}, nil
		},
	}
	bb := &stubBlindBoxRepo{
		getByIDFunc: func(c *gin.Context, id int64) (*mall.MallBlindBox, error) {
			return &mall.MallBlindBox{ID: id, SupplierID: 7, Status: mall.StatusActive, OnSale: true, Price: 100}, nil
		},
	}
	sup := &stubSupplierRepo{
		getByIDFunc: func(c *gin.Context, id int64) (*mall.MallSupplier, error) {
			return &mall.MallSupplier{ID: id, Status: mall.StatusActive}, nil
		},
	}
	promo := &stubPromoRepo{
		findActiveFunc: func(c *gin.Context, bbID int64, now time.Time) (*mall.MallPromotion, error) {
			return nil, nil
		},
	}

	ch := NewDrawCheckChain(CheckDeps{
		Users:     users,
		BlindBox:  bb,
		Promotion: promo,
		Seckill:   &stubSeckillRepo{},
		Supplier:  sup,
	})

	err := ch.Validate(newCtx(), &chain.DrawInput{
		Source: chain.SourceNormal, UserID: 1, BlindBoxID: 10, Now: time.Now(),
	})
	if err != nil {
		t.Fatalf("happy path should pass, got err = %v", err)
	}
}

// =============================================================================
// 端到端：DrawChain 失败出口 1——user 不存在
// =============================================================================

func TestDrawChain_RejectUserNotFound(t *testing.T) {
	users := &stubUserRepo{
		getByIDFunc: func(c *gin.Context, id int64) (*model.User, error) {
			return nil, gorm.ErrRecordNotFound
		},
	}
	ch := NewDrawCheckChain(CheckDeps{
		Users:     users,
		BlindBox:  &stubBlindBoxRepo{},
		Promotion: &stubPromoRepo{},
		Seckill:   &stubSeckillRepo{},
		Supplier:  &stubSupplierRepo{},
	})

	err := ch.Validate(newCtx(), &chain.DrawInput{
		Source: chain.SourceNormal, UserID: 1, BlindBoxID: 10, Now: time.Now(),
	})
	if !errors.Is(err, chain.ErrUserNotAvailable) {
		t.Fatalf("err = %v, want chain.ErrUserNotAvailable", err)
	}
}

// =============================================================================
// 端到端：DrawChain 失败出口 2——盲盒不可买
// =============================================================================

func TestDrawChain_RejectBlindBoxDisabled(t *testing.T) {
	users := &stubUserRepo{
		getByIDFunc: func(c *gin.Context, id int64) (*model.User, error) {
			return &model.User{ID: id, Balance: 1000, Status: 1}, nil
		},
	}
	bb := &stubBlindBoxRepo{
		getByIDFunc: func(c *gin.Context, id int64) (*mall.MallBlindBox, error) {
			return &mall.MallBlindBox{ID: id, Status: mall.StatusDisabled, OnSale: true}, nil
		},
	}
	ch := NewDrawCheckChain(CheckDeps{
		Users:     users,
		BlindBox:  bb,
		Promotion: &stubPromoRepo{},
		Seckill:   &stubSeckillRepo{},
		Supplier:  &stubSupplierRepo{},
	})

	err := ch.Validate(newCtx(), &chain.DrawInput{
		Source: chain.SourceNormal, UserID: 1, BlindBoxID: 10, Now: time.Now(),
	})
	if !errors.Is(err, chain.ErrBlindBoxNotAvailable) {
		t.Fatalf("err = %v, want chain.ErrBlindBoxNotAvailable", err)
	}
}

// =============================================================================
// 端到端：DrawChain 失败出口 3——余额不足
// =============================================================================

func TestDrawChain_RejectInsufficientBalance(t *testing.T) {
	users := &stubUserRepo{
		getByIDFunc: func(c *gin.Context, id int64) (*model.User, error) {
			return &model.User{ID: id, Balance: 10, Status: 1}, nil // 余额 10 不够 100
		},
	}
	bb := &stubBlindBoxRepo{
		getByIDFunc: func(c *gin.Context, id int64) (*mall.MallBlindBox, error) {
			return &mall.MallBlindBox{ID: id, SupplierID: 7, Status: mall.StatusActive, OnSale: true, Price: 100}, nil
		},
	}
	sup := &stubSupplierRepo{
		getByIDFunc: func(c *gin.Context, id int64) (*mall.MallSupplier, error) {
			return &mall.MallSupplier{ID: id, Status: mall.StatusActive}, nil
		},
	}
	ch := NewDrawCheckChain(CheckDeps{
		Users:     users,
		BlindBox:  bb,
		Promotion: &stubPromoRepo{},
		Seckill:   &stubSeckillRepo{},
		Supplier:  sup,
	})

	err := ch.Validate(newCtx(), &chain.DrawInput{
		Source: chain.SourceNormal, UserID: 1, BlindBoxID: 10, Now: time.Now(),
	})
	if !errors.Is(err, chain.ErrInsufficientBalance) {
		t.Fatalf("err = %v, want chain.ErrInsufficientBalance", err)
	}
}

// ====================================================================
// 端到端：DrawChain 短路——user 失败时后续节点不被触发
// ====================================================================

func TestDrawChain_FirstNodeFails_TerminatesImmediately(t *testing.T) {
	users := &stubUserRepo{
		getByIDFunc: func(c *gin.Context, id int64) (*model.User, error) {
			return nil, gorm.ErrRecordNotFound // user_active 失败
		},
	}
	bb := &stubBlindBoxRepo{
		getByIDFunc: func(c *gin.Context, id int64) (*mall.MallBlindBox, error) {
			t.Errorf("blindbox_buyable_check should not run after user_active fails")
			return nil, nil
		},
	}
	ch := NewDrawCheckChain(CheckDeps{
		Users:     users,
		BlindBox:  bb,
		Promotion: &stubPromoRepo{},
		Seckill:   &stubSeckillRepo{},
		Supplier:  &stubSupplierRepo{},
	})

	err := ch.Validate(newCtx(), &chain.DrawInput{
		Source: chain.SourceNormal, UserID: 1, BlindBoxID: 10, Now: time.Now(),
	})
	if !errors.Is(err, chain.ErrUserNotAvailable) {
		t.Fatalf("err = %v, want chain.ErrUserNotAvailable", err)
	}
}
