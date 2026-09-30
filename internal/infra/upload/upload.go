// Package upload 提供文件上传能力。
//
// 设计：
//   - 底层支持多驱动（local / oss / s3 / ...），驱动放 driver/ 子目录；
//     每个驱动一个文件，文件名即驱动名。
//   - Service 持有 Driver，对外暴露统一的 Upload / GetSuffix / IsImage。
//   - 配置驱动位于 config.Get().Upload：driver / max_size / suffixes / format /
//     local.base_dir / local.url_prefix 全部走 yaml，无需改代码。
//
// 启动流程：cmd/serve/main.go 在 database.Init() 之后调用 upload.Init()，
// 内部读取 config.Get().Upload.Driver 决定加载哪个驱动。
package upload

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"path"
	"path/filepath"
	"strings"
	"time"

	"ai-go-mall/internal/infra/config"
	"ai-go-mall/internal/infra/upload/driver"
)

// Sentinel errors：调用方通过 errors.Is 区分业务分支。
var (
	// ErrInvalidInput 入参非法（nil / 空 topic / 空 original name 等）。
	ErrInvalidInput = errors.New("upload: invalid input")

	// ErrFileTooLarge 文件大小超过配置上限。
	ErrFileTooLarge = errors.New("upload: file too large")

	// ErrInvalidSuffix 文件后缀不在白名单。
	ErrInvalidSuffix = errors.New("upload: invalid suffix")

	// ErrDriverFailure 驱动层操作失败（包装底层 error 供 errors.Is 识别）。
	ErrDriverFailure = errors.New("upload: driver failure")
)

// Driver 是上传存储后端的统一接口（re-export from driver 子包）。
//
// 实际定义在 internal/infra/upload/driver，避免循环导入；
// 这里 var 别名让调用方统一用 upload.Driver 写起来更短。
type Driver = driver.Driver

// UploadInput 是 Service.Upload 的入参。
type UploadInput struct {
	// Topic 业务分类（用作存储路径的一级目录前缀）。
	// 必填。
	Topic string

	// OriginalName 原始文件名（含后缀，如 "My Photo.JPG"）。
	// 用于白名单校验与 fileName 占位符。
	// 必填。
	OriginalName string

	// Reader 文件内容读取器。Service 内部会用 LimitReader 限制读取量。
	// 必填。
	Reader io.Reader
}

// UploadResult 是 Service.Upload 的返回。
type UploadResult struct {
	// StoredPath 模板渲染后的相对路径（如 "avatar/20260930/x3a7f.jpg"）。
	StoredPath string
	// Url driver 拼出的对外地址（如 "/uploads/avatar/20260930/x3a7f.jpg"）。
	Url string
	// Size 实际写入字节数。
	Size int64
	// Suffix 小写后缀（不含前导点，如 "jpg"）。
	Suffix string
}

// Service 是上传业务的对外门面，持有 Driver + 配置。
type Service struct {
	driver Driver
	cfg    config.UploadConfig
}

// mgr 是 Init 缓存的 Service 单例。Init 未调用或失败时为 nil。
var mgr *Service

// NewService 构造一个 *Service。
//
// 该入口便于测试时注入 mock Driver；生产代码由 Init() 间接调用。
func NewService(d Driver) *Service {
	return &Service{
		driver: d,
		cfg:    config.Get().Upload,
	}
}

// Init 读取 config.Get().Upload，按配置实例化对应 driver，
// 包装成 Service 缓存到包级变量。
//
// 同一进程重复调用是 no-op；如需强制重载，调用 Reset 后再调用 Init。
func Init() error {
	if mgr != nil {
		return nil
	}

	cfg := config.Get().Upload
	d, err := newDriver(cfg.Driver)
	if err != nil {
		return err
	}

	mgr = NewService(d)
	return nil
}

// Get 返回已初始化的 *Service。Init 未调用过或失败时返回 nil。
func Get() *Service {
	return mgr
}

// Reset 清空 Service 缓存。专供测试使用。
func Reset() {
	mgr = nil
}

// newDriver 是 driver 工厂：根据 driver 名字返回对应实例。
// 后续新增 driver（oss / s3）只需在此追加 case 并新增 driver/<name>.go。
func newDriver(name string) (Driver, error) {
	cfg := config.Get().Upload
	switch name {
	case "local", "":
		return driver.NewLocalDriver(cfg.Local.BaseDir, cfg.Local.URLPrefix), nil
	default:
		return nil, fmt.Errorf("upload: unknown driver %q", name)
	}
}

// maxSizeBytes 把 max_size 与 max_size_unit 换算成字节数。
func (s *Service) maxSizeBytes() int64 {
	unit := strings.ToUpper(s.cfg.MaxSizeUnit)
	multiplier := int64(1)
	switch unit {
	case "B":
		multiplier = 1
	case "KB":
		multiplier = 1024
	case "MB":
		multiplier = 1024 * 1024
	case "GB":
		multiplier = 1024 * 1024 * 1024
	default:
		multiplier = 1024 * 1024 // 兜底 MB
	}
	return s.cfg.MaxSize * multiplier
}

// Upload 按 spec ADDED Requirement 3 的 8 步流程处理一次上传：
//
//  1. nil / 空字段校验 → ErrInvalidInput
//  2. 后缀提取 + 白名单校验 → ErrInvalidSuffix
//  3. LimitReader 读取至 max_size×unit，超过 → ErrFileTooLarge
//  4. SHA1 计算（取前 16 hex）
//  5. format 模板渲染
//  6. driver.Save 写盘（用 bytes.Reader 回放 buffer）
//  7. 组装 UploadResult 返回
func (s *Service) Upload(in *UploadInput) (*UploadResult, error) {
	if in == nil {
		return nil, ErrInvalidInput
	}
	if in.Topic == "" || in.OriginalName == "" || in.Reader == nil {
		return nil, ErrInvalidInput
	}

	suffix := s.GetSuffix(in.OriginalName)
	if !s.isSuffixAllowed(suffix) {
		return nil, ErrInvalidSuffix
	}

	// 3. 限流读取到 buffer（同时算 SHA1）
	limit := s.maxSizeBytes() + 1 // +1 让超限的那一字节也读出来，触发 io.ErrUnexpectedEOF
	buf := make([]byte, 0, 4096)
	hasher := sha1.New()
	reader := io.TeeReader(io.LimitReader(in.Reader, limit), hasher)

	chunk := make([]byte, 4096)
	total := int64(0)
	for {
		n, err := reader.Read(chunk)
		if n > 0 {
			buf = append(buf, chunk[:n]...)
			total += int64(n)
		}
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("upload: read: %w", err)
		}
		if total > s.maxSizeBytes() {
			return nil, ErrFileTooLarge
		}
	}
	if total > s.maxSizeBytes() {
		return nil, ErrFileTooLarge
	}

	// 4. SHA1 前 16 hex
	sum := hasher.Sum(nil)
	fileSha1 := hex.EncodeToString(sum)[:16]

	// 5. 模板渲染
	storedPath := renderFormat(s.cfg.Format, formatVars{
		Topic:     sanitizeTopic(in.Topic),
		Year:      fmt.Sprintf("%04d", time.Now().Year()),
		Mon:       fmt.Sprintf("%02d", int(time.Now().Month())),
		Day:       fmt.Sprintf("%02d", time.Now().Day()),
		FileName:  sanitizeFileName(stripExt(in.OriginalName)),
		FileSha1:  fileSha1,
		DotSuffix: "." + suffix,
	})
	// 去掉前导斜杠（spec storedPath 不带前导 /）
	storedPath = strings.TrimLeft(storedPath, "/")
	if storedPath == "" {
		return nil, ErrInvalidInput
	}

	// 6. 写盘（bytes.Reader 回放）
	if err := s.driver.Save(context.Background(), strings.NewReader(string(buf)), storedPath); err != nil {
		return nil, fmt.Errorf("%w: save: %s", ErrDriverFailure, err.Error())
	}

	return &UploadResult{
		StoredPath: storedPath,
		Url:        s.driver.Url(storedPath),
		Size:       total,
		Suffix:     suffix,
	}, nil
}

// GetSuffix 返回文件名的小写后缀（不含前导点）；无后缀返回空串。
func (s *Service) GetSuffix(filename string) string {
	ext := filepath.Ext(filename)
	return strings.ToLower(strings.TrimPrefix(ext, "."))
}

// IsImage 判断文件是否为常见图片格式。
//
// 识别集合：jpg / jpeg / png / gif / webp / bmp / svg / tiff（大小写不敏感）。
func (s *Service) IsImage(filename string) bool {
	switch s.GetSuffix(filename) {
	case "jpg", "jpeg", "png", "gif", "webp", "bmp", "svg", "tiff":
		return true
	default:
		return false
	}
}

// isSuffixAllowed 判断后缀是否在白名单（大小写不敏感；空后缀视为不允许）。
func (s *Service) isSuffixAllowed(suffix string) bool {
	if suffix == "" {
		return false
	}
	for _, allowed := range s.cfg.Suffixes {
		if strings.EqualFold(suffix, allowed) {
			return true
		}
	}
	return false
}

// formatVars 是模板渲染的变量集合。
type formatVars struct {
	Topic     string
	Year      string
	Mon       string
	Day       string
	FileName  string
	FileSha1  string
	DotSuffix string
}

// renderFormat 按顺序替换占位符。
//
// 占位符：{topic} {year} {mon} {day} {fileName} {fileSha1} {.suffix}
// 同一占位符可重复出现。重复出现也会被全部替换（strings.ReplaceAll 语义）。
func renderFormat(format string, v formatVars) string {
	out := format
	out = strings.ReplaceAll(out, "{topic}", v.Topic)
	out = strings.ReplaceAll(out, "{year}", v.Year)
	out = strings.ReplaceAll(out, "{mon}", v.Mon)
	out = strings.ReplaceAll(out, "{day}", v.Day)
	out = strings.ReplaceAll(out, "{fileName}", v.FileName)
	out = strings.ReplaceAll(out, "{fileSha1}", v.FileSha1)
	out = strings.ReplaceAll(out, "{.suffix}", v.DotSuffix)
	return out
}

// sanitizeTopic 去掉 topic 里的路径分隔符与连续斜杠，避免越过 baseDir。
//
// 使用 path.Clean（强制 / 分隔）而非 filepath.Clean（Windows 下为 \），
// 让存储路径在跨平台下都保持 web 风格的前向斜杠。
func sanitizeTopic(topic string) string {
	clean := strings.ReplaceAll(topic, "\\", "/")
	clean = path.Clean("/" + clean)
	clean = strings.TrimLeft(clean, "/")
	if clean == "." || clean == "/" {
		return "default"
	}
	return clean
}

// sanitizeFileName 去掉文件名中的非法字符（保留 ASCII 字母数字 + 空格 + _-. 与中文）。
// 空字符串退化为 "file"。
func sanitizeFileName(name string) string {
	const allowed = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789 _-."
	var b strings.Builder
	for _, r := range name {
		if r >= 0x4e00 && r <= 0x9fff {
			b.WriteRune(r)
			continue
		}
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
			continue
		}
		if strings.ContainsRune(allowed, r) {
			b.WriteRune(r)
			continue
		}
		b.WriteByte('_')
	}
	out := strings.Trim(b.String(), ".")
	if out == "" {
		return "file"
	}
	return out
}

// stripExt 去掉文件名后缀（用于 {fileName} 占位符，避免路径里出现 ".jpg.jpg"）。
func stripExt(name string) string {
	if idx := strings.LastIndex(name, "."); idx >= 0 {
		return name[:idx]
	}
	return name
}
