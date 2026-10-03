package user

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"ai-go-mall/internal/infra/cache"
	"ai-go-mall/internal/infra/search"
)

// =============================================================================
// helpers
// =============================================================================

func newHomeCtx() *gin.Context {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", "/api/v1/user/home/feed", nil)
	return c
}

// fakeSearchClient 是 searchClient 的最小 mock 实现。
//
// search 索引非固定 idx（首字母小写避免冲突）字段存「按索引名 → 函数」的映射，
// 未配置时返回 (nil, 0, nil)，由测试按需精确注入。
type fakeSearchClient struct {
	search func(index string, query []byte) (hits []search.Hit, total int64, err error)

	// 调用索引按指标分组，方便断言「两条都查了」。
	searchCalls []string
}

func (f *fakeSearchClient) Search(ctx context.Context, index string, query []byte) (hits []search.Hit, total int64, err error) {
	f.searchCalls = append(f.searchCalls, index)
	if f.search != nil {
		return f.search(index, query)
	}
	return nil, 0, nil
}

var _ searchClient = (*fakeSearchClient)(nil)

// fakeCacheClient 是 cache.Cache 的最小 mock 实现。
//
// 与 infra/cache/multi_test.go 的 fakeDriver 不同：这里是 Cache 接口层级
// （不在 driver 层），仅记录调用并可注入返回 value / error。
type fakeCacheClient struct {
	mu sync.Mutex

	getFunc func(ctx context.Context, key string) (string, bool, error)
	setFunc func(ctx context.Context, key, value string, ttl time.Duration) error

	getCalls atomic.Int64
	setCalls atomic.Int64
	// lastGetKey / lastSetKey 用于断言"读/写的是正确的 key"。
	lastGetKey string
	lastSetKey string
	lastSetTTL time.Duration
	lastSetVal string
}

var _ cache.Cache = (*fakeCacheClient)(nil)

func (f *fakeCacheClient) Get(ctx context.Context, key string) (string, bool, error) {
	f.getCalls.Add(1)
	f.lastGetKey = key
	if f.getFunc != nil {
		return f.getFunc(ctx, key)
	}
	return "", false, errors.New("cache: miss") // 不精确（Cache 接口无 sentinel），但足以区分走 ES 路径
}

func (f *fakeCacheClient) Set(ctx context.Context, key, value string, ttl time.Duration) error {
	f.setCalls.Add(1)
	f.lastSetKey = key
	f.lastSetTTL = ttl
	f.lastSetVal = value
	if f.setFunc != nil {
		return f.setFunc(ctx, key, value, ttl)
	}
	return nil
}

func (f *fakeCacheClient) Del(ctx context.Context, key string) error { return nil }
func (f *fakeCacheClient) SetNX(ctx context.Context, key, value string, ttl time.Duration) (bool, error) {
	return false, nil
}
func (f *fakeCacheClient) Incr(ctx context.Context, key string) (int64, error) {
	return 0, nil
}
func (f *fakeCacheClient) Decr(ctx context.Context, key string) (int64, error) {
	return 0, nil
}

// 补一个 sync.Mutex 的 import 别名，防止 lint 报错
var _ = sync.Mutex{}

// =============================================================================
// 缓存集成测试（Phase 10 接入 MultiLevelCache）
// =============================================================================

// TestFeed_CacheHit 验证 cache hit 时不打 ES，直接返回缓存。
//
// 验证要点：
//   - 注入 fakeCache 让 Get 返回固定 Feed；
//   - fakeSearch 的 searchCalls 必须为 0（不打 ES）；
//   - 返回的 Feed 与缓存一致。
func TestFeed_CacheHit(t *testing.T) {
	cachedFeed := &Feed{
		BlindBoxes: []BlindBoxFeedItem{
			{ID: 99, SupplierName: "Cached", Name: "from-cache", Price: 1.0},
		},
		Suppliers: []SupplierFeedItem{
			{ID: 50, Name: "Cached Supplier"},
		},
		FetchedAt: time.Now(),
	}
	cachedJSON, _ := json.Marshal(cachedFeed)

	fakeCache := &fakeCacheClient{
		getFunc: func(ctx context.Context, key string) (string, bool, error) {
			return string(cachedJSON), true, nil
		},
	}
	fakeSearch := &fakeSearchClient{}

	svc := NewFeedService(FeedServiceDeps{
		Search:   fakeSearch,
		Cache:    fakeCache,
		CacheTTL: 5 * time.Minute,
	})

	feed, err := svc.Feed(newHomeCtx().Request.Context())
	if err != nil {
		t.Fatalf("Feed: %v", err)
	}
	if len(feed.BlindBoxes) != 1 || feed.BlindBoxes[0].ID != 99 {
		t.Errorf("got %+v, want cached BlindBox id=99", feed.BlindBoxes)
	}
	if len(fakeSearch.searchCalls) != 0 {
		t.Errorf("searchCalls = %v, want 0 (cache hit should skip ES)", fakeSearch.searchCalls)
	}
	if fakeCache.getCalls.Load() != 1 {
		t.Errorf("cache.Get called %d, want 1", fakeCache.getCalls.Load())
	}
}

// TestFeed_CacheMissESFallback 验证 cache miss 时打 ES 并回填 cache。
//
// 验证要点：
//   - fakeCache.Get 返回 error（模拟 miss）；
//   - ES 被调用；
//   - fakeCache.Set 被调用一次（回填），TTL 与 deps.CacheTTL 一致。
func TestFeed_CacheMissESFallback(t *testing.T) {
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

	const wantTTL = 5 * time.Minute
	svc := NewFeedService(FeedServiceDeps{
		Search:   fakeSearch,
		Cache:    fakeCache,
		CacheTTL: wantTTL,
	})

	_, err := svc.Feed(newHomeCtx().Request.Context())
	if err != nil {
		t.Fatalf("Feed: %v", err)
	}
	if len(fakeSearch.searchCalls) != 2 {
		t.Errorf("searchCalls = %v, want 2 (cache miss → ES)", fakeSearch.searchCalls)
	}
	if fakeCache.setCalls.Load() != 1 {
		t.Errorf("cache.Set called %d, want 1 (backfill)", fakeCache.setCalls.Load())
	}
	if !withinJitter(fakeCache.lastSetTTL, wantTTL) {
		t.Errorf("cache.Set TTL = %v, want in jitter range of %v", fakeCache.lastSetTTL, wantTTL)
	}
	if fakeCache.lastSetKey != homeFeedCacheKey {
		t.Errorf("cache.Set key = %q, want %q", fakeCache.lastSetKey, homeFeedCacheKey)
	}
}

// TestFeed_NoCache 验证 nil cache 时直走 ES，不报错。
//
// 主要用于生产环境 cache 未初始化的兜底场景（应 fail-open）。
func TestFeed_NoCache(t *testing.T) {
	fakeSearch := &fakeSearchClient{
		search: func(index string, query []byte) ([]search.Hit, int64, error) {
			return nil, 0, nil
		},
	}
	svc := NewFeedService(FeedServiceDeps{
		Search:   fakeSearch,
		Cache:    nil, // 显式 nil
		CacheTTL: 5 * time.Minute,
	})
	_, err := svc.Feed(newHomeCtx().Request.Context())
	if err != nil {
		t.Errorf("Feed with nil cache: %v, want nil", err)
	}
	if len(fakeSearch.searchCalls) != 2 {
		t.Errorf("searchCalls = %v, want 2", fakeSearch.searchCalls)
	}
}

// TestFeed_CacheTTLZero_NoBackfill 验证 CacheTTL=0 时不回填 cache（避免永驻）。
//
// 设计 D5：限流等场景需要 CacheTTL=0（永驻）但由专用路径处理；Feed 路径
// 应拒绝 0 TTL，防止误用导致 stale 数据。
func TestFeed_CacheTTLZero_NoBackfill(t *testing.T) {
	fakeCache := &fakeCacheClient{}
	fakeSearch := &fakeSearchClient{
		search: func(index string, query []byte) ([]search.Hit, int64, error) {
			return nil, 0, nil
		},
	}
	svc := NewFeedService(FeedServiceDeps{
		Search:   fakeSearch,
		Cache:    fakeCache,
		CacheTTL: 0, // 显式 0
	})
	_, _ = svc.Feed(newHomeCtx().Request.Context())
	if fakeCache.setCalls.Load() != 0 {
		t.Errorf("cache.Set called %d, want 0 (TTL=0 should skip backfill)", fakeCache.setCalls.Load())
	}
}

// =============================================================================
// 测试用例
// =============================================================================

func newFeedSvc(s searchClient) FeedService {
	return NewFeedService(FeedServiceDeps{Search: s})
}

func TestFeed_HappyPath(t *testing.T) {
	bb1JSON := []byte(`{
		"id": 1,
		"supplier_id": 10,
		"supplier_name": "Panini 旗舰店",
		"name": "2024 NBA 球星卡盲盒",
		"cover": "https://cdn.example.com/cover.jpg",
		"price": 99.0,
		"promo_price": 79.0,
		"rarity_summary": "SSR 3% / SR 12% / R 35% / N 50%",
		"hot_score": 95,
		"created_at": "2026-09-30T10:00:00Z"
	}`)
	bb2JSON := []byte(`{
		"id": 2,
		"supplier_id": 20,
		"supplier_name": "Topps 旗舰店",
		"name": "2024 欧冠球星卡盲盒",
		"cover": "https://cdn.example.com/cover2.jpg",
		"price": 120.0,
		"rarity_summary": "SSR 5% / SR 15% / R 35% / N 45%",
		"hot_score": 88,
		"created_at": "2026-09-29T08:00:00Z"
	}`)
	supJSON := []byte(`{
		"id": 10,
		"name": "Panini 旗舰店",
		"logo": "https://cdn.example.com/panini.png",
		"blind_box_count": 12,
		"featured_rank": 1
	}`)

	fake := &fakeSearchClient{
		search: func(index string, query []byte) (hits []search.Hit, total int64, err error) {
			switch index {
			case BlindBoxIndex:
				return []search.Hit{
					{ID: "1", Score: 95, Source: bb1JSON},
					{ID: "2", Score: 88, Source: bb2JSON},
				}, 2, nil
			case SupplierIndex:
				return []search.Hit{
					{ID: "10", Score: 1, Source: supJSON},
				}, 1, nil
			default:
				return nil, 0, errors.New("fake: unknown index " + index)
			}
		},
	}
	svc := newFeedSvc(fake)
	feed, err := svc.Feed(newHomeCtx().Request.Context())
	if err != nil {
		t.Fatalf("Feed: %v", err)
	}
	if feed == nil {
		t.Fatalf("Feed returned nil")
	}
	if len(feed.BlindBoxes) != 2 {
		t.Fatalf("len(BlindBoxes) = %d, want 2", len(feed.BlindBoxes))
	}
	if feed.BlindBoxes[0].ID != 1 || feed.BlindBoxes[0].SupplierName != "Panini 旗舰店" {
		t.Errorf("BlindBoxes[0] = %+v", feed.BlindBoxes[0])
	}
	if feed.BlindBoxes[0].PromoPrice == nil || *feed.BlindBoxes[0].PromoPrice != 79.0 {
		t.Errorf("BlindBoxes[0].PromoPrice = %v, want 79.0", feed.BlindBoxes[0].PromoPrice)
	}
	if feed.BlindBoxes[1].PromoPrice != nil {
		t.Errorf("BlindBoxes[1].PromoPrice = %v, want nil", feed.BlindBoxes[1].PromoPrice)
	}
	if len(feed.Suppliers) != 1 {
		t.Fatalf("len(Suppliers) = %d, want 1", len(feed.Suppliers))
	}
	if feed.Suppliers[0].ID != 10 || feed.Suppliers[0].BlindBoxCount != 12 {
		t.Errorf("Suppliers[0] = %+v", feed.Suppliers[0])
	}
	if feed.FetchedAt.IsZero() {
		t.Errorf("FetchedAt not set")
	}
	// 两条索引都查了。
	if len(fake.searchCalls) != 2 {
		t.Errorf("searchCalls = %v, want 2 entries", fake.searchCalls)
	}
}

func TestFeed_EmptyResults(t *testing.T) {
	fake := &fakeSearchClient{
		search: func(index string, query []byte) (hits []search.Hit, total int64, err error) {
			return nil, 0, nil
		},
	}
	svc := newFeedSvc(fake)
	feed, err := svc.Feed(newHomeCtx().Request.Context())
	if err != nil {
		t.Fatalf("Feed: %v", err)
	}
	if feed == nil {
		t.Fatalf("Feed returned nil")
	}
	if len(feed.BlindBoxes) != 0 {
		t.Errorf("len(BlindBoxes) = %d, want 0", len(feed.BlindBoxes))
	}
	if len(feed.Suppliers) != 0 {
		t.Errorf("len(Suppliers) = %d, want 0", len(feed.Suppliers))
	}
	// 空数组 vs nil：JSON 序列化语义不同，统一用 [] 而非 nil。
	if feed.BlindBoxes == nil {
		t.Errorf("BlindBoxes = nil, want [] (empty slice)")
	}
}

func TestFeed_BlindBoxSearchError(t *testing.T) {
	wantErr := errors.New("es: connection refused")
	fake := &fakeSearchClient{
		search: func(index string, query []byte) (hits []search.Hit, total int64, err error) {
			if index == BlindBoxIndex {
				return nil, 0, wantErr
			}
			return nil, 0, nil
		},
	}
	svc := newFeedSvc(fake)
	_, err := svc.Feed(newHomeCtx().Request.Context())
	if err == nil {
		t.Fatalf("Feed err = nil, want error")
	}
	if !errors.Is(err, ErrFeedSearchUnavailable) {
		t.Errorf("err = %v, want wraps ErrFeedSearchUnavailable", err)
	}
	if !errors.Is(err, wantErr) {
		t.Errorf("err = %v, want wraps %v", err, wantErr)
	}
}

func TestFeed_SupplierSearchError(t *testing.T) {
	wantErr := errors.New("es: supplier index 404")
	fake := &fakeSearchClient{
		search: func(index string, query []byte) (hits []search.Hit, total int64, err error) {
			if index == SupplierIndex {
				return nil, 0, wantErr
			}
			return nil, 0, nil
		},
	}
	svc := newFeedSvc(fake)
	_, err := svc.Feed(newHomeCtx().Request.Context())
	if err == nil {
		t.Fatalf("Feed err = nil, want error")
	}
	if !errors.Is(err, ErrFeedSearchUnavailable) {
		t.Errorf("err = %v, want wraps ErrFeedSearchUnavailable", err)
	}
	if !errors.Is(err, wantErr) {
		t.Errorf("err = %v, want wraps %v", err, wantErr)
	}
}

// TestFeed_BadJSON_ReturnsUnavailable 验证 ES 返回损坏文档 → 映射为不可用。
//
// 损坏的 schema 意味着 cmd/es-sync 与 reader schema 漂移，必须返 500 让运维介入；
// 不能静默吞掉（拼错 schema 时仍然给「首页 200 但内容空」会让客户端裸奔）。
func TestFeed_BadJSON_ReturnsUnavailable(t *testing.T) {
	fake := &fakeSearchClient{
		search: func(index string, query []byte) (hits []search.Hit, total int64, err error) {
			if index == BlindBoxIndex {
				return []search.Hit{
					{ID: "1", Source: []byte(`{"id": 1, "price": "not a float"`)}, // 截断 JSON
				}, 1, nil
			}
			return nil, 0, nil
		},
	}
	svc := newFeedSvc(fake)
	_, err := svc.Feed(newHomeCtx().Request.Context())
	if err == nil {
		t.Fatalf("Feed err = nil, want error on bad JSON")
	}
	if !errors.Is(err, ErrFeedSearchUnavailable) {
		t.Errorf("err = %v, want wraps ErrFeedSearchUnavailable", err)
	}
}

// TestFeed_UsesCorrectIndexNames 验证两条 ES 索引调用的是正确索引。
func TestFeed_UsesCorrectIndexNames(t *testing.T) {
	fake := &fakeSearchClient{
		search: func(index string, query []byte) (hits []search.Hit, total int64, err error) {
			return nil, 0, nil
		},
	}
	svc := newFeedSvc(fake)
	_, _ = svc.Feed(newHomeCtx().Request.Context())

	if len(fake.searchCalls) != 2 {
		t.Fatalf("searchCalls = %v, want 2", fake.searchCalls)
	}
	if fake.searchCalls[0] != BlindBoxIndex {
		t.Errorf("first call index = %q, want %q", fake.searchCalls[0], BlindBoxIndex)
	}
	if fake.searchCalls[1] != SupplierIndex {
		t.Errorf("second call index = %q, want %q", fake.searchCalls[1], SupplierIndex)
	}
}

// TestFeed_QueriesContainCorrectSort 验证查询 DSL 包含正确的 sort 字段（防 §5.2 配置漂移）。
func TestFeed_QueriesContainCorrectSort(t *testing.T) {
	var bbQuery, supQuery []byte
	fake := &fakeSearchClient{
		search: func(index string, query []byte) (hits []search.Hit, total int64, err error) {
			if index == BlindBoxIndex {
				bbQuery = append([]byte(nil), query...)
			} else if index == SupplierIndex {
				supQuery = append([]byte(nil), query...)
			}
			return nil, 0, nil
		},
	}
	svc := newFeedSvc(fake)
	_, _ = svc.Feed(newHomeCtx().Request.Context())

	// 盲盒 sort: hot_score desc, created_at desc
	for _, want := range []string{`"hot_score": "desc"`, `"created_at": "desc"`} {
		if !contains(bbQuery, want) {
			t.Errorf("blindbox query missing %s\nquery: %s", want, string(bbQuery))
		}
	}
	// 供应商 sort: featured_rank asc, blind_box_count desc
	for _, want := range []string{`"featured_rank": "asc"`, `"blind_box_count": "desc"`} {
		if !contains(supQuery, want) {
			t.Errorf("supplier query missing %s\nquery: %s", want, string(supQuery))
		}
	}
}

// contains 是标准库 bytes.Contains 的薄包装（让测试断言更可读）。
func contains(haystack []byte, needle string) bool {
	return bytes.Contains(haystack, []byte(needle))
}
