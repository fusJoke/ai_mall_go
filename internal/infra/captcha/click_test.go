package captcha

import (
	"context"
	"encoding/base64"
	"errors"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"ai-go-mall/internal/infra/config"
	"ai-go-mall/internal/model"
	captchaRepo "ai-go-mall/internal/repository/captcha"
)

// mockRepo 是 captcharepo.Repository 的可编程 mock；调用计数用 atomic.Int32。
type mockRepo struct {
	createFunc        func(ctx context.Context, cpt *model.Captcha) error
	getByKeyFunc      func(ctx context.Context, key string) (*model.Captcha, error)
	deleteByKeyFunc   func(ctx context.Context, key string) error
	deleteExpiredFunc func(ctx context.Context, now time.Time) (int64, error)

	createCalls  atomic.Int32
	getCalls     atomic.Int32
	deleteCalls  atomic.Int32
	expiredCalls atomic.Int32
}

func (m *mockRepo) Create(ctx context.Context, cpt *model.Captcha) error {
	m.createCalls.Add(1)
	if m.createFunc != nil {
		return m.createFunc(ctx, cpt)
	}
	return nil
}
func (m *mockRepo) GetByKey(ctx context.Context, key string) (*model.Captcha, error) {
	m.getCalls.Add(1)
	if m.getByKeyFunc != nil {
		return m.getByKeyFunc(ctx, key)
	}
	return nil, ErrNotFound
}
func (m *mockRepo) DeleteByKey(ctx context.Context, key string) error {
	m.deleteCalls.Add(1)
	if m.deleteByKeyFunc != nil {
		return m.deleteByKeyFunc(ctx, key)
	}
	return nil
}
func (m *mockRepo) DeleteExpired(ctx context.Context, now time.Time) (int64, error) {
	m.expiredCalls.Add(1)
	if m.deleteExpiredFunc != nil {
		return m.deleteExpiredFunc(ctx, now)
	}
	return 0, nil
}

func newTestManager(repo *mockRepo, cfg configSnap) *Manager {
	return &Manager{repo: repo, cfg: cfg}
}

// defaultCfg 返回测试用默认快照；资源路径为项目根下的绝对路径。
func defaultCfg() configSnap {
	root := projectRoot()
	return configSnap{
		Elements:      []string{ElementENUpper, ElementIcon},
		Length:        2,
		NoiseLength:   2,
		TTLSeconds:    600,
		ChineseChars:  []string{"的", "一", "是"}, // 即便 Elements 未启用，也填非空，方便单独测试 chinese 路径
		BackgroundDir: filepath.Join(root, "asset", "captcha", "click", "background"),
		IconDir:       filepath.Join(root, "asset", "captcha", "click", "icon"),
		FontPath:      filepath.Join(root, "asset", "font", "SourceHanSansCN-Normal.ttf"),
	}
}

// projectRoot 通过 runtime.Caller(0) 计算本测试文件所在目录，再上溯 3 层到项目根。
func projectRoot() string {
	_, file, _, _ := runtime.Caller(0)
	// click_test.go 在 internal/infra/captcha/ 下 → 上溯 3 层
	return filepath.Join(filepath.Dir(file), "..", "..", "..")
}

// ============================================================
// CreateClick
// ============================================================

func TestCreateClick_ReturnsValidCaptcha(t *testing.T) {
	repo := &mockRepo{}
	m := newTestManager(repo, defaultCfg())

	got, err := m.CreateClick(context.Background())
	if err != nil {
		t.Fatalf("CreateClick: %v", err)
	}
	if got.Key == "" {
		t.Error("Key empty")
	}
	if len(got.Elements) != defaultCfg().Length {
		t.Errorf("Elements len = %d, want %d", len(got.Elements), defaultCfg().Length)
	}
	for _, e := range got.Elements {
		if !isSingleUpperLetter(e) && !isIconName(e) {
			t.Errorf("Elements contains invalid %q", e)
		}
	}
	if !strings.HasPrefix(got.Image, "data:image/png;base64,") {
		t.Errorf("Image missing data URL prefix")
	}
	rawB64 := strings.TrimPrefix(got.Image, "data:image/png;base64,")
	if _, err := base64.StdEncoding.DecodeString(rawB64); err != nil {
		t.Errorf("Image not valid base64: %v", err)
	}
	if got.Width <= 0 || got.Height <= 0 {
		t.Errorf("Width/Height = %d/%d, want positive", got.Width, got.Height)
	}
	if n := repo.createCalls.Load(); n != 1 {
		t.Errorf("create calls = %d, want 1", n)
	}
	if n := repo.expiredCalls.Load(); n < 1 {
		t.Errorf("expected cleanup call, got %d", n)
	}
}

// ============================================================
// VerifyClick
// ============================================================

func TestVerifyClick_Success_ExactOrder(t *testing.T) {
	cfg := defaultCfg()
	answer := []string{"A", "icon-001"}
	stored := hashAnswer(answer)

	repo := &mockRepo{
		getByKeyFunc: func(ctx context.Context, key string) (*model.Captcha, error) {
			info := stored
			return &model.Captcha{Key: key, Info: &info, ExpiresAt: time.Now().Add(time.Hour)}, nil
		},
	}
	m := newTestManager(repo, cfg)

	if err := m.VerifyClick(context.Background(), &VerifyReq{Key: "test-key", Answer: answer}, true); err != nil {
		t.Fatalf("VerifyClick: %v", err)
	}
	if n := repo.deleteCalls.Load(); n != 1 {
		t.Errorf("delete calls = %d, want 1", n)
	}
}

func TestVerifyClick_Fail_WrongOrder(t *testing.T) {
	cfg := defaultCfg()
	stored := hashAnswer([]string{"A", "B"})

	repo := &mockRepo{
		getByKeyFunc: func(ctx context.Context, key string) (*model.Captcha, error) {
			info := stored
			return &model.Captcha{Key: key, Info: &info, ExpiresAt: time.Now().Add(time.Hour)}, nil
		},
	}
	m := newTestManager(repo, cfg)

	err := m.VerifyClick(context.Background(), &VerifyReq{Key: "k", Answer: []string{"B", "A"}}, false)
	if !errors.Is(err, ErrMismatch) {
		t.Fatalf("VerifyClick = %v, want ErrMismatch", err)
	}
	if n := repo.deleteCalls.Load(); n != 0 {
		t.Errorf("delete calls = %d, want 0", n)
	}
}

func TestVerifyClick_Fail_WrongCount(t *testing.T) {
	cfg := defaultCfg()
	stored := hashAnswer([]string{"A", "B"})

	repo := &mockRepo{
		getByKeyFunc: func(ctx context.Context, key string) (*model.Captcha, error) {
			info := stored
			return &model.Captcha{Key: key, Info: &info, ExpiresAt: time.Now().Add(time.Hour)}, nil
		},
	}
	m := newTestManager(repo, cfg)

	err := m.VerifyClick(context.Background(), &VerifyReq{Key: "k", Answer: []string{"A"}}, false)
	if !errors.Is(err, ErrMismatch) {
		t.Fatalf("VerifyClick = %v, want ErrMismatch", err)
	}
}

func TestVerifyClick_Fail_Expired(t *testing.T) {
	repo := &mockRepo{
		getByKeyFunc: func(ctx context.Context, key string) (*model.Captcha, error) {
			info := hashAnswer([]string{"A", "B"})
			return &model.Captcha{Key: key, Info: &info, ExpiresAt: time.Now().Add(-time.Minute)}, nil
		},
	}
	m := newTestManager(repo, defaultCfg())

	err := m.VerifyClick(context.Background(), &VerifyReq{Key: "k", Answer: []string{"A", "B"}}, false)
	if !errors.Is(err, ErrExpired) {
		t.Fatalf("VerifyClick = %v, want ErrExpired", err)
	}
}

func TestVerifyClick_Fail_NotFound(t *testing.T) {
	repo := &mockRepo{
		getByKeyFunc: func(ctx context.Context, key string) (*model.Captcha, error) {
			return nil, captchaRepo.ErrNotFound
		},
	}
	m := newTestManager(repo, defaultCfg())

	err := m.VerifyClick(context.Background(), &VerifyReq{Key: "missing", Answer: []string{"A", "B"}}, false)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("VerifyClick = %v, want ErrNotFound", err)
	}
}

func TestVerifyClick_InvalidInput(t *testing.T) {
	m := newTestManager(&mockRepo{}, defaultCfg())
	cases := []struct {
		name string
		req  *VerifyReq
	}{
		{"empty key", &VerifyReq{Key: "", Answer: []string{"A", "B"}}},
		{"empty answer", &VerifyReq{Key: "k"}},
		{"wrong count", &VerifyReq{Key: "k", Answer: []string{"A"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := m.VerifyClick(context.Background(), tc.req, false)
			if !errors.Is(err, ErrInvalidInput) && !errors.Is(err, ErrMismatch) {
				t.Errorf("VerifyClick = %v, want ErrInvalidInput or ErrMismatch", err)
			}
		})
	}
}

// ============================================================
// Hash / Collision / Cleanup
// ============================================================

func TestHashAnswer_Stable(t *testing.T) {
	a := []string{"A", "icon-001", "B"}
	got1 := hashAnswer(a)
	got2 := hashAnswer(a)
	if got1 != got2 {
		t.Errorf("hashAnswer not stable: %q != %q", got1, got2)
	}
	if len(got1) != 40 { // sha1 hex = 40 chars
		t.Errorf("hashAnswer length = %d, want 40", len(got1))
	}
}

func TestCheckCollision_AABB(t *testing.T) {
	tests := []struct {
		name      string
		candidate placedRect
		existing  []placedRect
		want      bool
	}{
		{"no overlap, far apart", placedRect{x: 100, y: 100, w: 30, h: 30}, []placedRect{{x: 0, y: 0, w: 30, h: 30}}, false},
		{"overlap inside", placedRect{x: 10, y: 10, w: 30, h: 30}, []placedRect{{x: 20, y: 20, w: 30, h: 30}}, true},
		{"touch edges (no overlap)", placedRect{x: 30, y: 0, w: 30, h: 30}, []placedRect{{x: 0, y: 0, w: 30, h: 30}}, false},
		{"overlap by 1px", placedRect{x: 29, y: 0, w: 30, h: 30}, []placedRect{{x: 0, y: 0, w: 30, h: 30}}, true},
		{"empty existing", placedRect{x: 10, y: 10, w: 30, h: 30}, nil, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := checkCollision(tc.candidate, tc.existing); got != tc.want {
				t.Errorf("checkCollision = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestMaybeCleanup_RunsOnEachUse(t *testing.T) {
	cfg := defaultCfg()
	stored := hashAnswer([]string{"A", "B"})

	repo := &mockRepo{
		getByKeyFunc: func(ctx context.Context, key string) (*model.Captcha, error) {
			info := stored
			return &model.Captcha{Key: key, Info: &info, ExpiresAt: time.Now().Add(time.Hour)}, nil
		},
	}
	m := newTestManager(repo, cfg)

	if err := m.VerifyClick(context.Background(), &VerifyReq{Key: "k", Answer: []string{"A", "B"}}, false); err != nil {
		t.Fatalf("VerifyClick: %v", err)
	}
	if n := repo.expiredCalls.Load(); n < 1 {
		t.Errorf("expected at least 1 cleanup call, got %d", n)
	}
}

// ============================================================
// 配置加载
// ============================================================

func TestLoadConfigSnapshot_RequiresAllFields(t *testing.T) {
	Reset()
	t.Cleanup(Reset)

	cfg := &config.Config{
		Server:   config.ServerConfig{Name: "test", Port: 8080},
		Database: config.DatabaseConfig{Type: "mysql"},
		Token:    config.TokenConfig{Driver: "database"},
		Captcha: config.CaptchaConfig{
			Elements:    []string{ElementENUpper},
			Length:      0, // invalid
			NoiseLength: 0,
			TTLSeconds:  600,
		},
	}
	setConfigForTest(cfg)
	t.Cleanup(func() { setConfigForTest(nil) })

	_, err := loadConfigSnap()
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("loadConfigSnap = %v, want ErrInvalidInput", err)
	}
}

func TestLoadConfigSnapshot_ChineseRequiresCharSet(t *testing.T) {
	Reset()
	t.Cleanup(Reset)

	cfg := &config.Config{
		Server:   config.ServerConfig{Name: "test", Port: 8080},
		Database: config.DatabaseConfig{Type: "mysql"},
		Token:    config.TokenConfig{Driver: "database"},
		Captcha: config.CaptchaConfig{
			Elements:     []string{ElementCN}, // 启用中文
			Length:       2,
			NoiseLength:  2,
			TTLSeconds:   600,
			ChineseChars: nil, // 但未配字符集 → 应报错
		},
	}
	setConfigForTest(cfg)
	t.Cleanup(func() { setConfigForTest(nil) })

	_, err := loadConfigSnap()
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("loadConfigSnap = %v, want ErrInvalidInput", err)
	}
}

func setConfigForTest(c *config.Config) {
	if c == nil {
		config.Reset()
		return
	}
	config.SetForTest(c)
}

// ============================================================
// helpers
// ============================================================

func isSingleUpperLetter(s string) bool {
	if len(s) != 1 {
		return false
	}
	c := s[0]
	return c >= 'A' && c <= 'Z'
}

// isIconName 兼容 icon-NNN 或具名（apple.png → "apple"）。
func isIconName(s string) bool {
	if len(s) == 0 {
		return false
	}
	if len(s) > 5 && s[:5] == "icon-" {
		for _, c := range s[5:] {
			if c < '0' || c > '9' {
				return false
			}
		}
		return true
	}
	for _, c := range s {
		if !((c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' || c == '_') {
			return false
		}
	}
	return true
}

// errRepoNotFoundForTest 已删除：直接用 captchaRepo.ErrNotFound 即可。
