// Package user — home.go 实现 C 端首页 Feed（spec Requirement "Home feed via Elasticsearch"
// ＋ openspec change `add-uni-h5-client` 的 capability `home-feed-fallback`）。
//
// 业务规则（严格照搬 spec）：
//   - 索引 `mall_blind_box_index`：已由 cmd/es-sync 按
//     (status=active AND on_sale=true AND lifecycle='selling'，且供应商 status=active)
//     预过滤；Feed 只需按 (hot_score DESC, created_at DESC) 排序读取。
//   - 索引 `mall_supplier_index`：已由 cmd/es-sync 按
//     (status=active AND is_featured=true) 预过滤；
//     Feed 只需按 (featured_rank ASC, blind_box_count DESC) 排序读取。
//
// 降级策略（本文件本期最重要的改动）：
//   - ES 查询报错 / 超时 / 反序列化失败（schema 漂移）→ 回落 MySQL 直查，
//     查询口径与 cmd/es-sync 逐字一致（见 repository/mall/feed_repository.go）；
//   - ES 正常返回但命中 0 条 → **不降级**（0 条是合法业务结果）；
//   - 降级结果与主路径 DTO 完全同构，客户端无感知；
//   - 降级结果**不写任何 Feed 缓存**（含 hotspot 与 TTL 缓存）—— 避免 ES
//     恢复后用户仍长时间看到降级期间的数据（spec「缓存回填约束」）；
//   - ES 与 MySQL 均失败 → 仍返回 ErrFeedSearchUnavailable（handler → 500
//     + code=search_unavailable）。
//
// 缓存策略（Phase 10 + D21）：
//   - 首选 Hotspot（永驻 + 逻辑过期 60s 异步刷新）；
//   - Hotspot 未装配时回落 MultiLevelCache（L1 内存 + L2 Redis），
//     TTL 由 config.Get().Cache.TTL.HomeFeed 决定；
//   - 两条缓存路径都只承载 ES 主路径结果；降级路径完全绕开缓存。
package user

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"ai-go-mall/internal/infra/cache"
	"ai-go-mall/internal/infra/cache/hotspot"
	"ai-go-mall/internal/infra/search"
	"ai-go-mall/internal/model/mall"
	mallRepo "ai-go-mall/internal/repository/mall"
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

// 降级可观测性常量（spec Requirement "降级可观测性"）。
const (
	// FeedSourceHeader 是标识 Feed 数据来源的响应头名。
	//
	// 取值见 FeedSourceES / FeedSourceMySQL。**仅供排障与灰度观察**，
	// 客户端 MUST NOT 依赖它决定渲染逻辑（spec 明确约束）。
	FeedSourceHeader = "X-Feed-Source"

	// FeedSourceES 主路径（Elasticsearch）。
	FeedSourceES = "es"

	// FeedSourceMySQL 降级路径（MySQL 直查）。
	FeedSourceMySQL = "mysql"
)

// esSearchTimeout 是主检索路径（ES）的单次查询超时。
//
// spec 要求「短超时（建议量级 800 毫秒）」，保证 ES 故障时能及时转入降级，
// 而不是让用户请求挂在 TCP 重试上。两条索引各自持有该预算（串行执行时
// 最坏 1.6s，仍是可接受的端到端耗时）。
const esSearchTimeout = 800 * time.Millisecond


// ES 查询 DSL（按文档顺序排序）。
//
// 双层防御（v2 商品即批次）：
//   - 写侧：cmd/es-sync 只收录 status=active AND on_sale AND lifecycle='selling'；
//   - 读侧：本 DSL 再对 lifecycle 做 term filter —— 即便写侧漏过（增量同步延迟、
//     旧索引残留），售罄/已结算商品也不会出现在 C 端 feed。
//     （用户点进去才发现「已售罄」是差体验，读时防线让隐藏准实时生效。）
var (
	blindBoxFeedQuery = fmt.Appendf(nil, `{
		"query": {
			"bool": {
				"filter": [
					{"term": {"lifecycle": "selling"}}
				]
			}
		},
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

// ErrFeedSearchUnavailable Feed 搜索不可用。
//
// 语义：**ES 与 MySQL 降级查询均失败**时返回（spec Scenario "MySQL 亦不可用"），
// handler 层映射为 500 + code=search_unavailable。
//
// 注意：ES 单侧失败不再直接导致该错误 —— 会先尝试 MySQL 降级
// （见 feedFromMySQL）。本 sentinel 同时被 wrapSearchErr 挂在 ES 侧错误上，
// 因此降级不可用（FallbackRepo 未装配）时仍会原样上抛。
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
	// 降级路径下为本次降级查询的时刻。
	FetchedAt time.Time `json:"fetched_at"`

	// Source 数据来源标识：FeedSourceES / FeedSourceMySQL。
	//
	// handler 据此设置 FeedSourceHeader 响应头。**主路径与降级路径都会填充**
	// 该字段，因此两条路径的字段集合仍然完全一致（spec「结构同构」要求）。
	// 客户端 MUST NOT 依赖它做渲染分支。
	Source string `json:"source"`
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
	// 降级语义：
	//   - ES 报错 / 超时 / 反序列化失败 → 回落 MySQL（同构 DTO，不写缓存）；
	//   - ES 正常返回 0 条 → 正常返回空数组，**不降级**；
	//   - ES 与 MySQL 均失败 → 返回 error（含 ErrFeedSearchUnavailable 包装），
	//     由 handler 映射为 500 + code=search_unavailable。
	//
	// 参数为 *gin.Context（而非 context.Context）：降级路径需要通过
	// repository.DB(c) 取请求作用域的 *gorm.DB（由 DBMiddleware 注入），
	// 与项目内其他 C 端 service 方法保持一致。
	Feed(c *gin.Context) (*Feed, error)
}

// FeedServiceDeps 注入 FeedService 所需的依赖。
type FeedServiceDeps struct {
	// Search ES 客户端接口（search.SearchClient 满足；测试时注入 fake）。
	Search searchClient

	// FallbackRepo 降级路径的 MySQL 数据源。
	//
	// nil = 关闭降级（保持历史语义：ES 失败即返回 error）。
	// 单测聚焦 ES 行为、或未装配 DB 的场景（如纯缓存服务）可留空。
	FallbackRepo mallRepo.FeedRepository

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
	fallback mallRepo.FeedRepository
	hotspot  hotspot.HotspotCache
	cache    cache.Cache
	cacheTTL time.Duration
}

// NewFeedService 接收依赖，返回 FeedService 接口。
func NewFeedService(d FeedServiceDeps) FeedService {
	return &baseFeedService{
		search:   d.Search,
		fallback: d.FallbackRepo,
		hotspot:  d.Hotspot,
		cache:    d.Cache,
		cacheTTL: d.CacheTTL,
	}
}

// Feed 实现见 FeedService 注释。
//
// 实现要点：
//  1. 热点缓存路径：命中直接返回（含逻辑过期后的旧值）；冷启动 loader 失败
//     说明 ES 不可用 → 转降级；
//  2. TTL 缓存路径：命中直接返回；miss → 拉 ES；ES 失败 → 转降级；
//  3. 降级结果**不写任何缓存**（spec「降级结果的缓存回填约束」）；
//  4. 反序列化失败（schema 漂移）→ 同样转降级（ES 数据不可信）；
//  5. ES 返回 0 条 → 正常返回空数组，nil err，**不降级**。
func (s *baseFeedService) Feed(c *gin.Context) (*Feed, error) {
	// 0) 热点缓存路径（D21）：永驻 + 逻辑过期 60s 异步刷新；loader 走 ES。
	if s.hotspot != nil {
		feed, err := hotspot.Load(c, s.hotspot, hotspot.KeyHomeFeed,
			func(ctx context.Context, _ string) (*Feed, error) {
				// 注意：此处 ctx 可能是 context.WithoutCancel 包装（异步刷新路径），
				// 因此只能做不依赖 gin.Context 的事（ES 查询）。降级路径需要
				// gin.Context 取 DB，故刻意放在 loadFeed 之外、由本函数同步触发。
				return s.loadFeed(ctx)
			}, hotspot.HomeFeedStaleAfter)
		if err == nil {
			// 命中路径可能反序列化出「source 字段上线前」写入的旧缓存（空串），
			// 统一归一化为 es（降级结果从不入缓存，缓存里必有 ES 数据）。
			normalizeFeedSource(feed)
			return feed, nil
		}
		// 冷启动 loader 失败 → ES 不可用 → 同步降级（此时仍在请求路径上，
		// c 有效；降级结果直接返回，不写 hotspot —— 见文件头「降级策略」）。
		return s.feedFromMySQL(c, err)
	}

	// 1) TTL 缓存路径（Hotspot 未装配时），见 feedFromCache。
	return s.feedFromCache(c)
}

// feedFromCache 是 TTL 缓存路径（Hotspot 未装配时）：命中直返 / miss 拉 ES / 回填。
//
// 降级语义：miss 后 loadFeed 失败 → 走 MySQL 降级，且**不回填** TTL 缓存
// （spec：降级结果 MUST NOT 按主路径正常 TTL 回填）。
func (s *baseFeedService) feedFromCache(c *gin.Context) (*Feed, error) {
	if s.cache != nil {
		// 三态 Get（17.1）：feed 的 loader（loadFeed）永不返回 ErrNotFound——
		// ES 查不到是正常空数组而非「key 不存在」，因此 feed 层不写也不读
		// notFound 占位（17.6）；!found && err==nil 在本路径按 miss 处理。
		if raw, found, err := s.cache.Get(c, homeFeedCacheKey); err == nil && found && raw != "" {
			var cached Feed
			if jerr := json.Unmarshal([]byte(raw), &cached); jerr == nil {
				normalizeFeedSource(&cached)
				return &cached, nil
			}
			// 反序列化失败：当作 miss，降级到 ES。
		}
	}

	feed, err := s.loadFeed(c)
	if err != nil {
		return s.feedFromMySQL(c, err)
	}

	// 回填缓存（失败不阻断主流程 —— 缓存是优化，不是正确性）。
	if s.cache != nil && s.cacheTTL > 0 {
		if payload, jerr := json.Marshal(feed); jerr == nil {
			// D22：TTL 加 ±10% 抖动，避免大批 key 同时到期导致 cache stampede。
			ttl := cache.JitterTTL(s.cacheTTL)
			_ = s.cache.Set(c, homeFeedCacheKey, string(payload), ttl)
		}
	}

	return feed, nil
}

// normalizeFeedSource 补齐缓存中缺失的 source 字段。
//
// 场景：缓存项的 value 由「source 字段上线前」的版本写入（无该字段），
// 反序列化后 Source 为空串。此时按 ES 处理 —— 降级结果从不入缓存，
// 所以缓存里能出现的数据必然来自 ES 主路径。
func normalizeFeedSource(feed *Feed) {
	if feed != nil && feed.Source == "" {
		feed.Source = FeedSourceES
	}
}

// loadFeed 拉 ES（盲盒 + 供应商两条索引）组装 Feed；Hotspot loader 与直查路径共用。
//
// 主路径短超时（spec「主路径超时约束」）：两条查询各自套 800ms 预算，
// 超时即返回 error，由上层转入降级，MUST NOT 让用户请求长时间挂在 ES 上。
func (s *baseFeedService) loadFeed(ctx context.Context) (*Feed, error) {
	// 拉盲盒 Feed。
	bbCtx, bbCancel := context.WithTimeout(ctx, esSearchTimeout)
	bbHits, _, err := s.search.Search(bbCtx, BlindBoxIndex, blindBoxFeedQuery)
	bbCancel()
	if err != nil {
		return nil, wrapSearchErr("blind_box index", err)
	}
	blindBoxes, err := decodeHits[BlindBoxFeedItem](bbHits)
	if err != nil {
		return nil, wrapSearchErr("decode blind_box items", err)
	}

	// 拉供应商 Feed。
	supCtx, supCancel := context.WithTimeout(ctx, esSearchTimeout)
	supHits, _, err := s.search.Search(supCtx, SupplierIndex, supplierFeedQuery)
	supCancel()
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
		Source:     FeedSourceES,
	}, nil
}

// feedFromMySQL 是 ES 主路径失败后的降级通道（spec Requirement "Feed 降级触发条件"）。
//
// 触发条件（由调用方保证）：loadFeed 返回 error —— 即 ES 报错 / 超时 /
// 反序列化失败。ES 正常返回 0 条不会走到这里（0 条不是 error）。
//
// 行为：
//   - FallbackRepo 未装配 → 保持历史语义，透传原 error；
//   - MySQL 查询失败 → 透传原 error（双层失败 → handler 映射 500
//     search_unavailable，spec Scenario "MySQL 亦不可用"）；
//   - MySQL 成功 → 组装与主路径**同构**的 Feed，Source=mysql，并记录 WARN 日志。
//
// 缓存：本函数**不写任何缓存**（hotspot / TTL 缓存都不写），满足 spec
// 「降级结果 MUST NOT 按主路径的正常 TTL 回填」。
func (s *baseFeedService) feedFromMySQL(c *gin.Context, cause error) (*Feed, error) {
	if s.fallback == nil {
		return nil, cause
	}

	start := time.Now()
	feed, err := s.buildFeedFromMySQL(c)
	if err != nil {
		// 双层失败：原样上抛 ES 侧错误（保留 ErrFeedSearchUnavailable 语义），
		// 同时把 MySQL 侧失败原因一并挂上，便于排障。
		log.Printf("level=WARN msg=%q es_err=%q mysql_err=%q",
			"user.home: feed fallback to mysql failed", cause.Error(), err.Error())
		return nil, errors.Join(cause, err)
	}
	elapsed := time.Since(start)

	// spec「降级可观测性」：WARN 日志 MUST 含触发原因 + 失败索引名 + 耗时。
	// 触发原因（cause）由 wrapSearchErr 携带索引名（"blind_box index" /
	// "supplier index"）与底层错误，可直接透出。
	log.Printf("level=WARN msg=%q trigger=%q blind_boxes=%d suppliers=%d elapsed_ms=%d source=%s",
		"user.home: feed degraded to mysql", cause.Error(),
		len(feed.BlindBoxes), len(feed.Suppliers), elapsed.Milliseconds(), FeedSourceMySQL)

	return feed, nil
}

// buildFeedFromMySQL 执行降级查询并组装 Feed。
//
// 与 cmd/es-sync 的投影口径逐字对齐（详见 repository/mall/feed_repository.go 文件头）：
//   - rarity_summary：同 buildRaritySummary 算法（weight>0 才计入，按 SSR→SR→R→N 输出）；
//   - promo_price：只填**查得到的**真实活动价；查不到保持 nil（MUST NOT 用原价
//     或任何默认值填充 —— spec Scenario "不返回错误价格"）；
//   - 空结果：数组用 `[]` 而非 nil（spec Scenario "空数组而非 null"）。
func (s *baseFeedService) buildFeedFromMySQL(c *gin.Context) (*Feed, error) {
	// 1) 盲盒行（含供应商名）。
	rows, err := s.fallback.FeedBlindBoxes(c, feedSize)
	if err != nil {
		return nil, fmt.Errorf("query blind boxes: %w", err)
	}

	now := time.Now()

	// 2) 卡池权重 → rarity_summary。
	boxIDs := make([]int64, 0, len(rows))
	for _, r := range rows {
		boxIDs = append(boxIDs, r.ID)
	}
	weights, err := s.fallback.FeedRarityWeights(c, boxIDs)
	if err != nil {
		return nil, fmt.Errorf("query rarity weights: %w", err)
	}
	weightsByBox := make(map[int64][]mallRepo.FeedRarityWeight, len(rows))
	for _, w := range weights {
		weightsByBox[w.BlindBoxID] = append(weightsByBox[w.BlindBoxID], w)
	}

	// 3) 当前生效活动价（口径与 es-sync 一致：status=active 且落在活动期内的活动）。
	promoByBox, err := s.fallback.FeedActivePromoPrices(c, now)
	if err != nil {
		return nil, fmt.Errorf("query active promotions: %w", err)
	}

	blindBoxes := make([]BlindBoxFeedItem, 0, len(rows))
	for _, r := range rows {
		item := BlindBoxFeedItem{
			ID:            r.ID,
			SupplierID:    r.SupplierID,
			SupplierName:  r.SupplierName,
			Name:          r.Name,
			Cover:         r.Cover,
			Price:         r.Price,
			RaritySummary: buildRaritySummary(weightsByBox[r.ID]),
			// HotScore：MVP 无热度数据源，主路径（es-sync 投影）同样恒为 0。
			HotScore:  0,
			CreatedAt: r.CreatedAt,
		}
		// promo_price 只在**确有生效活动**时填充；否则保持 nil。
		// 这里刻意不写 `else { item.PromoPrice = &r.Price }` 之类的兜底 ——
		// 展示原价是可接受的降级体验，展示错误价格是资损风险（spec）。
		if promo, ok := promoByBox[r.ID]; ok {
			p := promo
			item.PromoPrice = &p
		}
		blindBoxes = append(blindBoxes, item)
	}

	// 4) 供应商。
	supRows, err := s.fallback.FeedSuppliers(c, feedSize)
	if err != nil {
		return nil, fmt.Errorf("query suppliers: %w", err)
	}
	suppliers := make([]SupplierFeedItem, 0, len(supRows))
	for _, r := range supRows {
		suppliers = append(suppliers, SupplierFeedItem{
			ID:            r.ID,
			Name:          r.Name,
			Logo:          r.Logo,
			BlindBoxCount: r.BlindBoxCount,
			// FeaturedRank：MVP 无排位数据源，主路径投影同样恒为 0。
			FeaturedRank: 0,
		})
	}

	return &Feed{
		BlindBoxes: blindBoxes,
		Suppliers:  suppliers,
		FetchedAt:  now,
		Source:     FeedSourceMySQL,
	}, nil
}

// buildRaritySummary 由降级路径的 (rarity, weight) 行计算概率公示串。
//
// **必须与 cmd/es-sync 的 buildRaritySummary 保持一致**（同池加和 10000，
// weight/100 即整数百分比；weight=0 的稀有度不出现在公示里；固定 SSR→SR→R→N
// 输出顺序；空池返回空串）。两处修改需同步，否则降级与主路径的
// rarity_summary 会漂移。
func buildRaritySummary(rows []mallRepo.FeedRarityWeight) string {
	sum := map[mall.MallCardRarity]int{}
	for _, r := range rows {
		if r.Weight > 0 {
			sum[r.Rarity] += r.Weight
		}
	}
	// 固定输出顺序 SSR → SR → R → N，保证结果稳定。
	order := []mall.MallCardRarity{mall.RaritySSR, mall.RaritySR, mall.RarityR, mall.RarityN}
	parts := make([]string, 0, len(order))
	for _, r := range order {
		if w := sum[r]; w > 0 {
			parts = append(parts, fmt.Sprintf("%s %d%%", r, w/100))
		}
	}
	return strings.Join(parts, " / ")
}


// wrapSearchErr 把任意子错误包成「ErrFeedSearchUnavailable + context + 原 error」，
// 满足 handler 侧的 errors.Is(ErrFeedSearchUnavailable) 与排错源的 errors.Is(原 err)
// 同时成立（用 errors.Join 同时挂两个 wraps；Go 1.20+）。
func wrapSearchErr(ctx string, cause error) error {
	return fmt.Errorf("%s: %w", ctx, errors.Join(ErrFeedSearchUnavailable, cause))
}

// decodeHits 把 search.Hit[] 反序列化成目标 DTO 切片。
//
// 容错策略：任何一条反序列化失败都返 error，由上层转入 MySQL 降级；
// 降级也失败才映射为 500。
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
