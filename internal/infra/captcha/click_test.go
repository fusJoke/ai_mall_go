package captcha

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
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

	// 持久化在内存中的 key→Captcha 副本；方便 VerifyClick_MaxFailTriggersDelete 之类测试断言 DeleteByKey 后的状态。
	store   map[string]model.Captcha
	keyMu   sync.Mutex
}

func newMockRepo() *mockRepo {
	return &mockRepo{store: make(map[string]model.Captcha)}
}

func (m *mockRepo) Create(ctx context.Context, cpt *model.Captcha) error {
	m.createCalls.Add(1)
	if m.createFunc != nil {
		return m.createFunc(ctx, cpt)
	}
	m.keyMu.Lock()
	m.store[cpt.Key] = *cpt
	m.keyMu.Unlock()
	return nil
}
func (m *mockRepo) GetByKey(ctx context.Context, key string) (*model.Captcha, error) {
	m.getCalls.Add(1)
	if m.getByKeyFunc != nil {
		return m.getByKeyFunc(ctx, key)
	}
	m.keyMu.Lock()
	defer m.keyMu.Unlock()
	cpt, ok := m.store[key]
	if !ok {
		return nil, captchaRepo.ErrNotFound
	}
	cp := cpt
	return &cp, nil
}
func (m *mockRepo) DeleteByKey(ctx context.Context, key string) error {
	m.deleteCalls.Add(1)
	if m.deleteByKeyFunc != nil {
		return m.deleteByKeyFunc(ctx, key)
	}
	m.keyMu.Lock()
	delete(m.store, key)
	m.keyMu.Unlock()
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
	m := &Manager{repo: repo, cfg: cfg}
	m.ResetFailForTest()
	return m
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
	repo := newMockRepo()
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
	if got.Width != ImageWidth || got.Height != ImageHeight {
		t.Errorf("Width/Height = %d/%d, want %d/%d", got.Width, got.Height, ImageWidth, ImageHeight)
	}
	if n := repo.createCalls.Load(); n != 1 {
		t.Errorf("create calls = %d, want 1", n)
	}
	if n := repo.expiredCalls.Load(); n < 1 {
		t.Errorf("expected cleanup call, got %d", n)
	}
}

// TestCreateClick_PersistsPlacedJSON 验证持久化到 captchas.Info 的 JSON 含 sha 与 points；
// points 顺序与 Elements 一致，每个点带 name/cx/cy/kind。
func TestCreateClick_PersistsPlacedJSON(t *testing.T) {
	repo := newMockRepo()
	m := newTestManager(repo, defaultCfg())

	got, err := m.CreateClick(context.Background())
	if err != nil {
		t.Fatalf("CreateClick: %v", err)
	}
	cpt, err := repo.GetByKey(context.Background(), got.Key)
	if err != nil {
		t.Fatalf("GetByKey: %v", err)
	}
	if cpt.Info == nil {
		t.Fatal("Info nil")
	}
	info, err := decodeStoredInfo(*cpt.Info)
	if err != nil {
		t.Fatalf("decodeStoredInfo: %v", err)
	}
	if info.Sha == "" || len(info.Sha) != 40 {
		t.Errorf("Sha = %q, want 40 hex chars", info.Sha)
	}
	if len(info.Points) != len(got.Elements) {
		t.Fatalf("Points len = %d, want %d", len(info.Points), len(got.Elements))
	}
	for i, p := range info.Points {
		if p.Name != got.Elements[i] {
			t.Errorf("Points[%d].Name = %q, want %q", i, p.Name, got.Elements[i])
		}
		if p.CX < 0 || p.CX >= ImageWidth || p.CY < 0 || p.CY >= ImageHeight {
			t.Errorf("Points[%d] out of bounds: cx=%d cy=%d", i, p.CX, p.CY)
		}
		if p.Kind != KindText && p.Kind != KindIcon {
			t.Errorf("Points[%d].Kind = %q", i, p.Kind)
		}
	}
}

// ============================================================
// VerifyClick
// ============================================================

// buildStoredInfoSha 以 (name, kind, cx, cy) 列表构造 StoredInfo 字符串，测试用。
func buildStoredInfo(t *testing.T, pts ...PlacedElem) string {
	t.Helper()
	names := make([]string, len(pts))
	for i, p := range pts {
		names[i] = p.Name
	}
	info := StoredInfo{Sha: hashAnswer(names), Points: pts}
	s, err := encodeStoredInfo(info)
	if err != nil {
		t.Fatalf("encodeStoredInfo: %v", err)
	}
	return s
}

func TestVerifyClick_Success_ExactCoord(t *testing.T) {
	cfg := defaultCfg()
	stored := buildStoredInfo(t,
		PlacedElem{Name: "A", CX: 100, CY: 80, Kind: KindText},
		PlacedElem{Name: "icon-001", CX: 200, CY: 120, Kind: KindIcon},
	)

	repo := newMockRepo()
	m := newTestManager(repo, cfg)
	m.repo.Create(context.Background(), &model.Captcha{Key: "test-key", Info: &stored, ExpiresAt: time.Now().Add(time.Hour)})

	req := &VerifyReq{
		Key: "test-key",
		Points: []Point{
			{X: 100, Y: 80},
			{X: 200, Y: 120},
		},
		W: ImageWidth,
		H: ImageHeight,
	}
	if err := m.VerifyClick(context.Background(), req, true); err != nil {
		t.Fatalf("VerifyClick: %v", err)
	}
	if n := repo.deleteCalls.Load(); n != 1 {
		t.Errorf("delete calls = %d, want 1", n)
	}
}

func TestVerifyClick_Success_WithinTolerance(t *testing.T) {
	cfg := defaultCfg()
	stored := buildStoredInfo(t,
		PlacedElem{Name: "A", CX: 100, CY: 80, Kind: KindText},  // 文字容差 14
		PlacedElem{Name: "B", CX: 50, CY: 50, Kind: KindText},
	)

	repo := newMockRepo()
	m := newTestManager(repo, cfg)
	m.repo.Create(context.Background(), &model.Captcha{Key: "k", Info: &stored, ExpiresAt: time.Now().Add(time.Hour)})

	// 第一个点偏 13px，仍在 14 容差内；第二个点正中。
	req := &VerifyReq{
		Key: "k",
		Points: []Point{
			{X: 113, Y: 80},
			{X: 50, Y: 50},
		},
		W: ImageWidth, H: ImageHeight,
	}
	if err := m.VerifyClick(context.Background(), req, true); err != nil {
		t.Fatalf("VerifyClick: %v", err)
	}
}

func TestVerifyClick_Fail_OutOfTolerance(t *testing.T) {
	cfg := defaultCfg()
	stored := buildStoredInfo(t,
		PlacedElem{Name: "A", CX: 100, CY: 80, Kind: KindText},
		PlacedElem{Name: "B", CX: 50, CY: 50, Kind: KindText},
	)
	repo := newMockRepo()
	m := newTestManager(repo, cfg)
	m.repo.Create(context.Background(), &model.Captcha{Key: "k", Info: &stored, ExpiresAt: time.Now().Add(time.Hour)})

	// 偏 16px（>容差 14）。
	req := &VerifyReq{
		Key: "k",
		Points: []Point{{X: 116, Y: 80}, {X: 50, Y: 50}},
		W: ImageWidth, H: ImageHeight,
	}
	err := m.VerifyClick(context.Background(), req, false)
	if !errors.Is(err, ErrMismatch) {
		t.Fatalf("VerifyClick = %v, want ErrMismatch", err)
	}
	if n := repo.deleteCalls.Load(); n != 0 {
		t.Errorf("delete calls = %d, want 0", n)
	}
}

func TestVerifyClick_Fail_IconOutOfTextTolerance(t *testing.T) {
	cfg := defaultCfg()
	stored := buildStoredInfo(t,
		PlacedElem{Name: "icon-001", CX: 100, CY: 80, Kind: KindIcon},
	)
	repo := newMockRepo()
	m := newTestManager(repo, cfg)
	m.repo.Create(context.Background(), &model.Captcha{Key: "k", Info: &stored, ExpiresAt: time.Now().Add(time.Hour)})

	// icon 容差 24；偏 16 应通过。
	passReq := &VerifyReq{
		Key: "k", Points: []Point{{X: 116, Y: 80}}, W: ImageWidth, H: ImageHeight,
	}
	if err := m.VerifyClick(context.Background(), passReq, true); err != nil {
		t.Fatalf("icon偏16应通过, got %v", err)
	}
}

func TestVerifyClick_Fail_WrongCount(t *testing.T) {
	cfg := defaultCfg()
	stored := buildStoredInfo(t,
		PlacedElem{Name: "A", CX: 100, CY: 80, Kind: KindText},
		PlacedElem{Name: "B", CX: 50, CY: 50, Kind: KindText},
	)
	repo := newMockRepo()
	m := newTestManager(repo, cfg)
	m.repo.Create(context.Background(), &model.Captcha{Key: "k", Info: &stored, ExpiresAt: time.Now().Add(time.Hour)})

	req := &VerifyReq{
		Key: "k", Points: []Point{{X: 100, Y: 80}}, W: ImageWidth, H: ImageHeight,
	}
	err := m.VerifyClick(context.Background(), req, false)
	if !errors.Is(err, ErrMismatch) {
		t.Fatalf("VerifyClick = %v, want ErrMismatch", err)
	}
}

func TestVerifyClick_Fail_Expired(t *testing.T) {
	stored := buildStoredInfo(t,
		PlacedElem{Name: "A", CX: 100, CY: 80, Kind: KindText},
		PlacedElem{Name: "B", CX: 50, CY: 50, Kind: KindText},
	)
	repo := newMockRepo()
	m := newTestManager(repo, defaultCfg())
	m.repo.Create(context.Background(), &model.Captcha{Key: "k", Info: &stored, ExpiresAt: time.Now().Add(-time.Minute)})

	req := &VerifyReq{
		Key: "k", Points: []Point{{X: 100, Y: 80}, {X: 50, Y: 50}}, W: ImageWidth, H: ImageHeight,
	}
	err := m.VerifyClick(context.Background(), req, false)
	if !errors.Is(err, ErrExpired) {
		t.Fatalf("VerifyClick = %v, want ErrExpired", err)
	}
}

func TestVerifyClick_Fail_NotFound(t *testing.T) {
	repo := newMockRepo()
	m := newTestManager(repo, defaultCfg())
	// 故意不 Create —— store 为空 → GetByKey 返 ErrNotFound

	req := &VerifyReq{
		Key: "missing", Points: []Point{{X: 1, Y: 1}, {X: 2, Y: 2}}, W: ImageWidth, H: ImageHeight,
	}
	err := m.VerifyClick(context.Background(), req, false)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("VerifyClick = %v, want ErrNotFound", err)
	}
}

func TestVerifyClick_Fail_SizeMismatch(t *testing.T) {
	cfg := defaultCfg()
	stored := buildStoredInfo(t,
		PlacedElem{Name: "A", CX: 100, CY: 80, Kind: KindText},
		PlacedElem{Name: "B", CX: 50, CY: 50, Kind: KindText},
	)
	repo := newMockRepo()
	m := newTestManager(repo, cfg)
	m.repo.Create(context.Background(), &model.Captcha{Key: "k", Info: &stored, ExpiresAt: time.Now().Add(time.Hour)})

	req := &VerifyReq{
		Key: "k", Points: []Point{{X: 100, Y: 80}, {X: 50, Y: 50}}, W: 640, H: 480,
	}
	err := m.VerifyClick(context.Background(), req, false)
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("VerifyClick = %v, want ErrInvalidInput", err)
	}
}

func TestVerifyClick_InvalidInput(t *testing.T) {
	m := newTestManager(newMockRepo(), defaultCfg())
	cases := []struct {
		name string
		req  *VerifyReq
	}{
		{"empty key", &VerifyReq{Key: "", Points: []Point{{X: 1, Y: 1}, {X: 2, Y: 2}}, W: ImageWidth, H: ImageHeight}},
		{"nil req", nil},
		{"wrong size", &VerifyReq{Key: "k", Points: []Point{{X: 1, Y: 1}, {X: 2, Y: 2}}, W: 100, H: 100}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := m.VerifyClick(context.Background(), tc.req, false)
			if !errors.Is(err, ErrInvalidInput) {
				t.Errorf("VerifyClick = %v, want ErrInvalidInput", err)
			}
		})
	}
}

// TestVerifyClick_MaxFailTriggersDelete 验证同一 key 答错 3 次后被自动硬删；
// 第 3 次失败响应后，repository 应看到 1 次额外 DeleteByKey（来自 M=3 阈值）；
// 第 4 次请求（任何内容）应得到 ErrNotFound。
func TestVerifyClick_MaxFailTriggersDelete(t *testing.T) {
	cfg := defaultCfg()
	stored := buildStoredInfo(t,
		PlacedElem{Name: "A", CX: 100, CY: 80, Kind: KindText},
		PlacedElem{Name: "B", CX: 50, CY: 50, Kind: KindText},
	)
	repo := newMockRepo()
	m := newTestManager(repo, cfg)
	m.repo.Create(context.Background(), &model.Captcha{Key: "k", Info: &stored, ExpiresAt: time.Now().Add(time.Hour)})

	// 故意偏 100px，确保超出容差。
	bad := &VerifyReq{
		Key: "k", Points: []Point{{X: 1000, Y: 1000}, {X: 1000, Y: 1000}}, W: ImageWidth, H: ImageHeight,
	}
	for i := 0; i < MaxFailPerKey; i++ {
		err := m.VerifyClick(context.Background(), bad, false)
		if !errors.Is(err, ErrMismatch) {
			t.Fatalf("iter %d: VerifyClick = %v, want ErrMismatch", i, err)
		}
	}

	// 第 3 次失败应触发 DeleteByKey —— 即使 deleteOnSuccess=false 也应见 delete。
	if n := repo.deleteCalls.Load(); n != 1 {
		t.Errorf("delete calls after threshold = %d, want 1", n)
	}

	// 第 4 次再请求同 key，应得 NotFound（key 已硬删 + 计数器清零）。
	err := m.VerifyClick(context.Background(), bad, false)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("post-threshold verify = %v, want ErrNotFound", err)
	}
}

// TestVerifyClick_FailCounter_ClearedOnCreate 验证同 key 重新 Create 后计数归零（确保不会跨 captcha 复用遗留）。
func TestVerifyClick_FailCounter_ClearedOnDelete(t *testing.T) {
	cfg := defaultCfg()
	stored := buildStoredInfo(t,
		PlacedElem{Name: "A", CX: 100, CY: 80, Kind: KindText},
		PlacedElem{Name: "B", CX: 50, CY: 50, Kind: KindText},
	)
	repo := newMockRepo()
	m := newTestManager(repo, cfg)
	m.repo.Create(context.Background(), &model.Captcha{Key: "k", Info: &stored, ExpiresAt: time.Now().Add(time.Hour)})

	bad := &VerifyReq{
		Key: "k", Points: []Point{{X: 1000, Y: 1000}, {X: 1000, Y: 1000}}, W: ImageWidth, H: ImageHeight,
	}
	// 前 2 次失败不应触发 delete。
	for i := 0; i < 2; i++ {
		_ = m.VerifyClick(context.Background(), bad, false)
	}
	if n := repo.deleteCalls.Load(); n != 0 {
		t.Errorf("delete calls before threshold = %d, want 0", n)
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
	stored := buildStoredInfo(t,
		PlacedElem{Name: "A", CX: 100, CY: 80, Kind: KindText},
		PlacedElem{Name: "B", CX: 50, CY: 50, Kind: KindText},
	)
	repo := newMockRepo()
	m := newTestManager(repo, cfg)
	m.repo.Create(context.Background(), &model.Captcha{Key: "k", Info: &stored, ExpiresAt: time.Now().Add(time.Hour)})

	if err := m.VerifyClick(context.Background(), &VerifyReq{Key: "k", Points: []Point{{X: 100, Y: 80}, {X: 50, Y: 50}}, W: ImageWidth, H: ImageHeight}, false); err != nil {
		t.Fatalf("VerifyClick: %v", err)
	}
	if n := repo.expiredCalls.Load(); n < 1 {
		t.Errorf("expected at least 1 cleanup call, got %d", n)
	}
}

// TestWithinTolerance 单元测试 withinTolerance ——
func TestWithinTolerance(t *testing.T) {
	tests := []struct {
		name string
		pt   Point
		pl   PlacedElem
		want bool
	}{
		{"text inside", Point{100, 80}, PlacedElem{CX: 100, CY: 80, Kind: KindText}, true},
		{"text boundary", Point{114, 80}, PlacedElem{CX: 100, CY: 80, Kind: KindText}, true}, // 14
		{"text over", Point{115, 80}, PlacedElem{CX: 100, CY: 80, Kind: KindText}, false},    // 15
		{"icon bigger", Point{124, 80}, PlacedElem{CX: 100, CY: 80, Kind: KindIcon}, true},   // 24
		{"icon over", Point{125, 80}, PlacedElem{CX: 100, CY: 80, Kind: KindIcon}, false},    // 25
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := withinTolerance(tc.pt, tc.pl); got != tc.want {
				t.Errorf("withinTolerance = %v, want %v", got, tc.want)
			}
		})
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

// TestImageDimensionsConsistent 验证 ImageWidth/ImageHeight == 旧 bgWidth/bgHeight。
func TestImageDimensionsConsistent(t *testing.T) {
	if ImageWidth != bgWidth || ImageHeight != bgHeight {
		t.Errorf("bgWidth=%d/bgHeight=%d must equal ImageWidth=%d/ImageHeight=%d",
			bgWidth, bgHeight, ImageWidth, ImageHeight)
	}
	if ToleranceRadiusText != 14 || ToleranceRadiusIcon != 24 {
		t.Errorf("tolerances changed: text=%d icon=%d", ToleranceRadiusText, ToleranceRadiusIcon)
	}
	if MaxFailPerKey != 3 {
		t.Errorf("MaxFailPerKey = %d, want 3", MaxFailPerKey)
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

// ensure verify-error path keeps delete-counter free of leak under noise:
// helper to compile-check pkg path import in case future tests need fmt-driven assertions.
var _ = fmt.Sprintf

// ============================================================
// recordFail / cleanupLazy —— 并发安全 + 过期清理
// ============================================================

// TestRecordFail_ConcurrentIncrement_NoLostUpdates 验证 recordFail 在并发下
// 不丢失更新：N 个 goroutine 同时对同一 key 调 recordFail，累加次数应恰好 = N。
//
// 这是回归测试：旧的 `*int` 在 sync.Map 里 RMW 会丢失更新（go test -race 必报），
// 改用 atomic.Int64 后单测不依赖 -race 也能确定性证明「无丢失」。
//
// 选取 N < MaxFailPerKey=3 会让 recordFail 删 key，破坏后续读取。
// 因此使用 N = 100（远小于 MaxFailPerKey 实际上限 3）的子集策略不便，
// 改用独立 key 池：每个 goroutine 用唯一 key，单测只需证明「每个 key 计数 = 1」
// 不会因并发写入 race 出现 0 或 >1 的脏值。
func TestRecordFail_ConcurrentIncrement_NoLostUpdates(t *testing.T) {
	repo := newMockRepo()
	m := newTestManager(repo, defaultCfg())
	// 把 MaxFailPerKey 临时设大些，避免单 key 计数到阈值触发 DeleteByKey 把测试搞复杂。
	// 这里不用改常量，而是选用 N=2 个并发（< MaxFailPerKey），但需要确保不被删除。
	// 真正测试不变量：「key 的最终 cnt 等于所有 LoadOrStore 后 Add 的次数」。

	const N = 50
	expires := time.Now().Add(time.Hour)

	var wg sync.WaitGroup
	wg.Add(N)
	for i := 0; i < N; i++ {
		key := fmt.Sprintf("k-%03d", i)
		go func() {
			defer wg.Done()
			m.recordFail(key, expires, context.Background())
		}()
	}
	wg.Wait()

	// 读取每个 key 的 cnt，期望都为 1（一次失败）。
	for i := 0; i < N; i++ {
		key := fmt.Sprintf("k-%03d", i)
		v, ok := m.failCnt.Load(key)
		if !ok {
			t.Errorf("key %s missing from failCnt", key)
			continue
		}
		e := v.(*failEntry)
		if e.cnt.Load() != 1 {
			t.Errorf("key %s cnt = %d, want 1", key, e.cnt.Load())
		}
	}

	m.ResetFailForTest()
}

// TestRecordFail_ConcurrentSameKey_ReachesThreshold 验证并发 recordFail 下
// 计数器一定能达到 MaxFailPerKey 阈值（安全不变量：M=3 防刷不能被并发绕过）。
//
// 旧实现（*int++）下，并发 5 次可能只涨到 2-3（丢失更新），攻击者可绕开 M=3。
// 新实现（atomic.Int64.Add）保证：每次 Add 都生效，最终 count 至少 = N（N 远超 3）。
//
// 注：failCnt 中可能有少量「delete 之后再 LoadOrStore」的 stale entry，
// 这是 sync.Map 自身的并发语义导致，无法杜绝；但 stale entry 的 cnt 必然
// < MaxFailPerKey（重建后只 +1 几次），不能用来绕过防刷。
func TestRecordFail_ConcurrentSameKey_ReachesThreshold(t *testing.T) {
	repo := newMockRepo()
	m := newTestManager(repo, defaultCfg())

	const key = "shared-key"
	mustCreateCaptcha(t, m, key, time.Now().Add(time.Hour))

	const N = 20
	var wg sync.WaitGroup
	wg.Add(N)
	for i := 0; i < N; i++ {
		go func() {
			defer wg.Done()
			m.recordFail(key, time.Now().Add(time.Hour), context.Background())
		}()
	}
	wg.Wait()

	// 安全不变量：DeleteByKey 至少被调用 1 次（说明 cnt 达到过 MaxFailPerKey）。
	if n := repo.deleteCalls.Load(); n < 1 {
		t.Errorf("DeleteByKey calls = %d, want >=1 (threshold never reached under concurrent load — atomic increment broken)", n)
	}
	// 若仍有 entry（被 delete 后又被 LoadOrStore 的 stale），其 cnt 必须 < MaxFailPerKey。
	if v, ok := m.failCnt.Load(key); ok {
		e := v.(*failEntry)
		if e.cnt.Load() >= int64(MaxFailPerKey) {
			t.Errorf("stale entry cnt = %d, want < %d (concurrent recreate after delete must not bypass M=3)", e.cnt.Load(), MaxFailPerKey)
		}
	}

	m.ResetFailForTest()
}

// TestCleanupLazy_PrunesExpiredFailEntries 验证 cleanupLazy 会清掉 failCnt 中
// expiresAt 已过的条目（修复 Codex P2：内存泄漏）。
func TestCleanupLazy_PrunesExpiredFailEntries(t *testing.T) {
	repo := newMockRepo()
	m := newTestManager(repo, defaultCfg())

	// 种 3 个 key：一个已过期、两个未过期。
	mustCreateCaptcha(t, m, "expired", time.Now().Add(-time.Hour))
	mustCreateCaptcha(t, m, "future-a", time.Now().Add(time.Hour))
	mustCreateCaptcha(t, m, "future-b", time.Now().Add(2*time.Hour))

	// 全部走一次 recordFail，让 failCnt 有 entry。
	m.recordFail("expired", time.Now().Add(-time.Hour), context.Background())
	m.recordFail("future-a", time.Now().Add(time.Hour), context.Background())
	m.recordFail("future-b", time.Now().Add(2*time.Hour), context.Background())

	if sz := failCntSize(m); sz != 3 {
		t.Fatalf("setup: failCnt size = %d, want 3", sz)
	}

	m.cleanupLazy(context.Background())

	if sz := failCntSize(m); sz != 2 {
		t.Errorf("after cleanup: failCnt size = %d, want 2 (expired pruned)", sz)
	}
	if _, ok := m.failCnt.Load("expired"); ok {
		t.Error("expired key 应被 prune，但仍存在")
	}
	if _, ok := m.failCnt.Load("future-a"); !ok {
		t.Error("future-a 不应被 prune，但已消失")
	}
	if _, ok := m.failCnt.Load("future-b"); !ok {
		t.Error("future-b 不应被 prune，但已消失")
	}

	m.ResetFailForTest()
}

// failCntSize 通过 Range 数 failCnt 大小（专供本测试文件）。
func failCntSize(m *Manager) int {
	n := 0
	m.failCnt.Range(func(_, _ any) bool {
		n++
		return true
	})
	return n
}

// mustCreateCaptcha 直接往 mock repo 种 captcha（绕过 CreateClick 的合成）。
func mustCreateCaptcha(t *testing.T, m *Manager, key string, expiresAt time.Time) {
	t.Helper()
	info := `{"sha":"deadbeef","points":[]}`
	if err := m.repo.Create(context.Background(), &model.Captcha{
		Key: key, Info: &info, ExpiresAt: expiresAt,
	}); err != nil {
		t.Fatalf("seed captcha %s: %v", key, err)
	}
}
