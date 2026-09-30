package admin

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"ai-go-mall/internal/model"
	adminSvc "ai-go-mall/internal/service/admin"
)

// mockInitService 是 InitService 的 mock，用于 handler 测试。
type mockInitService struct {
	resp *adminSvc.InitResponse
	err  error
}

func (m *mockInitService) Init(c *gin.Context, uid uint) (*adminSvc.InitResponse, error) {
	return m.resp, m.err
}

// TestInitHandler_Init 覆盖三条分支：
//   - 成功：service 返回响应 → 200
//   - service 返 ErrAccountDisabled → 403
//   - 其他错误 → 500
//
// 用 AdminFromContext 模拟 middleware 写入的 admin（注入到 gin context）。
func TestInitHandler_Init(t *testing.T) {
	cases := []struct {
		name       string
		setupCtx   func(c *gin.Context) // 注入 context 的 hook
		resp       *adminSvc.InitResponse
		svcErr     error
		wantStatus int
		wantCode   string
	}{
		{
			name: "success returns 200 with body",
			setupCtx: func(c *gin.Context) {
				// AdminFromContext 内部做 *model.Admin 类型断言 —— 必须注入真实类型
				c.Set("admin.current", &model.Admin{ID: 1, Status: 1})
			},
			resp: &adminSvc.InitResponse{
				Admin: adminSvc.InitAdmin{
					ID:       1,
					Username: "alice",
				},
				SiteConfig: map[string]string{"name": "Mall"},
				Menus:      []map[string]any{{"name": "dashboard"}},
			},
			wantStatus: http.StatusOK,
		},
		{
			name: "ErrAccountDisabled maps to 403",
			setupCtx: func(c *gin.Context) {
				c.Set("admin.current", &model.Admin{ID: 2, Status: 0})
			},
			svcErr:     adminSvc.ErrAccountDisabled,
			wantStatus: http.StatusForbidden,
			wantCode:   "admin.account_disabled",
		},
		{
			name: "internal error maps to 500 with fixed message",
			setupCtx: func(c *gin.Context) {
				c.Set("admin.current", &model.Admin{ID: 3, Status: 1})
			},
			svcErr:     errors.New("sql: connection refused (DB host=db.internal:3306)"),
			wantStatus: http.StatusInternalServerError,
			wantCode:   "admin.init.internal",
		},
		{
			name: "missing admin in context returns 403",
			setupCtx: func(c *gin.Context) {
				// 不写入 admin —— AdminFromContext 返 nil
			},
			wantStatus: http.StatusForbidden,
			wantCode:   "admin.account_disabled",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest("GET", "/admin/init", nil)
			tc.setupCtx(c)

			h := NewInitHandler(&mockInitService{resp: tc.resp, err: tc.svcErr})
			h.Init(c)

			if w.Code != tc.wantStatus {
				t.Errorf("status = %d, want %d", w.Code, tc.wantStatus)
			}
			if tc.wantCode != "" {
				body := w.Body.String()
				if !strings.Contains(body, tc.wantCode) {
					t.Errorf("body %q does not contain code %q", body, tc.wantCode)
				}
			}
			// 500 分支额外断言：不泄漏 err.Error() 细节
			if tc.wantStatus == http.StatusInternalServerError {
				if strings.Contains(w.Body.String(), "DB host=") {
					t.Errorf("body leaks driver detail: %s", w.Body.String())
				}
			}
		})
	}
}

// adminSvcMockAdmin 是占位 —— 上面的 setupCtx 已直接使用 *model.Admin，
// 该类型留空仅为向后兼容。
type adminSvcMockAdmin struct{ id int64 }
