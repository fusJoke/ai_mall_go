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
	"encoding/json"
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
	"sync/atomic"
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
// CN / EN 取自 ISO 国别代码（i18n 惯例），比 Chi / Eng 更明确。
const (
	ElementCN      = "中文文字"
	ElementENUpper = "英文大写字母"
	ElementIcon    = "ICON"
)

// 渲染与精度常量。
const (
	// ImageWidth / ImageHeight 渲染 canvas 的固定尺寸；与 asset/captcha/click/background/*.png 一致。
	ImageWidth  = 350
	ImageHeight = 200

	// ToleranceRadiusText 文字类元素（CN/EN）的容差半径（像素）。
	ToleranceRadiusText = 14
	// ToleranceRadiusIcon ICON 类元素的容差半径（像素）。
	ToleranceRadiusIcon = 24

	// MaxFailPerKey 单个 key 累计允许的 verify 失败次数；超阈值立即硬删 key，前端必须重新 create。
	MaxFailPerKey = 3

	textGlyphW     = 24
	textGlyphH     = 28
	iconDrawSize   = 44
	textFontSizePt = 24.0
	maxPlaceTries  = 200
)

// bgWidth / bgHeight 是放置期的元素中心坐标随机范围，恒等于 ImageWidth / ImageHeight。
const (
	bgWidth  = ImageWidth
	bgHeight = ImageHeight
)

// ElemKind 元素类型，决定 verify 时的容差半径与 placeholderKind 字段。
const (
	KindText = "text"
	KindIcon = "icon"
)

// ============================================================
// 类型定义
// ============================================================

// placedRect 是元素在 canvas 上的矩形包围盒；仅在 compose 期间用于碰撞检测，不入库。
type placedRect struct {
	x, y, w, h int // 左上角 + 尺寸
}

// PlacedElem 是 composeImage 输出/持久化的「正确答案元素」结构。
// 与前端 VerifyReq.Points[i] 按下标顺序配对比对，按 Kind 选容差半径。
type PlacedElem struct {
	Name string `json:"name"`
	CX   int    `json:"cx"`
	CY   int    `json:"cy"`
	Kind string `json:"kind"`
}

// StoredInfo 是持久化到 captchas.Info 的 JSON 形态。
// Sha 是按 elements 顺序拼 join 后 sha1 hex（用于整体兜底/兼容性）；
// Points 是 PlacedElem 数组（用于精度比对）。
type StoredInfo struct {
	Sha    string       `json:"sha"`
	Points []PlacedElem `json:"points"`
}

// configSnap 是 Manager 持有的配置副本（与全局 config 解耦）。
type configSnap struct {
	Elements      []string
	Length        int
	NoiseLength   int
	TTLSeconds    int
	ChineseChars  []string
	BackgroundDir string
	IconDir       string
	FontPath      string
}

// composeResult 是 composeImage 的输出。
type composeResult struct {
	Image    *image.RGBA
	Placed   []PlacedElem
	Elements []string // 同 Placed[i].Name 顺序的元素名数组；保留用于 ClickCaptcha.Elements 响应字段
	Width    int
	Height   int
}

// ClickCaptcha 是 CreateClick 返回给前端的点选验证码。
type ClickCaptcha struct {
	Key      string
	Elements []string
	Image    string
	Width    int
	Height   int
}

// VerifyReq 是 VerifyClick 的入参；Points 是用户按点击顺序提交的原始像素坐标（与 ImageWidth/ImageHeight 同一坐标系）。
type VerifyReq struct {
	Key    string  `json:"key"`
	Points []Point `json:"points"`
	W      int     `json:"w"`
	H      int     `json:"h"`
}

// Point 是用户在图片坐标系下的点击坐标。
type Point struct {
	X int `json:"x"`
	Y int `json:"y"`
}

// failEntry 是单 key 的失败计数条目。
//
//   - cnt 是 atomic.Int64 指针：保证并发 recordFail 下的 RMW 不丢失更新
//     （`sync.Map` 只保护 map 自身读写，不保护指针指向的值）；
//   - expiresAt 是该 key 对应的 captcha 过期时间，让 cleanupLazy 能按过期
//     时间剔除计数项，避免泄漏（详见 cleanupLazy 的实现与设计 D4）。
type failEntry struct {
	cnt       *atomic.Int64
	expiresAt time.Time
}

// Manager 是 captcha 业务的对外门面。
type Manager struct {
	repo    captchaRepo.Repository
	cfg     configSnap
	failCnt sync.Map // map[string]*failEntry —— 单 key 失败计数；达到 MaxFailPerKey 即硬删
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

	snap, err := loadConfigSnap()
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

func loadConfigSnap() (configSnap, error) {
	globalCfg := config.Get()
	if globalCfg == nil {
		return configSnap{}, fmt.Errorf("%w: config not initialized", ErrInternal)
	}
	cc := globalCfg.Captcha

	allowed := map[string]struct{}{
		ElementCN:      {},
		ElementENUpper: {},
		ElementIcon:    {},
	}
	for _, e := range cc.Elements {
		if _, ok := allowed[e]; !ok {
			return configSnap{}, fmt.Errorf("%w: %q", ErrInvalidElem, e)
		}
	}
	if cc.Length <= 0 || cc.NoiseLength <= 0 {
		return configSnap{}, fmt.Errorf("%w: 长度/混淆点长度 must be positive", ErrInvalidInput)
	}
	if cc.TTLSeconds <= 0 {
		return configSnap{}, fmt.Errorf("%w: 过期时间 must be positive", ErrInvalidInput)
	}
	if cc.BackgroundDir == "" || cc.IconDir == "" || cc.FontPath == "" {
		return configSnap{}, fmt.Errorf("%w: 资源路径不能为空", ErrInvalidInput)
	}

	// 启用「中文文字」时强制要求显式配置字符集，不静默退到内置默认。
	hasChinese := false
	for _, e := range cc.Elements {
		if e == ElementCN {
			hasChinese = true
			break
		}
	}
	if hasChinese && len(cc.ChineseChars) == 0 {
		return configSnap{}, fmt.Errorf("%w: 启用「中文文字」时中文字符集必填", ErrInvalidInput)
	}

	return configSnap{
		Elements:      append([]string(nil), cc.Elements...),
		Length:        cc.Length,
		NoiseLength:   cc.NoiseLength,
		TTLSeconds:    cc.TTLSeconds,
		ChineseChars:  append([]string(nil), cc.ChineseChars...),
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
	m.cleanupLazy(ctx)

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

	elements := make([]string, len(result.Placed))
	for i, p := range result.Placed {
		elements[i] = p.Name
	}
	info := StoredInfo{
		Sha:    hashAnswer(elements),
		Points: result.Placed,
	}
	infoJSON, err := encodeStoredInfo(info)
	if err != nil {
		return nil, fmt.Errorf("%w: encode info: %v", ErrInternal, err)
	}

	key := uuid.NewString()
	cpt := &model.Captcha{
		Key:       key,
		Info:      &infoJSON,
		ExpiresAt: time.Now().Add(time.Duration(m.cfg.TTLSeconds) * time.Second),
	}
	if err := m.repo.Create(ctx, cpt); err != nil {
		return nil, fmt.Errorf("%w: persist captcha: %v", ErrInternal, err)
	}

	return &ClickCaptcha{
		Key:      key,
		Elements: elements,
		Image:    imageBase64,
		Width:    result.Width,
		Height:   result.Height,
	}, nil
}

// VerifyClick 按图片原始坐标精度比对：依次校验 W/H、Points 长度，再按 placed[i].Kind 选容差半径比欧氏距离。
// 成功按 deleteOnSuccess 决定是否立即硬删 key；任何失败会增加 failCnt，失败次数达 MaxFailPerKey 时硬删 key。
func (m *Manager) VerifyClick(ctx context.Context, req *VerifyReq, deleteOnSuccess bool) error {
	m.cleanupLazy(ctx)

	if req == nil || req.Key == "" {
		return fmt.Errorf("%w: empty key", ErrInvalidInput)
	}
	if req.W != ImageWidth || req.H != ImageHeight {
		return fmt.Errorf("%w: size mismatch (want %dx%d, got %dx%d)", ErrInvalidInput, ImageWidth, ImageHeight, req.W, req.H)
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
	if cpt.Info == nil {
		return fmt.Errorf("%w: empty info", ErrInternal)
	}
	info, err := decodeStoredInfo(*cpt.Info)
	if err != nil {
		return fmt.Errorf("%w: decode info: %v", ErrInternal, err)
	}
	if len(req.Points) != len(info.Points) {
		m.recordFail(req.Key, cpt.ExpiresAt, ctx)
		return fmt.Errorf("%w: points count %d, want %d", ErrMismatch, len(req.Points), len(info.Points))
	}
	for i, p := range req.Points {
		pl := info.Points[i]
		if !withinTolerance(p, pl) {
			m.recordFail(req.Key, cpt.ExpiresAt, ctx)
			return fmt.Errorf("%w: point %d out of tolerance", ErrMismatch, i)
		}
	}

	if deleteOnSuccess {
		if err := m.repo.DeleteByKey(ctx, req.Key); err != nil {
			return fmt.Errorf("%w: delete after success: %v", ErrInternal, err)
		}
		// 成功后也清空答错计数，避免 key 复用时残留（key 已经硬删，无残留风险但保险起见同步清理）
		m.failCnt.Delete(req.Key)
	}
	return nil
}

// recordFail 自增 key 的失败计数；累积达到 MaxFailPerKey 时立即 DeleteByKey 并清零计数。
//
// 并发安全：
//   - 计数本身用 atomic.Int64，LoadOrStore / Add 都是原子的；
//   - sync.Map 只保证 map 自身的并发读写安全，**不保护指针指向的值**，
//     因此指针所指计数器必须是 atomic 类型（或加 Mutex 保护）；
//   - 多个 goroutine 并发对同一 key 调 recordFail 时，每次 Add(1) 都会
//     被观察到，不会因丢失更新导致 counter 涨不到 MaxFailPerKey。
//
// 计数项携带 expiresAt，使 cleanupLazy 能按过期时间回收内存（详见该函数注释）。
// expiresAt 第一次遇到该 key 时确定，之后即使再次失败也不会变（captcha 不修改过期时间）。
func (m *Manager) recordFail(key string, expiresAt time.Time, ctx context.Context) {
	entry, _ := m.failCnt.LoadOrStore(key, &failEntry{cnt: new(atomic.Int64), expiresAt: expiresAt})
	e := entry.(*failEntry)
	cur := e.cnt.Add(1)
	if cur >= int64(MaxFailPerKey) {
		_ = m.repo.DeleteByKey(ctx, key)
		m.failCnt.Delete(key)
	}
}

// ResetFailForTest 清空失败计数；专供测试（避免不同测试间残留）。
func (m *Manager) ResetFailForTest() {
	m.failCnt.Range(func(k, _ any) bool {
		m.failCnt.Delete(k)
		return true
	})
}

// withinTolerance 比对用户点 p 与放置点 pl 的欧氏距离是否在 pl.Kind 对应的容差半径内。
func withinTolerance(p Point, pl PlacedElem) bool {
	radius := ToleranceRadiusText
	if pl.Kind == KindIcon {
		radius = ToleranceRadiusIcon
	}
	dx := p.X - pl.CX
	dy := p.Y - pl.CY
	return dx*dx+dy*dy <= radius*radius
}

// encodeStoredInfo 把 StoredInfo 序列化为紧凑 JSON 字符串；Points 按序输出便于调试。
func encodeStoredInfo(info StoredInfo) (string, error) {
	b, err := json.Marshal(info)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// decodeStoredInfo 反序列化 captchas.Info 字段。
func decodeStoredInfo(raw string) (StoredInfo, error) {
	var info StoredInfo
	if err := json.Unmarshal([]byte(raw), &info); err != nil {
		return StoredInfo{}, err
	}
	return info, nil
}

// cleanupLazy 每次使用触发一次懒清理。失败吞掉不外抛（清理失败不应阻断主流程）。
//
// 同时清理 DB（DeleteExpired）与内存答错计数 map（failCnt）。
// 计数项的 expiresAt 由 recordFail 在首次失败时写入，DB 过期清理时计数项
// 不再被任何代码路径引用——若不在这里同步剔除，会随时间无界增长，
// 违反设计 D4「过期清理会带走计数」的承诺。
func (m *Manager) cleanupLazy(ctx context.Context) {
	cleanupCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	now := time.Now()
	_, _ = m.repo.DeleteExpired(cleanupCtx, now)

	// pruneFailCnt 遍历 failCnt，剔除 expiresAt < now 的项。
	// 用 Range + Delete（不是 LoadAndDelete）是安全的：即便 prune 与并发
	// recordFail 撞车，最坏也只是「已删除的项又被 LoadOrStore 重建一次」
	// 或「未删除的项被下一次 prune 清掉」，两种情况都是幂等的。
	m.failCnt.Range(func(k, v any) bool {
		e, ok := v.(*failEntry)
		if !ok {
			m.failCnt.Delete(k)
			return true
		}
		if !e.expiresAt.After(now) {
			m.failCnt.Delete(k)
		}
		return true
	})
}

// hashAnswer 计算答案序列的 sha1 hex 摘要。
func hashAnswer(answer []string) string {
	sum := sha1.Sum([]byte(strings.Join(answer, ",")))
	return hex.EncodeToString(sum[:])
}

// ============================================================
// 资源加载（按需，无缓存）
// ============================================================

func loadAssets(cfg configSnap) ([]image.Image, map[string]image.Image, *truetype.Font, error) {
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

func composeImage(bgs []image.Image, icons map[string]image.Image, font *truetype.Font, cfg configSnap, rng *rand.Rand) (*composeResult, error) {
	bg := bgs[rng.Intn(len(bgs))]
	canvas := image.NewRGBA(bg.Bounds())
	draw.Draw(canvas, canvas.Bounds(), bg, image.Point{}, draw.Src)

	correct, err := pickElems(cfg.Elements, icons, cfg.ChineseChars, cfg.Length, nil, rng)
	if err != nil {
		return nil, err
	}
	// 从噪声池里排除已选中的正确答案名，避免出现两张视觉相同、坐标却只对其中一张有效的"陷阱图"
	// （Codex review 指出此类约 9% 概率出现，用户点击"对的那张"反而会被 withinTolerance 判为 Mismatch）。
	exclude := make(map[string]struct{}, len(correct))
	for _, n := range correct {
		exclude[n] = struct{}{}
	}
	noise, err := pickElems(cfg.Elements, icons, cfg.ChineseChars, cfg.NoiseLength, exclude, rng)
	if err != nil {
		return nil, err
	}

	var used []placedRect
	var placed []PlacedElem
	var elements []string

	// 先放正确答案（顺序敏感）
	for _, name := range correct {
		cx, cy, kind, rect, err := placeElem(canvas, name, icons, font, rng, used)
		if err != nil {
			return nil, fmt.Errorf("place answer %s: %w", name, err)
		}
		used = append(used, rect)
		placed = append(placed, PlacedElem{Name: name, CX: cx, CY: cy, Kind: kind})
		elements = append(elements, name)
	}

	// 再放干扰元素（放不下跳过，不致命）
	for _, name := range noise {
		_, _, _, rect, err := placeElem(canvas, name, icons, font, rng, used)
		if err != nil {
			continue
		}
		used = append(used, rect)
	}

	return &composeResult{
		Image:    canvas,
		Placed:   placed,
		Elements: elements,
		Width:    canvas.Bounds().Dx(),
		Height:   canvas.Bounds().Dy(),
	}, nil
}

// placeElem 尝试放置一个元素；最多 maxPlaceTries 次，失败返 ErrInternal。
// 返回 (cx, cy, kind, rect)：cx/cy 是元素在 canvas 上的中心坐标，kind 用于 verify 时选容差半径。
func placeElem(canvas draw.Image, name string, icons map[string]image.Image, font *truetype.Font, rng *rand.Rand, used []placedRect) (cx, cy int, kind string, rect placedRect, err error) {
	w, h := elemSize(name, icons)
	if _, ok := icons[name]; ok {
		kind = KindIcon
	} else {
		kind = KindText
	}
	for i := 0; i < maxPlaceTries; i++ {
		cx = rng.Intn(bgWidth)
		cy = rng.Intn(bgHeight)
		rect = placedRect{x: cx - w/2, y: cy - h/2, w: w, h: h}
		if rect.x < 0 || rect.y < 0 || rect.x+w > bgWidth || rect.y+h > bgHeight {
			continue
		}
		if checkCollision(rect, used) {
			continue
		}
		if err := drawElem(canvas, cx, cy, random.Color(rng), name, font, icons); err != nil {
			return 0, 0, "", placedRect{}, err
		}
		return cx, cy, kind, rect, nil
	}
	return 0, 0, "", placedRect{}, fmt.Errorf("%w: place %s: no free spot after %d tries", ErrInternal, name, maxPlaceTries)
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

// elemSize 按元素类型返回绘制尺寸。
func elemSize(name string, icons map[string]image.Image) (w, h int) {
	if _, ok := icons[name]; ok {
		return iconDrawSize, iconDrawSize
	}
	return textGlyphW, textGlyphH
}

// drawElem 把单个元素绘制到 canvas 的 (cx, cy) 中心。
func drawElem(canvas draw.Image, cx, cy int, col color.RGBA, name string, font *truetype.Font, icons map[string]image.Image) error {
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

// pickElems 从已启用的元素类型池里随机抽 count 个互不相同的元素。
func pickElems(enabled []string, icons map[string]image.Image, chineseChars []string, count int, exclude map[string]struct{}, rng *rand.Rand) ([]string, error) {
	if count <= 0 {
		return nil, fmt.Errorf("%w: count must be positive", ErrInvalidInput)
	}
	pool := expandEnabled(enabled, icons, chineseChars)
	if exclude != nil {
		filtered := pool[:0]
		for _, p := range pool {
			if _, skip := exclude[p]; skip {
				continue
			}
			filtered = append(filtered, p)
		}
		pool = filtered
	}
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

func expandEnabled(enabled []string, icons map[string]image.Image, chineseChars []string) []string {
	var pool []string
	for _, e := range enabled {
		switch e {
		case ElementCN:
			pool = append(pool, chineseChars...)
		case ElementENUpper:
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
