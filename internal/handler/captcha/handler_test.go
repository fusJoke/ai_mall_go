package captcha

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	captchaInfra "ai-go-mall/internal/infra/captcha"
	captchaSvc "ai-go-mall/internal/service/captcha"
)

// fakeService 是 captcha service 的可编程 mock。
type fakeService struct {
	createFunc  func(ctx context.Context) (*captchaSvc.ClickCaptcha, error)
	verifyFunc  func(ctx context.Context, req *captchaSvc.VerifyReq) error
	createCalls int
	verifyCalls int
}

func (f *fakeService) CreateClick(ctx context.Context) (*captchaSvc.ClickCaptcha, error) {
	f.createCalls++
	if f.createFunc != nil {
		return f.createFunc(ctx)
	}
	return &captchaSvc.ClickCaptcha{Key: "k", Elements: []string{"A"}, Image: "data:image/png;base64,xx", Width: 350, Height: 200}, nil
}

func (f *fakeService) VerifyClick(ctx context.Context, req *captchaSvc.VerifyReq) error {
	f.verifyCalls++
	if f.verifyFunc != nil {
		return f.verifyFunc(ctx, req)
	}
	return nil
}

func newTestRouter(svc captchaSvc.Service) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := NewHandler(svc)
	r.GET("/common/captcha/create", h.CreateClick)
	r.POST("/common/captcha/verify", h.VerifyClick)
	return r
}

func TestCreateClick_OK(t *testing.T) {
	svc := &fakeService{
		createFunc: func(ctx context.Context) (*captchaSvc.ClickCaptcha, error) {
			return &captchaSvc.ClickCaptcha{
				Key:      "abc-123",
				Elements: []string{"A", "B"},
				Image:    "data:image/png;base64,xx",
				Width:    350,
				Height:   200,
			}, nil
		},
	}
	r := newTestRouter(svc)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/common/captcha/create", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	var resp CreateClickResp
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Key != "abc-123" || len(resp.Elements) != 2 || resp.Width != 350 || resp.Height != 200 {
		t.Errorf("resp = %+v", resp)
	}
}

func TestCreateClick_InternalError(t *testing.T) {
	svc := &fakeService{
		createFunc: func(ctx context.Context) (*captchaSvc.ClickCaptcha, error) {
			return nil, captchaInfra.ErrInternal
		},
	}
	r := newTestRouter(svc)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/common/captcha/create", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500; body=%s", w.Code, w.Body.String())
	}
	var body map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["code"] != "captcha.internal" {
		t.Errorf("code = %q, want captcha.internal", body["code"])
	}
}

// TestVerifyClick_OK 验证成功路径：service.VerifyClick 不报错时返 200 {ok:true}。
func TestVerifyClick_OK(t *testing.T) {
	svc := &fakeService{}
	r := newTestRouter(svc)

	body := `{"key":"k","points":[{"x":100,"y":80},{"x":50,"y":50}],"w":350,"h":200}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/common/captcha/verify", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	if svc.verifyCalls != 1 {
		t.Errorf("verify calls = %d, want 1", svc.verifyCalls)
	}
}

func TestVerifyClick_Mismatch_401(t *testing.T) {
	svc := &fakeService{
		verifyFunc: func(ctx context.Context, req *captchaSvc.VerifyReq) error {
			return captchaInfra.ErrMismatch
		},
	}
	r := newTestRouter(svc)

	body := `{"key":"k","points":[{"x":1,"y":1},{"x":2,"y":2}],"w":350,"h":200}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/common/captcha/verify", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401; body=%s", w.Code, w.Body.String())
	}
	var body2 map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &body2); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body2["code"] != "captcha.mismatch" {
		t.Errorf("code = %q, want captcha.mismatch", body2["code"])
	}
}

func TestVerifyClick_NotFound_404(t *testing.T) {
	svc := &fakeService{
		verifyFunc: func(ctx context.Context, req *captchaSvc.VerifyReq) error {
			return captchaInfra.ErrNotFound
		},
	}
	r := newTestRouter(svc)

	body := `{"key":"k","points":[{"x":1,"y":1},{"x":2,"y":2}],"w":350,"h":200}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/common/captcha/verify", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body=%s", w.Code, w.Body.String())
	}
	var body2 map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &body2); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body2["code"] != "captcha.not_found" {
		t.Errorf("code = %q, want captcha.not_found", body2["code"])
	}
}

func TestVerifyClick_Expired_410(t *testing.T) {
	svc := &fakeService{
		verifyFunc: func(ctx context.Context, req *captchaSvc.VerifyReq) error {
			return captchaInfra.ErrExpired
		},
	}
	r := newTestRouter(svc)

	body := `{"key":"k","points":[{"x":1,"y":1},{"x":2,"y":2}],"w":350,"h":200}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/common/captcha/verify", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusGone {
		t.Fatalf("status = %d, want 410; body=%s", w.Code, w.Body.String())
	}
}

func TestVerifyClick_InvalidInput_400(t *testing.T) {
	svc := &fakeService{
		verifyFunc: func(ctx context.Context, req *captchaSvc.VerifyReq) error {
			return captchaInfra.ErrInvalidInput
		},
	}
	r := newTestRouter(svc)

	body := `{"key":"k","points":[{"x":1,"y":1},{"x":2,"y":2}],"w":100,"h":100}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/common/captcha/verify", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
	}
	var body2 map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &body2); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body2["code"] != "captcha.invalid_input" {
		t.Errorf("code = %q, want captcha.invalid_input", body2["code"])
	}
}

// TestVerifyClick_BindError_400 校验 JSON 解析失败（缺字段等）走 400 + invalid_input。
func TestVerifyClick_BindError_400(t *testing.T) {
	svc := &fakeService{}
	r := newTestRouter(svc)

	// 缺 key / points，触发 gin bind error。
	body := `{"w":350,"h":200}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/common/captcha/verify", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
	}
	if svc.verifyCalls != 0 {
		t.Errorf("verify calls = %d, want 0 (bind error should not call service)", svc.verifyCalls)
	}
}

// TestVerifyClick_PassesThroughReqFieldNames 验证 handler 透传字段名一致（key, points, w, h）。
func TestVerifyClick_PassesThroughReqFieldNames(t *testing.T) {
	var gotReq *captchaSvc.VerifyReq
	svc := &fakeService{
		verifyFunc: func(ctx context.Context, req *captchaSvc.VerifyReq) error {
			gotReq = req
			return nil
		},
	}
	r := newTestRouter(svc)

	body := `{"key":"abc","points":[{"x":120,"y":80},{"x":200,"y":120}],"w":350,"h":200}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/common/captcha/verify", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	if gotReq == nil {
		t.Fatal("service not called")
	}
	if gotReq.Key != "abc" || gotReq.W != 350 || gotReq.H != 200 {
		t.Errorf("req = %+v", gotReq)
	}
	if len(gotReq.Points) != 2 || gotReq.Points[0].X != 120 || gotReq.Points[0].Y != 80 {
		t.Errorf("points = %+v", gotReq.Points)
	}
}

// TestWriteError_FallsBackToInternal 验证非 sentinel error 走 500 + captcha.internal。
func TestWriteError_FallsBackToInternal(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	writeError(c, errors.New("random error"))
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", w.Code)
	}
}
