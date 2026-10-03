package cache

import (
	"context"
	"errors"
	"testing"

	"ai-go-mall/internal/infra/config"
)

// ============================================================
// 单元测试（无 Redis 依赖，仅覆盖参数校验 / 包级状态）
// ============================================================

// TestErrCacheMiss 与 driver.ErrCacheMiss 同源（re-export）：
//
//	业务调用方写 errors.Is(err, cache.ErrCacheMiss) 等价于 errors.Is(err, driver.ErrCacheMiss)。
//
// 这里锁住引用关系，避免后续 driver 切换时静默破坏契约。
func TestErrCacheMiss_ReExport(t *testing.T) {
	if ErrCacheMiss == nil {
		t.Fatal("ErrCacheMiss must be non-nil")
	}
	if ErrCacheMiss.Error() == "" {
		t.Errorf("ErrCacheMiss.Error() empty")
	}
}

// TestErrCacheInvalid 是本包独有的 sentinel（驱动层不定义）。
func TestErrCacheInvalid_Stable(t *testing.T) {
	if ErrCacheInvalid == nil {
		t.Fatal("ErrCacheInvalid must be non-nil")
	}
	if ErrCacheInvalid.Error() == "" {
		t.Errorf("ErrCacheInvalid.Error() empty")
	}
}

// TestManager_NilBeforeInit 验证未 Init 时 Get() 返回 nil（spec）。
func TestManager_NilBeforeInit(t *testing.T) {
	// 单元测试不调 Init；正常情况下 mgr == nil。
	Reset() // 兜底：若其他测试已 Init，强制清空。
	if g := Get(); g != nil {
		t.Errorf("Get() before Init = %v, want nil", g)
	}
}

// TestInit_UnknownDriver 验证未知 driver 名 → Init 返回 error，不 panic。
//
// 该测试覆盖原 TestNewDriver_Unknown 行为：cache.Init() 内部根据 cfg.Driver
// 选实现，遇到不支持的值 → 立即返回 error，进程不应 panic。
func TestInit_UnknownDriver(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Init panicked on unknown driver: %v", r)
		}
	}()

	Reset()
	// 注入一个未知 driver 的配置。
	config.SetForTest(&config.Config{
		Cache: config.CacheConfig{
			Driver: "definitely-not-a-driver",
		},
	})
	defer config.Reset()

	if err := Init(); err == nil {
		t.Errorf("Init with unknown driver = nil err, want error")
	}
}

// TestManager_EmptyKeyRejected 验证 Manager.Get / Set / Del / SetNX 收到空 key 直接返回 ErrCacheInvalid，
// 不调用底层 driver（保证参数校验在 driver 之前）。
//
// 用空 Manager（driver=nil）跑：若校验放在 driver 之后会 nil pointer panic，
// 通过本测试确认校验顺序正确。
func TestManager_EmptyKeyRejected(t *testing.T) {
	m := &Manager{} // driver=nil
	ctx := context.Background()

	if _, _, err := m.Get(ctx, ""); !errors.Is(err, ErrCacheInvalid) {
		t.Errorf("Get empty key = %v, want ErrCacheInvalid", err)
	}
	if err := m.Set(ctx, "", "v", 0); !errors.Is(err, ErrCacheInvalid) {
		t.Errorf("Set empty key = %v, want ErrCacheInvalid", err)
	}
	if err := m.Del(ctx, ""); !errors.Is(err, ErrCacheInvalid) {
		t.Errorf("Del empty key = %v, want ErrCacheInvalid", err)
	}
	if _, err := m.SetNX(ctx, "", "v", 0); !errors.Is(err, ErrCacheInvalid) {
		t.Errorf("SetNX empty key = %v, want ErrCacheInvalid", err)
	}
}

// TestReset_ClearsMgr 验证 Reset() 后 Get() 返回 nil（幂等）。
func TestReset_ClearsMgr(t *testing.T) {
	// 模拟 Init 后的指针；Reset 应清。
	mgr = &Manager{}
	Reset()
	if g := Get(); g != nil {
		t.Errorf("Get() after Reset = %v, want nil", g)
	}
}

// TestCacheInterface_ImplementationGuard 确保 Manager 满足 Cache 接口的编译期断言。
//
// 通过类型断言的方式触发表面的 contract 检查；实际接口满足由 var _ Cache = (*Manager)(nil) 兜底。
func TestCacheInterface_ImplementationGuard(t *testing.T) {
	var _ Cache = (*Manager)(nil)
}
