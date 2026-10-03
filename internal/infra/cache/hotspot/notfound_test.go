// Package hotspot — notfound_test.go：防穿透集成测试（D5.1 / 任务 17.7 / 17.8）。
package hotspot

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"ai-go-mall/internal/infra/cache"
)

// TestNotFound_PenetrationBlocked_1000x 任务 17.7 场景 1：
// 1000 并发请求 NotFound key → DB（loader）只被打 1 次；全部拿到 ErrNotFound。
func TestNotFound_PenetrationBlocked_1000x(t *testing.T) {
	c, _ := newMiniredisCache(t)
	ctx := context.Background()

	var dbHits atomic.Int64
	loader := func(ctx context.Context, key string) (any, error) {
		dbHits.Add(1)
		return nil, fmt.Errorf("row missing: %w", cache.ErrNotFound)
	}

	// 预热一次,让占位先落库（否则 1000 并发里 cold-start 合并也会只放 1 个,
	// 但前置一次让断言语义更纯粹：占位已存在时,后续读 0 次 DB）。
	if _, err := c.Get(ctx, "hotspot:blindbox:99999", loader, BlindBoxStaleAfter); !cache.IsNotFound(err) {
		t.Fatalf("first Get err = %v, want ErrNotFound family", err)
	}

	var wg sync.WaitGroup
	for range 1000 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := c.Get(ctx, "hotspot:blindbox:99999", loader, BlindBoxStaleAfter)
			if !cache.IsNotFound(err) {
				t.Errorf("err = %v, want ErrNotFound family", err)
			}
		}()
	}
	wg.Wait()

	if got := dbHits.Load(); got != 1 {
		t.Errorf("DB hits = %d, want 1 (占位拦截后续全部请求)", got)
	}
}

// TestNotFound_PlaceholderExpiry_ReloaderRuns 任务 17.7 场景 2：
// 占位逻辑过期（staleAfter）后 loader 重新查 DB —— 数据后来被创建时能被发现。
func TestNotFound_PlaceholderExpiry_ReloaderRuns(t *testing.T) {
	c, _ := newMiniredisCache(t)
	ctx := context.Background()

	refreshed := make(chan string, 4)
	c.SetRefreshObserver(func(key string) { refreshed <- key })

	var dbHits atomic.Int64
	created := atomic.Bool{}
	loader := func(ctx context.Context, key string) (any, error) {
		if dbHits.Add(1) == 1 {
			// 第一次:数据还不存在 → 写占位。
			return nil, fmt.Errorf("row missing: %w", cache.ErrNotFound)
		}
		// 重查:数据已被创建。
		created.Store(true)
		return map[string]any{"id": 99999, "name": "新上架盲盒"}, nil
	}

	const staleAfter = 50 * time.Millisecond
	if _, err := c.Get(ctx, "hotspot:blindbox:99999", loader, staleAfter); !cache.IsNotFound(err) {
		t.Fatalf("first Get = %v, want ErrNotFound family", err)
	}
	// 占位窗口内的读:仍 ErrNotFound,不打 DB。
	if _, err := c.Get(ctx, "hotspot:blindbox:99999", loader, staleAfter); !cache.IsNotFound(err) {
		t.Fatalf("within-window Get = %v, want ErrNotFound family", err)
	}
	if dbHits.Load() != 1 {
		t.Fatalf("DB hits = %d, want 1", dbHits.Load())
	}

	// 逻辑过期 → 下一次读触发异步重查（返回的仍是占位的 ErrNotFound）。
	time.Sleep(80 * time.Millisecond)
	if _, err := c.Get(ctx, "hotspot:blindbox:99999", loader, staleAfter); !cache.IsNotFound(err) {
		t.Fatalf("stale Get = %v, want ErrNotFound family", err)
	}
	select {
	case <-refreshed:
	case <-time.After(3 * time.Second):
		t.Fatal("refresh did not complete")
	}

	// 重查完成:占位被真实数据替换。
	v, err := c.Get(ctx, "hotspot:blindbox:99999", loader, staleAfter)
	if err != nil {
		t.Fatalf("post-refresh Get: %v", err)
	}
	if !created.Load() || dbHits.Load() != 2 {
		t.Errorf("dbHits = %d created = %v, want 2 / true", dbHits.Load(), created.Load())
	}
	if raw, ok := v.(json.RawMessage); !ok || json.Valid(raw) == false || len(raw) == 0 {
		t.Errorf("post-refresh value = %#v, want real data JSON", v)
	}
}

// TestNotFound_InvalidateLetsRealDataThrough 任务 17.8：
// admin 创建新 blind_box 后 hotspot.Invalidate → 下次读触发冷启动加载
// 真实数据（不会被 NotFound 占位误导——占位已被 DEL）。
func TestNotFound_InvalidateLetsRealDataThrough(t *testing.T) {
	c, _ := newMiniredisCache(t)
	ctx := context.Background()

	var dbHits atomic.Int64
	exists := atomic.Bool{}
	key := "hotspot:blindbox:777"
	loader := func(ctx context.Context, k string) (any, error) {
		n := dbHits.Add(1)
		if !exists.Load() {
			_ = n
			return nil, fmt.Errorf("row missing: %w", cache.ErrNotFound)
		}
		return map[string]any{"id": 777, "name": "刚创建的盲盒"}, nil
	}

	// 1) 数据不存在 → 占位写入;窗口内读全是 ErrNotFound。
	if _, err := c.Get(ctx, key, loader, BlindBoxStaleAfter); !cache.IsNotFound(err) {
		t.Fatalf("first Get = %v", err)
	}
	if _, err := c.Get(ctx, key, loader, BlindBoxStaleAfter); !cache.IsNotFound(err) {
		t.Fatalf("placeholder Get = %v", err)
	}

	// 2) 业务创建数据(模拟 admin 创建盲盒)→ 写路径主动失效占位。
	exists.Store(true)
	if err := c.Invalidate(ctx, key); err != nil {
		t.Fatalf("Invalidate: %v", err)
	}

	// 3) 下一次读:同步冷启动,直接拿到真实数据(loader 原始值,类型保持),
	// 不被占位误导。
	v, err := c.Get(ctx, key, loader, BlindBoxStaleAfter)
	if err != nil {
		t.Fatalf("Get after invalidate: %v", err)
	}
	if m, ok := v.(map[string]any); !ok || m["name"] != "刚创建的盲盒" {
		t.Errorf("value = %#v, want real data", v)
	}
	if dbHits.Load() != 2 {
		t.Errorf("dbHits = %d, want 2 (miss + invalidate 后冷启动)", dbHits.Load())
	}
}

// TestNotFoundSentinels 任务 17.4:IsNotFound / Wrap / Unwrap 语义。
func TestNotFoundSentinels(t *testing.T) {
	if !cache.IsNotFound(cache.ErrNotFound) {
		t.Error("IsNotFound(ErrNotFound) = false")
	}
	// 包装的业务 sentinel 双语义成立。
	wrapped := fmt.Errorf("biz: %w", cache.ErrNotFound)
	if !cache.IsNotFound(wrapped) {
		t.Error("IsNotFound(wrapped) = false")
	}
	// 普通错误不误判。
	if cache.IsNotFound(errors.New("boom")) || cache.IsNotFound(nil) {
		t.Error("IsNotFound false positive")
	}
	// 值包装/解包。
	p := cache.WrapNotFound(nil)
	if _, ok := cache.UnwrapNotFound(p); !ok {
		t.Error("UnwrapNotFound(WrapNotFound(nil)) not ok")
	}
	if _, ok := cache.UnwrapNotFound("plain"); ok {
		t.Error("UnwrapNotFound(plain) should be false")
	}
}
