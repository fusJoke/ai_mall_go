package user

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"ai-go-mall/internal/infra/cache/hotspot"
	"ai-go-mall/internal/infra/search"
	"ai-go-mall/internal/model/mall"
	mallRepo "ai-go-mall/internal/repository/mall"
)

// =============================================================================
// 降级路径测试（openspec change `add-uni-h5-client`，capability home-feed-fallback）
//
// 覆盖 spec 的四条硬约束：
//   - 触发条件：ES error → 降级；ES 0 条 → 不降级；双失败 → search_unavailable；
//   - 结构同构：降级与主路径的 JSON 字段集合完全一致；
//   - 活动价：查不到 → null，且 price 仍为真实原价（无估算/默认填充）；
//   - 缓存约束：降级结果不写入任何 Feed 缓存；
//   - 可观测性：Source 字段标识来源（handler 据此设响应头）；
//   - 超时：ES 无响应时在 800ms 量级转入降级。
// =============================================================================

// fakeFeedRepo 是 mallRepo.FeedRepository 的 mock 实现。
//
// 任一方法返回 err（非 nil 时短路），用于模拟「MySQL 亦不可用」。
type fakeFeedRepo struct {
	boxes   []mallRepo.FeedBlindBoxRow
	weights []mallRepo.FeedRarityWeight
	promos  map[int64]float64
	sups    []mallRepo.FeedSupplierRow
	err     error

	// 调用计数，用于断言「降级未被触发」。
	boxesCalls  int
	supCalls    int
	promoCalls  int
	weightCalls int
}

var _ mallRepo.FeedRepository = (*fakeFeedRepo)(nil)

func (f *fakeFeedRepo) FeedBlindBoxes(_ *gin.Context, _ int) ([]mallRepo.FeedBlindBoxRow, error) {
	f.boxesCalls++
	if f.err != nil {
		return nil, f.err
	}
	return f.boxes, nil
}

func (f *fakeFeedRepo) FeedRarityWeights(_ *gin.Context, _ []int64) ([]mallRepo.FeedRarityWeight, error) {
	f.weightCalls++
	if f.err != nil {
		return nil, f.err
	}
	return f.weights, nil
}

func (f *fakeFeedRepo) FeedActivePromoPrices(_ *gin.Context, _ time.Time) (map[int64]float64, error) {
	f.promoCalls++
	if f.err != nil {
		return nil, f.err
	}
	return f.promos, nil
}

func (f *fakeFeedRepo) FeedSuppliers(_ *gin.Context, _ int) ([]mallRepo.FeedSupplierRow, error) {
	f.supCalls++
	if f.err != nil {
		return nil, f.err
	}
	return f.sups, nil
}

// fakeHotspot 是 hotspot.HotspotCache 的最小 mock：Get 直接回源 loader
// （等价于「冷启动、无缓存」），用于覆盖 Hotspot 已装配时的降级路径。
type fakeHotspot struct {
	// loaderErr 非 nil 时，Get 直接返回该错误（模拟缓存层故障）。
	loaderErr error
}

var _ hotspot.HotspotCache = (*fakeHotspot)(nil)

func (f *fakeHotspot) Get(ctx context.Context, key string, loader hotspot.HotspotLoader, _ time.Duration) (any, error) {
	if f.loaderErr != nil {
		return nil, f.loaderErr
	}
	return loader(ctx, key)
}

func (f *fakeHotspot) Invalidate(_ context.Context, _ string) error { return nil }

// esDownSearch 返回一个「ES 连接失败」的 fake search client。
func esDownSearch() *fakeSearchClient {
	return &fakeSearchClient{
		search: func(index string, query []byte) ([]search.Hit, int64, error) {
			return nil, 0, errors.New("es: connection refused")
		},
	}
}

// =============================================================================
// ① 触发条件
// =============================================================================

// TestFeedFallback_ESError_UsesMySQL 验证 ES 报错 → 走 MySQL 降级、不返回 error。
//
// spec Scenario "ES 不可用触发降级"。
func TestFeedFallback_ESError_UsesMySQL(t *testing.T) {
	repo := &fakeFeedRepo{
		boxes: []mallRepo.FeedBlindBoxRow{
			{ID: 7, SupplierID: 3, SupplierName: "Kayou 卡游旗舰店", Name: "NBA 全明星盲盒",
				Cover: "https://cdn.example.com/7.jpg", Price: 99.0},
		},
		sups: []mallRepo.FeedSupplierRow{
			{ID: 3, Name: "Kayou 卡游旗舰店", Logo: "https://cdn.example.com/kayou.png", BlindBoxCount: 2},
		},
	}
	svc := NewFeedService(FeedServiceDeps{Search: esDownSearch(), FallbackRepo: repo})

	feed, err := svc.Feed(newHomeCtx())
	if err != nil {
		t.Fatalf("Feed: %v, want nil (fallback should swallow ES error)", err)
	}
	if feed == nil {
		t.Fatal("Feed returned nil")
	}
	if feed.Source != FeedSourceMySQL {
		t.Errorf("Source = %q, want %q", feed.Source, FeedSourceMySQL)
	}
	if len(feed.BlindBoxes) != 1 || feed.BlindBoxes[0].ID != 7 {
		t.Errorf("BlindBoxes = %+v, want one item with ID=7", feed.BlindBoxes)
	}
	if feed.BlindBoxes[0].SupplierName != "Kayou 卡游旗舰店" {
		t.Errorf("SupplierName = %q", feed.BlindBoxes[0].SupplierName)
	}
	if len(feed.Suppliers) != 1 || feed.Suppliers[0].BlindBoxCount != 2 {
		t.Errorf("Suppliers = %+v", feed.Suppliers)
	}
	if feed.FetchedAt.IsZero() {
		t.Error("FetchedAt not set")
	}
	if repo.boxesCalls != 1 || repo.supCalls != 1 {
		t.Errorf("repo calls = boxes:%d sups:%d, want 1/1", repo.boxesCalls, repo.supCalls)
	}
}

// TestFeedFallback_ESEmpty_NoFallback 验证 ES 正常返回 0 条 → **不降级**。
//
// spec Scenario "ES 返回空结果不降级"：0 条是合法业务结果，查 MySQL 会掩盖
// 「索引被清空 / 同步失败」这类真问题。
func TestFeedFallback_ESEmpty_NoFallback(t *testing.T) {
	repo := &fakeFeedRepo{
		boxes: []mallRepo.FeedBlindBoxRow{{ID: 1}},
		sups:  []mallRepo.FeedSupplierRow{{ID: 1}},
	}
	fakeSearch := &fakeSearchClient{
		search: func(index string, query []byte) ([]search.Hit, int64, error) {
			return nil, 0, nil // 成功但 0 条
		},
	}
	svc := NewFeedService(FeedServiceDeps{Search: fakeSearch, FallbackRepo: repo})

	feed, err := svc.Feed(newHomeCtx())
	if err != nil {
		t.Fatalf("Feed: %v", err)
	}
	if feed.Source != FeedSourceES {
		t.Errorf("Source = %q, want %q (empty ES result must not degrade)", feed.Source, FeedSourceES)
	}
	if feed.BlindBoxes == nil || feed.Suppliers == nil {
		t.Error("empty result must be [] not nil")
	}
	if len(feed.BlindBoxes) != 0 || len(feed.Suppliers) != 0 {
		t.Errorf("want empty arrays, got %d/%d", len(feed.BlindBoxes), len(feed.Suppliers))
	}
	if repo.boxesCalls != 0 || repo.supCalls != 0 {
		t.Errorf("fallback repo was called (boxes:%d sups:%d), want 0/0",
			repo.boxesCalls, repo.supCalls)
	}
}

// TestFeedFallback_BothFail_ReturnsUnavailable 验证 ES 与 MySQL 均失败 → 500 语义。
//
// spec Scenario "MySQL 亦不可用"。
func TestFeedFallback_BothFail_ReturnsUnavailable(t *testing.T) {
	repo := &fakeFeedRepo{err: errors.New("mysql: connection refused")}
	svc := NewFeedService(FeedServiceDeps{Search: esDownSearch(), FallbackRepo: repo})

	_, err := svc.Feed(newHomeCtx())
	if err == nil {
		t.Fatal("Feed err = nil, want error")
	}
	if !errors.Is(err, ErrFeedSearchUnavailable) {
		t.Errorf("err = %v, want wraps ErrFeedSearchUnavailable", err)
	}
}

// TestFeedFallback_NilRepo_KeepsLegacySemantics 验证降级通道未装配时保持历史语义。
func TestFeedFallback_NilRepo_KeepsLegacySemantics(t *testing.T) {
	svc := NewFeedService(FeedServiceDeps{Search: esDownSearch(), FallbackRepo: nil})

	_, err := svc.Feed(newHomeCtx())
	if err == nil {
		t.Fatal("Feed err = nil, want error (no fallback repo configured)")
	}
	if !errors.Is(err, ErrFeedSearchUnavailable) {
		t.Errorf("err = %v, want wraps ErrFeedSearchUnavailable", err)
	}
}

// TestFeedFallback_HotspotColdStart_ESError_UsesMySQL 验证 Hotspot 已装配时
// （生产实际路径）冷启动 loader 失败同样转入降级。
func TestFeedFallback_HotspotColdStart_ESError_UsesMySQL(t *testing.T) {
	repo := &fakeFeedRepo{
		boxes: []mallRepo.FeedBlindBoxRow{{ID: 42, Price: 49.0}},
	}
	svc := NewFeedService(FeedServiceDeps{
		Search:       esDownSearch(),
		FallbackRepo: repo,
		Hotspot:      &fakeHotspot{},
	})

	feed, err := svc.Feed(newHomeCtx())
	if err != nil {
		t.Fatalf("Feed: %v", err)
	}
	if feed.Source != FeedSourceMySQL {
		t.Errorf("Source = %q, want %q", feed.Source, FeedSourceMySQL)
	}
	if len(feed.BlindBoxes) != 1 || feed.BlindBoxes[0].ID != 42 {
		t.Errorf("BlindBoxes = %+v", feed.BlindBoxes)
	}
}

// =============================================================================
// ② 结构同构（spec Requirement "降级响应与主路径结构同构"）
// =============================================================================

// TestFeedFallback_DTOShapeMatchesESPath 验证两条路径的 JSON **字段集合**完全一致。
//
// 做法：同一批业务数据分别喂给 ES 路径（构造 Hit）与 MySQL 路径（构造行），
// 序列化后逐层比较 key 集合 —— 客户端因此不需要任何条件分支。
func TestFeedFallback_DTOShapeMatchesESPath(t *testing.T) {
	// --- ES 路径 ---
	esBoxJSON := []byte(`{
		"id": 7, "supplier_id": 3, "supplier_name": "Kayou 卡游旗舰店",
		"name": "NBA 全明星盲盒", "cover": "https://cdn.example.com/7.jpg",
		"price": 99.0, "rarity_summary": "SSR 3% / SR 12% / R 35% / N 50%",
		"hot_score": 0, "created_at": "2026-10-01T10:00:00Z"
	}`)
	esSupJSON := []byte(`{
		"id": 3, "name": "Kayou 卡游旗舰店", "logo": "https://cdn.example.com/kayou.png",
		"blind_box_count": 2, "featured_rank": 0
	}`)
	esSearch := &fakeSearchClient{
		search: func(index string, query []byte) ([]search.Hit, int64, error) {
			switch index {
			case BlindBoxIndex:
				return []search.Hit{{ID: "7", Source: esBoxJSON}}, 1, nil
			case SupplierIndex:
				return []search.Hit{{ID: "3", Source: esSupJSON}}, 1, nil
			}
			return nil, 0, nil
		},
	}
	esSvc := NewFeedService(FeedServiceDeps{Search: esSearch})
	esFeed, err := esSvc.Feed(newHomeCtx())
	if err != nil {
		t.Fatalf("ES path Feed: %v", err)
	}

	// --- MySQL 降级路径（同一批业务数据）---
	repo := &fakeFeedRepo{
		boxes: []mallRepo.FeedBlindBoxRow{{
			ID: 7, SupplierID: 3, SupplierName: "Kayou 卡游旗舰店",
			Name: "NBA 全明星盲盒", Cover: "https://cdn.example.com/7.jpg",
			Price: 99.0, CreatedAt: time.Date(2026, 10, 1, 10, 0, 0, 0, time.UTC),
		}},
		weights: []mallRepo.FeedRarityWeight{
			{BlindBoxID: 7, Rarity: mall.RaritySSR, Weight: 300},
			{BlindBoxID: 7, Rarity: mall.RaritySR, Weight: 1200},
			{BlindBoxID: 7, Rarity: mall.RarityR, Weight: 3500},
			{BlindBoxID: 7, Rarity: mall.RarityN, Weight: 5000},
		},
		sups: []mallRepo.FeedSupplierRow{
			{ID: 3, Name: "Kayou 卡游旗舰店", Logo: "https://cdn.example.com/kayou.png", BlindBoxCount: 2},
		},
	}
	mySvc := NewFeedService(FeedServiceDeps{Search: esDownSearch(), FallbackRepo: repo})
	myFeed, err := mySvc.Feed(newHomeCtx())
	if err != nil {
		t.Fatalf("MySQL path Feed: %v", err)
	}

	// --- 比较 ---
	assertSameKeys(t, "top-level", esFeed, myFeed)
	assertSameKeys(t, "blind_boxes[0]", esFeed.BlindBoxes[0], myFeed.BlindBoxes[0])
	assertSameKeys(t, "suppliers[0]", esFeed.Suppliers[0], myFeed.Suppliers[0])

	// 同批业务数据下，业务字段值也应一致（source 除外 —— 它正是用来区分来源的）。
	if myFeed.BlindBoxes[0].RaritySummary != esFeed.BlindBoxes[0].RaritySummary {
		t.Errorf("rarity_summary mismatch: mysql=%q es=%q",
			myFeed.BlindBoxes[0].RaritySummary, esFeed.BlindBoxes[0].RaritySummary)
	}
	if myFeed.BlindBoxes[0].Price != esFeed.BlindBoxes[0].Price {
		t.Errorf("price mismatch: mysql=%v es=%v", myFeed.BlindBoxes[0].Price, esFeed.BlindBoxes[0].Price)
	}
	if myFeed.Suppliers[0].BlindBoxCount != esFeed.Suppliers[0].BlindBoxCount {
		t.Errorf("blind_box_count mismatch")
	}
}

// assertSameKeys 断言两个值序列化后的 JSON key 集合完全一致。
func assertSameKeys(t *testing.T, label string, a, b any) {
	t.Helper()
	ka := jsonKeys(t, a)
	kb := jsonKeys(t, b)
	if len(ka) != len(kb) {
		t.Fatalf("%s: key count %d != %d\n a=%v\n b=%v", label, len(ka), len(kb), ka, kb)
	}
	for k := range ka {
		if !kb[k] {
			t.Errorf("%s: key %q present in ES path but missing in MySQL path", label, k)
		}
	}
	for k := range kb {
		if !ka[k] {
			t.Errorf("%s: key %q present in MySQL path but missing in ES path", label, k)
		}
	}
}

// jsonKeys 返回序列化后的顶层 key 集合。
func jsonKeys(t *testing.T, v any) map[string]bool {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	out := make(map[string]bool, len(m))
	for k := range m {
		out[k] = true
	}
	return out
}

// =============================================================================
// ③ 活动价（spec Requirement "降级路径的活动价处理"）
// =============================================================================

// TestFeedFallback_PromoPrice_NilWhenUnknown 验证查不到活动价 → promo_price 为
// null，且 price 仍为真实原价（MUST NOT 估算或默认填充）。
func TestFeedFallback_PromoPrice_NilWhenUnknown(t *testing.T) {
	repo := &fakeFeedRepo{
		boxes: []mallRepo.FeedBlindBoxRow{
			{ID: 1, Price: 99.0},
			{ID: 2, Price: 49.0},
		},
		promos: nil, // 无任何生效活动
	}
	svc := NewFeedService(FeedServiceDeps{Search: esDownSearch(), FallbackRepo: repo})

	feed, err := svc.Feed(newHomeCtx())
	if err != nil {
		t.Fatalf("Feed: %v", err)
	}
	for _, item := range feed.BlindBoxes {
		if item.PromoPrice != nil {
			t.Errorf("box %d PromoPrice = %v, want nil", item.ID, *item.PromoPrice)
		}
	}
	if feed.BlindBoxes[0].Price != 99.0 || feed.BlindBoxes[1].Price != 49.0 {
		t.Errorf("price must remain the real original price: %+v", feed.BlindBoxes)
	}

	// JSON 层面：promo_price 因 omitempty 缺省（spec 允许「置 null 或字段缺省」）。
	raw, _ := json.Marshal(feed.BlindBoxes[0])
	var m map[string]json.RawMessage
	_ = json.Unmarshal(raw, &m)
	if _, ok := m["promo_price"]; ok {
		t.Errorf("promo_price should be omitted when unknown, got %s", m["promo_price"])
	}
}

// TestFeedFallback_PromoPrice_FilledWhenKnown 验证确有生效活动时填真实值。
func TestFeedFallback_PromoPrice_FilledWhenKnown(t *testing.T) {
	repo := &fakeFeedRepo{
		boxes:  []mallRepo.FeedBlindBoxRow{{ID: 1, Price: 99.0}},
		promos: map[int64]float64{1: 79.0},
	}
	svc := NewFeedService(FeedServiceDeps{Search: esDownSearch(), FallbackRepo: repo})

	feed, err := svc.Feed(newHomeCtx())
	if err != nil {
		t.Fatalf("Feed: %v", err)
	}
	got := feed.BlindBoxes[0].PromoPrice
	if got == nil || *got != 79.0 {
		t.Errorf("PromoPrice = %v, want 79.0", got)
	}
	if feed.BlindBoxes[0].Price != 99.0 {
		t.Errorf("Price = %v, want 99.0 (原价不受活动价影响)", feed.BlindBoxes[0].Price)
	}
}

// TestBuildRaritySummary_MatchesEsSync 验证降级路径的概率公示串算法与
// cmd/es-sync 的 buildRaritySummary 一致（同池加和 10000，weight/100 取整）。
func TestBuildRaritySummary_MatchesEsSync(t *testing.T) {
	rows := []mallRepo.FeedRarityWeight{
		{Rarity: mall.RarityN, Weight: 5000},
		{Rarity: mall.RaritySSR, Weight: 300},
		{Rarity: mall.RarityR, Weight: 3500},
		{Rarity: mall.RaritySR, Weight: 1200},
	}
	want := "SSR 3% / SR 12% / R 35% / N 50%"
	if got := buildRaritySummary(rows); got != want {
		t.Errorf("buildRaritySummary = %q, want %q", got, want)
	}

	// weight=0 的稀有度不出现；空池返回空串。
	if got := buildRaritySummary([]mallRepo.FeedRarityWeight{{Rarity: mall.RaritySSR, Weight: 0}}); got != "" {
		t.Errorf("all-zero weights should yield empty summary, got %q", got)
	}
	if got := buildRaritySummary(nil); got != "" {
		t.Errorf("nil rows should yield empty summary, got %q", got)
	}
}

// =============================================================================
// ④ 缓存约束（spec Requirement "降级结果的缓存回填约束"）
// =============================================================================

// TestFeedFallback_DoesNotWriteCache 验证降级结果不写入 Feed 缓存。
//
// 这是 spec 允许的两种实现之一（完全不回填）。选它的原因：hotspot 是「永驻」
// 语义，写入后即便逻辑过期也只会异步刷新失败并继续返回旧值，ES 恢复后
// 长时间无法自愈。
func TestFeedFallback_DoesNotWriteCache(t *testing.T) {
	fakeCache := &fakeCacheClient{
		getFunc: func(ctx context.Context, key string) (string, bool, error) {
			return "", false, errors.New("cache: miss")
		},
	}
	repo := &fakeFeedRepo{boxes: []mallRepo.FeedBlindBoxRow{{ID: 1, Price: 10.0}}}

	svc := NewFeedService(FeedServiceDeps{
		Search:       esDownSearch(),
		FallbackRepo: repo,
		Cache:        fakeCache,
		CacheTTL:     5 * time.Minute,
	})

	feed, err := svc.Feed(newHomeCtx())
	if err != nil {
		t.Fatalf("Feed: %v", err)
	}
	if feed.Source != FeedSourceMySQL {
		t.Fatalf("Source = %q, want %q", feed.Source, FeedSourceMySQL)
	}
	if fakeCache.setCalls.Load() != 0 {
		t.Errorf("cache.Set called %d times, want 0 (degraded result must not be cached)",
			fakeCache.setCalls.Load())
	}
}

// TestFeedFallback_MainPathStillBackfills 验证主路径恢复后按正常 TTL 回填缓存。
//
// spec Scenario "主路径恢复后回填正常"。
func TestFeedFallback_MainPathStillBackfills(t *testing.T) {
	fakeCache := &fakeCacheClient{
		getFunc: func(ctx context.Context, key string) (string, bool, error) {
			return "", false, errors.New("cache: miss")
		},
	}
	fakeSearch := &fakeSearchClient{
		search: func(index string, query []byte) ([]search.Hit, int64, error) {
			return nil, 0, nil
		},
	}
	svc := NewFeedService(FeedServiceDeps{
		Search:       fakeSearch,
		FallbackRepo: &fakeFeedRepo{},
		Cache:        fakeCache,
		CacheTTL:     5 * time.Minute,
	})

	feed, err := svc.Feed(newHomeCtx())
	if err != nil {
		t.Fatalf("Feed: %v", err)
	}
	if feed.Source != FeedSourceES {
		t.Errorf("Source = %q, want %q", feed.Source, FeedSourceES)
	}
	if fakeCache.setCalls.Load() != 1 {
		t.Errorf("cache.Set called %d times, want 1 (main path backfill)", fakeCache.setCalls.Load())
	}
}

// TestFeed_CacheHit_EmptySourceNormalizedToES 验证 source 字段上线前写入的
// 缓存项在读取时被归一化为 es（降级结果从不入缓存，故缓存数据必来自 ES）。
func TestFeed_CacheHit_EmptySourceNormalizedToES(t *testing.T) {
	// 模拟旧版缓存 payload：无 source 字段。
	stale := []byte(`{"blind_boxes":[],"suppliers":[],"fetched_at":"2026-10-01T00:00:00Z"}`)
	fakeCache := &fakeCacheClient{
		getFunc: func(ctx context.Context, key string) (string, bool, error) {
			return string(stale), true, nil
		},
	}
	svc := NewFeedService(FeedServiceDeps{
		Search:   &fakeSearchClient{},
		Cache:    fakeCache,
		CacheTTL: 5 * time.Minute,
	})

	feed, err := svc.Feed(newHomeCtx())
	if err != nil {
		t.Fatalf("Feed: %v", err)
	}
	if feed.Source != FeedSourceES {
		t.Errorf("Source = %q, want %q (normalized)", feed.Source, FeedSourceES)
	}
}

// =============================================================================
// ⑤ 主路径超时（spec Requirement "主路径超时约束"）
// =============================================================================

// TestFeedFallback_ESTimeout_DegradesFast 验证 ES 无响应时在超时阈值附近转入降级，
// 且**先于** ES 自己返回。用 2s 的「假死 ES」模拟无响应。
func TestFeedFallback_ESTimeout_DegradesFast(t *testing.T) {
	// 用尊重 context 的假 client（与真实 ES client 行为一致：超时即返回 ctx.Err()）。
	slow := &ctxAwareSearch{delay: 2 * time.Second}
	repo := &fakeFeedRepo{boxes: []mallRepo.FeedBlindBoxRow{{ID: 5, Price: 1.0}}}
	svc := NewFeedService(FeedServiceDeps{Search: slow, FallbackRepo: repo})

	start := time.Now()
	feed, err := svc.Feed(newHomeCtx())
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("Feed: %v", err)
	}
	if feed.Source != FeedSourceMySQL {
		t.Errorf("Source = %q, want %q", feed.Source, FeedSourceMySQL)
	}
	// 两条索引串行，各自 800ms 预算 → 上界略高于 1.6s 但远低于 2s×2。
	if elapsed > 1900*time.Millisecond {
		t.Errorf("elapsed = %v, want <= ~1.6s (short timeout should trigger degradation)", elapsed)
	}
	if elapsed < 700*time.Millisecond {
		t.Errorf("elapsed = %v, too fast — timeout may not have been applied", elapsed)
	}
}

// ctxAwareSearch 是一个尊重 context 超时的 searchClient（模拟真实 ES client）。
type ctxAwareSearch struct {
	delay time.Duration
}

func (s *ctxAwareSearch) Search(ctx context.Context, index string, query []byte) ([]search.Hit, int64, error) {
	select {
	case <-ctx.Done():
		return nil, 0, ctx.Err()
	case <-time.After(s.delay):
		return nil, 0, nil
	}
}
