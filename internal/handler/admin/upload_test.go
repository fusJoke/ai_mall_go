package admin

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"ai-go-mall/internal/infra/upload"
)

// mockUploadService 记录最近一次 Upload 调用参数并返回预设 (result, err)。
type mockUploadService struct {
	result *upload.UploadResult
	err    error
	calls  int

	lastInput *upload.UploadInput
}

func (m *mockUploadService) Upload(in *upload.UploadInput) (*upload.UploadResult, error) {
	m.calls++
	m.lastInput = in
	return m.result, m.err
}

// buildMultipartRequest 构造一个含 file 字段的 multipart POST 请求。
//
// fileName 用于 form 中的 filename 字段（也是 OriginalName 传到 svc 的值）；
// fileContent 为文件字节。
func buildMultipartRequest(t *testing.T, target, fileName, fileContent string) *http.Request {
	t.Helper()

	body := &bytes.Buffer{}
	mw := multipart.NewWriter(body)
	fw, err := mw.CreateFormFile("file", fileName)
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := io.WriteString(fw, fileContent); err != nil {
		t.Fatalf("write form file: %v", err)
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, target, body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	return req
}

// TestUploadHandler_Upload 覆盖以下场景：
//   - 成功 → 200 + {code: 0, data.file.url = ...}
//   - 缺失 file 字段 → 400 upload.invalid_input
//   - ErrInvalidInput → 400 upload.invalid_input
//   - ErrInvalidSuffix → 400 upload.invalid_suffix
//   - ErrFileTooLarge → 413 upload.file_too_large
//   - ErrDriverFailure → 500 upload.internal
//   - 未知错误 → 500 upload.internal（不泄漏 err.Error()）
func TestUploadHandler_Upload(t *testing.T) {
	type bodyShape struct {
		Code    int            `json:"code"`
		Message string         `json:"message"`
		Data    uploadResponse `json:"data"`
	}

	cases := []struct {
		name          string
		fileName      string
		fileContent   string
		target        string // 含 query
		svcResult     *upload.UploadResult
		svcErr        error
		wantStatus    int
		wantCode      string // 字符串内容
		wantBodyHas   func(t *testing.T, body []byte)
		wantLastInput func(t *testing.T, in *upload.UploadInput)
	}{
		{
			name:        "success returns 200 with file payload",
			fileName:    "avatar.jpg",
			fileContent: "fake jpg bytes",
			target:      "/admin/ajax/upload?topic=avatar",
			svcResult: &upload.UploadResult{
				StoredPath: "avatar/20260930/avatar3a7f.jpg",
				Url:        "/uploads/avatar/20260930/avatar3a7f.jpg",
				Size:       14,
				Suffix:     "jpg",
			},
			wantStatus: http.StatusOK,
			wantBodyHas: func(t *testing.T, body []byte) {
				t.Helper()
				var got bodyShape
				if err := json.Unmarshal(body, &got); err != nil {
					t.Fatalf("unmarshal: %v; body=%s", err, string(body))
				}
				if got.Code != 0 {
					t.Errorf("code = %d, want 0", got.Code)
				}
				if got.Data.File.URL != "/uploads/avatar/20260930/avatar3a7f.jpg" {
					t.Errorf("file.url = %q", got.Data.File.URL)
				}
				if got.Data.File.StoredPath != "avatar/20260930/avatar3a7f.jpg" {
					t.Errorf("file.stored_path = %q", got.Data.File.StoredPath)
				}
				if got.Data.File.Suffix != "jpg" {
					t.Errorf("file.suffix = %q, want jpg", got.Data.File.Suffix)
				}
				if got.Data.File.Size != 14 {
					t.Errorf("file.size = %d, want 14", got.Data.File.Size)
				}
			},
			wantLastInput: func(t *testing.T, in *upload.UploadInput) {
				t.Helper()
				if in == nil {
					t.Fatal("svc.Upload not called")
				}
				if in.Topic != "avatar" {
					t.Errorf("Topic = %q, want avatar", in.Topic)
				}
				if in.OriginalName != "avatar.jpg" {
					t.Errorf("OriginalName = %q, want avatar.jpg", in.OriginalName)
				}
				if in.Reader == nil {
					t.Error("Reader is nil")
				}
			},
		},
		{
			name:       "missing file field returns 400 invalid_input",
			target:     "/admin/ajax/upload",
			svcResult:  nil,
			svcErr:     nil,
			wantStatus: http.StatusBadRequest,
			wantCode:   "upload.invalid_input",
		},
		{
			name:        "ErrInvalidInput maps to 400 invalid_input",
			fileName:    "x.jpg",
			fileContent: "x",
			target:      "/admin/ajax/upload",
			svcErr:      upload.ErrInvalidInput,
			wantStatus:  http.StatusBadRequest,
			wantCode:    "upload.invalid_input",
		},
		{
			name:        "ErrInvalidSuffix maps to 400 invalid_suffix",
			fileName:    "x.exe",
			fileContent: "x",
			target:      "/admin/ajax/upload",
			svcErr:      upload.ErrInvalidSuffix,
			wantStatus:  http.StatusBadRequest,
			wantCode:    "upload.invalid_suffix",
		},
		{
			name:        "ErrFileTooLarge maps to 413 file_too_large",
			fileName:    "big.jpg",
			fileContent: "x",
			target:      "/admin/ajax/upload",
			svcErr:      upload.ErrFileTooLarge,
			wantStatus:  http.StatusRequestEntityTooLarge,
			wantCode:    "upload.file_too_large",
		},
		{
			name:        "ErrDriverFailure maps to 500 internal with fixed message",
			fileName:    "x.jpg",
			fileContent: "x",
			target:      "/admin/ajax/upload",
			svcErr:      upload.ErrDriverFailure,
			wantStatus:  http.StatusInternalServerError,
			wantCode:    "upload.internal",
			wantBodyHas: func(t *testing.T, body []byte) {
				t.Helper()
				if strings.Contains(string(body), "driver failure") {
					t.Errorf("body leaks driver error detail: %s", string(body))
				}
			},
		},
		{
			name:        "unknown error maps to 500 internal",
			fileName:    "x.jpg",
			fileContent: "x",
			target:      "/admin/ajax/upload",
			svcErr:      errors.New("disk: ENOSPC on /var/lib/uploads"),
			wantStatus:  http.StatusInternalServerError,
			wantCode:    "upload.internal",
			wantBodyHas: func(t *testing.T, body []byte) {
				t.Helper()
				if strings.Contains(string(body), "ENOSPC") {
					t.Errorf("body leaks driver error detail: %s", string(body))
				}
			},
		},
		{
			name:        "topic defaults to admin when query missing",
			fileName:    "x.jpg",
			fileContent: "x",
			target:      "/admin/ajax/upload",
			svcResult: &upload.UploadResult{
				StoredPath: "admin/x.jpg",
				Url:        "/uploads/admin/x.jpg",
				Size:       1,
				Suffix:     "jpg",
			},
			wantStatus: http.StatusOK,
			wantLastInput: func(t *testing.T, in *upload.UploadInput) {
				t.Helper()
				if in.Topic != "admin" {
					t.Errorf("Topic = %q, want admin", in.Topic)
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)

			var req *http.Request
			if tc.fileName != "" {
				req = buildMultipartRequest(t, tc.target, tc.fileName, tc.fileContent)
			} else {
				// 缺失 file 字段：构造一个不带 file 的 multipart 也行，但
				// 直接用空 body + 普通 POST 更直观 —— gin 的 FormFile 会返错。
				req = httptest.NewRequest(http.MethodPost, tc.target, strings.NewReader(""))
				req.Header.Set("Content-Type", "multipart/form-data")
			}

			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = req

			mock := &mockUploadService{result: tc.svcResult, err: tc.svcErr}
			h := NewUploadHandler(mock)
			h.Upload(c)

			if w.Code != tc.wantStatus {
				t.Errorf("status = %d, want %d (body=%s)", w.Code, tc.wantStatus, w.Body.String())
			}
			if tc.wantCode != "" {
				if !strings.Contains(w.Body.String(), tc.wantCode) {
					t.Errorf("body %q does not contain code %q", w.Body.String(), tc.wantCode)
				}
			}
			if tc.wantBodyHas != nil {
				tc.wantBodyHas(t, w.Body.Bytes())
			}
			if tc.wantLastInput != nil {
				tc.wantLastInput(t, mock.lastInput)
			}
		})
	}
}

// TestUploadHandler_ReaderExhausted 验证 svc 拿到的是 io.Reader，能读完所有字节。
//
// 即使我们不消费 Reader 内容，构造时的 Reader 必须非 nil 且可读。
func TestUploadHandler_ReaderExhausted(t *testing.T) {
	gin.SetMode(gin.TestMode)

	const content = "hello world"
	req := buildMultipartRequest(t, "/admin/ajax/upload", "note.txt", content)

	mock := &mockUploadService{
		result: &upload.UploadResult{Url: "/uploads/note.txt", Suffix: "txt", Size: int64(len(content))},
	}
	h := NewUploadHandler(mock)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	h.Upload(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	if mock.calls != 1 {
		t.Fatalf("svc.Upload calls = %d, want 1", mock.calls)
	}
	if mock.lastInput == nil || mock.lastInput.Reader == nil {
		t.Fatal("svc.Upload received nil input/reader")
	}
	// Reader 必须能读出全部字节（验证 body 未被提前消费）
	all, err := io.ReadAll(mock.lastInput.Reader)
	if err != nil {
		t.Fatalf("read all from Reader: %v", err)
	}
	if string(all) != content {
		t.Errorf("reader content = %q, want %q", string(all), content)
	}
}
