package driver

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/patrickmn/go-cache"
)

// ============================================================
// memory driver 单测：覆盖 L1 全部方法 + L1 不支持原子操作的语义
// ============================================================

// TestNewMemory_Defaults 验证零值 MemoryConfig 不会 panic 且返回非 nil driver。
func TestNewMemory_Defaults(t *testing.T) {
	d, err := NewMemory(MemoryConfig{})
	if err != nil {
		t.Fatalf("NewMemory(zero) = %v, want nil err", err)
	}
	if d == nil {
		t.Fatal("NewMemory(zero) returned nil driver")
	}
}

// TestMemory_GetSet 验证 L1 Get/Set 正常路径。
func TestMemory_GetSet(t *testing.T) {
	d, _ := NewMemory(MemoryConfig{DefaultTTL: time.Second})
	ctx := context.Background()

	if err := d.Set(ctx, "k", "v", 0); err != nil {
		t.Fatalf("Set = %v, want nil", err)
	}
	got, err := d.Get(ctx, "k")
	if err != nil {
		t.Fatalf("Get = %v, want nil", err)
	}
	if got != "v" {
		t.Errorf("Get = %q, want %q", got, "v")
	}
}

// TestMemory_GetMiss 验证 L1 miss 返回 ErrCacheMiss。
func TestMemory_GetMiss(t *testing.T) {
	d, _ := NewMemory(MemoryConfig{DefaultTTL: time.Second})
	_, err := d.Get(context.Background(), "missing")
	if !errors.Is(err, ErrCacheMiss) {
		t.Errorf("Get(missing) = %v, want ErrCacheMiss", err)
	}
}

// TestMemory_Del 验证 Del 对不存在的 key no-op 不报错。
func TestMemory_Del(t *testing.T) {
	d, _ := NewMemory(MemoryConfig{DefaultTTL: time.Second})
	ctx := context.Background()

	if err := d.Del(ctx, "never-set"); err != nil {
		t.Errorf("Del(never-set) = %v, want nil", err)
	}
	_ = d.Set(ctx, "k", "v", 0)
	if err := d.Del(ctx, "k"); err != nil {
		t.Errorf("Del(k) = %v, want nil", err)
	}
	if _, err := d.Get(ctx, "k"); !errors.Is(err, ErrCacheMiss) {
		t.Errorf("after Del, Get(k) = %v, want ErrCacheMiss", err)
	}
}

// TestMemory_TTLExpiry 验证 ttl > 0 时条目在过期后被回收。
func TestMemory_TTLExpiry(t *testing.T) {
	d, _ := NewMemory(MemoryConfig{DefaultTTL: 10 * time.Second, CleanupInterval: 10 * time.Millisecond})
	ctx := context.Background()

	_ = d.Set(ctx, "k", "v", 50*time.Millisecond)
	// 立即读：命中
	if _, err := d.Get(ctx, "k"); err != nil {
		t.Fatalf("immediate Get = %v, want nil", err)
	}
	// 等过期
	time.Sleep(120 * time.Millisecond)
	if _, err := d.Get(ctx, "k"); !errors.Is(err, ErrCacheMiss) {
		t.Errorf("after expiry, Get(k) = %v, want ErrCacheMiss", err)
	}
}

// TestMemory_NoExpiration 验证 ttl < 0 → 不过期（NoExpiration）。
func TestMemory_NoExpiration(t *testing.T) {
	d, _ := NewMemory(MemoryConfig{DefaultTTL: time.Second})
	ctx := context.Background()

	_ = d.Set(ctx, "k", "v", -1) // -1 → NoExpiration
	// 等一段「通常会过期」的时间
	time.Sleep(50 * time.Millisecond)
	if v, err := d.Get(ctx, "k"); err != nil || v != "v" {
		t.Errorf("after 50ms, Get = (%q, %v), want (\"v\", nil)", v, err)
	}
}

// TestMemory_DefaultTTLForZero 验证 ttl=0 走 DefaultExpiration（go-cache 行为）。
func TestMemory_DefaultTTLForZero(t *testing.T) {
	d, _ := NewMemory(MemoryConfig{DefaultTTL: 50 * time.Millisecond})
	ctx := context.Background()

	_ = d.Set(ctx, "k", "v", 0)
	// 立即可读
	if _, err := d.Get(ctx, "k"); err != nil {
		t.Fatalf("immediate Get = %v, want nil", err)
	}
	// 等过 DefaultTTL
	time.Sleep(120 * time.Millisecond)
	if _, err := d.Get(ctx, "k"); !errors.Is(err, ErrCacheMiss) {
		t.Errorf("after DefaultTTL expiry, Get = %v, want ErrCacheMiss", err)
	}
}

// ============================================================
// L1 不支持的操作：SetNX / Incr / Decr 必须返回 ErrL1NotSupported
// ============================================================

func TestMemory_AtomicOpsNotSupported(t *testing.T) {
	d, _ := NewMemory(MemoryConfig{DefaultTTL: time.Second})
	ctx := context.Background()

	if _, err := d.SetNX(ctx, "k", "v", 0); !errors.Is(err, ErrL1NotSupported) {
		t.Errorf("SetNX = %v, want ErrL1NotSupported", err)
	}
	if _, err := d.Incr(ctx, "k"); !errors.Is(err, ErrL1NotSupported) {
		t.Errorf("Incr = %v, want ErrL1NotSupported", err)
	}
	if _, err := d.Decr(ctx, "k"); !errors.Is(err, ErrL1NotSupported) {
		t.Errorf("Decr = %v, want ErrL1NotSupported", err)
	}
}

// TestMemory_Ping 验证 Ping 永远 nil。
func TestMemory_Ping(t *testing.T) {
	d, _ := NewMemory(MemoryConfig{DefaultTTL: time.Second})
	if err := d.Ping(context.Background()); err != nil {
		t.Errorf("Ping = %v, want nil", err)
	}
}

// TestMemory_TypeAssertionFallback 防御性测试：直接通过 go-cache.Set 写入非 string
// 值后，Driver.Get 应返回 ErrCacheMiss 而非 panic。
//
// 该路径正常业务不会触发（Driver.Set 仅存 string），但防止未来重构时漏改。
func TestMemory_TypeAssertionFallback(t *testing.T) {
	d, _ := NewMemory(MemoryConfig{DefaultTTL: time.Second})
	md := d.(*memoryDriver)

	md.c.Set("k", 12345, cache.DefaultExpiration) // 故意写入非 string
	_, err := md.Get(context.Background(), "k")
	if !errors.Is(err, ErrCacheMiss) {
		t.Errorf("Get(non-string) = %v, want ErrCacheMiss", err)
	}
}
