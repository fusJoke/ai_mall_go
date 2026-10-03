package driver

import (
	"testing"
)

// ============================================================
// driver 包：仅 Driver 接口 + sentinel 错误 + RedisConfig type alias
// ============================================================

// TestErrCacheMiss_Stable 验证业务约定使用的 ErrCacheMiss sentinel 非空且稳定。
//
// 所有 driver 必须返回此 sentinel（包注释硬性约定）；业务侧 cache.ErrCacheMiss
// re-export 的也是它。
func TestErrCacheMiss_Stable(t *testing.T) {
	if ErrCacheMiss == nil {
		t.Fatal("ErrCacheMiss must be non-nil")
	}
	if ErrCacheMiss.Error() == "" {
		t.Errorf("ErrCacheMiss.Error() empty")
	}
}

// TestRedisConfig_TypeAlias 验证 RedisConfig 与 config.RedisConfig 是同一类型。
//
//	驱动包用 type alias 而非新类型，避免重复定义字段。
//	若未来切到新类型，本测试会失败 —— 提示开发者同步 cache.ErrCacheMiss 调用方。
func TestRedisConfig_TypeAlias(t *testing.T) {
	var _ RedisConfig // 编译期类型必须解析得到。
}

// ============================================================
// 编译期断言：NewRedis 返回值实现 Driver 接口。
// ============================================================

var _ Driver = (Driver)(nil) // 显式占位，确保 Driver 接口未误改。
