package upload

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"ai-go-mall/internal/infra/config"
)

// mockDriver 记录 Save 调用次数与最后一次参数；其他 Driver 方法返回固定值。
type mockDriver struct {
	saveCount    int
	lastStored   string
	lastContent  []byte
	deleteCalled bool
}

func (m *mockDriver) Save(ctx context.Context, content io.Reader, storedPath string) error {
	m.saveCount++
	m.lastStored = storedPath
	buf := new(bytes.Buffer)
	if _, err := buf.ReadFrom(content); err != nil {
		return err
	}
	m.lastContent = buf.Bytes()
	return nil
}

func (m *mockDriver) Delete(ctx context.Context, storedPath string) error {
	m.deleteCalled = true
	return nil
}

func (m *mockDriver) Url(storedPath string) string {
	return "/uploads/" + storedPath
}

func (m *mockDriver) Exists(storedPath string) bool { return false }

func (m *mockDriver) FullPath(storedPath string) string { return storedPath }

// withUploadConfig 临时把 UploadConfig 注入到 config 单例，测试结束后恢复。
// 用 t.Cleanup 而非 defer，保证 helper 返回后注入的配置仍生效到测试函数结束。
func withUploadConfig(t *testing.T, cfg config.UploadConfig) {
	t.Helper()

	prev := config.Get()
	if prev == nil {
		prev = &config.Config{}
	}
	t.Cleanup(func() { config.SetForTest(prev) })

	full := *prev
	full.Upload = cfg
	config.SetForTest(&full)
}

func TestUpload_Success(t *testing.T) {
	withUploadConfig(t, config.UploadConfig{
		Driver:      "local",
		MaxSize:     10,
		MaxSizeUnit: "MB",
		Suffixes:    []string{"jpg", "png"},
		Format:      "/{topic}/{year}{mon}{day}/{fileName}{fileSha1}{.suffix}",
		Local:       config.UploadLocalConfig{BaseDir: "/tmp", URLPrefix: "/uploads"},
	})

	mock := &mockDriver{}
	svc := NewService(mock)

	body := bytes.Repeat([]byte("a"), 2*1024*1024) // 2 MB
	res, err := svc.Upload(&UploadInput{
		Topic:        "avatar",
		OriginalName: "My Photo.JPG",
		Reader:       bytes.NewReader(body),
	})
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if mock.saveCount != 1 {
		t.Errorf("Save count = %d, want 1", mock.saveCount)
	}
	if res.Size != int64(len(body)) {
		t.Errorf("Size = %d, want %d", res.Size, len(body))
	}
	if res.Suffix != "jpg" {
		t.Errorf("Suffix = %q, want %q", res.Suffix, "jpg")
	}
	if !strings.HasPrefix(res.StoredPath, "avatar/") {
		t.Errorf("StoredPath = %q, want prefix avatar/", res.StoredPath)
	}
	if res.Url != "/uploads/"+res.StoredPath {
		t.Errorf("Url = %q, want %q", res.Url, "/uploads/"+res.StoredPath)
	}
	// 校验 fileSha1 是 16 hex
	if len(res.StoredPath) < 16 {
		t.Fatal("storedPath too short")
	}
	// SHA1 hex 部分应为 16 字符（出现在路径中）
	parts := strings.Split(res.StoredPath, "/")
	filename := parts[len(parts)-1] // like "my photo<sha1>.jpg"
	dotIdx := strings.LastIndex(filename, ".")
	if dotIdx < 16 {
		t.Fatalf("filename %q too short to contain 16-hex sha1", filename)
	}
	sha1Part := filename[dotIdx-16 : dotIdx]
	for _, c := range sha1Part {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			t.Errorf("sha1Part %q contains non-hex char", sha1Part)
			break
		}
	}
}

func TestUpload_FileTooLarge(t *testing.T) {
	withUploadConfig(t, config.UploadConfig{
		Driver:      "local",
		MaxSize:     1,
		MaxSizeUnit: "MB",
		Suffixes:    []string{"jpg"},
		Format:      "/{topic}/{year}{mon}{day}/{fileName}{fileSha1}{.suffix}",
	})

	mock := &mockDriver{}
	svc := NewService(mock)

	body := bytes.Repeat([]byte("a"), 2*1024*1024) // 2 MB > 1 MB
	_, err := svc.Upload(&UploadInput{
		Topic:        "avatar",
		OriginalName: "big.jpg",
		Reader:       bytes.NewReader(body),
	})
	if !errors.Is(err, ErrFileTooLarge) {
		t.Errorf("err = %v, want ErrFileTooLarge", err)
	}
	if mock.saveCount != 0 {
		t.Errorf("Save should not be called, got %d", mock.saveCount)
	}
}

func TestUpload_InvalidSuffix(t *testing.T) {
	withUploadConfig(t, config.UploadConfig{
		Driver:      "local",
		MaxSize:     10,
		MaxSizeUnit: "MB",
		Suffixes:    []string{"jpg", "png"},
		Format:      "/{topic}/{year}{mon}{day}/{fileName}{fileSha1}{.suffix}",
	})

	mock := &mockDriver{}
	svc := NewService(mock)

	body := []byte("MZ")
	_, err := svc.Upload(&UploadInput{
		Topic:        "avatar",
		OriginalName: "malware.exe",
		Reader:       bytes.NewReader(body),
	})
	if !errors.Is(err, ErrInvalidSuffix) {
		t.Errorf("err = %v, want ErrInvalidSuffix", err)
	}
	if mock.saveCount != 0 {
		t.Errorf("Save should not be called, got %d", mock.saveCount)
	}
}

func TestUpload_InvalidInput(t *testing.T) {
	withUploadConfig(t, config.UploadConfig{
		Driver:  "local",
		MaxSize: 10, MaxSizeUnit: "MB",
		Suffixes: []string{"jpg"},
		Format:   "/{topic}/{year}{mon}{day}/{fileName}{fileSha1}{.suffix}",
	})
	mock := &mockDriver{}
	svc := NewService(mock)

	cases := []struct {
		name string
		in   *UploadInput
	}{
		{"nil input", nil},
		{"empty topic", &UploadInput{Topic: "", OriginalName: "a.jpg", Reader: bytes.NewReader([]byte("x"))}},
		{"empty original name", &UploadInput{Topic: "x", OriginalName: "", Reader: bytes.NewReader([]byte("x"))}},
		{"nil reader", &UploadInput{Topic: "x", OriginalName: "a.jpg", Reader: nil}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.Upload(tc.in)
			if !errors.Is(err, ErrInvalidInput) {
				t.Errorf("err = %v, want ErrInvalidInput", err)
			}
		})
	}
	if mock.saveCount != 0 {
		t.Errorf("Save should not be called, got %d", mock.saveCount)
	}
}

func TestGetSuffix(t *testing.T) {
	svc := NewService(&mockDriver{})

	cases := []struct {
		in, want string
	}{
		{"photo.JPG", "jpg"},
		{"a.tar.gz", "gz"},
		{"README", ""},
		{"file.PnG", "png"},
		{".hidden", "hidden"},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			got := svc.GetSuffix(tc.in)
			if got != tc.want {
				t.Errorf("GetSuffix(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestIsImage(t *testing.T) {
	svc := NewService(&mockDriver{})

	cases := []struct {
		in   string
		want bool
	}{
		{"a.png", true},
		{"a.JPG", true},
		{"a.svg", true},
		{"a.pdf", false},
		{"a.exe", false},
		{"README", false},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			got := svc.IsImage(tc.in)
			if got != tc.want {
				t.Errorf("IsImage(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestRenderFormat_AllPlaceholders(t *testing.T) {
	got := renderFormat(
		"/{topic}/{year}{mon}{day}/{fileName}{fileSha1}{.suffix}",
		formatVars{Topic: "avatar", Year: "2026", Mon: "09", Day: "30", FileName: "my photo", FileSha1: "3a7fbeadcafebabe", DotSuffix: ".jpg"},
	)
	want := "/avatar/20260930/my photo3a7fbeadcafebabe.jpg"
	if got != want {
		t.Errorf("renderFormat = %q, want %q", got, want)
	}
}

func TestRenderFormat_DuplicatePlaceholder(t *testing.T) {
	got := renderFormat("{year}-{year}", formatVars{Year: "2026"})
	if got != "2026-2026" {
		t.Errorf("got %q, want %q", got, "2026-2026")
	}
}
