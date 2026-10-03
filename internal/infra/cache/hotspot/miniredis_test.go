// Package hotspot — miniredis_test.go：基于 miniredis（真实 Redis 协议 +
// Lua 解释器）的行为测试，覆盖任务 16.3 的 Lua 锁释放与 16.9 的并发集成场景。
package hotspot

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

// newMiniredisCache 起一个进程内 miniredis + 真实 go-redis 客户端。
func newMiniredisCache(t *testing.T) (*RedisCache, *miniredis.Miniredis) {
	t.Helper()
	s := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: s.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	return NewRedis(client), s
}

// TestUnlockScript_OnlyDeletesOwnLock 验证释放锁 Lua 脚本在真实 Redis 语义下：
// token 不匹配 → 不删（保护后继持锁者）；token 匹配 → 删除。
func TestUnlockScript_OnlyDeletesOwnLock(t *testing.T) {
	c, s := newMiniredisCache(t)
	ctx := context.Background()

	lockKey := "lock:k"
	// A 抢到锁。
	ok, err := c.rdb.SetNX(ctx, lockKey, "token-A", DefaultLockTTL)
	if err != nil || !ok {
		t.Fatalf("SetNX A: ok=%v err=%v", ok, err)
	}
	// 锁过期后 B 抢到新锁（模拟 A 超时）。
	s.FastForward(DefaultLockTTL + time.Second)
	ok, err = c.rdb.SetNX(ctx, lockKey, "token-B", DefaultLockTTL)
	if err != nil || !ok {
		t.Fatalf("SetNX B: ok=%v err=%v", ok, err)
	}

	// A 迟到的释放（token-A）必须被拒绝。
	c.releaseLock(ctx, lockKey, "token-A")
	if v, _ := c.rdb.Get(ctx, lockKey); v != "token-B" {
		t.Fatalf("stale holder deleted B's lock: %q", v)
	}

	// B 用自己的 token 释放成功。
	c.releaseLock(ctx, lockKey, "token-B")
	if _, err := c.rdb.Get(ctx, lockKey); err == nil {
		t.Fatal("owner release did not delete the lock")
	}
}

// TestGet_ColdStartConcurrentMerge_100x 任务 16.9 场景 1：
// 100 并发请求 cold start → singleflight 合并后只 1 个打 DB。
func TestGet_ColdStartConcurrentMerge_100x(t *testing.T) {
	c, _ := newMiniredisCache(t)
	ctx := context.Background()

	// loader 阻塞：确保 100 个请求全部进入 Get 后才放行，
	// 这样它们都落在同一个 singleflight 窗口内。
	start := make(chan struct{})
	release := make(chan struct{})
	var dbHits atomic.Int64
	loader := func(ctx context.Context, key string) (any, error) {
		dbHits.Add(1)
		<-release
		return "cold-value", nil
	}

	var wg sync.WaitGroup
	results := make(chan any, 100)
	for range 100 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			v, err := c.Get(ctx, "hotspot:blindbox:1", loader, BlindBoxStaleAfter)
			if err != nil {
				t.Errorf("Get: %v", err)
				return
			}
			results <- v
		}()
	}
	close(start)
	// 给 goroutines 一点时间全部阻塞在 loader 上。
	deadline := time.Now().Add(2 * time.Second)
	for dbHits.Load() < 1 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	close(release)
	wg.Wait()
	close(results)

	if dbHits.Load() != 1 {
		t.Errorf("DB hits = %d, want 1 (singleflight merge)", dbHits.Load())
	}
	count := 0
	for v := range results {
		count++
		// 100 个请求拿到的都是同一个 cold-value（合并共享结果）。
		if fmt.Sprint(v) != "cold-value" {
			t.Errorf("result = %#v, want cold-value", v)
		}
	}
	if count != 100 {
		t.Errorf("collected %d results, want 100", count)
	}
	// 且值已永驻 Redis。
	if _, err := c.rdb.Get(ctx, "hotspot:blindbox:1"); err != nil {
		t.Fatalf("value not persisted: %v", err)
	}
}

// TestGet_StaleRefresh_OneHolderRestStale_100x 任务 16.9 场景 2：
// 逻辑时间到期后 100 并发请求 → 1 个抢锁异步刷新，其余 99 个返回老数据。
func TestGet_StaleRefresh_OneHolderRestStale_100x(t *testing.T) {
	c, _ := newMiniredisCache(t)
	ctx := context.Background()

	refreshed := make(chan string, 4)
	c.SetRefreshObserver(func(key string) { refreshed <- key })

	var dbHits atomic.Int64
	value := "v1"
	refreshGate := make(chan struct{})
	loader := func(ctx context.Context, key string) (any, error) {
		n := dbHits.Add(1)
		if n == 1 {
			return "v1", nil // 冷启动
		}
		// 异步刷新：阻塞，验证刷新期间读路径仍然秒回老数据。
		<-refreshGate
		value = "v2"
		return value, nil
	}

	// 冷启动写 v1。
	if _, err := c.Get(ctx, "hotspot:blindbox:1", loader, 50*time.Millisecond); err != nil {
		t.Fatalf("cold Get: %v", err)
	}
	// 等逻辑时间过期（staleAfter=50ms）。
	time.Sleep(80 * time.Millisecond)

	// 100 并发：全部应立即拿到 v1（老数据），且只有 1 个抢到锁。
	var wg sync.WaitGroup
	for range 100 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			v, err := c.Get(ctx, "hotspot:blindbox:1", loader, 50*time.Millisecond)
			if err != nil {
				t.Errorf("Get: %v", err)
				return
			}
			if raw, ok := v.(json.RawMessage); !ok || string(raw) != `"v1"` {
				t.Errorf("got %v, want stale \"v1\"", v)
			}
		}()
	}
	wg.Wait()

	// 等异步刷新完成后校验：DB 只被多打 1 次（1 cold + 1 refresh）。
	close(refreshGate)
	select {
	case <-refreshed:
	case <-time.After(3 * time.Second):
		t.Fatal("async refresh did not complete")
	}
	if got := dbHits.Load(); got != 2 {
		t.Errorf("DB hits = %d, want 2 (cold + 1 async refresh)", got)
	}
	// Redis 里已是 v2；下一个读拿到新值。
	v, err := c.Get(ctx, "hotspot:blindbox:1", loader, 50*time.Millisecond)
	if err != nil {
		t.Fatalf("post-refresh Get: %v", err)
	}
	if raw, ok := v.(json.RawMessage); !ok || string(raw) != `"v2"` {
		t.Errorf("post-refresh value = %v, want \"v2\"", v)
	}
}
