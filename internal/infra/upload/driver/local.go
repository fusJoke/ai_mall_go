// Package driver 存放上传存储后端的具体实现。
//
// 文件名 = 驱动名：local.go / oss.go / s3.go / ...；
// 每个文件只放一个驱动的实现，由 internal/infra/upload 包的 newDriver() 工厂选用。
package driver

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// LocalDriver 是基于本地磁盘的上传驱动实现。
//
// baseDir 是磁盘存储根目录（绝对路径，或相对调用方 cwd 的相对路径）；
// urlPrefix 是对外暴露的 HTTP 路径前缀（不含协议 / 域名），Url() 用其拼外部地址。
//
// 所有 Driver 接口方法都对 storedPath 做 filepath.Clean 防御性处理，避免
// 业务侧传入 `../etc/passwd` 这类路径穿越。
type LocalDriver struct {
	baseDir   string
	urlPrefix string
}

// NewLocalDriver 构造一个 LocalDriver。
//
// baseDir 与 urlPrefix 由 UploadConfig.Local 提供，Init 时传入；
// 也可在测试中直接传临时目录构造。
func NewLocalDriver(baseDir, urlPrefix string) *LocalDriver {
	return &LocalDriver{
		baseDir:   filepath.Clean(baseDir),
		urlPrefix: strings.TrimRight(urlPrefix, "/"),
	}
}

// compile-time assertion: LocalDriver 必须实现 Driver 接口。
var _ Driver = (*LocalDriver)(nil)

// Save 把 content 写入 baseDir + storedPath；若父目录不存在则递归创建。
//
// storedPath 用相对路径传入（与渲染后的 format 输出一致），如 "avatar/20260930/x.jpg"；
// 实际落盘路径为 filepath.Join(baseDir, storedPath)。
func (d *LocalDriver) Save(ctx context.Context, content io.Reader, storedPath string) error {
	target, err := d.resolve(storedPath)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return fmt.Errorf("upload: mkdir: %w", err)
	}

	f, err := os.Create(target)
	if err != nil {
		return fmt.Errorf("upload: create: %w", err)
	}
	defer func() { _ = f.Close() }()

	if _, err := io.Copy(f, content); err != nil {
		_ = os.Remove(target) // 写入失败时清理半成品
		return fmt.Errorf("upload: copy: %w", err)
	}
	return nil
}

// Delete 删除 baseDir + storedPath。文件不存在时返回 nil（幂等）。
func (d *LocalDriver) Delete(ctx context.Context, storedPath string) error {
	target, err := d.resolve(storedPath)
	if err != nil {
		return err
	}
	if err := os.Remove(target); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("upload: remove: %w", err)
	}
	return nil
}

// Url 返回文件对外可访问的 HTTP 路径。
//
// 形如 "/uploads/avatar/20260930/x.jpg"，部署侧负责把该前缀反代到 baseDir。
func (d *LocalDriver) Url(storedPath string) string {
	clean := filepath.ToSlash(filepath.Clean(storedPath))
	clean = strings.TrimLeft(clean, "/")
	if d.urlPrefix == "" {
		return "/" + clean
	}
	return d.urlPrefix + "/" + clean
}

// Exists 判断 baseDir + storedPath 是否存在（任意类型：文件或目录）。
func (d *LocalDriver) Exists(storedPath string) bool {
	target, err := d.resolve(storedPath)
	if err != nil {
		return false
	}
	_, err = os.Stat(target)
	return err == nil
}

// FullPath 返回文件在磁盘上的绝对 / 完整路径。
//
// 仅 local 驱动能给出真正的文件系统路径；其他驱动（如 OSS）会返回对象 key。
func (d *LocalDriver) FullPath(storedPath string) string {
	target, err := d.resolve(storedPath)
	if err != nil {
		return ""
	}
	return target
}

// resolve 把 storedPath 拼到 baseDir 上并 Clean 防御性处理。
// 若路径穿越 baseDir（解析后不再以 baseDir 为前缀），返回 error。
func (d *LocalDriver) resolve(storedPath string) (string, error) {
	clean := filepath.Clean(storedPath)
	if clean == "." || strings.HasPrefix(clean, "..") {
		return "", fmt.Errorf("upload: invalid path %q", storedPath)
	}
	target := filepath.Join(d.baseDir, clean)
	// 再次校验 target 不跳出 baseDir（防御性；上面已挡 ..，这里兜底符号链接等场景）
	absBase, err := filepath.Abs(d.baseDir)
	if err == nil {
		absTarget, err := filepath.Abs(target)
		if err == nil && !strings.HasPrefix(absTarget, absBase+string(filepath.Separator)) && absTarget != absBase {
			return "", fmt.Errorf("upload: path escape %q", storedPath)
		}
	}
	return target, nil
}
