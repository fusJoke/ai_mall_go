// Package mall — feed_repository.go 是首页 Feed 的 **MySQL 降级数据源**。
//
// 背景（openspec change `add-uni-h5-client`，capability `home-feed-fallback`）：
// 首页 Feed 的主路径走 Elasticsearch；ES 不可用时降级到 MySQL 直查，避免
// 「ES 抖动即首页白屏」。本文件只提供降级所需的四条只读查询，投影组装
// （DTO 形态、rarity_summary 计算、promo_price 关联）由 service 层完成。
//
// 口径一致性（最关键的设计约束）：
//
//	本文件的过滤条件 MUST 与 `cmd/es-sync` 的收录口径逐字一致，否则降级会
//	返回与主路径不同的商品集合（多出或漏掉商品），破坏 spec 要求的
//	「对客户端完全透明」。对应关系：
//	  - 盲盒：`status=active AND on_sale=true AND lifecycle='selling'`，
//	    且其供应商 `status=active`（es-sync 用 filterBoxesOfActiveSuppliers
//	    达成同一效果）。注意 **不筛 `is_featured`** —— 自
//	    `add-batch-settlement` 起 is_featured 已降为排序字段，不再是收录门槛。
//	  - 供应商：`status=active AND is_featured=true`；
//	    `blind_box_count` 口径为「该供应商 status=active AND on_sale=true
//	    的盲盒数」（与 es-sync 的 countBySupplier 一致）。
//
// 排序退化说明：
//
//	ES 侧排序键 `hot_score` / `featured_rank` 在 MVP 中恒为 0（无热度/排位
//	数据源），因此主路径实际退化为 `created_at DESC` / `blind_box_count DESC`。
//	MySQL 侧无这两列，直接按同样退化后的键排序。
//
//	额外追加 `id` 作为 tie-breaker（盲盒 `id DESC`、供应商 `id ASC`）：MySQL
//	需要它保证分页/重放稳定。**注意**：当排序键取值完全相同时（如 seed 数据
//	同一批插入导致 created_at 一致），本路径给出的顺序可能与 ES 的文档内部
//	顺序不同 —— 排序 tie-break 不属于 spec 约束的「结构同构」范畴（spec 要求
//	的是字段集合一致），且 ES 的 _doc 顺序本身不保证稳定（段合并会变），
//	故不追求逐条同序；客户端渲染的是推荐流，顺序差异不影响可用性。
package mall

import (
	"sort"
	"time"

	"github.com/gin-gonic/gin"

	"ai-go-mall/internal/model/mall"
	"ai-go-mall/internal/repository"
)

// FeedBlindBoxRow 是降级路径的盲盒原始行（与 ES 文档的字段一一对应）。
type FeedBlindBoxRow struct {
	// ID 盲盒主键（对应 ES 文档 id）。
	ID int64

	// SupplierID 所属供应商 ID。
	SupplierID int64

	// SupplierName 供应商名称（JOIN 得到，对应 ES 文档 supplier_name）。
	SupplierName string

	// Name 盲盒名称。
	Name string

	// Cover 封面图 URL。
	Cover string

	// Price 原价（元）。
	Price float64

	// CreatedAt 创建时间（排序键之一）。
	CreatedAt time.Time
}

// FeedRarityWeight 是卡池条目的稀有度权重行（`rarity_summary` 的计算输入）。
//
// 与 `cmd/es-sync` 的 buildRaritySummary 输入（[]mall.MallCardPoolItem）语义
// 等价，只带计算所需的三个字段，避免把整行拉进内存。
type FeedRarityWeight struct {
	// BlindBoxID 所属盲盒 ID（反查用）。
	BlindBoxID int64

	// Rarity 稀有度。
	Rarity mall.MallCardRarity

	// Weight 权重（同池加和=10000）。
	Weight int
}

// FeedSupplierRow 是降级路径的供应商行。
type FeedSupplierRow struct {
	// ID 供应商主键。
	ID int64

	// Name 供应商名称。
	Name string

	// Logo LOGO URL。
	Logo string

	// BlindBoxCount 在售盲盒数（口径见本文件头的「口径一致性」小节）。
	BlindBoxCount int
}

// FeedRepository 是首页 Feed 的 MySQL 降级数据源接口。
//
// 单独成接口（而非复用 BlindBoxRepository / SupplierRepository）的原因：
//   - 降级路径需要的是**跨表聚合后的投影行**，不是领域实体；
//   - service 层单测需要注入 fake，接口越窄越容易 mock。
type FeedRepository interface {
	// FeedBlindBoxes 拉降级路径的盲盒列表（按 created_at DESC）。
	// limit <= 0 时短路返回空切片。
	FeedBlindBoxes(c *gin.Context, limit int) ([]FeedBlindBoxRow, error)

	// FeedRarityWeights 批量拉盲盒卡池条目的 (rarity, weight)。
	// 空 ids 短路返回空切片。
	FeedRarityWeights(c *gin.Context, boxIDs []int64) ([]FeedRarityWeight, error)

	// FeedActivePromoPrices 拉当前生效活动价，返回 blind_box_id → promo_price。
	//
	// 口径与 es-sync 完全一致：`status=active AND start_at<=now AND end_at>now`。
	// 查不到活动价的盲盒**不会出现在返回的 map 中** —— 调用方据此置
	// promo_price=null，MUST NOT 用原价或任何默认值填充。
	FeedActivePromoPrices(c *gin.Context, now time.Time) (map[int64]float64, error)

	// FeedSuppliers 拉降级路径的供应商列表（featured，按在售盲盒数 DESC）。
	FeedSuppliers(c *gin.Context, limit int) ([]FeedSupplierRow, error)
}

// gormFeedRepository 是 FeedRepository 的 *gorm.DB 实现。
type gormFeedRepository struct{}

// NewFeedRepository 返回 FeedRepository 接口。
func NewFeedRepository() FeedRepository {
	return &gormFeedRepository{}
}

// FeedBlindBoxes 实现见 FeedRepository 注释。
func (r *gormFeedRepository) FeedBlindBoxes(c *gin.Context, limit int) ([]FeedBlindBoxRow, error) {
	if limit <= 0 {
		return []FeedBlindBoxRow{}, nil
	}

	var rows []FeedBlindBoxRow
	err := repository.DB(c).WithContext(c.Request.Context()).
		Table("mall_blind_boxes AS b").
		Select("b.id AS id, b.supplier_id AS supplier_id, s.name AS supplier_name, "+
			"b.name AS name, b.cover AS cover, b.price AS price, b.created_at AS created_at").
		Joins("JOIN mall_suppliers AS s ON s.id = b.supplier_id AND s.deleted_at IS NULL").
		Where("b.deleted_at IS NULL").
		Where("b.status = ? AND b.on_sale = ? AND b.lifecycle = ?",
			mall.StatusActive, true, mall.LifecycleSelling).
		Where("s.status = ?", mall.StatusActive).
		Order("b.created_at DESC, b.id DESC").
		Limit(limit).
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	if rows == nil {
		rows = []FeedBlindBoxRow{}
	}
	return rows, nil
}

// FeedRarityWeights 实现见 FeedRepository 注释。
func (r *gormFeedRepository) FeedRarityWeights(c *gin.Context, boxIDs []int64) ([]FeedRarityWeight, error) {
	if len(boxIDs) == 0 {
		return []FeedRarityWeight{}, nil
	}

	var rows []FeedRarityWeight
	err := repository.DB(c).WithContext(c.Request.Context()).
		Table("mall_card_pool_items AS i").
		Select("p.blind_box_id AS blind_box_id, i.rarity AS rarity, i.weight AS weight").
		Joins("JOIN mall_card_pools AS p ON p.id = i.pool_id AND p.deleted_at IS NULL").
		Where("i.deleted_at IS NULL").
		Where("p.blind_box_id IN ?", boxIDs).
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	if rows == nil {
		rows = []FeedRarityWeight{}
	}
	return rows, nil
}

// FeedActivePromoPrices 实现见 FeedRepository 注释。
func (r *gormFeedRepository) FeedActivePromoPrices(c *gin.Context, now time.Time) (map[int64]float64, error) {
	var promos []mall.MallPromotion
	if err := repository.DB(c).WithContext(c.Request.Context()).
		Where("status = ? AND start_at <= ? AND end_at > ?", mall.StatusActive, now, now).
		Find(&promos).Error; err != nil {
		return nil, err
	}

	out := make(map[int64]float64, len(promos))
	for _, p := range promos {
		out[p.BlindBoxID] = p.PromoPrice
	}
	return out, nil
}

// FeedSuppliers 实现见 FeedRepository 注释。
//
// 拆两条查询（供应商行 + 按供应商分组的在售盲盒数）而非一条 LEFT JOIN 子查询：
// 与 es-sync 的 countBySupplier 完全同构，口径漂移风险最低。
func (r *gormFeedRepository) FeedSuppliers(c *gin.Context, limit int) ([]FeedSupplierRow, error) {
	if limit <= 0 {
		return []FeedSupplierRow{}, nil
	}

	var suppliers []mall.MallSupplier
	if err := repository.DB(c).WithContext(c.Request.Context()).
		Where("status = ? AND is_featured = ?", mall.StatusActive, true).
		Find(&suppliers).Error; err != nil {
		return nil, err
	}
	if len(suppliers) == 0 {
		return []FeedSupplierRow{}, nil
	}

	ids := make([]int64, 0, len(suppliers))
	for _, s := range suppliers {
		ids = append(ids, s.ID)
	}

	// 注意：此处刻意**不筛 lifecycle** —— 与 es-sync 的 blind_box_count 口径
	// 保持一致（该计数统计的是「在售」而非「在售且未售罄」）。
	var counts []struct {
		SupplierID int64
		Total      int64
	}
	if err := repository.DB(c).WithContext(c.Request.Context()).
		Model(&mall.MallBlindBox{}).
		Select("supplier_id, COUNT(*) AS total").
		Where("status = ? AND on_sale = ? AND supplier_id IN ?", mall.StatusActive, true, ids).
		Group("supplier_id").
		Scan(&counts).Error; err != nil {
		return nil, err
	}
	byID := make(map[int64]int, len(counts))
	for _, c2 := range counts {
		byID[c2.SupplierID] = int(c2.Total)
	}

	rows := make([]FeedSupplierRow, 0, len(suppliers))
	for _, s := range suppliers {
		rows = append(rows, FeedSupplierRow{
			ID:            s.ID,
			Name:          s.Name,
			Logo:          s.Logo,
			BlindBoxCount: byID[s.ID],
		})
	}

	// ES 侧排序键 featured_rank 恒为 0，实际退化为 blind_box_count DESC；
	// 追加 id ASC 作为 tie-breaker 保证确定性。
	sortSuppliersByCountDesc(rows)
	if len(rows) > limit {
		rows = rows[:limit]
	}
	return rows, nil
}

// sortSuppliersByCountDesc 按在售盲盒数降序、id 升序排列（原地排序）。
//
// 与 ES 侧 `featured_rank ASC, blind_box_count DESC` 的退化结果一致：
// featured_rank 恒为 0 → 该键失效 → 实际按 blind_box_count DESC。
// 用 SliceStable + id 升序兜底，保证同 count 时的顺序在不同查询/分页下稳定。
func sortSuppliersByCountDesc(rows []FeedSupplierRow) {
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].BlindBoxCount != rows[j].BlindBoxCount {
			return rows[i].BlindBoxCount > rows[j].BlindBoxCount
		}
		return rows[i].ID < rows[j].ID
	})
}
