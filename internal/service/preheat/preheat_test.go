package preheat

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"ai-go-mall/internal/infra/cache/hotspot"
	"ai-go-mall/internal/model/mall"
)

// fakeHotspot 是 HotspotCache 的可编程 fake，用于断言 preheat 调了 Get + 传了正确的 key。
type fakeHotspot struct {
	getCalls   atomic.Int32
	lastKey    atomic.Value // string
	lastStale  atomic.Value // time.Duration
	getErr     error
	getReturns any
}

func newFakeHotspot() *fakeHotspot { return &fakeHotspot{} }

func (f *fakeHotspot) Get(ctx context.Context, key string, loader hotspot.HotspotLoader, stale time.Duration) (any, error) {
	f.getCalls.Add(1)
	f.lastKey.Store(key)
	f.lastStale.Store(stale)
	if f.getErr != nil {
		return nil, f.getErr
	}
	// 模拟冷启动：调 loader 一次拿到真实数据。
	return loader(ctx, key)
}

func (f *fakeHotspot) Invalidate(_ context.Context, _ string) error { return nil }

// TestPromotionPreheat_HappyPath 验证 Hotspot.Get 被调一次，key 含 blindBoxID。
func TestPromotionPreheat_HappyPath(t *testing.T) {
	hs := newFakeHotspot()
	calls := atomic.Int32{}
	loader := func(ctx context.Context, id int64) (any, error) {
		calls.Add(1)
		return &mall.MallBlindBox{ID: id}, nil
	}

	PromotionPreheat(context.Background(), &mall.MallPromotion{
		BlindBoxID: 42,
	}, Deps{Hotspot: hs, BlindBoxLoader: loader, StaleAfter: 30 * time.Second})

	if got := hs.getCalls.Load(); got != 1 {
		t.Errorf("Hotspot.Get called %d times, want 1", got)
	}
	if got, _ := hs.lastKey.Load().(string); got != "hotspot:blindbox:42" {
		t.Errorf("Hotspot.Get key = %q, want hotspot:blindbox:42", got)
	}
	if got, _ := hs.lastStale.Load().(time.Duration); got != 30*time.Second {
		t.Errorf("Hotspot.Get staleAfter = %v, want 30s", got)
	}
	if got := calls.Load(); got != 1 {
		t.Errorf("loader called %d times, want 1", got)
	}
}

// TestPromotionPreheat_LoaderError_DoesNotPanic 验证 loader 报错时 preheat 不 panic、不向上抛。
func TestPromotionPreheat_LoaderError_DoesNotPanic(t *testing.T) {
	hs := newFakeHotspot()
	hs.getErr = errors.New("loader boom")

	loader := func(ctx context.Context, id int64) (any, error) {
		return nil, errors.New("loader boom")
	}

	// PromotionPreheat 无返回值，期望不 panic。
	PromotionPreheat(context.Background(), &mall.MallPromotion{BlindBoxID: 1}, Deps{
		Hotspot:        hs,
		BlindBoxLoader: loader,
	})

	if got := hs.getCalls.Load(); got != 1 {
		t.Errorf("Hotspot.Get called %d times, want 1", got)
	}
}

// TestSeckillPreheat_NilSafe 验证 promo / sec 为 nil 时静默跳过。
func TestPreheat_NilSafe(t *testing.T) {
	hs := newFakeHotspot()
	loader := func(ctx context.Context, id int64) (any, error) { return nil, nil }

	PromotionPreheat(context.Background(), nil, Deps{Hotspot: hs, BlindBoxLoader: loader})
	SeckillPreheat(context.Background(), nil, Deps{Hotspot: hs, BlindBoxLoader: loader})

	if got := hs.getCalls.Load(); got != 0 {
		t.Errorf("Hotspot.Get called %d times, want 0 (nil entity should skip)", got)
	}
}

// TestPreheat_ZeroBlindBoxID 验证 BlindBoxID <= 0 时静默跳过（不会调 loader）。
func TestPreheat_ZeroBlindBoxID(t *testing.T) {
	hs := newFakeHotspot()
	loader := func(ctx context.Context, id int64) (any, error) {
		t.Errorf("loader should not be called for zero ID")
		return nil, nil
	}

	PromotionPreheat(context.Background(), &mall.MallPromotion{BlindBoxID: 0}, Deps{Hotspot: hs, BlindBoxLoader: loader})
	SeckillPreheat(context.Background(), &mall.MallSeckillActivity{BlindBoxID: -1}, Deps{Hotspot: hs, BlindBoxLoader: loader})

	if got := hs.getCalls.Load(); got != 0 {
		t.Errorf("Hotspot.Get called %d times, want 0", got)
	}
}

// TestPreheat_NilDeps 验证 Hotspot / loader 为 nil 时短路返回。
func TestPreheat_NilDeps(t *testing.T) {
	// Hotspot == nil：内部早返回。
	PromotionPreheat(context.Background(), &mall.MallPromotion{BlindBoxID: 1}, Deps{Hotspot: nil, BlindBoxLoader: nil})
	// Loader == nil：内部早返回。
	hs := newFakeHotspot()
	PromotionPreheat(context.Background(), &mall.MallPromotion{BlindBoxID: 1}, Deps{Hotspot: hs, BlindBoxLoader: nil})

	if got := hs.getCalls.Load(); got != 0 {
		t.Errorf("Hotspot.Get called %d times, want 0", got)
	}
}

// TestPreheat_DefaultStaleAfter 验证 StaleAfter <= 0 时回落 staleAfterDefault (30s)。
func TestPreheat_DefaultStaleAfter(t *testing.T) {
	hs := newFakeHotspot()
	loader := func(ctx context.Context, id int64) (any, error) { return nil, nil }

	PromotionPreheat(context.Background(), &mall.MallPromotion{BlindBoxID: 1}, Deps{Hotspot: hs, BlindBoxLoader: loader})
	PromotionPreheat(context.Background(), &mall.MallPromotion{BlindBoxID: 1}, Deps{Hotspot: hs, BlindBoxLoader: loader, StaleAfter: -1})

	if got, _ := hs.lastStale.Load().(time.Duration); got != staleAfterDefault {
		t.Errorf("staleAfter = %v, want default %v", got, staleAfterDefault)
	}
	if got := hs.getCalls.Load(); got != 2 {
		t.Errorf("Hotspot.Get called %d times, want 2", got)
	}
}