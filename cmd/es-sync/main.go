// Command es-sync 把 MySQL 里的 mall 数据全量同步到 Elasticsearch 索引
// （spec Requirement "ES sync script"；design D6 / D7）。
//
// 同步范围（预过滤，应用侧 Feed 只 match_all + sort）：
//   - mall_blind_box_index：盲盒 status=active AND on_sale=true AND is_featured=true
//   - mall_supplier_index：供应商 status=active AND is_featured=true
//
// 同步策略：全量覆盖 —— 读取 ES 现有文档 ID 集合，BulkIndex upsert 新文档，
// 再 Delete 「ES 有但 MySQL 已不满足条件」的 stale 文档；幂等可重跑
// （spec Scenario "Idempotent re-run"）。
//
// 用法：go run ./cmd/es-sync（需 config/*.yaml + .env.yaml、MySQL、ES 可达）
// 触发方式：手动（开发态）/ CI pipeline / cron（见 scripts/es-sync.md）。
//
// 错误处理（任务 5.2）：config / DB / ES 任一初始化失败，或同步过程出错，
// log.Fatalf 输出错误并以非零退出码结束。
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"

	"ai-go-mall/internal/infra/config"
	"ai-go-mall/internal/infra/database"
	"ai-go-mall/internal/infra/search"
	"ai-go-mall/internal/model/mall"
	userSvc "ai-go-mall/internal/service/user"
)

func main() {
	if err := config.Init("."); err != nil {
		log.Fatalf("es-sync: init config: %v", err)
	}
	if err := database.Init(); err != nil {
		log.Fatalf("es-sync: init database: %v", err)
	}
	if err := search.Init(); err != nil {
		log.Fatalf("es-sync: init search: %v", err)
	}
	if err := run(database.Get(), search.Get()); err != nil {
		log.Fatalf("es-sync: %v", err)
	}
}

// run 执行两个索引的全量同步；任一失败立即返回 error（main 侧非零退出）。
func run(db *gorm.DB, client search.SearchClient) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	start := time.Now()
	n, err := syncBlindBoxes(ctx, db, client)
	if err != nil {
		return fmt.Errorf("sync %s: %w", userSvc.BlindBoxIndex, err)
	}
	log.Printf("%s: %d records synced in %d ms",
		userSvc.BlindBoxIndex, n, time.Since(start).Milliseconds())

	start = time.Now()
	m, err := syncSuppliers(ctx, db, client)
	if err != nil {
		return fmt.Errorf("sync %s: %w", userSvc.SupplierIndex, err)
	}
	log.Printf("%s: %d records synced in %d ms",
		userSvc.SupplierIndex, m, time.Since(start).Milliseconds())
	return nil
}

// =============================================================================
// 盲盒索引
// =============================================================================

// blindBoxInput 是投影所需的聚合输入（纯函数便于单测）。
type blindBoxInput struct {
	ID           int64
	SupplierID   int64
	SupplierName string
	Name         string
	Cover        string
	Price        float64
	PromoPrice   *float64
	Items        []mall.MallCardPoolItem
	CreatedAt    time.Time
}

// blindBoxDoc 是 mall_blind_box_index 的文档形态（D7 投影，snake_case）。
type blindBoxDoc struct {
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

// projectBlindBox 把 MySQL 行聚合成 ES 投影文档。
func projectBlindBox(in blindBoxInput) blindBoxDoc {
	return blindBoxDoc{
		ID:            in.ID,
		SupplierID:    in.SupplierID,
		SupplierName:  in.SupplierName,
		Name:          in.Name,
		Cover:         in.Cover,
		Price:         in.Price,
		PromoPrice:    in.PromoPrice,
		RaritySummary: buildRaritySummary(in.Items),
		HotScore:      0, // MVP 无热度来源，默认 0；排序退化为 created_at DESC
		CreatedAt:     in.CreatedAt,
	}
}

// buildRaritySummary 由卡池条目计算概率公示串："SSR 3% / SR 12% / ..."。
//
// weight 同池加和固定 10000（D2），weight/100 即整数百分比。
// weight=0 的稀有度不出现在公示里；空池返回空串。
func buildRaritySummary(items []mall.MallCardPoolItem) string {
	// 按稀有度聚合权重（同稀有度可能多条目）。
	sum := map[mall.MallCardRarity]int{}
	for _, it := range items {
		if it.Weight > 0 {
			sum[it.Rarity] += it.Weight
		}
	}
	// 固定输出顺序 SSR → SR → R → N，保证文档幂等（map 遍历序随机）。
	order := []mall.MallCardRarity{mall.RaritySSR, mall.RaritySR, mall.RarityR, mall.RarityN}
	parts := make([]string, 0, len(order))
	for _, r := range order {
		if w := sum[r]; w > 0 {
			parts = append(parts, fmt.Sprintf("%s %d%%", r, w/100))
		}
	}
	return strings.Join(parts, " / ")
}

// syncBlindBoxes 全量同步盲盒索引，返回同步条数。
func syncBlindBoxes(ctx context.Context, db *gorm.DB, client search.SearchClient) (int, error) {
	now := time.Now()

	// 1. MySQL 侧：满足条件的盲盒 + 供应商名。
	var boxes []mall.MallBlindBox
	if err := db.WithContext(ctx).
		Where("status = ? AND on_sale = ? AND is_featured = ?", mall.StatusActive, true, true).
		Find(&boxes).Error; err != nil {
		return 0, fmt.Errorf("query blind boxes: %w", err)
	}

	boxIDs := make([]int64, 0, len(boxes))
	for _, b := range boxes {
		boxIDs = append(boxIDs, b.ID)
	}

	supplierNames := map[int64]string{}
	activeSuppliers := map[int64]struct{}{}
	if len(boxes) > 0 {
		var suppliers []mall.MallSupplier
		if err := db.WithContext(ctx).
			Where("id IN ?", uniqueIDs(boxIDs)).
			Find(&suppliers).Error; err != nil {
			return 0, fmt.Errorf("query suppliers: %w", err)
		}
		for _, sp := range suppliers {
			supplierNames[sp.ID] = sp.Name
			if sp.Status == mall.StatusActive {
				activeSuppliers[sp.ID] = struct{}{}
			}
		}
	}

	// 1.5 过滤被禁供应商的盲盒（spec "Admin disables a supplier"）。
	boxes = filterBoxesOfActiveSuppliers(boxes, activeSuppliers)
	if len(boxes) == 0 {
		// 无可同步文档，仍要走 deleteStale 清理 ES 残留。
		deleted, err := deleteStale(ctx, client, userSvc.BlindBoxIndex, map[string]struct{}{})
		if err != nil {
			return 0, err
		}
		log.Printf("%s: %d stale docs deleted", userSvc.BlindBoxIndex, deleted)
		return 0, nil
	}

	// 2. 当前生效的活动（D10：同盲盒至多 1 个 active）。
	promoPriceByBox := map[int64]float64{}
	if len(boxes) > 0 {
		var promos []mall.MallPromotion
		if err := db.WithContext(ctx).
			Where("status = ? AND start_at <= ? AND end_at > ?", mall.StatusActive, now, now).
			Find(&promos).Error; err != nil {
			return 0, fmt.Errorf("query promotions: %w", err)
		}
		for _, p := range promos {
			promoPriceByBox[p.BlindBoxID] = p.PromoPrice
		}
	}

	// 3. 卡池条目（rarity_summary 来源）。
	poolItemsByBox := map[int64][]mall.MallCardPoolItem{}
	if len(boxes) > 0 {
		var pools []mall.MallCardPool
		if err := db.WithContext(ctx).
			Where("blind_box_id IN ?", uniqueIDs(boxIDs)).
			Find(&pools).Error; err != nil {
			return 0, fmt.Errorf("query card pools: %w", err)
		}
		poolIDs := make([]int64, 0, len(pools))
		poolByBox := map[int64]int64{}
		for _, p := range pools {
			poolIDs = append(poolIDs, p.ID)
			poolByBox[p.BlindBoxID] = p.ID
		}
		if len(poolIDs) > 0 {
			var items []mall.MallCardPoolItem
			if err := db.WithContext(ctx).
				Where("pool_id IN ?", uniqueIDs(poolIDs)).
				Find(&items).Error; err != nil {
				return 0, fmt.Errorf("query pool items: %w", err)
			}
			for _, it := range items {
				boxID := reverseLookup(poolByBox, it.PoolID)
				poolItemsByBox[boxID] = append(poolItemsByBox[boxID], it)
			}
		}
	}

	// 4. 投影 + BulkIndex。
	docs := make([]search.BulkDoc, 0, len(boxes))
	keptIDs := make(map[string]struct{}, len(boxes))
	for _, b := range boxes {
		in := blindBoxInput{
			ID:           b.ID,
			SupplierID:   b.SupplierID,
			SupplierName: supplierNames[b.SupplierID],
			Name:         b.Name,
			Cover:        b.Cover,
			Price:        b.Price,
			Items:        poolItemsByBox[b.ID],
			CreatedAt:    b.CreatedAt,
		}
		if promo, ok := promoPriceByBox[b.ID]; ok {
			in.PromoPrice = &promo
		}
		body, err := json.Marshal(projectBlindBox(in))
		if err != nil {
			return 0, fmt.Errorf("marshal blind box %d: %w", b.ID, err)
		}
		id := strconv.FormatInt(b.ID, 10)
		docs = append(docs, search.BulkDoc{ID: id, Body: body})
		keptIDs[id] = struct{}{}
	}
	if len(docs) > 0 {
		if err := client.BulkIndex(ctx, userSvc.BlindBoxIndex, docs); err != nil {
			return 0, fmt.Errorf("bulk index: %w", err)
		}
	}

	// 5. 清掉 ES 有但 MySQL 已不满足条件的 stale 文档。
	deleted, err := deleteStale(ctx, client, userSvc.BlindBoxIndex, keptIDs)
	if err != nil {
		return 0, err
	}
	log.Printf("%s: %d stale docs deleted", userSvc.BlindBoxIndex, deleted)
	return len(docs), nil
}

// =============================================================================
// 供应商索引
// =============================================================================

// supplierInput 是投影所需的聚合输入。
type supplierInput struct {
	ID            int64
	Name          string
	Logo          string
	BlindBoxCount int
}

// supplierDoc 是 mall_supplier_index 的文档形态（D7 投影）。
type supplierDoc struct {
	ID            int64  `json:"id"`
	Name          string `json:"name"`
	Logo          string `json:"logo"`
	BlindBoxCount int    `json:"blind_box_count"`
	FeaturedRank  int    `json:"featured_rank"`
}

// projectSupplier 把 MySQL 行聚合成 ES 投影文档。
func projectSupplier(in supplierInput) supplierDoc {
	return supplierDoc{
		ID:            in.ID,
		Name:          in.Name,
		Logo:          in.Logo,
		BlindBoxCount: in.BlindBoxCount,
		FeaturedRank:  0, // MVP 无排位来源，默认 0；排序退化为 blind_box_count DESC
	}
}

// syncSuppliers 全量同步供应商索引，返回同步条数。
func syncSuppliers(ctx context.Context, db *gorm.DB, client search.SearchClient) (int, error) {
	var suppliers []mall.MallSupplier
	if err := db.WithContext(ctx).
		Where("status = ? AND is_featured = ?", mall.StatusActive, true).
		Find(&suppliers).Error; err != nil {
		return 0, fmt.Errorf("query suppliers: %w", err)
	}

	// blind_box_count：该供应商 active+on_sale 的盲盒数（含未 featured 的在售盒）。
	countBySupplier := map[int64]int{}
	var counts []struct {
		SupplierID int64
		Total      int64
	}
	if err := db.WithContext(ctx).
		Model(&mall.MallBlindBox{}).
		Select("supplier_id, COUNT(*) AS total").
		Where("status = ? AND on_sale = ?", mall.StatusActive, true).
		Group("supplier_id").
		Scan(&counts).Error; err != nil {
		return 0, fmt.Errorf("count blind boxes: %w", err)
	}
	for _, c := range counts {
		countBySupplier[c.SupplierID] = int(c.Total)
	}

	docs := make([]search.BulkDoc, 0, len(suppliers))
	keptIDs := make(map[string]struct{}, len(suppliers))
	for _, sp := range suppliers {
		body, err := json.Marshal(projectSupplier(supplierInput{
			ID:            sp.ID,
			Name:          sp.Name,
			Logo:          sp.Logo,
			BlindBoxCount: countBySupplier[sp.ID],
		}))
		if err != nil {
			return 0, fmt.Errorf("marshal supplier %d: %w", sp.ID, err)
		}
		id := strconv.FormatInt(sp.ID, 10)
		docs = append(docs, search.BulkDoc{ID: id, Body: body})
		keptIDs[id] = struct{}{}
	}
	if len(docs) > 0 {
		if err := client.BulkIndex(ctx, userSvc.SupplierIndex, docs); err != nil {
			return 0, fmt.Errorf("bulk index: %w", err)
		}
	}

	deleted, err := deleteStale(ctx, client, userSvc.SupplierIndex, keptIDs)
	if err != nil {
		return 0, err
	}
	log.Printf("%s: %d stale docs deleted", userSvc.SupplierIndex, deleted)
	return len(docs), nil
}

// =============================================================================
// 共用 helper
// =============================================================================

// deleteStale 删除 ES 里存在但不在 keptIDs 里的文档（全量覆盖的"清旧"步骤）。
func deleteStale(ctx context.Context, client search.SearchClient, index string, keptIDs map[string]struct{}) (int, error) {
	hits, _, err := client.Search(ctx, index, []byte(`{"query": {"match_all": {}}, "_source": false, "size": 10000}`))
	if err != nil {
		return 0, fmt.Errorf("list existing docs: %w", err)
	}
	deleted := 0
	for _, h := range hits {
		if _, keep := keptIDs[h.ID]; keep {
			continue
		}
		if err := client.Delete(ctx, index, h.ID); err != nil {
			return deleted, fmt.Errorf("delete stale doc %s: %w", h.ID, err)
		}
		deleted++
	}
	return deleted, nil
}

// uniqueIDs 去重并排序（稳定 IN 查询 + 缓存友好）。
func uniqueIDs(in []int64) []int64 {
	seen := make(map[int64]struct{}, len(in))
	out := make([]int64, 0, len(in))
	for _, v := range in {
		if _, ok := seen[v]; !ok {
			seen[v] = struct{}{}
			out = append(out, v)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// reverseLookup 按 value 找 key（pool_id → blind_box_id）。
func reverseLookup(m map[int64]int64, v int64) int64 {
	for k, val := range m {
		if val == v {
			return k
		}
	}
	return 0
}

// promoPriceOf 测试辅助：构造 *float64。
func promoPriceOf(v float64) *float64 { return &v }

// filterBoxesOfActiveSuppliers 过滤掉被禁供应商的盲盒（spec Scenario
// "Admin disables a supplier"：被禁供应商的盲盒不得出现在前端 feed；
// ES 路径的过滤责任在同步端，同步时跳过、deleteStale 清理残留）。
func filterBoxesOfActiveSuppliers(boxes []mall.MallBlindBox, activeSuppliers map[int64]struct{}) []mall.MallBlindBox {
	out := make([]mall.MallBlindBox, 0, len(boxes))
	for _, b := range boxes {
		if _, active := activeSuppliers[b.SupplierID]; active {
			out = append(out, b)
		}
	}
	return out
}
