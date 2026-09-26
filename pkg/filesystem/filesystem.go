// Package filesystem 提供文件系统通用工具，不依赖具体业务。
package filesystem

import (
	"os"
	"sort"
	"strings"
)

// ListByExt 列出 dir 目录下指定扩展名的文件名（不含路径，按名字升序）。
// ext 需含 "."（如 ".png"），大小写不敏感。
func ListByExt(dir, ext string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	ext = strings.ToLower(ext)
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if strings.HasSuffix(strings.ToLower(e.Name()), ext) {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	return names, nil
}
