package middleware

import (
	"context"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"

	"ai-go-mall/internal/infra/token"
	"ai-go-mall/internal/model"
	"ai-go-mall/internal/model/mall"
)

// =============================================================================
// helpers (扩展自 auth_test.go 的 withXxx 模式)
// =============================================================================

// withUserLookup 在测试期间替换 lookupUser；同 withAdminLookup 模式。
func withUserLookup(t *testing.T, fn func(c *gin.Context, uid uint) (*model.User, error)) {
	t.Helper()
	prev := lookupUser
	if fn == nil {
		lookupUser = prev
	} else {
		lookupUser = fn
	}
	t.Cleanup(func() { lookupUser = prev })
}

// withSupplierLookup 在测试期间替换 lookupSupplierUser；同 withAdminLookup 模式。
func withSupplierLookup(t *testing.T, fn func(c *gin.Context, uid uint) (*mall.MallSupplierUser, error)) {
	t.Helper()
	prev := lookupSupplierUser
	if fn == nil {
		lookupSupplierUser = prev
	} else {
		lookupSupplierUser = fn
	}
	t.Cleanup(func() { lookupSupplierUser = prev })
}

// newUserAuthEngine 拼一个 UserAuth 中间件 + 命中 handler 的最小 gin 引擎。
// 命中 handler 把 user_in_context / user_id / user_username 写进响应体便于测试。
func newUserAuthEngine() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/protected", UserAuth(), func(c *gin.Context) {
		u := UserFromContext(c)
		if u == nil {
			c.JSON(http.StatusOK, gin.H{"reached": true, "user_in_context": false})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"reached":         true,
			"user_in_context": true,
			"user_id":         u.ID,
			"user_username":   u.Username,
		})
	})
	return r
}

// newSupplierAuthEngine 拼一个 SupplierAuth 中间件 + 命中 handler 的最小 gin 引擎。
func newSupplierAuthEngine() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/protected", SupplierAuth(), func(c *gin.Context) {
		s := SupplierFromContext(c)
		if s == nil {
			c.JSON(http.StatusOK, gin.H{"reached": true, "supplier_in_context": false})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"reached":              true,
			"supplier_in_context":  true,
			"supplier_id":          s.ID,
			"supplier_username":    s.Username,
		})
	})
	return r
}

// =============================================================================
// UserAuth tests
// =============================================================================

// --- L1: 解析层（与 AdminAuth 共享同一套 Bearer scheme 解析逻辑）---

func TestUserAuth_MissingHeader(t *testing.T) {
	r := newUserAuthEngine()
	withCheckToken(t, func(context.Context, string) (*model.Token, error) {
		t.Fatalf("checkToken should NOT be called when Authorization is missing")
		return nil, nil
	})

	code, body := runReq(t, r, "GET", "/protected", "")
	if code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", code)
	}
	if body["code"] != "auth.missing_token" {
		t.Errorf("body.code = %v, want auth.missing_token", body["code"])
	}
}

func TestUserAuth_NonBearerScheme(t *testing.T) {
	r := newUserAuthEngine()
	withCheckToken(t, func(context.Context, string) (*model.Token, error) {
		t.Fatalf("checkToken should NOT be called for non-Bearer scheme")
		return nil, nil
	})

	code, _ := runReq(t, r, "GET", "/protected", "Token abc")
	if code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", code)
	}
}

// --- L2: 校验层（覆盖 checkToken 各分支）---

func TestUserAuth_ValidToken_PassesThrough(t *testing.T) {
	r := newUserAuthEngine()
	withCheckToken(t, func(ctx context.Context, rawToken string) (*model.Token, error) {
		return &model.Token{UserID: 1, Type: "user"}, nil
	})

	code, body := runReq(t, r, "GET", "/protected", "Bearer valid-token")
	if code != http.StatusOK {
		t.Errorf("status = %d, want 200", code)
	}
	if body["reached"] != true {
		t.Errorf("body.reached = %v, want true", body["reached"])
	}
}

func TestUserAuth_ExpiredToken_401(t *testing.T) {
	r := newUserAuthEngine()
	withCheckToken(t, func(context.Context, string) (*model.Token, error) {
		return nil, token.ErrTokenExpired
	})

	code, body := runReq(t, r, "GET", "/protected", "Bearer expired")
	if code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", code)
	}
	if body["code"] != "auth.token_expired" {
		t.Errorf("body.code = %v, want auth.token_expired", body["code"])
	}
}

func TestUserAuth_NotFoundToken_401(t *testing.T) {
	r := newUserAuthEngine()
	withCheckToken(t, func(context.Context, string) (*model.Token, error) {
		return nil, token.ErrTokenNotFound
	})

	code, body := runReq(t, r, "GET", "/protected", "Bearer ghost")
	if code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", code)
	}
	if body["code"] != "auth.token_not_found" {
		t.Errorf("body.code = %v, want auth.token_not_found", body["code"])
	}
}

func TestUserAuth_TokenManagerUnavailable_500(t *testing.T) {
	r := newUserAuthEngine()
	withCheckToken(t, func(context.Context, string) (*model.Token, error) {
		return nil, errTokenManagerUnavailable
	})

	code, body := runReq(t, r, "GET", "/protected", "Bearer any")
	if code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", code)
	}
	if body["code"] != "auth.internal" {
		t.Errorf("body.code = %v, want auth.internal", body["code"])
	}
}

// --- L3: token 类型校验（仅允许 type=user）---

func TestUserAuth_TokenTypeAdmin_Rejects(t *testing.T) {
	r := newUserAuthEngine()
	withCheckToken(t, func(context.Context, string) (*model.Token, error) {
		return &model.Token{UserID: 1, Type: "admin"}, nil // admin token → user 路由应被拒
	})

	code, body := runReq(t, r, "GET", "/protected", "Bearer admin-token")
	if code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", code)
	}
	if body["code"] != "auth.token_type_mismatch" {
		t.Errorf("body.code = %v, want auth.token_type_mismatch", body["code"])
	}
}

func TestUserAuth_TokenTypeSupplier_Rejects(t *testing.T) {
	r := newUserAuthEngine()
	withCheckToken(t, func(context.Context, string) (*model.Token, error) {
		return &model.Token{UserID: 1, Type: "supplier"}, nil
	})

	code, body := runReq(t, r, "GET", "/protected", "Bearer supplier-token")
	if code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", code)
	}
	if body["code"] != "auth.token_type_mismatch" {
		t.Errorf("body.code = %v, want auth.token_type_mismatch", body["code"])
	}
}

// --- L4: context 写入 / lookup 错误映射 ---

func TestUserAuth_SetsUserInContext_OnValidLookup(t *testing.T) {
	r := newUserAuthEngine()
	withCheckToken(t, func(context.Context, string) (*model.Token, error) {
		return &model.Token{UserID: 7, Type: "user"}, nil
	})
	withUserLookup(t, func(c *gin.Context, uid uint) (*model.User, error) {
		if uid != 7 {
			t.Errorf("lookupUser got uid = %d, want 7", uid)
		}
		return &model.User{ID: 7, Username: "alice"}, nil
	})

	code, body := runReq(t, r, "GET", "/protected", "Bearer valid")
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200", code)
	}
	if body["user_in_context"] != true {
		t.Errorf("user_in_context = %v, want true", body["user_in_context"])
	}
	if body["user_id"] != float64(7) {
		t.Errorf("user_id = %v, want 7", body["user_id"])
	}
	if body["user_username"] != "alice" {
		t.Errorf("user_username = %v, want alice", body["user_username"])
	}
}

func TestUserAuth_UserNotFound_PassesThroughNilContext(t *testing.T) {
	r := newUserAuthEngine()
	withCheckToken(t, func(context.Context, string) (*model.Token, error) {
		return &model.Token{UserID: 999, Type: "user"}, nil
	})
	withUserLookup(t, func(c *gin.Context, uid uint) (*model.User, error) {
		return nil, nil
	})

	code, body := runReq(t, r, "GET", "/protected", "Bearer valid")
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (token 合法，user 缺失不应 401)", code)
	}
	if body["user_in_context"] != false {
		t.Errorf("user_in_context = %v, want false", body["user_in_context"])
	}
}

func TestUserAuth_LookupError_500(t *testing.T) {
	r := newUserAuthEngine()
	withCheckToken(t, func(context.Context, string) (*model.Token, error) {
		return &model.Token{UserID: 1, Type: "user"}, nil
	})
	withUserLookup(t, func(c *gin.Context, uid uint) (*model.User, error) {
		return nil, stringError("db down")
	})

	code, body := runReq(t, r, "GET", "/protected", "Bearer valid")
	if code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", code)
	}
	if body["code"] != "auth.internal" {
		t.Errorf("body.code = %v, want auth.internal", body["code"])
	}
}

func TestUserAuth_InvalidToken_NoLookup(t *testing.T) {
	r := newUserAuthEngine()
	withCheckToken(t, func(context.Context, string) (*model.Token, error) {
		return &model.Token{UserID: 1, Type: "admin"}, nil // 类型不符
	})
	withUserLookup(t, func(c *gin.Context, uid uint) (*model.User, error) {
		t.Fatalf("lookupUser should NOT be called when token type mismatches")
		return nil, nil
	})

	code, _ := runReq(t, r, "GET", "/protected", "Bearer admin-token")
	if code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", code)
	}
}

// =============================================================================
// SupplierAuth tests (与 UserAuth 测试结构同构；只换 token type / context key)
// =============================================================================

func TestSupplierAuth_MissingHeader(t *testing.T) {
	r := newSupplierAuthEngine()
	withCheckToken(t, func(context.Context, string) (*model.Token, error) {
		t.Fatalf("checkToken should NOT be called when Authorization is missing")
		return nil, nil
	})

	code, body := runReq(t, r, "GET", "/protected", "")
	if code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", code)
	}
	if body["code"] != "auth.missing_token" {
		t.Errorf("body.code = %v, want auth.missing_token", body["code"])
	}
}

func TestSupplierAuth_ValidToken_PassesThrough(t *testing.T) {
	r := newSupplierAuthEngine()
	withCheckToken(t, func(context.Context, string) (*model.Token, error) {
		return &model.Token{UserID: 1, Type: "supplier"}, nil
	})

	code, body := runReq(t, r, "GET", "/protected", "Bearer valid")
	if code != http.StatusOK {
		t.Errorf("status = %d, want 200", code)
	}
	if body["reached"] != true {
		t.Errorf("body.reached = %v, want true", body["reached"])
	}
}

func TestSupplierAuth_ExpiredToken_401(t *testing.T) {
	r := newSupplierAuthEngine()
	withCheckToken(t, func(context.Context, string) (*model.Token, error) {
		return nil, token.ErrTokenExpired
	})

	code, body := runReq(t, r, "GET", "/protected", "Bearer expired")
	if code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", code)
	}
	if body["code"] != "auth.token_expired" {
		t.Errorf("body.code = %v, want auth.token_expired", body["code"])
	}
}

func TestSupplierAuth_TokenTypeAdmin_Rejects(t *testing.T) {
	r := newSupplierAuthEngine()
	withCheckToken(t, func(context.Context, string) (*model.Token, error) {
		return &model.Token{UserID: 1, Type: "admin"}, nil
	})

	code, body := runReq(t, r, "GET", "/protected", "Bearer admin-token")
	if code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", code)
	}
	if body["code"] != "auth.token_type_mismatch" {
		t.Errorf("body.code = %v, want auth.token_type_mismatch", body["code"])
	}
}

func TestSupplierAuth_TokenTypeUser_Rejects(t *testing.T) {
	r := newSupplierAuthEngine()
	withCheckToken(t, func(context.Context, string) (*model.Token, error) {
		return &model.Token{UserID: 1, Type: "user"}, nil
	})

	code, body := runReq(t, r, "GET", "/protected", "Bearer user-token")
	if code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", code)
	}
	if body["code"] != "auth.token_type_mismatch" {
		t.Errorf("body.code = %v, want auth.token_type_mismatch", body["code"])
	}
}

func TestSupplierAuth_SetsSupplierInContext_OnValidLookup(t *testing.T) {
	r := newSupplierAuthEngine()
	withCheckToken(t, func(context.Context, string) (*model.Token, error) {
		return &model.Token{UserID: 9, Type: "supplier"}, nil
	})
	withSupplierLookup(t, func(c *gin.Context, uid uint) (*mall.MallSupplierUser, error) {
		if uid != 9 {
			t.Errorf("lookupSupplierUser got uid = %d, want 9", uid)
		}
		return &mall.MallSupplierUser{ID: 9, Username: "panini"}, nil
	})

	code, body := runReq(t, r, "GET", "/protected", "Bearer valid")
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200", code)
	}
	if body["supplier_in_context"] != true {
		t.Errorf("supplier_in_context = %v, want true", body["supplier_in_context"])
	}
	if body["supplier_id"] != float64(9) {
		t.Errorf("supplier_id = %v, want 9", body["supplier_id"])
	}
	if body["supplier_username"] != "panini" {
		t.Errorf("supplier_username = %v, want panini", body["supplier_username"])
	}
}

func TestSupplierAuth_SupplierNotFound_PassesThroughNilContext(t *testing.T) {
	r := newSupplierAuthEngine()
	withCheckToken(t, func(context.Context, string) (*model.Token, error) {
		return &model.Token{UserID: 999, Type: "supplier"}, nil
	})
	withSupplierLookup(t, func(c *gin.Context, uid uint) (*mall.MallSupplierUser, error) {
		return nil, nil
	})

	code, body := runReq(t, r, "GET", "/protected", "Bearer valid")
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200", code)
	}
	if body["supplier_in_context"] != false {
		t.Errorf("supplier_in_context = %v, want false", body["supplier_in_context"])
	}
}

func TestSupplierAuth_LookupError_500(t *testing.T) {
	r := newSupplierAuthEngine()
	withCheckToken(t, func(context.Context, string) (*model.Token, error) {
		return &model.Token{UserID: 1, Type: "supplier"}, nil
	})
	withSupplierLookup(t, func(c *gin.Context, uid uint) (*mall.MallSupplierUser, error) {
		return nil, stringError("db down")
	})

	code, body := runReq(t, r, "GET", "/protected", "Bearer valid")
	if code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", code)
	}
	if body["code"] != "auth.internal" {
		t.Errorf("body.code = %v, want auth.internal", body["code"])
	}
}

func TestSupplierAuth_InvalidToken_NoLookup(t *testing.T) {
	r := newSupplierAuthEngine()
	withCheckToken(t, func(context.Context, string) (*model.Token, error) {
		return &model.Token{UserID: 1, Type: "admin"}, nil
	})
	withSupplierLookup(t, func(c *gin.Context, uid uint) (*mall.MallSupplierUser, error) {
		t.Fatalf("lookupSupplierUser should NOT be called when token type mismatches")
		return nil, nil
	})

	code, _ := runReq(t, r, "GET", "/protected", "Bearer admin-token")
	if code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", code)
	}
}