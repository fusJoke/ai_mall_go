// Package supplier — product.go 实现 B 端供应商对自家盲盒的增删改查。
//
// 业务规则（spec Requirement "Supplier product management"）：
//   - List：仅返回 supplier_id=当前 supplier 的盲盒，按 id DESC。
//   - Create：单事务内同时建盲盒 + 卡池 + 卡池 items；items.weight 加和 MUST = 10000（D2 关键约束）。
//   - Update：仅允许修改「自家盲盒」（盲盒.SupplierID == supplierID）；成功后失效 C 端详情缓存。
//   - ToggleOnSale：同上 ownership 自检；用于「快速上下架」。
//
// 与 service/user.Draw 共用 txRunner 抽象：tests 可注入 mockTxRunner 同步跑 fn，
// 无需真 DB 也能验证事务内调用是否被正确顺序触发。
package supplier

import (
	"errors"
	"regexp"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"ai-go-mall/internal/infra/cache"
	"ai-go-mall/internal/infra/cache/hotspot"
	"ai-go-mall/internal/model/mall"
	"ai-go-mall/internal/repository"
	mallRepo "ai-go-mall/internal/repository/mall"
	userSvc "ai-go-mall/internal/service/user"
)

// 业务错误。
var (
	// ErrBlindBoxNotFound 盲盒不可见（不存在 / 已软删）。
	ErrBlindBoxNotFound = errors.New("supplier.product: blindbox not found")

	// ErrBlindBoxForbidden 越权 —— 盲盒不属于当前 supplier。
	//
	// 越权读 / 改写他人盲盒 → 一律 ErrBlindBoxForbidden；handler 映射 403。
	// 与 service/user.Detail 的「404 模糊存在性」策略相反：
	// 供应商后台是登录态，调用方已知自身 ID，「越权」是可识别的。
	ErrBlindBoxForbidden = errors.New("supplier.product: blindbox not owned by current supplier")

	// ErrInvalidWeightSum 卡池 items 的 weight 加和 ≠ 10000（D2 关键约束）。
	ErrInvalidWeightSum = errors.New("supplier.product: card pool items weight sum must equal 10000")

	// ErrEmptyPool 卡池 items 为空。
	ErrEmptyPool = errors.New("supplier.product: card pool must have at least one item")

	// ErrLifecycleForbidden 生命周期不允许该操作（→ 409）。
	//
	// 设计状态机要点（openspec: add-batch-settlement）：sold_out 是「卖完了」
	// 不可逆进入结算，settled 是归档终态；off_sale 只适用于 selling。
	// 对 sold_out/settled 商品 toggle-onsale 会把已归档批次重新推向 C 端曝光。
	ErrLifecycleForbidden = errors.New("supplier.product: blindbox lifecycle does not allow this operation")
)

// 业务常量。
const (
	// weightTotal 卡池 items.weight 加和阈值（设计 D2：10000）。
	weightTotal = 10000
)

// txRunner 复用 service/user.Draw 同样的抽象：transaction 执行的最小抽象。
//
// *repository 默认实现走 repository.RunInTx；测试可注入 mock。
type txRunner interface {
	Run(c *gin.Context, fn func(txCtx *gin.Context) error) error
}

// ProductService 是 B 端供应商的盲盒管理接口。
type ProductService interface {
	// List 拉当前 supplier 的盲盒（按 id DESC）。
	// lifecycle 空 = 全部生命周期；非空 = 仅该生命周期（商品即批次 v2 状态筛选）。
	List(c *gin.Context, supplierID int64, lifecycle mall.MallBlindBoxLifecycle, opts repository.ListOptions) (items []mall.MallBlindBox, total int64, err error)

	// PoolStats 批量取盲盒销售进度（openspec: add-batch-settlement 任务 5.3）。
	//
	// 口径：Remaining = Σ pool_items.stock（当前剩余可抽），
	// Sold = COUNT(draw_orders WHERE status='drawn')（成功售出），
	// 前端总名额 = Remaining + Sold。
	// ownership：不属于该 supplier 的 boxID 直接剔除（防越权枚举库存）。
	// 返回 map 缺 key = 未统计（0/0）；入参 boxIDs 空或全被剔除 → 空 map。
	PoolStats(c *gin.Context, supplierID int64, boxIDs []int64) (map[int64]PoolStat, error)

	// Create 单事务内建盲盒 + 卡池 + 卡池 items。
	//
	// 业务规则：
	//  - name 必须非空（前端兜底校验，本处双保险）；
	//  - items 必须非空（ErrEmptyPool）；
	//  - items.weight 加和 MUST == 10000（ErrInvalidWeightSum）；
	//  - 成功返回新建 blindbox（含 ID）；
	//  - 事务内任一步失败 → 全部回滚。
	Create(c *gin.Context, supplierID int64, blindBox *mall.MallBlindBox, items []mall.MallCardPoolItem) (*mall.MallBlindBox, error)

	// Update 只改「传入的字段」；nil 字段保持 DB 原值，成功后失效 C 端详情缓存。
	//
	// 业务规则：
	//  - blindBoxID 不允许为空 / 0（ErrBlindBoxNotFound）；
	//  - 盲盒.SupplierID MUST == supplierID（ErrBlindBoxForbidden）；
	//  - 不修改 supplier_id / status / is_featured 字段（这些是 admin 的领域）。
	//
	// 为什么入参是 patch 而不是整个 *MallBlindBox：仓储的 Update 是 db.Save
	// （**全列写**），交给它的一定得是完整行；而 handler 只拿得到「客户端传了
	// 哪些字段」（指针），其余字段在结构体里是 Go 零值。把那种半成品直接 Save
	// 会把未传字段清零，created_at 归零还会直接撞 MySQL 1292 —— 本接口曾因此
	// 在「只改价格」时恒定 500（handler/supplier/edit_integration_test.go 锁定）。
	Update(c *gin.Context, supplierID, blindBoxID int64, patch ProductPatch) error

	// ToggleOnSale 切换在售状态（快速上下架）。
	//
	// 业务规则同 Update：ownership 自检；成功后失效详情缓存。
	ToggleOnSale(c *gin.Context, supplierID, blindBoxID int64, onSale bool) error

	// Duplicate 复制一个历史商品为新批次（openspec: add-batch-settlement「再来一批」）。
	//
	// 复制矩阵：
	//   - 复制：描述 / 封面 / 价格 / 卡池结构（card_id + rarity + weight）；
	//   - 改写：名称按「第 N 期」递增（源名称无期数 → 追加「第 2 期」）；
	//   - 重置：卡池 items.stock 全部置 0（新批次必须重填库存）、lifecycle=selling、
	//     on_sale=false（默认下架，填好库存再上架）、is_featured=false（admin 领域）；
	//   - 不复制：订单 / 账单（新商品无历史关联）。
	//
	// ownership 自检同 Update；事务内复制（box → pool → items，与 Create 同序），
	// 返回新 blindBoxID。
	Duplicate(c *gin.Context, supplierID, sourceBoxID int64) (int64, error)
}

// ProductPatch 是商品编辑的「只改传入字段」入参：nil = 该字段不改。
//
// 字段与 handler 的 EditRequest 指针一一对应；由 service 在 DB 原行上合并，
// 合并结果才交给仓储（db.Save 全列写）。
type ProductPatch struct {
	Name        *string
	Cover       *string
	Price       *float64
	Description *string
}

// PoolStat 是单个盲盒的销售进度快照（PoolStats 返回元素）。
type PoolStat struct {
	Remaining int64 `json:"remaining"` // 当前剩余可抽数（Σ pool_items.stock）
	Sold      int64 `json:"sold"`      // 已售出数（drawn 订单计数）
}

// ProductServiceDeps 注入 ProductService 所需的依赖。
type ProductServiceDeps struct {
	BlindBoxRepo  mallRepo.BlindBoxRepository
	CardPoolRepo  mallRepo.CardPoolRepository
	DrawOrderRepo mallRepo.DrawOrderRepository // PoolStats 销量口径；必填（缺失即 panic fail-fast）
	Cache         cache.Cache                  // 用于 Update / ToggleOnSale 失效 C 端详情缓存；nil 时跳过
	Hotspot       hotspot.HotspotCache         // D21：写路径同时失效热点详情；nil 时跳过
	TxRunner      txRunner                     // 可选；nil 时走 repository.RunInTx
}

// baseProductService 是 ProductService 的默认实现。
type baseProductService struct {
	bb      mallRepo.BlindBoxRepository
	cp      mallRepo.CardPoolRepository
	do      mallRepo.DrawOrderRepository
	cache   cache.Cache
	hotspot hotspot.HotspotCache
	txr     txRunner
}

// NewProductService 接收依赖，返回 ProductService 接口。
func NewProductService(d ProductServiceDeps) ProductService {
	txr := d.TxRunner
	if txr == nil {
		txr = defaultTxRunner{}
	}
	return &baseProductService{
		bb:      d.BlindBoxRepo,
		cp:      d.CardPoolRepo,
		do:      d.DrawOrderRepo,
		cache:   d.Cache,
		hotspot: d.Hotspot,
		txr:     txr,
	}
}

// defaultTxRunner 走 repository.RunInTx（生产路径）。
type defaultTxRunner struct{}

func (defaultTxRunner) Run(c *gin.Context, fn func(txCtx *gin.Context) error) error {
	return repository.RunInTx(c, fn)
}

// List 实现见 ProductService 注释。
func (s *baseProductService) List(c *gin.Context, supplierID int64, lifecycle mall.MallBlindBoxLifecycle, opts repository.ListOptions) ([]mall.MallBlindBox, int64, error) {
	if supplierID <= 0 {
		return []mall.MallBlindBox{}, 0, nil
	}
	return s.bb.ListBySupplier(c, supplierID, lifecycle, opts)
}

// PoolStats 实现见 ProductService 注释。
func (s *baseProductService) PoolStats(c *gin.Context, supplierID int64, boxIDs []int64) (map[int64]PoolStat, error) {
	out := make(map[int64]PoolStat)
	if supplierID <= 0 || len(boxIDs) == 0 {
		return out, nil
	}

	// ownership：只统计属于该供应商的 box。
	boxes, err := s.bb.ListByIDs(c, boxIDs)
	if err != nil {
		return nil, err
	}
	valid := make([]int64, 0, len(boxes))
	for _, b := range boxes {
		if b.SupplierID == supplierID {
			valid = append(valid, b.ID)
		}
	}
	if len(valid) == 0 {
		return out, nil
	}

	// 剩余可抽：pools（1:1 box）→ items 按 pool 聚合 → 归到 box。
	pools, err := s.cp.GetPoolsByBlindBoxIDs(c, valid)
	if err != nil {
		return nil, err
	}
	poolIDs := make([]int64, 0, len(pools))
	boxByPool := make(map[int64]int64, len(pools))
	for _, p := range pools {
		poolIDs = append(poolIDs, p.ID)
		boxByPool[p.ID] = p.BlindBoxID
	}
	items, err := s.cp.ListItemsByPoolIDs(c, poolIDs)
	if err != nil {
		return nil, err
	}
	for _, it := range items {
		boxID, ok := boxByPool[it.PoolID]
		if !ok {
			continue
		}
		st := out[boxID]
		st.Remaining += int64(it.Stock)
		out[boxID] = st
	}

	// 已售：drawn 订单计数（终态口径）。
	sold, err := s.do.CountDrawnByBoxIDs(c, valid)
	if err != nil {
		return nil, err
	}
	for boxID, cnt := range sold {
		st := out[boxID]
		st.Sold = cnt
		out[boxID] = st
	}
	return out, nil
}

// Create 实现见 ProductService 注释。
func (s *baseProductService) Create(c *gin.Context, supplierID int64, blindBox *mall.MallBlindBox, items []mall.MallCardPoolItem) (*mall.MallBlindBox, error) {
	if supplierID <= 0 {
		return nil, ErrBlindBoxForbidden
	}
	if blindBox == nil {
		return nil, ErrBlindBoxNotFound
	}
	if blindBox.Name == "" {
		return nil, ErrBlindBoxNotFound // 复用「数据无效」错误（handler 映射 422）
	}
	if len(items) == 0 {
		return nil, ErrEmptyPool
	}

	// D2 校验：items.weight 加和 == 10000。
	var totalWeight int64
	for _, it := range items {
		totalWeight += int64(it.Weight)
	}
	if totalWeight != weightTotal {
		return nil, ErrInvalidWeightSum
	}

	// 强制赋 supplierID = 当前 supplier（防越权赋值）。
	blindBox.SupplierID = supplierID

	// 事务内：INSERT blind_box → INSERT pool → INSERT items。
	err := s.txr.Run(c, func(txCtx *gin.Context) error {
		if err := s.bb.Create(txCtx, blindBox); err != nil {
			return err
		}
		pool := &mall.MallCardPool{BlindBoxID: blindBox.ID}
		if err := s.cp.CreatePool(txCtx, pool); err != nil {
			return err
		}
		// 回填 items.PoolID。
		for i := range items {
			items[i].PoolID = pool.ID
		}
		return s.cp.CreateItems(txCtx, items)
	})
	if err != nil {
		return nil, err
	}
	return blindBox, nil
}

// Update 实现见 ProductService 注释。
func (s *baseProductService) Update(c *gin.Context, supplierID, blindBoxID int64, patch ProductPatch) error {
	if supplierID <= 0 {
		return ErrBlindBoxForbidden
	}
	if blindBoxID <= 0 {
		return ErrBlindBoxNotFound
	}

	// Ownership 自检：先查到原行，比对 SupplierID；原行同时作为字段合并的基准。
	existing, err := s.bb.GetByID(c, blindBoxID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrBlindBoxNotFound
		}
		return err
	}
	if existing == nil {
		return ErrBlindBoxNotFound
	}
	if existing.SupplierID != supplierID {
		return ErrBlindBoxForbidden
	}

	// 只覆盖传入的字段（nil = 不改）。supplier_id / status / is_featured 天然保持
	// 原值（admin 的领域），created_at / updated_at 也来自原行 —— 交给仓储的
	// 因此是完整行，db.Save 不会再把未传字段写成零值。
	if patch.Name != nil {
		existing.Name = *patch.Name
	}
	if patch.Cover != nil {
		existing.Cover = *patch.Cover
	}
	if patch.Price != nil {
		existing.Price = *patch.Price
	}
	if patch.Description != nil {
		existing.Description = *patch.Description
	}

	if err := s.bb.Update(c, existing); err != nil {
		return err
	}

	// 失效 C 端详情缓存（best-effort；失败不阻塞主流程）。
	if s.cache != nil {
		_ = userSvc.InvalidateDetail(c.Request.Context(), s.cache, s.hotspot, existing.ID)
	}
	return nil
}

// ToggleOnSale 实现见 ProductService 注释。
func (s *baseProductService) ToggleOnSale(c *gin.Context, supplierID, blindBoxID int64, onSale bool) error {
	if supplierID <= 0 {
		return ErrBlindBoxForbidden
	}
	if blindBoxID <= 0 {
		return ErrBlindBoxNotFound
	}

	// Ownership 自检（同 Update）。
	existing, err := s.bb.GetByID(c, blindBoxID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrBlindBoxNotFound
		}
		return err
	}
	if existing == nil {
		return ErrBlindBoxNotFound
	}
	if existing.SupplierID != supplierID {
		return ErrBlindBoxForbidden
	}

	// 生命周期守卫：sold_out（售罄结算中）/ settled（已结算归档）不允许再上架 ——
	// 卖完的批次不可逆进入结算；允许 toggle 会让归档商品重新进入 C 端曝光。
	if existing.Lifecycle != mall.LifecycleSelling {
		return ErrLifecycleForbidden
	}

	if err := s.bb.ToggleOnSale(c, blindBoxID, onSale); err != nil {
		return err
	}

	if s.cache != nil {
		_ = userSvc.InvalidateDetail(c.Request.Context(), s.cache, s.hotspot, blindBoxID)
	}
	return nil
}

// Duplicate 实现见 ProductService 注释。
func (s *baseProductService) Duplicate(c *gin.Context, supplierID, sourceBoxID int64) (int64, error) {
	if supplierID <= 0 {
		return 0, ErrBlindBoxForbidden
	}
	if sourceBoxID <= 0 {
		return 0, ErrBlindBoxNotFound
	}

	// ownership 自检（同 Update）：不属于当前 supplier → 403。
	src, err := s.bb.GetByID(c, sourceBoxID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, ErrBlindBoxNotFound
		}
		return 0, err
	}
	if src == nil || src.SupplierID != supplierID {
		return 0, ErrBlindBoxForbidden
	}

	// 拉源卡池结构（1 商品 1 卡池）。
	srcPool, err := s.cp.GetPoolByBlindBoxID(c, sourceBoxID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, ErrEmptyPool
		}
		return 0, err
	}
	srcItems, err := s.cp.ListItemsByPoolID(c, srcPool.ID)
	if err != nil {
		return 0, err
	}
	if len(srcItems) == 0 {
		return 0, ErrEmptyPool
	}

	// 构造新商品实例（商品即批次：新批次 = 新商品）。
	box := &mall.MallBlindBox{
		SupplierID:  supplierID,
		Name:        nextBatchName(src.Name),
		Cover:       src.Cover,
		Price:       src.Price,
		Status:      mall.StatusActive,
		OnSale:      false, // 创建默认下架：新批次 stock=0，填好库存再上架
		Lifecycle:   mall.LifecycleSelling,
		Description: src.Description,
		// IsFeatured 不复制：推荐是 admin 领域。
	}

	// 复制卡池 items：weight 结构保留，stock 全部置 0（必须重填）。
	items := make([]mall.MallCardPoolItem, 0, len(srcItems))
	for _, it := range srcItems {
		items = append(items, mall.MallCardPoolItem{
			CardID: it.CardID,
			Rarity: it.Rarity,
			Weight: it.Weight,
			Stock:  0,
		})
	}

	// 事务内复制：INSERT box → pool → items（与 Create 同序，任一步失败整体回滚）。
	if err := s.txr.Run(c, func(txCtx *gin.Context) error {
		if err := s.bb.Create(txCtx, box); err != nil {
			return err
		}
		pool := &mall.MallCardPool{BlindBoxID: box.ID}
		if err := s.cp.CreatePool(txCtx, pool); err != nil {
			return err
		}
		for i := range items {
			items[i].PoolID = pool.ID
		}
		return s.cp.CreateItems(txCtx, items)
	}); err != nil {
		return 0, err
	}
	return box.ID, nil
}

// batchPeriodRe 匹配名称中的「第 N 期」片段（允许空格）。
var batchPeriodRe = regexp.MustCompile(`第\s*(\d+)\s*期`)

// nextBatchName 生成下一期名称：
//   - 名称含「第 N 期」→ 取最后一个片段，N 递增（"星愿 第 3 期" → "星愿 第 4 期"）；
//   - 无期数 → 追加「第 2 期」（"星愿卡包" → "星愿卡包 第 2 期"）。
func nextBatchName(name string) string {
	locs := batchPeriodRe.FindAllStringSubmatchIndex(name, -1)
	if len(locs) == 0 {
		return strings.TrimSpace(name) + " 第 2 期"
	}
	loc := locs[len(locs)-1]
	n, err := strconv.Atoi(name[loc[2]:loc[3]])
	if err != nil {
		// 正则保证 \d+，防御性兜底：退化为追加。
		return strings.TrimSpace(name) + " 第 2 期"
	}
	// 只替换数字捕获组，「第 / 期」字样保留。
	return name[:loc[2]] + strconv.Itoa(n+1) + name[loc[3]:]
}

// 编译期断言：baseProductService 必须实现 ProductService。
var _ ProductService = (*baseProductService)(nil)
