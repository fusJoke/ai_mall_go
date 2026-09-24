package database

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestFromContext_PanicsOnMissingKey(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())

	defer func() {
		if recover() == nil {
			t.Fatal("expected panic when *gorm.DB is missing from gin.Context")
		}
	}()
	FromContext(c)
}

func TestFromContext_PanicsOnWrongType(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set(CtxKey, "not a gorm.DB")

	defer func() {
		if recover() == nil {
			t.Fatal("expected panic when context value has wrong type")
		}
	}()
	FromContext(c)
}

func TestDBMiddleware_PanicsWhenDBNotInitialized(t *testing.T) {
	// 保证 db 为 nil（其他测试可能设置过）。
	Reset()
	t.Cleanup(Reset)

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	mw := DBMiddleware()

	defer func() {
		if recover() == nil {
			t.Fatal("expected panic when global *gorm.DB is nil")
		}
	}()
	mw(c)
}
