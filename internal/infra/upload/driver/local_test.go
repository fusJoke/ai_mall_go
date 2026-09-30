package driver

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestLocalDriver_Save_CreatesParentDirs 验证 Save 会自动创建不存在的父目录。
func TestLocalDriver_Save_CreatesParentDirs(t *testing.T) {
	base := t.TempDir()
	d := NewLocalDriver(base, "/uploads")

	ctx := context.Background()
	content := bytes.NewReader([]byte("hello world"))
	if err := d.Save(ctx, content, "avatar/20260930/x.jpg"); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(base, "avatar", "20260930", "x.jpg"))
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if string(got) != "hello world" {
		t.Errorf("content = %q, want %q", got, "hello world")
	}
}

// TestLocalDriver_Delete_Idempotent 验证 Delete 对已存在/不存在文件都返回 nil。
func TestLocalDriver_Delete_Idempotent(t *testing.T) {
	base := t.TempDir()
	d := NewLocalDriver(base, "/uploads")
	ctx := context.Background()

	// 已存在
	if err := d.Save(ctx, bytes.NewReader([]byte("x")), "a.txt"); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := d.Delete(ctx, "a.txt"); err != nil {
		t.Errorf("Delete existing: %v", err)
	}
	// 不存在 → 幂等
	if err := d.Delete(ctx, "a.txt"); err != nil {
		t.Errorf("Delete missing (idempotent): %v", err)
	}
	if err := d.Delete(ctx, "never/existed.txt"); err != nil {
		t.Errorf("Delete never-existed: %v", err)
	}
}

// TestLocalDriver_Url_ReturnsPrefixedPath 验证 Url 拼接 urlPrefix + storedPath。
func TestLocalDriver_Url_ReturnsPrefixedPath(t *testing.T) {
	d := NewLocalDriver("/var/data/uploads", "/uploads")

	got := d.Url("avatar/20260930/x3a7f.jpg")
	want := "/uploads/avatar/20260930/x3a7f.jpg"
	if got != want {
		t.Errorf("Url = %q, want %q", got, want)
	}

	// 空 urlPrefix → 仅返回斜杠前缀
	d2 := NewLocalDriver("/var/data/uploads", "")
	got2 := d2.Url("a/b.jpg")
	if got2 != "/a/b.jpg" {
		t.Errorf("Url with empty prefix = %q, want %q", got2, "/a/b.jpg")
	}
}

// TestLocalDriver_Exists 区分 true / false。
func TestLocalDriver_Exists(t *testing.T) {
	base := t.TempDir()
	d := NewLocalDriver(base, "/uploads")
	ctx := context.Background()

	if d.Exists("missing.txt") {
		t.Error("Exists on missing file = true, want false")
	}
	if err := d.Save(ctx, bytes.NewReader([]byte("y")), "present.txt"); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if !d.Exists("present.txt") {
		t.Error("Exists on present file = false, want true")
	}
}

// TestLocalDriver_FullPath_JoinsBaseAndStoredPath 验证 FullPath 走 filepath.Join + Clean。
func TestLocalDriver_FullPath_JoinsBaseAndStoredPath(t *testing.T) {
	base := filepath.Clean("/var/data/uploads")
	d := NewLocalDriver(base, "/uploads")

	got := d.FullPath("avatar/20260930/x.jpg")
	want := filepath.Join(base, "avatar", "20260930", "x.jpg")
	if got != want {
		t.Errorf("FullPath = %q, want %q", got, want)
	}
}

// TestLocalDriver_Resolve_BlocksPathTraversal 验证路径穿越被拒绝。
func TestLocalDriver_Resolve_BlocksPathTraversal(t *testing.T) {
	d := NewLocalDriver("/var/data/uploads", "/uploads")

	// 显式 ..
	if err := d.Save(context.Background(), bytes.NewReader([]byte("x")), "../etc/passwd"); err == nil {
		t.Error("Save ../etc/passwd: expected error, got nil")
	}
	// 隐式 ../foo
	if err := d.Save(context.Background(), bytes.NewReader([]byte("x")), "foo/../../escape"); err == nil {
		t.Error("Save foo/../../escape: expected error, got nil")
	}
}

// TestNewLocalDriver_TrimsTrailingSlash 构造器清理 urlPrefix 末尾斜杠。
func TestNewLocalDriver_TrimsTrailingSlash(t *testing.T) {
	d := NewLocalDriver("/base", "/uploads/")
	got := d.Url("a.jpg")
	if !strings.HasPrefix(got, "/uploads/") || strings.Contains(got, "//") {
		t.Errorf("Url = %q, want no double slash", got)
	}
}
