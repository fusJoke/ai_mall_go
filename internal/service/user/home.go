// Package user — home.go 实现 C 端首页 Feed（spec Requirement "Home feed via Elasticsearch"）。
//
// 业务规则（严格照搬 spec）：
//   - 索引 `mall_blind_box_index`：已由 cmd/es-sync 按
//     (status=active AND on_sale=true AND is_featured=true) 预过滤；
//     Feed 只需按 (hot_score DESC, created_at DESC) 排序读取。
//   - 索引 `mall_supplier_index`：已由 cmd/es-sync 按
//     (status=active AND is_featured=true) 预过滤；
//     Feed 只需按 (featured_rank ASC, blind_box_count DESC) 排序读取。
//   - ES 不可用 → 透传 error，由 handler 映射为 500 + code=search_unavailable，
//     不 fallback 到 MySQL（spec 明确「不查 MySQL 兜底」）。
//
// 缓存策略（Phase 10）：
//   - 多层缓存：MultiLevelCache（L1 内存 + L2 Redis），TTL 由 config.Get().Cache.TTL.HomeFeed 决定。
//   - Feed 是高读低写场景（每用户首页每次访问都打 1 次），缓存命中后省掉 ES 请求。
//   - 失效方式：TTL 自然过期（feed 内容随盲盒上下架变化，由 cmd/es-sync 重写）。
package user

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/gin-gonic/gin"

	"ai-go-mall/internal/infra/cache"
	"ai-go-mall/internal/infra/cache/hotspot"
	"ai-go-mall/internal/infra/search"
)

// 索引 / ES 排序常量。
//
// 索引名按 design.md D6 / D7 的约定，硬编码在 service 层（与 cmd/es-sync 一致）；
// 不引入 IndexPrefix 运行时拼接，避免跨模块配置漂移。
const (
	// BlindBoxIndex 盲盒 ES 索引名（按 es-sync 写入端约定）。
	BlindBoxIndex = "mall_blind_box_index"

	// SupplierIndex 供应商 ES 索引名（按 es-sync 写入端约定）。
	SupplierIndex = "mall_supplier_index"

	// feedSize 单次 ES 查询最多拉多少条；首页 Feed 上限。MVP 阶段 50 足够。
	feedSize = 50

	// homeFeedCacheKey 首页 feed 缓存 key（MVP：单一 key，未来按 query sig 分桶）。
	// 设计 D5 场景表中 `home:feed:{query_sig}` 的 MVP 简化版。
	homeFeedCacheKey = "home:feed:v1"
)

// ES 查询 DSL（按文档顺序排序）。
//
// 索引内容已预过滤（见 spec Requirement "ES sync script"），所以 query 阶段
// 只 match_all + sort 即可；过滤规则下推到 cmd/es-sync 写入端。
var (
	blindBoxFeedQuery = fmt.Appendf(nil, `{
		"query": {"match_all": {}},
		"sort": [
			{"hot_score": "desc"},
			{"created_at": "desc"}
		],
		"size": %d
	}`, feedSize)

	supplierFeedQuery = fmt.Appendf(nil, `{
		"query": {"match_all": {}},
		"sort": [
			{"featured_rank": "asc"},
			{"blind_box_count": "desc"}
		],
		"size": %d
	}`, feedSize)
)

// ErrFeedSearchUnavailable Feed 搜索不可用（ES 故障 / 超时）。
//
// handler 层映射为 500 + code=search_unavailable（spec 不 fallback MySQL）。
var ErrFeedSearchUnavailable = errors.New("user.home: feed search unavailable")

// BlindBoxFeedItem 是首页 Feed 中的盲盒条目（ES 文档反序列化形态）。
//
// 字段命名遵循 dto 风格（snake_case JSON），handler 层直接序列化即可。
// 与 design.md D7 的 ES 投影结构一一对应。
type BlindBoxFeedItem struct {
	ID            int64     `json:"id"`
	SupplierID    int64     `json:"supplier_id"`
	SupplierName  string    `json:"supplier_name"`
	Name          string    `json:"name"`
	Cover         string    `json:"cover"`
	Price         float64   `json:"price"`
	PromoPrice    *float64  `json:"promo_price,omitempty"`
	RaritySummary string    `json:"rarity_summary"`
	HotScore      int       `json:"hot_score"`
	CreatedAt     time.Time `json:"created_at"`
}

// SupplierFeedItem 是首页 Feed 中的供应商条目（ES 文档反序列化形态）。
type SupplierFeedItem struct {
	ID            int64  `json:"id"`
	Name          string `json:"name"`
	Logo          string `json:"logo"`
	BlindBoxCount int    `json:"blind_box_count"`
	FeaturedRank  int    `json:"featured_rank"`
}

// Feed 是首页 Feed 聚合结果。
//
// BlindBoxes + Suppliers 各自独立：客户端可按需渲染。
type Feed struct {
	// BlindBoxes 推荐盲盒列表（按 hot_score DESC, created_at DESC）。
	BlindBoxes []BlindBoxFeedItem `json:"blind_boxes"`

	// Suppliers 推荐供应商列表（按 featured_rank ASC, blind_box_count DESC）。
	Suppliers []SupplierFeedItem `json:"suppliers"`

	// FetchedAt 拉取时间戳（便于调试 + 客户端缓存判断）。
	FetchedAt time.Time `json:"fetched_at"`
}

// searchClient 是 Home / Feed 用的搜索子集接口（search.SearchClient 子集）。
//
// 取子集而非整接口，便于单测 mock（避免实现 BulkIndex / Index / Delete）。
type searchClient interface {
	Search(ctx context.Context, index string, query []byte) (hits []search.Hit, total int64, err error)
}

// FeedService 是首页 Feed 业务接口。
type FeedService interface {
	// Feed 拉首页 Feed（盲盒 + 供应商）。
	//
	// 缓存语义：
	//   - 命中 cache：直接反序列化返回，不打 ES。
	//   - miss：拉 ES 后回填 cache（TTL 走 config.Cache.TTL.HomeFeed.L2）。
	//
	// 失败语义：ES 不可用 → 返回 error（含 ErrFeedSearchUnavailable 包装），
	// 由 handler 映射为 500 + code=search_unavailable。
	Feed(ctx context.Context) (*Feed, error)
}

// FeedServiceDeps 注入 FeedService 所需的依赖。
type FeedServiceDeps struct {
	// Search ES 客户端接口（search.SearchClient 满足；测试时注入 fake）。
	Search searchClient

	// Hotspot 热点缓存（D21）：Feed 优先走它（永驻 + 逻辑过期 60s 异步刷新）。
	// nil 时回落 Cache 的 TTL 缓存路径（单测 / 未装配场景）。
	Hotspot hotspot.HotspotCache

	// Cache 缓存接口（cache.Cache 满足；测试时注入 fake）。
	// nil 时跳过缓存（直走 ES）—— 便于单测聚焦 ES 行为。
	Cache cache.Cache
	// CacheTTL feed 缓存 TTL（业务侧显式传入，避免 service 直接读 config 包）。
	CacheTTL time.Duration
}

// baseFeedService 是 FeedService 的默认实现。
type baseFeedService struct {
	search   searchClient
	hotspot  hotspot.HotspotCache
	cache    cache.Cache
	cacheTTL time.Duration
}

// NewFeedService 接收依赖，返回 FeedService 接口。
func NewFeedService(d FeedServiceDeps) FeedService {
	return &baseFeedService{
		search:   d.Search,
		hotspot:  d.Hotspot,
		cache:    d.Cache,
		cacheTTL: d.CacheTTL,
	}
}

// Feed 实现见 FeedService 注释。
//
// 实现要点：
//  1. 缓存命中直接反序列化返回（不打 ES）；
//  2. 缓存 miss / 反序列化失败 → 拉 ES（盲盒 + 供应商两条索引）；
//  3. ES 成功后回填 cache（ttl = deps.CacheTTL，0 时不设过期）；
//  4. 反序列化失败 → 整体返回 ErrFeedSearchUnavailable（避免混入损坏数据）；
//  5. ES 返回 0 条 → 正常返回空数组，nil err。
func (s *baseFeedService) Feed(ctx context.Context) (*Feed, error) {
	// 0) 热点缓存路径（D21）：永驻 + 逻辑过期 60s 异步刷新；loader 走 ES。
	if s.hotspot != nil {
		return hotspot.Load(ctx, s.hotspot, hotspot.KeyHomeFeed,
			func(ctx context.Context, _ string) (*Feed, error) {
				return s.loadFeed(ctx)
			}, hotspot.HomeFeedStaleAfter)
	}

	// 1) TTL 缓存路径（Hotspot 未装配时），见 feedFromCache。
	return s.feedFromCache(ctx)
}

// feedFromCache 是 TTL 缓存路径（Hotspot 未装配时）：命中直返 / miss 拉 ES / 回填。
func (s *baseFeedService) feedFromCache(ctx context.Context) (*Feed, error) {
	if s.cache != nil {
		// 三态 Get（17.1）：feed 的 loader（loadFeed）永不返回 ErrNotFound——
		// ES 查不到是正常空数组而非「key 不存在」，因此 feed 层不写也不读
		// notFound 占位（17.6）；!found && err==nil 在本路径按 miss 处理。
		if raw, found, err := s.cache.Get(ctx, homeFeedCacheKey); err == nil && found && raw != "" {
			var cached Feed
			if jerr := json.Unmarshal([]byte(raw), &cached); jerr == nil {
				return &cached, nil
			}
			// 反序列化失败：当作 miss，降级到 ES。
		}
	}

	feed, err := s.loadFeed(ctx)
	if err != nil {
		return nil, err
	}

	// 回填缓存（失败不阻断主流程 —— 缓存是优化，不是正确性）。
	if s.cache != nil && s.cacheTTL > 0 {
		if payload, jerr := json.Marshal(feed); jerr == nil {
			// D22：TTL 加 ±10% 抖动，避免大批 key 同时到期导致 cache stampede。
			ttl := cache.JitterTTL(s.cacheTTL)
			_ = s.cache.Set(ctx, homeFeedCacheKey, string(payload), ttl)
		}
	}

	return feed, nil
}

// loadFeed 拉 ES（盲盒 + 供应商两条索引）组装 Feed；Hotspot loader 与直查路径共用。
func (s *baseFeedService) loadFeed(ctx context.Context) (*Feed, error) {
	// 拉盲盒 Feed。
	bbHits, _, err := s.search.Search(ctx, BlindBoxIndex, blindBoxFeedQuery)
	if err != nil {
		return nil, wrapSearchErr("blind_box index", err)
	}
	blindBoxes, err := decodeHits[BlindBoxFeedItem](bbHits)
	if err != nil {
		return nil, wrapSearchErr("decode blind_box items", err)
	}

	// 拉供应商 Feed。
	supHits, _, err := s.search.Search(ctx, SupplierIndex, supplierFeedQuery)
	if err != nil {
		return nil, wrapSearchErr("supplier index", err)
	}
	suppliers, err := decodeHits[SupplierFeedItem](supHits)
	if err != nil {
		return nil, wrapSearchErr("decode supplier items", err)
	}

	return &Feed{
		BlindBoxes: blindBoxes,
		Suppliers:  suppliers,
		FetchedAt:  time.Now(),
	}, nil
}

// wrapSearchErr 把任意子错误包成「ErrFeedSearchUnavailable + context + 原 error」，
// 满足 handler 侧的 errors.Is(ErrFeedSearchUnavailable) 与排错源的 errors.Is(原 err)
// 同时成立（用 errors.Join 同时挂两个 wraps；Go 1.20+）。
func wrapSearchErr(ctx string, cause error) error {
	return fmt.Errorf("%s: %w", ctx, errors.Join(ErrFeedSearchUnavailable, cause))
}

// decodeHits 把 search.Hit[] 反序列化成目标 DTO 切片。
//
// 容错策略：任何一条反序列化失败都返 error，由 Feed 整体映射为 500。
// 为什么不跳过错位文档：首页 Feed 是受信任的 ES 投影（由 cmd/es-sync 全量写入），
// 反序列化失败说明 schema 已漂移 —— 是配置/部署问题，不是单条数据问题。
func decodeHits[T any](hits []search.Hit) ([]T, error) {
	if len(hits) == 0 {
		return []T{}, nil
	}
	out := make([]T, 0, len(hits))
	for _, h := range hits {
		var item T
		if err := json.Unmarshal(h.Source, &item); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, nil
}

// 编译期断言：baseFeedService 必须实现 FeedService。
var _ FeedService = (*baseFeedService)(nil)

// 避免 gin 被 unused 编译器警告（handler 阶段会用到）。
var _ = (*gin.Context)(nil)
