// Package captcha 点选验证码（click captcha）。
//
// 设计要点：
//   - 懒初始化：首次调用 GetManager() 时从 viper 读 captcha.yaml 完成初始化。
//   - 资源按需加载：每次 CreateClick 都从磁盘读资产，不缓存。
//   - 懒清理：每次使用（CreateClick / VerifyClick）触发一次 DeleteExpired(now)。
//   - sha1 摘要：正确答案序列以 sha1 hex 存入 Info，验证时直接哈希比对。
//   - 碰撞检测：放置元素时检查与已放置元素的包围盒是否重叠。
//
// 公开 API：
//   - GetManager()                                       懒初始化获取 *Manager
//   - (*Manager).CreateClick(ctx)                         生成一道点选验证码
//   - (*Manager).VerifyClick(ctx, req, deleteOnSuccess)   校验
//   - Reset()                                            清空单例（专供测试）
package captcha

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/golang/freetype"
	"github.com/golang/freetype/truetype"
	"github.com/google/uuid"

	"ai-go-mall/internal/infra/config"
	"ai-go-mall/internal/model"
	captchaRepo "ai-go-mall/internal/repository/captcha"
	"ai-go-mall/pkg/filesystem"
	"ai-go-mall/pkg/random"
)

// 元素类型常量（与 yaml 元素字段对应）。
const (
	ElementChinese      = "中文文字"
	ElementEnglishUpper = "英文大写字母"
	ElementIcon         = "ICON"
)

// 渲染常量。
const (
	bgWidth        = 320
	bgHeight       = 180
	textGlyphW     = 24
	textGlyphH     = 28
	iconDrawSize   = 44
	textFontSizePt = 24.0
	maxPlaceTries  = 200
)

// ============================================================
// 类型定义
// ============================================================

// placedRect 是元素在 canvas 上的矩形包围盒；仅在 compose 期间用于碰撞检测，不入库。
type placedRect struct {
	x, y, w, h int // 左上角 + 尺寸
}

// configSnapshot 是 Manager 持有的配置副本（与全局 config 解耦）。
type configSnapshot struct {
	Elements      []string
	Length        int
	NoiseLength   int
	TTLSeconds    int
	BackgroundDir string
	IconDir       string
	FontPath      string
}

// composeResult 是 composeImage 的输出。
type composeResult struct {
	Image       *image.RGBA
	AnswerOrder []string
	Width       int
	Height      int
}

// ClickCaptcha 是 CreateClick 返回给前端的点选验证码。
type ClickCaptcha struct {
	Key      string
	Elements []string
	Image    string
	Width    int
	Height   int
}

// VerifyRequest 是 VerifyClick 的入参；Answer 是用户按点击顺序提交的元素名。
type VerifyRequest struct {
	Key    string   `json:"key"`
	Answer []string `json:"answer"`
}

// Manager 是 captcha 业务的对外门面。
type Manager struct {
	repo captchaRepo.Repository
	cfg  configSnapshot
}

var (
	mgrMu sync.Mutex
	mgr   *Manager
)

// ============================================================
// 单例
// ============================================================

// GetManager 懒初始化获取 *Manager；同一进程重复调用只触发一次初始化。
func GetManager() (*Manager, error) {
	mgrMu.Lock()
	defer mgrMu.Unlock()
	if mgr != nil {
		return mgr, nil
	}

	snap, err := loadConfigSnapshot()
	if err != nil {
		return nil, err
	}
	repo := captchaRepo.New()
	if repo == nil {
		return nil, fmt.Errorf("%w: repository init failed", ErrInternal)
	}

	mgr = &Manager{repo: repo, cfg: snap}
	return mgr, nil
}

// Reset 清空 Manager 单例；专供测试。
func Reset() {
	mgrMu.Lock()
	defer mgrMu.Unlock()
	mgr = nil
}

func loadConfigSnapshot() (configSnapshot, error) {
	globalCfg := config.Get()
	if globalCfg == nil {
		return configSnapshot{}, fmt.Errorf("%w: config not initialized", ErrInternal)
	}
	cc := globalCfg.Captcha

	allowed := map[string]struct{}{
		ElementChinese:      {},
		ElementEnglishUpper: {},
		ElementIcon:         {},
	}
	for _, e := range cc.Elements {
		if _, ok := allowed[e]; !ok {
			return configSnapshot{}, fmt.Errorf("%w: %q", ErrInvalidElement, e)
		}
	}
	if cc.Length <= 0 || cc.NoiseLength <= 0 {
		return configSnapshot{}, fmt.Errorf("%w: 长度/混淆点长度 must be positive", ErrInvalidInput)
	}
	if cc.TTLSeconds <= 0 {
		return configSnapshot{}, fmt.Errorf("%w: 过期时间 must be positive", ErrInvalidInput)
	}
	if cc.BackgroundDir == "" || cc.IconDir == "" || cc.FontPath == "" {
		return configSnapshot{}, fmt.Errorf("%w: 资源路径不能为空", ErrInvalidInput)
	}

	return configSnapshot{
		Elements:      append([]string(nil), cc.Elements...),
		Length:        cc.Length,
		NoiseLength:   cc.NoiseLength,
		TTLSeconds:    cc.TTLSeconds,
		BackgroundDir: cc.BackgroundDir,
		IconDir:       cc.IconDir,
		FontPath:      cc.FontPath,
	}, nil
}

// ============================================================
// 公开 API
// ============================================================

// CreateClick 生成一道点选验证码。
func (m *Manager) CreateClick(ctx context.Context) (*ClickCaptcha, error) {
	m.maybeCleanup(ctx)

	bgs, icons, font, err := loadAssets(m.cfg)
	if err != nil {
		return nil, fmt.Errorf("%w: load assets: %v", ErrInternal, err)
	}

	rng := random.New()
	result, err := composeImage(bgs, icons, font, m.cfg, rng)
	if err != nil {
		return nil, err
	}

	pngBytes, err := encodePNG(result.Image)
	if err != nil {
		return nil, fmt.Errorf("%w: encode png: %v", ErrInternal, err)
	}
	imageBase64 := "data:image/png;base64," + base64.StdEncoding.EncodeToString(pngBytes)

	info := hashAnswer(result.AnswerOrder)

	key := uuid.NewString()
	cpt := &model.Captcha{
		Key:       key,
		Info:      &info,
		ExpiresAt: time.Now().Add(time.Duration(m.cfg.TTLSeconds) * time.Second),
	}
	if err := m.repo.Create(ctx, cpt); err != nil {
		return nil, fmt.Errorf("%w: persist captcha: %v", ErrInternal, err)
	}

	return &ClickCaptcha{
		Key:      key,
		Elements: result.AnswerOrder,
		Image:    imageBase64,
		Width:    result.Width,
		Height:   result.Height,
	}, nil
}

// VerifyClick 校验用户提交的答案序列；与库内 sha1 摘要比对。
func (m *Manager) VerifyClick(ctx context.Context, req *VerifyRequest, deleteOnSuccess bool) error {
	m.maybeCleanup(ctx)

	if req == nil || req.Key == "" {
		return fmt.Errorf("%w: empty key", ErrInvalidInput)
	}
	if len(req.Answer) != m.cfg.Length {
		return fmt.Errorf("%w: answer count %d, want %d", ErrMismatch, len(req.Answer), m.cfg.Length)
	}

	cpt, err := m.repo.GetByKey(ctx, req.Key)
	if err != nil {
		if errors.Is(err, captchaRepo.ErrNotFound) {
			return ErrNotFound
		}
		return fmt.Errorf("%w: %v", ErrInternal, err)
	}
	if time.Now().After(cpt.ExpiresAt) {
		return ErrExpired
	}
	if cpt.Info == nil || *cpt.Info != hashAnswer(req.Answer) {
		return fmt.Errorf("%w: answer mismatch", ErrMismatch)
	}

	if deleteOnSuccess {
		if err := m.repo.DeleteByKey(ctx, req.Key); err != nil {
			return fmt.Errorf("%w: delete after success: %v", ErrInternal, err)
		}
	}
	return nil
}

// maybeCleanup 每次使用触发一次懒清理。失败吞掉不外抛（清理失败不应阻断主流程）。
func (m *Manager) maybeCleanup(ctx context.Context) {
	cleanupCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_, _ = m.repo.DeleteExpired(cleanupCtx, time.Now())
}

// hashAnswer 计算答案序列的 sha1 hex 摘要。
func hashAnswer(answer []string) string {
	sum := sha1.Sum([]byte(strings.Join(answer, ",")))
	return hex.EncodeToString(sum[:])
}

// ============================================================
// 资源加载（按需，无缓存）
// ============================================================

func loadAssets(cfg configSnapshot) ([]image.Image, map[string]image.Image, *truetype.Font, error) {
	bgNames, err := filesystem.ListByExt(cfg.BackgroundDir, ".png")
	if err != nil {
		return nil, nil, nil, fmt.Errorf("background dir %q: %w", cfg.BackgroundDir, err)
	}
	bgs := make([]image.Image, 0, len(bgNames))
	for _, name := range bgNames {
		img, err := readPNG(filepath.Join(cfg.BackgroundDir, name))
		if err != nil {
			return nil, nil, nil, fmt.Errorf("read bg %s: %w", name, err)
		}
		bgs = append(bgs, img)
	}
	if len(bgs) == 0 {
		return nil, nil, nil, fmt.Errorf("no PNG in %q", cfg.BackgroundDir)
	}

	iconNames, err := filesystem.ListByExt(cfg.IconDir, ".png")
	if err != nil {
		return nil, nil, nil, fmt.Errorf("icon dir %q: %w", cfg.IconDir, err)
	}
	icons := make(map[string]image.Image, len(iconNames))
	for _, name := range iconNames {
		img, err := readPNG(filepath.Join(cfg.IconDir, name))
		if err != nil {
			return nil, nil, nil, fmt.Errorf("read icon %s: %w", name, err)
		}
		icons[strings.TrimSuffix(name, ".png")] = img
	}

	fontData, err := os.ReadFile(cfg.FontPath)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("read font %q: %w", cfg.FontPath, err)
	}
	font, err := truetype.Parse(fontData)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("parse font: %w", err)
	}
	return bgs, icons, font, nil
}

func readPNG(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return png.Decode(f)
}

// ============================================================
// 图像合成 + 碰撞检测
// ============================================================

func composeImage(bgs []image.Image, icons map[string]image.Image, font *truetype.Font, cfg configSnapshot, rng *rand.Rand) (*composeResult, error) {
	bg := bgs[rng.Intn(len(bgs))]
	canvas := image.NewRGBA(bg.Bounds())
	draw.Draw(canvas, canvas.Bounds(), bg, image.Point{}, draw.Src)

	correct, err := pickElements(cfg.Elements, icons, cfg.Length, rng)
	if err != nil {
		return nil, err
	}
	noise, err := pickElements(cfg.Elements, icons, cfg.NoiseLength, rng)
	if err != nil {
		return nil, err
	}

	var used []placedRect
	var answerOrder []string

	// 先放正确答案（顺序敏感）
	for _, name := range correct {
		rect, err := placeElement(canvas, name, icons, font, rng, used)
		if err != nil {
			return nil, fmt.Errorf("place answer %s: %w", name, err)
		}
		used = append(used, rect)
		answerOrder = append(answerOrder, name)
	}

	// 再放干扰元素（放不下跳过，不致命）
	for _, name := range noise {
		rect, err := placeElement(canvas, name, icons, font, rng, used)
		if err != nil {
			continue
		}
		used = append(used, rect)
	}

	return &composeResult{
		Image:       canvas,
		AnswerOrder: answerOrder,
		Width:       canvas.Bounds().Dx(),
		Height:      canvas.Bounds().Dy(),
	}, nil
}

// placeElement 尝试放置一个元素；最多 maxPlaceTries 次，失败返 ErrInternal。
func placeElement(canvas draw.Image, name string, icons map[string]image.Image, font *truetype.Font, rng *rand.Rand, used []placedRect) (placedRect, error) {
	w, h := elementSize(name, icons)
	for i := 0; i < maxPlaceTries; i++ {
		cx := rng.Intn(bgWidth)
		cy := rng.Intn(bgHeight)
		rect := placedRect{x: cx - w/2, y: cy - h/2, w: w, h: h}
		if rect.x < 0 || rect.y < 0 || rect.x+w > bgWidth || rect.y+h > bgHeight {
			continue
		}
		if checkCollision(rect, used) {
			continue
		}
		if err := drawElement(canvas, cx, cy, random.Color(rng), name, font, icons); err != nil {
			return placedRect{}, err
		}
		return rect, nil
	}
	return placedRect{}, fmt.Errorf("%w: place %s: no free spot after %d tries", ErrInternal, name, maxPlaceTries)
}

// checkCollision 标准 AABB 重叠检测。
func checkCollision(candidate placedRect, existing []placedRect) bool {
	for _, e := range existing {
		if !(candidate.x+candidate.w <= e.x ||
			candidate.x >= e.x+e.w ||
			candidate.y+candidate.h <= e.y ||
			candidate.y >= e.y+e.h) {
			return true
		}
	}
	return false
}

// elementSize 按元素类型返回绘制尺寸。
func elementSize(name string, icons map[string]image.Image) (w, h int) {
	if _, ok := icons[name]; ok {
		return iconDrawSize, iconDrawSize
	}
	return textGlyphW, textGlyphH
}

// drawElement 把单个元素绘制到 canvas 的 (cx, cy) 中心。
func drawElement(canvas draw.Image, cx, cy int, col color.RGBA, name string, font *truetype.Font, icons map[string]image.Image) error {
	if img, ok := icons[name]; ok {
		half := iconDrawSize / 2
		rect := image.Rect(cx-half, cy-half, cx+iconDrawSize-half, cy+iconDrawSize-half)
		draw.Draw(canvas, rect, img, image.Point{}, draw.Over)
		return nil
	}
	if font == nil {
		return fmt.Errorf("%w: font not loaded", ErrInternal)
	}
	c := freetype.NewContext()
	c.SetDPI(72)
	c.SetFont(font)
	c.SetFontSize(textFontSizePt)
	c.SetClip(canvas.Bounds())
	c.SetDst(canvas)
	c.SetSrc(image.NewUniform(col))
	pt := freetype.Pt(cx-textGlyphW/2, cy+textGlyphH/3)
	_, err := c.DrawString(name, pt)
	return err
}

// ============================================================
// 元素抽样
// ============================================================

// pickElements 从已启用的元素类型池里随机抽 count 个互不相同的元素。
func pickElements(enabled []string, icons map[string]image.Image, count int, rng *rand.Rand) ([]string, error) {
	if count <= 0 {
		return nil, fmt.Errorf("%w: count must be positive", ErrInvalidInput)
	}
	pool := expandEnabled(enabled, icons)
	if len(pool) == 0 {
		return nil, fmt.Errorf("%w: enabled elements expand to empty pool", ErrInvalidInput)
	}
	if count > len(pool) {
		return nil, fmt.Errorf("%w: need %d but pool has %d", ErrInvalidInput, count, len(pool))
	}
	shuffled := make([]string, len(pool))
	copy(shuffled, pool)
	rng.Shuffle(len(shuffled), func(i, j int) { shuffled[i], shuffled[j] = shuffled[j], shuffled[i] })
	return shuffled[:count], nil
}

func expandEnabled(enabled []string, icons map[string]image.Image) []string {
	var pool []string
	for _, e := range enabled {
		switch e {
		case ElementChinese:
			pool = append(pool, commonChineseChars...)
		case ElementEnglishUpper:
			for r := 'A'; r <= 'Z'; r++ {
				pool = append(pool, string(r))
			}
		case ElementIcon:
			pool = append(pool, mapKeys(icons)...)
		}
	}
	return pool
}

func mapKeys(m map[string]image.Image) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

var commonChineseChars = []string{
	"的", "一", "是", "不", "了", "人", "我", "在", "有", "他",
	"这", "为", "之", "大", "来", "以", "个", "中", "上", "们",
	"到", "说", "时", "要", "就", "出", "会", "也", "你", "对",
	"生", "能", "而", "子", "那", "得", "于", "着", "下", "自",
	"年", "过", "发", "后", "面",
}

// ============================================================
// 编码
// ============================================================

func encodePNG(img image.Image) ([]byte, error) {
	var buf bytes.Buffer
	if err := (&png.Encoder{}).Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
