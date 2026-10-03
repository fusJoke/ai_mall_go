// Package user — blindbox.go 实现 C 端盲盒浏览服务。
//
// 业务规则：
//   - List 走「推荐 + 启用 + 在售」三条件 AND（首页推荐位 / 列表位共用），
//     二次过滤「供应商被禁用」的行 —— 即使盲盒本身 status=active+on_sale=true，
//     供应商一旦被 admin ToggleStatus 标 disabled，对应盲盒也不再对 C 端可见。
//   - Detail 聚合盲盒 + 供应商 + 卡池 + 卡池 items + 当前活动促销，
//     用缓存抗读热点（cache miss 时回源 DB 并回填 cache）。
//
// 缓存策略（Phase 10）：
//   - 多层缓存：MultiLevelCache（L1 内存 + L2 Redis），TTL 由 config.Get().Cache.TTL.BlindBoxDetail 决定。
//   - 回填时 Set 传 L2 TTL（10min）—— L1 也接受此 TTL 是 MultiLevelCache 当前实现的折中：
//     业务侧通过调用 multiCache.Del 主动失效时 L1/L2 同时清，不依赖 TTL 兜底。
//   - 后续优化（如需 L1 TTL 远短于 L2）需要扩展 Cache 接口或 MultiLevelCache 加 SetLayered 方法。
package user

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"ai-go-mall/internal/infra/cache"
	"ai-go-mall/internal/infra/cache/hotspot"
	"ai-go-mall/internal/infra/database"
	"ai-go-mall/internal/model/mall"
	"ai-go-mall/internal/repository"
	mallRepo "ai-go-mall/internal/repository/mall"
	supplierRepo "ai-go-mall/internal/repository/supplier"
)

// ErrBlindBoxNotFound 盲盒不可见（不存在 / 已下架 / 供应商已禁用）。
//
// Detail 走统一 404 文案，避免暴露「是盲盒下架了」还是「供应商被禁」的具体原因。
// ErrBlindBoxNotFound 盲盒不可见（不存在 / 已下架 / 供应商已禁用）。
//
// 包装 cache.ErrNotFound（D5.1）：handler 侧 errors.Is(err, ErrBlindBoxNotFound)
// 照常映射 404；缓存侧 cache.IsNotFound(err) 据此写 notFound 占位 —— 一个错误
// 两个语义域同时成立，service 不必为缓存单独翻译。
var ErrBlindBoxNotFound = fmt.Errorf("user.blindbox: not found: %w", cache.ErrNotFound)

// BlindBoxDetail 是 C 端盲盒详情聚合。
//
// 字段命名遵循 dto 风格（snake_case JSON），handler 层直接序列化即可。
type BlindBoxDetail struct {
	// BlindBox 盲盒基础信息。
	BlindBox mall.MallBlindBox `json:"blind_box"`

	// Supplier 供应商主体（用于详情页头部展示供应商名 / logo）。
	Supplier mall.MallSupplier `json:"supplier"`

	// Pool 卡池基本信息（id + 创建时间）。
	Pool mall.MallCardPool `json:"pool"`

	// Items 卡池内所有卡条目（含 stock=0，前端用于公示概率）。
	Items []mall.MallCardPoolItem `json:"items"`

	// PoolTotalStock 卡池总库存（sum items.stock）。
	PoolTotalStock int64 `json:"pool_total_stock"`

	// ActivePromotion 当前生效的活动；nil 表示无活动。
	ActivePromotion *mall.MallPromotion `json:"active_promotion,omitempty"`

	// EffectivePrice 实际售价（活动价优先，回退原价）。
	EffectivePrice float64 `json:"effective_price"`

	// CachedAt 缓存时间戳（便于调试 + 调试接口判断是否走缓存）。
	CachedAt time.Time `json:"cached_at"`
}

// defaultCacheTTL 详情缓存默认 TTL —— 抗读热点 + 配合 supplier.ToggleStatus 等的失效语义。
//
// 实际 TTL 由 deps.CacheTTL 决定（来自 config.Cache.TTL.BlindBoxDetail.L2，
// 默认 10 分钟）；deps 未指定时回落本常量。
const defaultCacheTTL = 5 * time.Minute

// cacheKey 构造详情缓存 key，统一前缀 + 盲盒 ID。
//
// 拼 prefix 方便后续按需按前缀批量失效（如「供应商维度失效」）。
func cacheKey(blindBoxID int64) string {
	return fmt.Sprintf("mall:blindbox:detail:%d", blindBoxID)
}

// BlindBoxService 是 C 端盲盒业务接口。
//
// 独立于 auth.go 的 Service —— 业务边界清晰：auth 只管账号登录，
// BlindBoxService 只管商品浏览。cmd 装配时把两者组合到一个聚合 service 上。
type BlindBoxService interface {
	// List 拉 C 端可见的盲盒列表。
	//
	// 业务规则：
	//  1. 仅 status=active AND on_sale=true 的盲盒；
	//  2. 供应商已被禁用的盲盒（即使盲盒本身仍 active+onsale）也不可见。
	List(c *gin.Context, opts repository.ListOptions) (items []mall.MallBlindBox, total int64, err error)

	// Detail 拉单个盲盒的详情（含卡池 items + 当前活动），命中缓存走缓存。
	//
	// 业务规则：
	//  - 盲盒不存在 / 状态非 active / 未在售 → ErrBlindBoxNotFound；
	//  - 供应商已禁用 → ErrBlindBoxNotFound（不暴露具体原因）。
	Detail(c *gin.Context, id int64) (*BlindBoxDetail, error)
}

// BlindBoxServiceDeps 注入 BlindBoxService 所需的依赖。
type BlindBoxServiceDeps struct {
	BlindBoxRepo  mallRepo.BlindBoxRepository
	CardPoolRepo  mallRepo.CardPoolRepository
	PromotionRepo mallRepo.PromotionRepository
	SupplierRepo  supplierRepo.SupplierRepository

	// Hotspot 热点缓存（D21）：Detail 优先走它（永驻 + 逻辑过期 30s 异步刷新）。
	// nil 时回落 Cache 的 TTL 缓存路径（单测 / 未装配场景）。
	Hotspot hotspot.HotspotCache

	// Cache TTL 缓存（Phase 10 遗留路径，Hotspot 未装配时 Detail 使用）。
	Cache cache.Cache
	// CacheTTL 详情缓存 TTL（来自 config.Cache.TTL.BlindBoxDetail.L2）。
	// 0 时回落 defaultCacheTTL。MultiLevelCache 模式下 Set 时两层都用此 TTL。
	CacheTTL time.Duration
}

// baseBlindBoxService 是 BlindBoxService 的默认实现。
type baseBlindBoxService struct {
	bb       mallRepo.BlindBoxRepository
	cp       mallRepo.CardPoolRepository
	promo    mallRepo.PromotionRepository
	sup      supplierRepo.SupplierRepository
	hotspot  hotspot.HotspotCache
	cache    cache.Cache
	cacheTTL time.Duration
}

// NewBlindBoxService 接收依赖，返回 BlindBoxService 接口。
func NewBlindBoxService(d BlindBoxServiceDeps) BlindBoxService {
	ttl := d.CacheTTL
	if ttl <= 0 {
		ttl = defaultCacheTTL
	}
	return &baseBlindBoxService{
		bb:       d.BlindBoxRepo,
		cp:       d.CardPoolRepo,
		promo:    d.PromotionRepo,
		sup:      d.SupplierRepo,
		hotspot:  d.Hotspot,
		cache:    d.Cache,
		cacheTTL: ttl,
	}
}

// List 实现见 BlindBoxService 注释。
//
// 实现要点：
//  1. 用 BlindBoxRepository.ListFeaturedOnSale 走「推荐 + 启用 + 在售」三条件 AND（DB 单条 SQL）；
//  2. 用本页返回的 supplier_id 集合批量查 supplier 状态，剔除已禁用的；
//  3. total 是「过滤前」的总数（与 ListFeaturedOnSale 一致）；前端看到的分页数会比 total 略少，
//     这是可接受的——MVP 量级下供应商禁用是稀有事件。
func (s *baseBlindBoxService) List(c *gin.Context, opts repository.ListOptions) ([]mall.MallBlindBox, int64, error) {
	items, total, err := s.bb.ListFeaturedOnSale(c, opts)
	if err != nil {
		return nil, 0, err
	}
	if len(items) == 0 {
		return items, total, nil
	}

	// 二次过滤：剔除供应商已禁用的盲盒。
	supplierIDs := make([]int64, 0, len(items))
	for _, b := range items {
		supplierIDs = append(supplierIDs, b.SupplierID)
	}
	suppliers, err := s.sup.ListByIDs(c, supplierIDs)
	if err != nil {
		return nil, 0, err
	}
	disabledSet := make(map[int64]struct{}, len(suppliers))
	for _, sp := range suppliers {
		if sp.Status == mall.StatusDisabled {
			disabledSet[sp.ID] = struct{}{}
		}
	}
	filtered := make([]mall.MallBlindBox, 0, len(items))
	for _, b := range items {
		if _, disabled := disabledSet[b.SupplierID]; disabled {
			continue
		}
		filtered = append(filtered, b)
	}
	return filtered, total, nil
}

// Detail 实现见 BlindBoxService 注释。
//
// 实现要点：
//  1. Hotspot 装配时（生产路径）：走热点缓存（D21）——永驻 + 逻辑过期 30s
//     异步刷新，读路径永远秒回；loader 用脱离请求生命周期的 DB 会话，
//     保证后台刷新 goroutine 不受请求结束影响；
//  2. Hotspot 未装配时（旧单测 / 降级）：走 TTL 缓存（cache.Get）；
//  3. 双缓存都未装配：直查 DB。回源聚合逻辑统一在 loadDetail。
func (s *baseBlindBoxService) Detail(c *gin.Context, id int64) (*BlindBoxDetail, error) {
	if id <= 0 {
		return nil, ErrBlindBoxNotFound
	}

	// 1) 热点缓存路径（D21）。
	if s.hotspot != nil {
		detail, err := hotspot.Load(c.Request.Context(), s.hotspot, hotspot.BlindBoxKey(id),
			func(ctx context.Context, _ string) (*BlindBoxDetail, error) {
				return s.loadDetail(database.DetachedGinContext(ctx), id)
			}, hotspot.BlindBoxStaleAfter)
		if err == nil {
			return detail, nil
		}
		// 占位命中（D5.1）：hotspot 返回 cache.ErrNotFound → 翻译回业务 sentinel。
		if cache.IsNotFound(err) {
			return nil, ErrBlindBoxNotFound
		}
		// 热点缓存失败（Redis 故障 / loader 报错如 ErrBlindBoxNotFound）：
		// ErrBlindBoxNotFound 必须原样上抛（业务语义）；其余（基础设施）也上抛，
		// 与 spec「ES 不可用透传 error」同策略 —— 缓存故障不该被伪装成空数据。
		return nil, err
	}

	// 2) TTL 缓存路径（Hotspot 未装配时），三态语义 + 防穿透占位（D5.1 / 17.2）。
	if s.cache != nil {
		raw, found, err := s.cache.Get(c.Request.Context(), cacheKey(id))
		if err == nil && found {
			var cached BlindBoxDetail
			if jerr := json.Unmarshal([]byte(raw), &cached); jerr == nil {
				return &cached, nil
			}
			// 反序列化失败视作缓存损坏，降级到 DB 读。
		}
		if err == nil && !found {
			// 占位命中：key 确认不存在，不打 DB。
			return nil, ErrBlindBoxNotFound
		}
		// ErrCacheMiss / 损坏 → 走 DB；loader 报 ErrNotFound 时写 30s 占位。
		detail, derr := s.loadDetail(c, id)
		if derr != nil {
			if cache.IsNotFound(derr) {
				// D22：占位 TTL 也加 ±10% 抖动，避免一批「不存在的 key」
				// 同时到期回源 DB。
				ttl := cache.JitterTTL(cache.NotFoundPlaceholderTTL)
				_ = s.cache.Set(c.Request.Context(), cacheKey(id),
					cache.NotFoundPlaceholderValue(), ttl)
			}
			return nil, derr
		}
		if payload, jerr := json.Marshal(detail); jerr == nil {
			// D22：详情缓存 TTL 加 ±10% 抖动避免雪崩。
			ttl := cache.JitterTTL(s.cacheTTL)
			_ = s.cache.Set(c.Request.Context(), cacheKey(id), string(payload), ttl)
		}
		return detail, nil
	}

	// 3) 无缓存兜底：直查 DB。
	return s.loadDetail(c, id)
}

// loadDetail 聚合盲盒详情：盲盒 → 供应商（disable 则 404）→ 卡池 → items
// → 当前活动 → 组装（活动价优先）。Hotspot loader 与直查路径共用。
func (s *baseBlindBoxService) loadDetail(c *gin.Context, id int64) (*BlindBoxDetail, error) {
	// 回源。
	bb, err := s.bb.GetByID(c, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrBlindBoxNotFound
		}
		return nil, err
	}
	if bb == nil || bb.Status != mall.StatusActive || !bb.OnSale {
		return nil, ErrBlindBoxNotFound
	}

	sup, err := s.sup.GetByID(c, bb.SupplierID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrBlindBoxNotFound
		}
		return nil, err
	}
	if sup == nil || sup.Status == mall.StatusDisabled {
		return nil, ErrBlindBoxNotFound
	}

	pool, err := s.cp.GetPoolByBlindBoxID(c, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// 卡池缺失：理论上不允许（创建盲盒必须建池），但返回详情时不阻断，让前端展示空卡池。
			pool = &mall.MallCardPool{}
		} else {
			return nil, err
		}
	}

	items := make([]mall.MallCardPoolItem, 0)
	if pool.ID != 0 {
		items, err = s.cp.ListItemsByPoolID(c, pool.ID)
		if err != nil {
			return nil, err
		}
	}

	var totalStock int64
	for _, it := range items {
		totalStock += int64(it.Stock)
	}

	now := time.Now()
	promo, err := s.promo.FindActiveByBlindBoxID(c, id, now)
	if err != nil {
		return nil, err
	}

	detail := &BlindBoxDetail{
		BlindBox:        *bb,
		Supplier:        *sup,
		Pool:            *pool,
		Items:           items,
		PoolTotalStock:  totalStock,
		ActivePromotion: promo,
		EffectivePrice:  bb.Price,
		CachedAt:        now,
	}
	if promo != nil {
		detail.EffectivePrice = promo.PromoPrice
	}

	return detail, nil
}

// 编译期断言：baseBlindBoxService 必须实现 BlindBoxService。
var _ BlindBoxService = (*baseBlindBoxService)(nil)

// InvalidateDetail 主动失效盲盒详情缓存（L1/L2 TTL + D21 热点两层一起删）。
//
// 触发点：supplier 服务更新盲盒 / 调整卡池 / 启停活动 等「可能影响详情」的写路径；
// 该函数为 package-level helper，由其他 service 调用。
//
// 设计取舍：本函数刻意留在 BlindBoxService 包而非独立 cache pkg，是为了把
// 「key 拼装规则」与「service 行为」绑在一起，避免散落。
// D21 写路径约定：Hotspot.Invalidate（热点层）+ Cache.Del（L1/L2 层）都要失效，
// 否则两层返回的数据会互相矛盾（热点永驻 vs TTL 过期）。
func InvalidateDetail(ctx context.Context, c cache.Cache, h hotspot.HotspotCache, blindBoxID int64) error {
	if h != nil {
		if err := h.Invalidate(ctx, hotspot.BlindBoxKey(blindBoxID)); err != nil {
			return err
		}
	}
	if c == nil {
		return nil
	}
	return c.Del(ctx, cacheKey(blindBoxID))
}
