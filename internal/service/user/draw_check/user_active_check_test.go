package draw_check

import (
	"errors"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"ai-go-mall/internal/domain/chain"
	"ai-go-mall/internal/model"
	"ai-go-mall/internal/model/mall"
	"ai-go-mall/internal/repository"
	userRepo "ai-go-mall/internal/repository/user"
)

// =============================================================================
// helpers
// =============================================================================

func newCtx() *gin.Context {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/test", nil)
	return c
}

// stubUserRepo 是 userRepo.UserRepository 的最小 mock；只覆盖本 check 用到的 GetByID。
type stubUserRepo struct {
	getByIDFunc func(c *gin.Context, id int64) (*model.User, error)
}

func (s *stubUserRepo) GetByID(c *gin.Context, id int64) (*model.User, error) {
	if s.getByIDFunc != nil {
		return s.getByIDFunc(c, id)
	}
	return nil, gorm.ErrRecordNotFound
}

// 其他方法 stub return（userRepo.UserRepository 接口很大，本 check 只用 GetByID）。
func (s *stubUserRepo) GetByUsername(c *gin.Context, name string) (*model.User, error) {
	return nil, nil
}
func (s *stubUserRepo) Create(c *gin.Context, u *model.User) error { return nil }
func (s *stubUserRepo) Update(c *gin.Context, u *model.User) error { return nil }
func (s *stubUserRepo) Delete(c *gin.Context, id int64) error      { return nil }
func (s *stubUserRepo) List(c *gin.Context, opts repository.ListOptions) ([]model.User, int64, error) {
	return nil, 0, nil
}
func (s *stubUserRepo) Get(c *gin.Context, id int64) (*model.User, error) {
	return nil, nil
}
func (s *stubUserRepo) DecrementBalance(c *gin.Context, id int64, amount float64) error {
	return nil
}
func (s *stubUserRepo) IncrementBalance(c *gin.Context, id int64, amount float64) error {
	return nil
}
func (s *stubUserRepo) UpdateLoginSuccess(c *gin.Context, id int64, ip string, at time.Time) error {
	return nil
}
func (s *stubUserRepo) IncrementLoginFailure(c *gin.Context, id int64) (int, error) {
	return 0, nil
}
func (s *stubUserRepo) UpdateStatus(c *gin.Context, id int64, status int8) error { return nil }

var _ userRepo.UserRepository = (*stubUserRepo)(nil)

// =============================================================================
// 单测
// =============================================================================

func TestUserActiveCheck_Pass(t *testing.T) {
	users := &stubUserRepo{
		getByIDFunc: func(c *gin.Context, id int64) (*model.User, error) {
			return &model.User{ID: id, Status: 1, Balance: 100}, nil
		},
	}
	check := NewUserActiveCheck(users)

	err := check.Validate(newCtx(), &chain.DrawInput{
		Source: chain.SourceNormal, UserID: 42, BlindBoxID: 1, Now: time.Now(),
	})
	if err != nil {
		t.Fatalf("expected pass, got err = %v", err)
	}
}

func TestUserActiveCheck_RejectUserNotFound(t *testing.T) {
	users := &stubUserRepo{
		getByIDFunc: func(c *gin.Context, id int64) (*model.User, error) {
			return nil, gorm.ErrRecordNotFound
		},
	}
	check := NewUserActiveCheck(users)

	err := check.Validate(newCtx(), &chain.DrawInput{
		Source: chain.SourceNormal, UserID: 42, BlindBoxID: 1, Now: time.Now(),
	})
	if !errors.Is(err, chain.ErrUserNotAvailable) {
		t.Fatalf("err = %v, want chain.ErrUserNotAvailable", err)
	}
}

func TestUserActiveCheck_RejectUserDisabled(t *testing.T) {
	users := &stubUserRepo{
		getByIDFunc: func(c *gin.Context, id int64) (*model.User, error) {
			return &model.User{ID: id, Status: 0}, nil
		},
	}
	check := NewUserActiveCheck(users)

	err := check.Validate(newCtx(), &chain.DrawInput{
		Source: chain.SourceNormal, UserID: 42, BlindBoxID: 1, Now: time.Now(),
	})
	if !errors.Is(err, chain.ErrUserNotAvailable) {
		t.Fatalf("err = %v, want chain.ErrUserNotAvailable", err)
	}
}

func TestUserActiveCheck_RejectUserIDZero(t *testing.T) {
	users := &stubUserRepo{} // mock 不会被调用（防御编程错误：input.UserID<=0 → 直接 reject）
	check := NewUserActiveCheck(users)

	err := check.Validate(newCtx(), &chain.DrawInput{
		Source: chain.SourceNormal, UserID: 0, BlindBoxID: 1, Now: time.Now(),
	})
	if !errors.Is(err, chain.ErrUserNotAvailable) {
		t.Fatalf("err = %v, want chain.ErrUserNotAvailable", err)
	}
}

func TestUserActiveCheck_PropagatesDBError(t *testing.T) {
	dbErr := errors.New("db down")
	users := &stubUserRepo{
		getByIDFunc: func(c *gin.Context, id int64) (*model.User, error) {
			return nil, dbErr
		},
	}
	check := NewUserActiveCheck(users)

	err := check.Validate(newCtx(), &chain.DrawInput{
		Source: chain.SourceNormal, UserID: 42, BlindBoxID: 1, Now: time.Now(),
	})
	if !errors.Is(err, dbErr) {
		t.Fatalf("err = %v, want wrap dbErr", err)
	}
}

func TestUserActiveCheck_Name(t *testing.T) {
	check := NewUserActiveCheck(&stubUserRepo{})
	if check.Name() != "user_active" {
		t.Errorf("Name() = %q, want %q", check.Name(), "user_active")
	}
}

// 编译期断言：盲盒模型字段被引用（防止 import 被 unused 警告）。
var _ = mall.StatusActive
