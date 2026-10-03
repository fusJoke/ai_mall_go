// Package hotspot — fake_port_test.go：用可编程的内存 redisPort 覆盖
// Get/Invalidate 的全部逻辑路径（冷启动 / 命中 / 逻辑过期异步刷新 / 锁竞争）。
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

	"ai-go-mall/internal/infra/cache/driver"
)

// fakePort 是 redisPort 的内存实现：带 TTL 过期语义 + Eval 释放锁语义。
type fakePort struct {
	mu    sync.Mutex
	store map[string]fakeEntry
	// evalCalls 记录 Eval 调用次数（断言 Lua 释放路径被走到）。
	evalCalls int
}

type fakeEntry struct {
	value    string
	expireAt time.Time // zero = 永不过期
}

func newFakePort() *fakePort {
	return &fakePort{store: map[string]fakeEntry{}}
}

func (f *fakePort) get(key string) (string, bool) {
	e, ok := f.store[key]
	if !ok {
		return "", false
	}
	if !e.expireAt.IsZero() && time.Now().After(e.expireAt) {
		delete(f.store, key)
		return "", false
	}
	return e.value, true
}

func (f *fakePort) Get(_ context.Context, key string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if v, ok := f.get(key); ok {
		return v, nil
	}
	return "", driver.ErrCacheMiss
}

func (f *fakePort) Set(_ context.Context, key, value string, ttl time.Duration) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	e := fakeEntry{value: value}
	if ttl > 0 {
		e.expireAt = time.Now().Add(ttl)
	}
	f.store[key] = e
	return nil
}

func (f *fakePort) Del(_ context.Context, key string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.store, key)
	return nil
}

func (f *fakePort) SetNX(_ context.Context, key, value string, ttl time.Duration) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.get(key); ok {
		return false, nil
	}
	e := fakeEntry{value: value}
	if ttl > 0 {
		e.expireAt = time.Now().Add(ttl)
	}
	f.store[key] = e
	return true, nil
}

// Eval 实现释放锁脚本（scriptUnlock）的语义：GET == ARGV[0] 才 DEL。
// 真实 Lua 行为由 miniredis 测试覆盖（redis_hotspot_miniredis_test.go），
// 这里保证逻辑测试不依赖外部依赖。
func (f *fakePort) Eval(_ context.Context, script string, keys []string, args ...any) (any, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.evalCalls++
	if script != scriptUnlock {
		return nil, fmt.Errorf("fakePort: unsupported script")
	}
	if len(keys) != 1 || len(args) != 1 {
		return nil, fmt.Errorf("fakePort: bad eval args")
	}
	token, _ := args[0].(string)
	if v, ok := f.get(keys[0]); ok && v == token {
		delete(f.store, keys[0])
		return int64(1), nil
	}
	return int64(0), nil
}

func (f *fakePort) evalCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.evalCalls
}

// newTestCache 构造注入 fake port + 可控时钟的 RedisCache。
func newTestCache() (*RedisCache, *fakePort, *clockFixture) {
	p := newFakePort()
	c := newWithPort(p)
	clk := newClock()
	c.SetNowFn(clk.now)
	return c, p, clk
}

// clockFixture 是可推进的可控时钟。
type clockFixture struct {
	mu   sync.Mutex
	base time.Time
}

func newClock() *clockFixture {
	return &clockFixture{base: time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)}
}

func (c *clockFixture) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.base
}

func (c *clockFixture) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.base = c.base.Add(d)
}

// waitObserver 等待异步刷新完成（带超时防挂死）。
func waitObserver(t *testing.T, ch chan string) string {
	t.Helper()
	select {
	case k := <-ch:
		return k
	case <-time.After(3 * time.Second):
		t.Fatal("async refresh did not complete within 3s")
		return ""
	}
}

func TestGet_ColdStart(t *testing.T) {
	c, p, clk := newTestCache()
	ctx := context.Background()

	loads := atomic.Int64{}
	got, err := c.Get(ctx, "hotspot:blindbox:1", func(ctx context.Context, key string) (any, error) {
		loads.Add(1)
		return map[string]any{"id": 1, "name": "NBA"}, nil
	}, BlindBoxStaleAfter)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if loads.Load() != 1 {
		t.Errorf("loader calls = %d, want 1", loads.Load())
	}
	// 冷启动路径返回 loader 原始值（类型保持）。
	m, ok := got.(map[string]any)
	if !ok || m["name"] != "NBA" {
		t.Errorf("got = %#v, want loader's original map", got)
	}
	// 值已写回 Redis（永驻，含 refreshed_at 信封）。
	raw, ok := p.store["hotspot:blindbox:1"]
	if !ok {
		t.Fatal("value not stored in redis")
	}
	var env envelope
	if err := json.Unmarshal([]byte(raw.value), &env); err != nil {
		t.Fatalf("stored value is not an envelope: %v", err)
	}
	if !env.RefreshedAt.Equal(clk.now()) {
		t.Errorf("RefreshedAt = %v, want %v", env.RefreshedAt, clk.now())
	}
}

func TestGet_HitWithinStaleWindow_SkipsLoader(t *testing.T) {
	c, _, clk := newTestCache()
	ctx := context.Background()

	loads := atomic.Int64{}
	loader := func(ctx context.Context, key string) (any, error) {
		loads.Add(1)
		return "v1", nil
	}
	if _, err := c.Get(ctx, "k", loader, BlindBoxStaleAfter); err != nil {
		t.Fatalf("first Get: %v", err)
	}
	clk.advance(10 * time.Second) // < 30s staleAfter
	for range 5 {
		if _, err := c.Get(ctx, "k", loader, BlindBoxStaleAfter); err != nil {
			t.Fatalf("Get: %v", err)
		}
	}
	if loads.Load() != 1 {
		t.Errorf("loader calls = %d, want 1 (stale window内不打 loader)", loads.Load())
	}
}

func TestGet_Stale_TriggersSingleAsyncRefresh(t *testing.T) {
	c, _, clk := newTestCache()
	ctx := context.Background()

	refreshed := make(chan string, 8)
	c.SetRefreshObserver(func(key string) { refreshed <- key })

	loads := atomic.Int64{}
	loader := func(ctx context.Context, key string) (any, error) {
		loads.Add(1)
		return fmt.Sprintf("v%d", loads.Load()), nil
	}

	if _, err := c.Get(ctx, "k", loader, time.Second); err != nil {
		t.Fatalf("cold Get: %v", err)
	}
	clk.advance(2 * time.Second) // > staleAfter(1s) → 逻辑过期

	// 读路径立即返回老数据（v1），不等待刷新。
	got, err := c.Get(ctx, "k", loader, time.Second)
	if err != nil {
		t.Fatalf("stale Get: %v", err)
	}
	if raw, ok := got.(json.RawMessage); !ok || string(raw) != `"v1"` {
		t.Errorf("stale Get returned %#v, want old value v1", got)
	}

	waitObserver(t, refreshed)
	if loads.Load() != 2 {
		t.Errorf("loader calls = %d, want 2 (cold + async refresh)", loads.Load())
	}
	// 刷新完成后读到新值。
	got2, err := c.Get(ctx, "k", loader, time.Second)
	if err != nil {
		t.Fatalf("post-refresh Get: %v", err)
	}
	if raw, ok := got2.(json.RawMessage); !ok || string(raw) != `"v2"` {
		t.Errorf("post-refresh Get = %#v, want v2", got2)
	}
}

func TestGet_StaleContention_OneRefreshOnly(t *testing.T) {
	c, _, clk := newTestCache()
	ctx := context.Background()

	refreshed := make(chan string, 4)
	c.SetRefreshObserver(func(key string) { refreshed <- key })

	loads := atomic.Int64{}
	// gate 阻塞刷新路径的 loader:确保 100 个并发 Get 全部完成各自的
	// 「读到老数据 / SetNX 抢锁」之后才放行刷新 —— 否则晚到的 stale 读
	// 会在第一次刷新释放锁后再次抢锁(生产语义允许的额外刷新,测试要确定性)。
	gate := make(chan struct{})
	loader := func(ctx context.Context, key string) (any, error) {
		switch loads.Add(1) {
		case 1:
			return "v1", nil
		default:
			<-gate
			return "v1", nil
		}
	}
	if _, err := c.Get(ctx, "k", loader, time.Second); err != nil {
		t.Fatalf("cold Get: %v", err)
	}
	clk.advance(2 * time.Second)

	// 100 个并发请求发现逻辑过期：只允许 1 个抢到锁刷新。
	var wg sync.WaitGroup
	for range 100 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if v, err := c.Get(ctx, "k", loader, time.Second); err != nil {
				t.Errorf("Get: %v", err)
			} else if raw, ok := v.(json.RawMessage); !ok || string(raw) != `"v1"` {
				t.Errorf("stale Get returned %#v, want v1", v)
			}
		}()
	}
	wg.Wait()

	if loads.Load() != 2 {
		t.Fatalf("loader calls = %d, want 2 (cold + 1 blocked refresh)", loads.Load())
	}
	close(gate)
	waitObserver(t, refreshed)
}

func TestInvalidate_ForcesColdReload(t *testing.T) {
	c, p, _ := newTestCache()
	ctx := context.Background()

	loads := atomic.Int64{}
	loader := func(ctx context.Context, key string) (any, error) {
		loads.Add(1)
		return fmt.Sprintf("v%d", loads.Load()), nil
	}
	if _, err := c.Get(ctx, "k", loader, BlindBoxStaleAfter); err != nil {
		t.Fatalf("cold Get: %v", err)
	}
	if err := c.Invalidate(ctx, "k"); err != nil {
		t.Fatalf("Invalidate: %v", err)
	}
	if _, ok := p.store["k"]; ok {
		t.Error("key still in redis after Invalidate")
	}
	got, err := c.Get(ctx, "k", loader, BlindBoxStaleAfter)
	if err != nil {
		t.Fatalf("Get after invalidate: %v", err)
	}
	if loads.Load() != 2 {
		t.Errorf("loader calls = %d, want 2 (invalidate 后同步冷启动)", loads.Load())
	}
	// 失效后重载走冷启动路径，返回 loader 的原始值（类型保持）。
	if s, ok := got.(string); !ok || s != "v2" {
		t.Errorf("got = %#v, want \"v2\"", got)
	}
}

func TestGet_InvalidInput(t *testing.T) {
	c, _, _ := newTestCache()
	ctx := context.Background()
	if _, err := c.Get(ctx, "", func(ctx context.Context, key string) (any, error) { return nil, nil }, time.Second); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("empty key err = %v, want ErrInvalidInput", err)
	}
	if _, err := c.Get(ctx, "k", nil, time.Second); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("nil loader err = %v, want ErrInvalidInput", err)
	}
	if err := c.Invalidate(ctx, ""); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("Invalidate empty key err = %v, want ErrInvalidInput", err)
	}
}

func TestLoad_TypedWrapper(t *testing.T) {
	c, _, clk := newTestCache()
	ctx := context.Background()

	type detail struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
	}
	loads := atomic.Int64{}
	loader := func(ctx context.Context, key string) (*detail, error) {
		loads.Add(1)
		return &detail{ID: 7, Name: "NBA 全明星"}, nil
	}

	// 冷启动：直接返回 loader 类型（无 JSON 往返）。
	d1, err := Load(ctx, c, "k", loader, BlindBoxStaleAfter)
	if err != nil {
		t.Fatalf("Load cold: %v", err)
	}
	if d1.Name != "NBA 全明星" {
		t.Errorf("d1 = %+v", d1)
	}

	// 命中路径：RawMessage → 解码回 *detail。
	clk.advance(10 * time.Second)
	d2, err := Load(ctx, c, "k", loader, BlindBoxStaleAfter)
	if err != nil {
		t.Fatalf("Load hit: %v", err)
	}
	if d2.ID != 7 || d2.Name != "NBA 全明星" {
		t.Errorf("d2 = %+v", d2)
	}
	if loads.Load() != 1 {
		t.Errorf("loader calls = %d, want 1", loads.Load())
	}
}
