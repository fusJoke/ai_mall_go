package token

import (
	"context"
	"errors"
	"testing"
	"time"

	"ai-go-mall/internal/infra/config"
	"ai-go-mall/internal/model"
)

// mockDriver 是 token.Driver 的最小可编程实现，专供 Manager.Check 测试。
//
// 字段通过测试侧直接赋值控制行为，避免依赖真实数据库。
type mockDriver struct {
	getFunc func(ctx context.Context, rawToken string) (*model.Token, error)
	// 其他方法在 Check 测试里用不到，给个 no-op 默认实现。
}

func (m *mockDriver) Create(ctx context.Context, t *model.Token) error { return nil }
func (m *mockDriver) Get(ctx context.Context, rawToken string) (*model.Token, error) {
	if m.getFunc != nil {
		return m.getFunc(ctx, rawToken)
	}
	return nil, ErrTokenNotFound
}
func (m *mockDriver) Delete(ctx context.Context, rawToken string) error { return nil }
func (m *mockDriver) Clear(ctx context.Context, userID int64, tokenType string) error {
	return nil
}

// TestManager_Check_Expired：mock 返回的记录 ExpiresAt 早于当前时间，Check 应返回 ErrTokenExpired。
func TestManager_Check_Expired(t *testing.T) {
	mgr := &Manager{driver: &mockDriver{
		getFunc: func(ctx context.Context, rawToken string) (*model.Token, error) {
			return &model.Token{
				Token:     "hashed",
				Type:      "admin",
				UserID:    1,
				ExpiresAt: time.Now().Add(-time.Minute), // 1 分钟前过期
			}, nil
		},
	}}

	tok, err := mgr.Check(context.Background(), "raw")
	if !errors.Is(err, ErrTokenExpired) {
		t.Fatalf("Check = %v, want ErrTokenExpired", err)
	}
	if tok != nil {
		t.Errorf("Check returned non-nil token on expired, got %+v", tok)
	}
}

// TestManager_Check_NotFound：mock 返回 ErrTokenNotFound，Check 应原样透传。
func TestManager_Check_NotFound(t *testing.T) {
	mgr := &Manager{driver: &mockDriver{
		getFunc: func(ctx context.Context, rawToken string) (*model.Token, error) {
			return nil, ErrTokenNotFound
		},
	}}

	tok, err := mgr.Check(context.Background(), "raw")
	if !errors.Is(err, ErrTokenNotFound) {
		t.Fatalf("Check = %v, want ErrTokenNotFound", err)
	}
	if tok != nil {
		t.Errorf("Check returned non-nil token on not-found, got %+v", tok)
	}
}

// TestManager_Check_EmptyInput：Check("") 应被前置拦截返回 ErrTokenInvalid，不会触达 driver。
func TestManager_Check_EmptyInput(t *testing.T) {
	called := false
	mgr := &Manager{driver: &mockDriver{
		getFunc: func(ctx context.Context, rawToken string) (*model.Token, error) {
			called = true
			return nil, nil
		},
	}}

	tok, err := mgr.Check(context.Background(), "")
	if !errors.Is(err, ErrTokenInvalid) {
		t.Fatalf("Check(\"\") = %v, want ErrTokenInvalid", err)
	}
	if called {
		t.Errorf("driver.Get should not be called on empty input")
	}
	if tok != nil {
		t.Errorf("Check returned non-nil token on invalid, got %+v", tok)
	}
}

// TestManager_Check_Valid：ExpiresAt 在未来，Check 应返回记录 + nil。
func TestManager_Check_Valid(t *testing.T) {
	want := &model.Token{
		Token:     "hashed",
		Type:      "admin",
		UserID:    1,
		ExpiresAt: time.Now().Add(time.Hour),
	}
	mgr := &Manager{driver: &mockDriver{
		getFunc: func(ctx context.Context, rawToken string) (*model.Token, error) {
			return want, nil
		},
	}}

	got, err := mgr.Check(context.Background(), "raw")
	if err != nil {
		t.Fatalf("Check = %v, want nil", err)
	}
	if got != want {
		t.Errorf("Check returned %+v, want %+v", got, want)
	}
}

// TestManager_ParamValidation：Create / Delete / Clear 的入参校验。
func TestManager_ParamValidation(t *testing.T) {
	mgr := &Manager{driver: &mockDriver{}}

	if err := mgr.Create(context.Background(), nil); !errors.Is(err, ErrTokenInvalid) {
		t.Errorf("Create(nil) = %v, want ErrTokenInvalid", err)
	}
	if err := mgr.Delete(context.Background(), ""); !errors.Is(err, ErrTokenInvalid) {
		t.Errorf("Delete(\"\") = %v, want ErrTokenInvalid", err)
	}
	if err := mgr.Clear(context.Background(), 0, "admin"); !errors.Is(err, ErrTokenInvalid) {
		t.Errorf("Clear(0, admin) = %v, want ErrTokenInvalid", err)
	}
	if err := mgr.Clear(context.Background(), 1, ""); !errors.Is(err, ErrTokenInvalid) {
		t.Errorf("Clear(1, \"\") = %v, want ErrTokenInvalid", err)
	}
	if _, err := mgr.Get(context.Background(), ""); !errors.Is(err, ErrTokenInvalid) {
		t.Errorf("Get(\"\") = %v, want ErrTokenInvalid", err)
	}
}

// TestInit_UnknownDriver：配置 driver=foo，Init 返回错误，mgr 不被设置。
func TestInit_UnknownDriver(t *testing.T) {
	Reset()
	t.Cleanup(Reset)

	cfg := config.Get()
	if cfg == nil {
		t.Skip("config not initialized; cannot run Init test")
	}
	cfg.Token.Driver = "foo"

	if err := Init(); err == nil {
		t.Fatal("Init() with unknown driver = nil error, want error")
	}
	if Get() != nil {
		t.Errorf("Get() should be nil after failed Init")
	}
}

// TestInit_DefaultsDriver：清空 Driver 字段后 Init，applyDefaults 应填回 "database"。
// 这里只验证 Init 能通过默认配置完成（database.Get() 在没初始化时返回 nil，但 NewDatabase(nil) 不报错）。
func TestInit_DefaultsDriver(t *testing.T) {
	Reset()
	t.Cleanup(Reset)

	cfg := config.Get()
	if cfg == nil {
		t.Skip("config not initialized; cannot run Init test")
	}
	cfg.Token.Driver = "" // 清空，让 Init 内部用默认值

	if err := Init(); err != nil {
		t.Fatalf("Init() with empty driver = %v, want nil (defaults to database)", err)
	}
	if Get() == nil {
		t.Errorf("Get() = nil after successful Init")
	}
}

// TestInit_Idempotent：重复 Init 不应重新实例化。
func TestInit_Idempotent(t *testing.T) {
	cfg := config.Get()
	if cfg == nil {
		t.Skip("config not initialized; cannot run Init test")
	}
	Reset()
	t.Cleanup(Reset)

	if err := Init(); err != nil {
		t.Fatalf("Init first: %v", err)
	}
	first := Get()
	if first == nil {
		t.Fatal("Get() = nil after first Init")
	}
	if err := Init(); err != nil {
		t.Fatalf("Init second: %v", err)
	}
	if Get() != first {
		t.Errorf("Init not idempotent: second Init replaced the manager")
	}
}

// TestReset：Reset 后 Get 返回 nil。
func TestReset(t *testing.T) {
	cfg := config.Get()
	if cfg == nil {
		t.Skip("config not initialized; cannot run Init test")
	}
	if err := Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	Reset()
	if Get() != nil {
		t.Errorf("Get() after Reset = %+v, want nil", Get())
	}
}
