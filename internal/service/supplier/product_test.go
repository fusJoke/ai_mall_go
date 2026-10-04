package supplier

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"ai-go-mall/internal/infra/cache"
	"ai-go-mall/internal/model/mall"
	"ai-go-mall/internal/repository"
	mallRepo "ai-go-mall/internal/repository/mall"
)

// =============================================================================
// helpers
// =============================================================================

func newProductCtx() *gin.Context {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", "/api/v1/supplier/products", nil)
	return c
}

// mockTxRunner 是 txRunner 的最小实现：同步执行 fn，不开真事务。
//
// 单元测试场景下 repo 已被 mock，所以「不开 tx」是 OK 的。
type mockTxRunner struct {
	runFunc func(c *gin.Context, fn func(txCtx *gin.Context) error) error
	runs    int
}

func (m *mockTxRunner) Run(c *gin.Context, fn func(txCtx *gin.Context) error) error {
	m.runs++
	if m.runFunc != nil {
		return m.runFunc(c, fn)
	}
	return fn(c)
}

var _ txRunner = (*mockTxRunner)(nil)

// =============================================================================
// Mocks
// =============================================================================

type productBlindBoxRepo struct {
	createFunc         func(c *gin.Context, e *mall.MallBlindBox) error
	updateFunc         func(c *gin.Context, e *mall.MallBlindBox) error
	getByIDFunc        func(c *gin.Context, id int64) (*mall.MallBlindBox, error)
	listBySupplierFunc func(c *gin.Context, sid int64, lifecycle mall.MallBlindBoxLifecycle, opts repository.ListOptions) ([]mall.MallBlindBox, int64, error)
	listByIDsFunc      func(c *gin.Context, ids []int64) ([]mall.MallBlindBox, error)
	toggleOnSaleFunc   func(c *gin.Context, id int64, onSale bool) error

	createCalls, updateCalls, getByIDCalls, toggleOnSaleCalls int
	lastCreated                                               *mall.MallBlindBox
	lastUpdated                                               *mall.MallBlindBox
	lastToggleID                                              int64
	lastToggleOnSale                                          bool
}

func (m *productBlindBoxRepo) Create(c *gin.Context, e *mall.MallBlindBox) error {
	m.createCalls++
	m.lastCreated = e
	// 主键自增（DB 列足够，count 即可）。
	e.ID = int64(m.createCalls)
	if m.createFunc != nil {
		return m.createFunc(c, e)
	}
	return nil
}
func (m *productBlindBoxRepo) Update(c *gin.Context, e *mall.MallBlindBox) error {
	m.updateCalls++
	m.lastUpdated = e
	if m.updateFunc != nil {
		return m.updateFunc(c, e)
	}
	return nil
}
func (m *productBlindBoxRepo) GetByID(c *gin.Context, id int64) (*mall.MallBlindBox, error) {
	m.getByIDCalls++
	if m.getByIDFunc != nil {
		return m.getByIDFunc(c, id)
	}
	return nil, gorm.ErrRecordNotFound
}
func (m *productBlindBoxRepo) ListBySupplier(c *gin.Context, sid int64, lifecycle mall.MallBlindBoxLifecycle, opts repository.ListOptions) ([]mall.MallBlindBox, int64, error) {
	if m.listBySupplierFunc != nil {
		return m.listBySupplierFunc(c, sid, lifecycle, opts)
	}
	return nil, 0, nil
}
func (m *productBlindBoxRepo) ToggleOnSale(c *gin.Context, id int64, onSale bool) error {
	m.toggleOnSaleCalls++
	m.lastToggleID = id
	m.lastToggleOnSale = onSale
	if m.toggleOnSaleFunc != nil {
		return m.toggleOnSaleFunc(c, id, onSale)
	}
	return nil
}

// 其余 BlindBoxRepository 接口方法：no-op。
func (m *productBlindBoxRepo) List(c *gin.Context, opts repository.ListOptions) ([]mall.MallBlindBox, int64, error) {
	return nil, 0, nil
}
func (m *productBlindBoxRepo) ListFeaturedOnSale(c *gin.Context, opts repository.ListOptions) ([]mall.MallBlindBox, int64, error) {
	return nil, 0, nil
}
func (m *productBlindBoxRepo) ListActiveOnSaleBySupplierIDs(c *gin.Context, ids []int64, opts repository.ListOptions) ([]mall.MallBlindBox, int64, error) {
	return nil, 0, nil
}
func (m *productBlindBoxRepo) ListByIDs(c *gin.Context, ids []int64) ([]mall.MallBlindBox, error) {
	if m.listByIDsFunc != nil {
		return m.listByIDsFunc(c, ids)
	}
	return nil, nil
}
func (m *productBlindBoxRepo) ToggleStatus(c *gin.Context, id int64, s mall.MallBlindBoxStatus) error {
	return nil
}
func (m *productBlindBoxRepo) ToggleFeatured(c *gin.Context, id int64, f bool) error { return nil }
func (m *productBlindBoxRepo) Delete(c *gin.Context, id int64) error                 { return nil }

var _ mallRepo.BlindBoxRepository = (*productBlindBoxRepo)(nil)

type productCardPoolRepo struct {
	createPoolFunc        func(f *m_unused_anchor) error // 占位避免编译器报 unused-arg
	createItemsFunc       func(c *gin.Context, items []mall.MallCardPoolItem) error
	getPoolFunc           func(c *gin.Context, bbID int64) (*mall.MallCardPool, error)
	getPoolsFunc           func(c *gin.Context, bbIDs []int64) ([]mall.MallCardPool, error)
	listItemsByPoolIDFunc  func(c *gin.Context, poolID int64) ([]mall.MallCardPoolItem, error)
	listItemsByPoolIDsFunc func(c *gin.Context, poolIDs []int64) ([]mall.MallCardPoolItem, error)
	deleteItemsFunc        func(c *gin.Context, poolID int64) error

	createPoolCalls, createItemsCalls int
	deleteItemsCalls                  int
	lastPoolID                        int64
	lastDeletedPoolID                 int64
	lastItems                         []mall.MallCardPoolItem
}

// m_unused_anchor 是占位类型，本文件其他地方会用到，避免 go vet 误报。
type m_unused_anchor struct{}

func (m *productCardPoolRepo) CreatePool(c *gin.Context, pool *mall.MallCardPool) error {
	m.createPoolCalls++
	if pool != nil {
		m.lastPoolID = pool.BlindBoxID
		pool.ID = int64(m.createPoolCalls * 100) // 给个非零 ID 便于断言 items[i].PoolID 回填
	}
	if m.createPoolFunc != nil {
		return m.createPoolFunc(nil)
	}
	return nil
}
func (m *productCardPoolRepo) CreateItems(c *gin.Context, items []mall.MallCardPoolItem) error {
	m.createItemsCalls++
	m.lastItems = items
	if m.createItemsFunc != nil {
		return m.createItemsFunc(c, items)
	}
	return nil
}
func (m *productCardPoolRepo) DeleteItemsByPoolID(c *gin.Context, poolID int64) error {
	m.deleteItemsCalls++
	m.lastDeletedPoolID = poolID
	if m.deleteItemsFunc != nil {
		return m.deleteItemsFunc(c, poolID)
	}
	return nil
}
func (m *productCardPoolRepo) GetPoolByBlindBoxID(c *gin.Context, bbID int64) (*mall.MallCardPool, error) {
	if m.getPoolFunc != nil {
		return m.getPoolFunc(c, bbID)
	}
	return nil, gorm.ErrRecordNotFound
}

// GetPoolsByBlindBoxIDs PoolStats 用批量查（task 5.3）。
func (m *productCardPoolRepo) GetPoolsByBlindBoxIDs(c *gin.Context, bbIDs []int64) ([]mall.MallCardPool, error) {
	if m.getPoolsFunc != nil {
		return m.getPoolsFunc(c, bbIDs)
	}
	return nil, nil
}

// 其他方法 no-op。
func (m *productCardPoolRepo) ListItemsByPoolID(c *gin.Context, poolID int64) ([]mall.MallCardPoolItem, error) {
	if m.listItemsByPoolIDFunc != nil {
		return m.listItemsByPoolIDFunc(c, poolID)
	}
	return nil, nil
}
func (m *productCardPoolRepo) ListItemsForUpdateByPoolID(c *gin.Context, poolID int64) ([]mall.MallCardPoolItem, error) {
	return nil, nil
}
func (m *productCardPoolRepo) ListItemsByPoolIDs(c *gin.Context, poolIDs []int64) ([]mall.MallCardPoolItem, error) {
	if m.listItemsByPoolIDsFunc != nil {
		return m.listItemsByPoolIDsFunc(c, poolIDs)
	}
	return nil, nil
}
func (m *productCardPoolRepo) CreateItem(c *gin.Context, item *mall.MallCardPoolItem) error {
	return nil
}
func (m *productCardPoolRepo) DecrementStock(c *gin.Context, itemID int64) error { return nil }
func (m *productCardPoolRepo) IncrementStock(c *gin.Context, itemID int64, delta int) error {
	return nil
}
func (m *productCardPoolRepo) SumWeightByPoolID(c *gin.Context, poolID int64) (int64, error) {
	return 0, nil
}
func (m *productCardPoolRepo) SubtotalStockByPoolID(c *gin.Context, poolID int64) (int64, error) {
	return 0, nil
}

var _ mallRepo.CardPoolRepository = (*productCardPoolRepo)(nil)

// mockCache 模拟 cache.Cache，记录最后一次 Del。
type mockCache struct {
	delFunc  func(ctx context.Context, key string) error
	delCalls int
	lastKey  string
}

func (m *mockCache) Get(ctx context.Context, key string) (string, bool, error) { return "", false, nil }
func (m *mockCache) Set(ctx context.Context, key string, value string, ttl time.Duration) error {
	return nil
}
func (m *mockCache) Del(ctx context.Context, key string) error {
	m.delCalls++
	m.lastKey = key
	if m.delFunc != nil {
		return m.delFunc(ctx, key)
	}
	return nil
}
func (m *mockCache) SetNX(ctx context.Context, key string, value string, ttl time.Duration) (bool, error) {
	return true, nil
}
func (m *mockCache) Incr(ctx context.Context, key string) (int64, error) {
	return 0, nil
}
func (m *mockCache) Decr(ctx context.Context, key string) (int64, error) {
	return 0, nil
}

// productDrawOrderRepo 嵌入 nil 接口 + 按需 override（PoolStats 用）。
type productDrawOrderRepo struct {
	mallRepo.DrawOrderRepository

	countDrawnFunc  func(c *gin.Context, boxIDs []int64) (map[int64]int64, error)
	countDrawnCalls int
	lastBoxIDs      []int64
}

func (m *productDrawOrderRepo) CountDrawnByBoxIDs(c *gin.Context, boxIDs []int64) (map[int64]int64, error) {
	m.countDrawnCalls++
	m.lastBoxIDs = boxIDs
	if m.countDrawnFunc != nil {
		return m.countDrawnFunc(c, boxIDs)
	}
	return map[int64]int64{}, nil
}

var _ cache.Cache = (*mockCache)(nil)

// =============================================================================
// 默认依赖 + 工厂
// =============================================================================

type productDeps struct {
	bb    *productBlindBoxRepo
	cp    *productCardPoolRepo
	do    *productDrawOrderRepo
	cache *mockCache
	txr   *mockTxRunner
}

func defaultProductDeps() productDeps {
	return productDeps{
		bb:    &productBlindBoxRepo{},
		cp:    &productCardPoolRepo{},
		do:    &productDrawOrderRepo{},
		cache: &mockCache{},
		txr:   &mockTxRunner{},
	}
}

func newProductSvcFromDeps(d productDeps) ProductService {
	return NewProductService(ProductServiceDeps{
		BlindBoxRepo:  d.bb,
		CardPoolRepo:  d.cp,
		DrawOrderRepo: d.do,
		Cache:         d.cache,
		TxRunner:      d.txr,
	})
}

// =============================================================================
// List
// =============================================================================

func TestProductList_InvalidSupplierID(t *testing.T) {
	d := defaultProductDeps()
	svc := newProductSvcFromDeps(d)
	items, total, err := svc.List(newProductCtx(), 0, "", repository.ListOptions{Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(items) != 0 || total != 0 {
		t.Errorf("got (%d, %d), want (0, 0)", len(items), total)
	}
}

func TestProductList_DelegatesToRepo(t *testing.T) {
	d := defaultProductDeps()
	d.bb.listBySupplierFunc = func(c *gin.Context, sid int64, lifecycle mall.MallBlindBoxLifecycle, opts repository.ListOptions) ([]mall.MallBlindBox, int64, error) {
		if sid != 7 {
			t.Errorf("supplierID = %d, want 7", sid)
		}
		if lifecycle != mall.LifecycleSelling {
			t.Errorf("lifecycle = %q, want selling", lifecycle)
		}
		return []mall.MallBlindBox{
			{ID: 1, SupplierID: 7, Name: "Box A"},
			{ID: 2, SupplierID: 7, Name: "Box B"},
		}, 2, nil
	}
	svc := newProductSvcFromDeps(d)
	items, total, err := svc.List(newProductCtx(), 7, mall.LifecycleSelling, repository.ListOptions{Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if total != 2 || len(items) != 2 {
		t.Errorf("got (%d, %d), want (2, 2)", total, len(items))
	}
}

// =============================================================================
// PoolStats（任务 5.3 销售进度）
// =============================================================================

// TestProductService_PoolStats_Aggregates 验证剩余库存跨 pool 聚合 + 销量归箱。
func TestProductService_PoolStats_Aggregates(t *testing.T) {
	d := defaultProductDeps()
	d.bb.listByIDsFunc = func(c *gin.Context, ids []int64) ([]mall.MallBlindBox, error) {
		if len(ids) != 2 {
			t.Errorf("ListByIDs ids = %v, want [1 2]", ids)
		}
		return []mall.MallBlindBox{{ID: 1, SupplierID: 7}, {ID: 2, SupplierID: 7}}, nil
	}
	d.cp.getPoolsFunc = func(c *gin.Context, bbIDs []int64) ([]mall.MallCardPool, error) {
		return []mall.MallCardPool{{ID: 11, BlindBoxID: 1}, {ID: 12, BlindBoxID: 2}}, nil
	}
	d.cp.listItemsByPoolIDsFunc = func(c *gin.Context, poolIDs []int64) ([]mall.MallCardPoolItem, error) {
		if len(poolIDs) != 2 {
			t.Errorf("ListItemsByPoolIDs poolIDs = %v, want [11 12]", poolIDs)
		}
		// pool11 两行 (stock 3+4) 验证同 box 聚合；pool12 一行 (stock 0)。
		return []mall.MallCardPoolItem{
			{PoolID: 11, Stock: 3},
			{PoolID: 11, Stock: 4},
			{PoolID: 12, Stock: 0},
		}, nil
	}
	d.do.countDrawnFunc = func(c *gin.Context, boxIDs []int64) (map[int64]int64, error) {
		return map[int64]int64{1: 97, 2: 5}, nil
	}
	svc := newProductSvcFromDeps(d)

	out, err := svc.PoolStats(newProductCtx(), 7, []int64{1, 2})
	if err != nil {
		t.Fatalf("PoolStats: %v", err)
	}
	if got := out[1]; got.Remaining != 7 || got.Sold != 97 {
		t.Errorf("stats[1] = %+v, want remaining=7 sold=97", got)
	}
	if got := out[2]; got.Remaining != 0 || got.Sold != 5 {
		t.Errorf("stats[2] = %+v, want remaining=0 sold=5", got)
	}
	if d.do.countDrawnCalls != 1 {
		t.Errorf("CountDrawnByBoxIDs calls = %d, want 1", d.do.countDrawnCalls)
	}
}

// TestProductService_PoolStats_FiltersForeignBoxes 验证越权 box 被剔除：
// 非本供应商的 boxID 不进 pool / order 查询（防跨供应商枚举库存）。
func TestProductService_PoolStats_FiltersForeignBoxes(t *testing.T) {
	d := defaultProductDeps()
	d.bb.listByIDsFunc = func(c *gin.Context, ids []int64) ([]mall.MallBlindBox, error) {
		return []mall.MallBlindBox{
			{ID: 1, SupplierID: 7},   // 本供应商
			{ID: 2, SupplierID: 99},  // 越权：别人的批次
		}, nil
	}
	gotPoolIDs := []int64(nil)
	d.cp.getPoolsFunc = func(c *gin.Context, bbIDs []int64) ([]mall.MallCardPool, error) {
		if len(bbIDs) != 1 || bbIDs[0] != 1 {
			t.Errorf("pools query boxIDs = %v, want [1] (越权 box 已剔除)", bbIDs)
		}
		gotPoolIDs = bbIDs
		return []mall.MallCardPool{{ID: 11, BlindBoxID: 1}}, nil
	}
	d.cp.listItemsByPoolIDsFunc = func(c *gin.Context, poolIDs []int64) ([]mall.MallCardPoolItem, error) {
		return []mall.MallCardPoolItem{{PoolID: 11, Stock: 9}}, nil
	}
	d.do.countDrawnFunc = func(c *gin.Context, boxIDs []int64) (map[int64]int64, error) {
		return map[int64]int64{1: 3}, nil
	}
	svc := newProductSvcFromDeps(d)

	out, err := svc.PoolStats(newProductCtx(), 7, []int64{1, 2})
	if err != nil {
		t.Fatalf("PoolStats: %v", err)
	}
	if _, exists := out[2]; exists {
		t.Errorf("foreign box 2 must not appear in stats: %v", out)
	}
	if got := out[1]; got.Remaining != 9 || got.Sold != 3 {
		t.Errorf("stats[1] = %+v, want remaining=9 sold=3", got)
	}
	if gotPoolIDs == nil {
		t.Errorf("pools query never ran")
	}
}

// TestProductService_PoolStats_EmptyInput 验证空 boxIDs 短路。
func TestProductService_PoolStats_EmptyInput(t *testing.T) {
	d := defaultProductDeps()
	svc := newProductSvcFromDeps(d)
	out, err := svc.PoolStats(newProductCtx(), 7, nil)
	if err != nil {
		t.Fatalf("PoolStats: %v", err)
	}
	if len(out) != 0 {
		t.Errorf("out = %v, want empty", out)
	}
}

// =============================================================================
// Detail / UpdatePool（任务 5.3：编辑页预填 + 卡池重填闭环）
// =============================================================================

func TestProductService_Detail_Success(t *testing.T) {
	d := defaultProductDeps()
	d.bb.getByIDFunc = func(c *gin.Context, id int64) (*mall.MallBlindBox, error) {
		return &mall.MallBlindBox{ID: 9, SupplierID: 7, Lifecycle: mall.LifecycleSelling}, nil
	}
	d.cp.getPoolFunc = func(c *gin.Context, bbID int64) (*mall.MallCardPool, error) {
		return &mall.MallCardPool{ID: 55, BlindBoxID: 9}, nil
	}
	d.cp.listItemsByPoolIDFunc = func(c *gin.Context, poolID int64) ([]mall.MallCardPoolItem, error) {
		return []mall.MallCardPoolItem{{PoolID: 55, CardID: 1, Stock: 10}}, nil
	}
	svc := newProductSvcFromDeps(d)

	box, items, err := svc.Detail(newProductCtx(), 7, 9)
	if err != nil {
		t.Fatalf("Detail: %v", err)
	}
	if box.ID != 9 || len(items) != 1 || items[0].Stock != 10 {
		t.Errorf("box=%+v items=%+v", box, items)
	}
}

func TestProductService_Detail_Forbidden(t *testing.T) {
	d := defaultProductDeps()
	d.bb.getByIDFunc = func(c *gin.Context, id int64) (*mall.MallBlindBox, error) {
		return &mall.MallBlindBox{ID: 9, SupplierID: 99}, nil
	}
	svc := newProductSvcFromDeps(d)

	_, _, err := svc.Detail(newProductCtx(), 7, 9)
	if !errors.Is(err, ErrBlindBoxForbidden) {
		t.Errorf("err = %v, want ErrBlindBoxForbidden", err)
	}
}

func TestProductService_UpdatePool_Success(t *testing.T) {
	d := defaultProductDeps()
	d.bb.getByIDFunc = func(c *gin.Context, id int64) (*mall.MallBlindBox, error) {
		return &mall.MallBlindBox{ID: 9, SupplierID: 7, Lifecycle: mall.LifecycleSelling}, nil
	}
	d.do.countDrawnFunc = func(c *gin.Context, boxIDs []int64) (map[int64]int64, error) {
		return map[int64]int64{9: 0}, nil // 零订单
	}
	d.cp.getPoolFunc = func(c *gin.Context, bbID int64) (*mall.MallCardPool, error) {
		return &mall.MallCardPool{ID: 55, BlindBoxID: 9}, nil
	}
	svc := newProductSvcFromDeps(d)

	items := validItems() // weight 加和 10000
	err := svc.UpdatePool(newProductCtx(), 7, 9, items)
	if err != nil {
		t.Fatalf("UpdatePool: %v", err)
	}
	if d.cp.deleteItemsCalls != 1 || d.cp.lastDeletedPoolID != 55 {
		t.Errorf("delete calls=%d poolID=%d, want 1/55", d.cp.deleteItemsCalls, d.cp.lastDeletedPoolID)
	}
	if d.cp.lastItems == nil || len(d.cp.lastItems) != len(items) {
		t.Fatalf("created items = %v", d.cp.lastItems)
	}
	for _, it := range d.cp.lastItems {
		if it.PoolID != 55 {
			t.Errorf("item PoolID = %d, want 55（回填新池）", it.PoolID)
		}
	}
	if d.cache.delCalls == 0 {
		t.Errorf("detail cache should be invalidated")
	}
}

func TestProductService_UpdatePool_LifecycleForbidden(t *testing.T) {
	d := defaultProductDeps()
	d.bb.getByIDFunc = func(c *gin.Context, id int64) (*mall.MallBlindBox, error) {
		return &mall.MallBlindBox{ID: 9, SupplierID: 7, Lifecycle: mall.LifecycleSoldOut}, nil
	}
	svc := newProductSvcFromDeps(d)

	err := svc.UpdatePool(newProductCtx(), 7, 9, validItems())
	if !errors.Is(err, ErrLifecycleForbidden) {
		t.Errorf("err = %v, want ErrLifecycleForbidden", err)
	}
	if d.cp.deleteItemsCalls != 0 {
		t.Errorf("delete calls = %d, want 0", d.cp.deleteItemsCalls)
	}
}

func TestProductService_UpdatePool_PoolLocked(t *testing.T) {
	d := defaultProductDeps()
	d.bb.getByIDFunc = func(c *gin.Context, id int64) (*mall.MallBlindBox, error) {
		return &mall.MallBlindBox{ID: 9, SupplierID: 7, Lifecycle: mall.LifecycleSelling}, nil
	}
	d.do.countDrawnFunc = func(c *gin.Context, boxIDs []int64) (map[int64]int64, error) {
		return map[int64]int64{9: 3}, nil // 已开卖
	}
	svc := newProductSvcFromDeps(d)

	err := svc.UpdatePool(newProductCtx(), 7, 9, validItems())
	if !errors.Is(err, ErrPoolLocked) {
		t.Errorf("err = %v, want ErrPoolLocked", err)
	}
	if d.cp.deleteItemsCalls != 0 {
		t.Errorf("delete calls = %d, want 0", d.cp.deleteItemsCalls)
	}
}

func TestProductService_UpdatePool_InvalidWeightSum(t *testing.T) {
	d := defaultProductDeps()
	svc := newProductSvcFromDeps(d)

	bad := []mall.MallCardPoolItem{{CardID: 1, Rarity: mall.RaritySSR, Weight: 1, Stock: 5}}
	err := svc.UpdatePool(newProductCtx(), 7, 9, bad)
	if !errors.Is(err, ErrInvalidWeightSum) {
		t.Errorf("err = %v, want ErrInvalidWeightSum", err)
	}
	if d.bb.getByIDCalls != 0 {
		t.Errorf("ownership check should not run before weight validation")
	}
}

// =============================================================================
// Create
// =============================================================================

func validItems() []mall.MallCardPoolItem {
	return []mall.MallCardPoolItem{
		{CardID: 1, Rarity: mall.RaritySSR, Weight: 100, Stock: 10},
		{CardID: 2, Rarity: mall.RarityN, Weight: 9900, Stock: 100},
	}
}

func TestProductCreate_InvalidSupplierID(t *testing.T) {
	d := defaultProductDeps()
	svc := newProductSvcFromDeps(d)
	_, err := svc.Create(newProductCtx(), 0, &mall.MallBlindBox{Name: "Box"}, validItems())
	if !errors.Is(err, ErrBlindBoxForbidden) {
		t.Errorf("err = %v, want ErrBlindBoxForbidden", err)
	}
	if d.bb.createCalls != 0 {
		t.Errorf("repo.Create called %d times, want 0", d.bb.createCalls)
	}
}

func TestProductCreate_NilBlindBox(t *testing.T) {
	d := defaultProductDeps()
	svc := newProductSvcFromDeps(d)
	_, err := svc.Create(newProductCtx(), 1, nil, validItems())
	if !errors.Is(err, ErrBlindBoxNotFound) {
		t.Errorf("err = %v, want ErrBlindBoxNotFound", err)
	}
}

func TestProductCreate_EmptyName(t *testing.T) {
	d := defaultProductDeps()
	svc := newProductSvcFromDeps(d)
	_, err := svc.Create(newProductCtx(), 1, &mall.MallBlindBox{}, validItems())
	if !errors.Is(err, ErrBlindBoxNotFound) {
		t.Errorf("err = %v, want ErrBlindBoxNotFound (空 name)", err)
	}
}

func TestProductCreate_EmptyItems(t *testing.T) {
	d := defaultProductDeps()
	svc := newProductSvcFromDeps(d)
	_, err := svc.Create(newProductCtx(), 1, &mall.MallBlindBox{Name: "Box"}, []mall.MallCardPoolItem{})
	if !errors.Is(err, ErrEmptyPool) {
		t.Errorf("err = %v, want ErrEmptyPool", err)
	}
}

func TestProductCreate_InvalidWeightSum(t *testing.T) {
	d := defaultProductDeps()
	svc := newProductSvcFromDeps(d)
	items := []mall.MallCardPoolItem{
		{CardID: 1, Rarity: mall.RaritySSR, Weight: 50},
		{CardID: 2, Rarity: mall.RarityN, Weight: 50}, // 总和 = 100 ≠ 10000
	}
	_, err := svc.Create(newProductCtx(), 1, &mall.MallBlindBox{Name: "Box"}, items)
	if !errors.Is(err, ErrInvalidWeightSum) {
		t.Errorf("err = %v, want ErrInvalidWeightSum", err)
	}
	if d.txr.runs != 0 {
		t.Errorf("txRunner.Runs = %d, want 0 (校验未过不应开事务)", d.txr.runs)
	}
}

func TestProductCreate_Success(t *testing.T) {
	d := defaultProductDeps()
	svc := newProductSvcFromDeps(d)
	in := &mall.MallBlindBox{
		SupplierID: 999, // 应被覆盖为当前 supplierID
		Name:       "Box 2024 NBA",
		Price:      100,
	}
	res, err := svc.Create(newProductCtx(), 7, in, validItems())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if res == nil {
		t.Fatalf("Create returned nil")
	}
	if res.SupplierID != 7 {
		t.Errorf("res.SupplierID = %d, want 7 (override)", res.SupplierID)
	}
	if res.ID == 0 {
		t.Errorf("res.ID = 0, want > 0")
	}
	if d.txr.runs != 1 {
		t.Errorf("txRunner.Runs = %d, want 1", d.txr.runs)
	}
	if d.bb.createCalls != 1 {
		t.Errorf("BlindBox.Create called %d times, want 1", d.bb.createCalls)
	}
	if d.cp.createPoolCalls != 1 {
		t.Errorf("CardPool.CreatePool called %d times, want 1", d.cp.createPoolCalls)
	}
	if d.cp.createItemsCalls != 1 {
		t.Errorf("CardPool.CreateItems called %d times, want 1", d.cp.createItemsCalls)
	}
	// items[i].PoolID 必须非空（事务内回填）。
	for i, it := range d.cp.lastItems {
		if it.PoolID == 0 {
			t.Errorf("items[%d].PoolID = 0, want > 0", i)
		}
	}
}

func TestProductCreate_RepoErrorRollsBack(t *testing.T) {
	d := defaultProductDeps()
	wantErr := errors.New("cardpool: insert fail")
	d.cp.createPoolFunc = func(*m_unused_anchor) error {
		return wantErr
	}
	svc := newProductSvcFromDeps(d)
	_, err := svc.Create(newProductCtx(), 7, &mall.MallBlindBox{Name: "Box"}, validItems())
	if !errors.Is(err, wantErr) {
		t.Errorf("err = %v, want %v", err, wantErr)
	}
	// pool 创建失败时，不应再调 CreateItems（事务模拟同步执行，fn 后续步骤被跳过）。
	if d.cp.createItemsCalls != 0 {
		t.Errorf("CreateItems called %d times, want 0 (pool failed before)", d.cp.createItemsCalls)
	}
}

// =============================================================================
// Update
// =============================================================================

// ptrStr / ptrF64 构造「传了这个字段」的 patch 值（nil = 不改）。
func ptrStr(s string) *string   { return &s }
func ptrF64(v float64) *float64 { return &v }

func TestProductUpdate_InvalidSupplierID(t *testing.T) {
	d := defaultProductDeps()
	svc := newProductSvcFromDeps(d)
	err := svc.Update(newProductCtx(), 0, 1, ProductPatch{Name: ptrStr("x")})
	if !errors.Is(err, ErrBlindBoxForbidden) {
		t.Errorf("err = %v, want ErrBlindBoxForbidden", err)
	}
	if d.bb.getByIDCalls != 0 {
		t.Errorf("GetByID called %d times, want 0", d.bb.getByIDCalls)
	}
}

func TestProductUpdate_InvalidBlindBoxID(t *testing.T) {
	d := defaultProductDeps()
	svc := newProductSvcFromDeps(d)
	err := svc.Update(newProductCtx(), 7, 0, ProductPatch{})
	if !errors.Is(err, ErrBlindBoxNotFound) {
		t.Errorf("err = %v, want ErrBlindBoxNotFound", err)
	}
}

func TestProductUpdate_NotFound(t *testing.T) {
	d := defaultProductDeps()
	d.bb.getByIDFunc = func(c *gin.Context, id int64) (*mall.MallBlindBox, error) {
		return nil, gorm.ErrRecordNotFound
	}
	svc := newProductSvcFromDeps(d)
	err := svc.Update(newProductCtx(), 7, 99, ProductPatch{})
	if !errors.Is(err, ErrBlindBoxNotFound) {
		t.Errorf("err = %v, want ErrBlindBoxNotFound", err)
	}
}

func TestProductUpdate_Forbidden(t *testing.T) {
	d := defaultProductDeps()
	d.bb.getByIDFunc = func(c *gin.Context, id int64) (*mall.MallBlindBox, error) {
		// 盲盒属于 supplier 999，不是当前 supplier 7。
		return &mall.MallBlindBox{ID: id, SupplierID: 999, Name: "Other Supplier's Box"}, nil
	}
	svc := newProductSvcFromDeps(d)
	err := svc.Update(newProductCtx(), 7, 1, ProductPatch{Name: ptrStr("hijack")})
	if !errors.Is(err, ErrBlindBoxForbidden) {
		t.Errorf("err = %v, want ErrBlindBoxForbidden", err)
	}
	if d.bb.updateCalls != 0 {
		t.Errorf("Update called %d times, want 0 (forbidden)", d.bb.updateCalls)
	}
}

func TestProductUpdate_Success_AndCacheInvalidation(t *testing.T) {
	d := defaultProductDeps()
	d.bb.getByIDFunc = func(c *gin.Context, id int64) (*mall.MallBlindBox, error) {
		return &mall.MallBlindBox{ID: id, SupplierID: 7, Name: "Old"}, nil
	}
	svc := newProductSvcFromDeps(d)
	// 只传 Name / Price；其余字段由 service 从原行（Name="Old"、SupplierID=7）合并。
	patch := ProductPatch{Name: ptrStr("New Name"), Price: ptrF64(120)}
	if err := svc.Update(newProductCtx(), 7, 5, patch); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if d.bb.updateCalls != 1 {
		t.Errorf("Update called %d times, want 1", d.bb.updateCalls)
	}
	// 合并结果：传入字段被覆盖，未传字段保持原行值，SupplierID 不可被前端覆盖。
	if d.bb.lastUpdated.Name != "New Name" {
		t.Errorf("lastUpdated.Name = %q, want %q", d.bb.lastUpdated.Name, "New Name")
	}
	if d.bb.lastUpdated.Price != 120 {
		t.Errorf("lastUpdated.Price = %v, want 120", d.bb.lastUpdated.Price)
	}
	if d.bb.lastUpdated.ID != 5 {
		t.Errorf("lastUpdated.ID = %d, want 5", d.bb.lastUpdated.ID)
	}
	// SupplierID 强制 = 7（不可被前端覆盖）。
	if d.bb.lastUpdated.SupplierID != 7 {
		t.Errorf("lastUpdated.SupplierID = %d, want 7", d.bb.lastUpdated.SupplierID)
	}
	if d.cache.delCalls != 1 {
		t.Errorf("cache.Del called %d times, want 1 (invalidate detail)", d.cache.delCalls)
	}
	if d.cache.lastKey == "" {
		t.Errorf("cache.Del called with empty key")
	}
}

// =============================================================================
// ToggleOnSale
// =============================================================================

func TestProductToggleOnSale_InvalidSupplierID(t *testing.T) {
	d := defaultProductDeps()
	svc := newProductSvcFromDeps(d)
	err := svc.ToggleOnSale(newProductCtx(), 0, 1, true)
	if !errors.Is(err, ErrBlindBoxForbidden) {
		t.Errorf("err = %v, want ErrBlindBoxForbidden", err)
	}
}

func TestProductToggleOnSale_InvalidBlindBoxID(t *testing.T) {
	d := defaultProductDeps()
	svc := newProductSvcFromDeps(d)
	err := svc.ToggleOnSale(newProductCtx(), 7, 0, true)
	if !errors.Is(err, ErrBlindBoxNotFound) {
		t.Errorf("err = %v, want ErrBlindBoxNotFound", err)
	}
}

func TestProductToggleOnSale_NotFound(t *testing.T) {
	d := defaultProductDeps()
	d.bb.getByIDFunc = func(c *gin.Context, id int64) (*mall.MallBlindBox, error) {
		return nil, gorm.ErrRecordNotFound
	}
	svc := newProductSvcFromDeps(d)
	err := svc.ToggleOnSale(newProductCtx(), 7, 99, false)
	if !errors.Is(err, ErrBlindBoxNotFound) {
		t.Errorf("err = %v, want ErrBlindBoxNotFound", err)
	}
}

func TestProductToggleOnSale_Forbidden(t *testing.T) {
	d := defaultProductDeps()
	d.bb.getByIDFunc = func(c *gin.Context, id int64) (*mall.MallBlindBox, error) {
		return &mall.MallBlindBox{ID: id, SupplierID: 999}, nil
	}
	svc := newProductSvcFromDeps(d)
	err := svc.ToggleOnSale(newProductCtx(), 7, 1, true)
	if !errors.Is(err, ErrBlindBoxForbidden) {
		t.Errorf("err = %v, want ErrBlindBoxForbidden", err)
	}
	if d.bb.toggleOnSaleCalls != 0 {
		t.Errorf("ToggleOnSale called %d times, want 0", d.bb.toggleOnSaleCalls)
	}
}

func TestProductToggleOnSale_Success_OnAndOff(t *testing.T) {
	d := defaultProductDeps()
	d.bb.getByIDFunc = func(c *gin.Context, id int64) (*mall.MallBlindBox, error) {
		return &mall.MallBlindBox{ID: id, SupplierID: 7, Lifecycle: mall.LifecycleSelling}, nil
	}
	svc := newProductSvcFromDeps(d)

	// on
	if err := svc.ToggleOnSale(newProductCtx(), 7, 1, true); err != nil {
		t.Fatalf("ToggleOnSale true: %v", err)
	}
	if d.bb.toggleOnSaleCalls != 1 || !d.bb.lastToggleOnSale || d.bb.lastToggleID != 1 {
		t.Errorf("on-toggle state = (%d, %v, id=%d), want (1, true, 1)",
			d.bb.toggleOnSaleCalls, d.bb.lastToggleOnSale, d.bb.lastToggleID)
	}
	if d.cache.delCalls != 1 {
		t.Errorf("cache.Del called %d times, want 1", d.cache.delCalls)
	}

	// off
	if err := svc.ToggleOnSale(newProductCtx(), 7, 1, false); err != nil {
		t.Fatalf("ToggleOnSale false: %v", err)
	}
	if d.bb.toggleOnSaleCalls != 2 || d.bb.lastToggleOnSale {
		t.Errorf("off-toggle state = (%d, %v), want (2, false)", d.bb.toggleOnSaleCalls, d.bb.lastToggleOnSale)
	}
	if d.cache.delCalls != 2 {
		t.Errorf("cache.Del called %d times, want 2", d.cache.delCalls)
	}
}

// TestProductToggleOnSale_LifecycleForbidden 售罄/已结算批次不可再上架。
//
// 设计状态机要点：sold_out 不可逆进入结算、settled 是归档终态，off_sale 只适用于
// selling。允许 toggle 会让归档商品重新进入 C 端曝光（配合 Duplicate 的
//「复制 sold_out 商品」路径，攻击面真实存在）。
func TestProductToggleOnSale_LifecycleForbidden(t *testing.T) {
	for _, lc := range []mall.MallBlindBoxLifecycle{mall.LifecycleSoldOut, mall.LifecycleSettled} {
		t.Run(string(lc), func(t *testing.T) {
			d := defaultProductDeps()
			d.bb.getByIDFunc = func(c *gin.Context, id int64) (*mall.MallBlindBox, error) {
				return &mall.MallBlindBox{ID: id, SupplierID: 7, Lifecycle: lc}, nil
			}
			svc := newProductSvcFromDeps(d)

			err := svc.ToggleOnSale(newProductCtx(), 7, 1, true)
			if !errors.Is(err, ErrLifecycleForbidden) {
				t.Errorf("ToggleOnSale(%s) = %v, want ErrLifecycleForbidden", lc, err)
			}
			if d.bb.toggleOnSaleCalls != 0 {
				t.Errorf("ToggleOnSale called %d times, want 0 (守卫必须先于写库)", d.bb.toggleOnSaleCalls)
			}
		})
	}
}

// TestProductUpdate_NilCache_NoPanic 验证 cache == nil 时不阻塞。
//
// 注意：必须用 nil interface 而非 typed nil *mockCache 注入；否则 interface
// 包裹了 typed nil，`s.cache != nil` 仍为 true，会走到 Del 触发 nil receiver panic
// （经典 Go 坑）。
func TestProductUpdate_NilCache_NoPanic(t *testing.T) {
	d := defaultProductDeps()
	d.bb.getByIDFunc = func(c *gin.Context, id int64) (*mall.MallBlindBox, error) {
		return &mall.MallBlindBox{ID: id, SupplierID: 7}, nil
	}
	svc := NewProductService(ProductServiceDeps{
		BlindBoxRepo: d.bb,
		CardPoolRepo: d.cp,
		Cache:        nil, // 真正的 nil interface
		TxRunner:     d.txr,
	})
	if err := svc.Update(newProductCtx(), 7, 1, ProductPatch{Name: ptrStr("x")}); err != nil {
		t.Errorf("Update with nil cache: %v", err)
	}
}

// =============================================================================
// Duplicate（openspec: add-batch-settlement，task 3.4「再来一批」）
// =============================================================================

// ownedSourceBox 返回一个属于 supplier 7 的源商品 fixture（第 3 期）。
func ownedSourceBox() *mall.MallBlindBox {
	return &mall.MallBlindBox{
		ID:          9,
		SupplierID:  7,
		Name:        "星愿卡包 第 3 期",
		Cover:       "cover.png",
		Price:       59.9,
		Description: "老批次描述",
	}
}

// srcPoolItems 返回源卡池 items（weight 保留、stock 有值）。
func srcPoolItems() []mall.MallCardPoolItem {
	return []mall.MallCardPoolItem{
		{ID: 1, PoolID: 500, CardID: 11, Rarity: mall.RaritySSR, Weight: 100, Stock: 0},
		{ID: 2, PoolID: 500, CardID: 12, Rarity: mall.RarityN, Weight: 9900, Stock: 95},
	}
}

// withSourceBox 把 fake 装配成「源商品存在且属于 supplier 7」的形态。
func withSourceBox(d productDeps) productDeps {
	d.bb.getByIDFunc = func(c *gin.Context, id int64) (*mall.MallBlindBox, error) {
		return ownedSourceBox(), nil
	}
	d.cp.getPoolFunc = func(c *gin.Context, bbID int64) (*mall.MallCardPool, error) {
		return &mall.MallCardPool{ID: 500, BlindBoxID: bbID}, nil
	}
	d.cp.listItemsByPoolIDFunc = func(c *gin.Context, poolID int64) ([]mall.MallCardPoolItem, error) {
		return srcPoolItems(), nil
	}
	return d
}

// TestProductDuplicate_Happy 正常复制：字段/卡池/重置规则/期数递增全链路。
func TestProductDuplicate_Happy(t *testing.T) {
	d := withSourceBox(defaultProductDeps())
	svc := newProductSvcFromDeps(d)

	newID, err := svc.Duplicate(newProductCtx(), 7, 9)
	if err != nil {
		t.Fatalf("Duplicate: %v", err)
	}
	if newID != 1 {
		t.Errorf("newID = %d, want 1 (fake 自增)", newID)
	}

	// 新商品：名称期数递增、字段复制、重置项正确。
	box := d.bb.lastCreated
	if box == nil {
		t.Fatal("no box created")
	}
	if box.Name != "星愿卡包 第 4 期" {
		t.Errorf("name = %q, want 期数递增 %q", box.Name, "星愿卡包 第 4 期")
	}
	if box.Cover != "cover.png" || box.Price != 59.9 || box.Description != "老批次描述" {
		t.Errorf("copied fields = %s/%v/%s", box.Cover, box.Price, box.Description)
	}
	if box.SupplierID != 7 || box.Status != mall.StatusActive {
		t.Errorf("supplier/status = %d/%s, want 7/active", box.SupplierID, box.Status)
	}
	if box.OnSale {
		t.Errorf("on_sale = true, want false (新批次默认下架)")
	}
	if box.Lifecycle != mall.LifecycleSelling {
		t.Errorf("lifecycle = %s, want selling", box.Lifecycle)
	}
	if box.IsFeatured {
		t.Errorf("is_featured = true, want false (admin 领域不复制)")
	}

	// 新卡池：结构复制、weight 保留、stock 全部置 0。
	items := d.cp.lastItems
	if d.cp.createItemsCalls != 1 || len(items) != 2 {
		t.Fatalf("items = %d calls / %d rows, want 1 call / 2 rows", d.cp.createItemsCalls, len(items))
	}
	for i, it := range items {
		if it.Stock != 0 {
			t.Errorf("items[%d].stock = %d, want 0 (必须重填)", i, it.Stock)
		}
		if it.PoolID != 100 { // fake CreatePool 首池 ID = 1*100
			t.Errorf("items[%d].poolID = %d, want 100 (回填新池)", i, it.PoolID)
		}
	}
	if items[0].CardID != 11 || items[0].Weight != 100 || items[1].CardID != 12 || items[1].Weight != 9900 {
		t.Errorf("card/weight structure not preserved: %+v", items)
	}
}

// TestProductDuplicate_NoPeriodSuffix 源名称无期数 → 追加「第 2 期」。
func TestProductDuplicate_NoPeriodSuffix(t *testing.T) {
	d := withSourceBox(defaultProductDeps())
	d.bb.getByIDFunc = func(c *gin.Context, id int64) (*mall.MallBlindBox, error) {
		box := ownedSourceBox()
		box.Name = "星愿卡包"
		return box, nil
	}

	if _, err := newProductSvcFromDeps(d).Duplicate(newProductCtx(), 7, 9); err != nil {
		t.Fatalf("Duplicate: %v", err)
	}
	if got := d.bb.lastCreated.Name; got != "星愿卡包 第 2 期" {
		t.Errorf("name = %q, want %q", got, "星愿卡包 第 2 期")
	}
}

// TestProductDuplicate_Forbidden 复制他人商品 → 403。
func TestProductDuplicate_Forbidden(t *testing.T) {
	d := withSourceBox(defaultProductDeps())
	svc := newProductSvcFromDeps(d)

	if _, err := svc.Duplicate(newProductCtx(), 8, 9); !errors.Is(err, ErrBlindBoxForbidden) {
		t.Errorf("Duplicate(other's box) = %v, want ErrBlindBoxForbidden", err)
	}
	if d.bb.createCalls != 0 {
		t.Errorf("createCalls = %d, want 0 (ownership check must short-circuit)", d.bb.createCalls)
	}
}

// TestProductDuplicate_SourceNotFound 源商品不存在 → 404。
func TestProductDuplicate_SourceNotFound(t *testing.T) {
	d := defaultProductDeps() // GetByID → gorm.ErrRecordNotFound
	if _, err := newProductSvcFromDeps(d).Duplicate(newProductCtx(), 7, 9); !errors.Is(err, ErrBlindBoxNotFound) {
		t.Errorf("Duplicate(missing source) = %v, want ErrBlindBoxNotFound", err)
	}
}

// TestProductDuplicate_EmptyPool 源卡池无 items → ErrEmptyPool。
func TestProductDuplicate_EmptyPool(t *testing.T) {
	d := withSourceBox(defaultProductDeps())
	d.cp.listItemsByPoolIDFunc = func(c *gin.Context, poolID int64) ([]mall.MallCardPoolItem, error) {
		return nil, nil // 空 items
	}
	if _, err := newProductSvcFromDeps(d).Duplicate(newProductCtx(), 7, 9); !errors.Is(err, ErrEmptyPool) {
		t.Errorf("Duplicate(empty pool) = %v, want ErrEmptyPool", err)
	}
}

// TestProductDuplicate_TxRollback 复制中途失败 → 整体回滚（返回错误）。
func TestProductDuplicate_TxRollback(t *testing.T) {
	d := withSourceBox(defaultProductDeps())
	d.cp.createItemsFunc = func(c *gin.Context, items []mall.MallCardPoolItem) error {
		return errors.New("simulated create items failure")
	}
	if _, err := newProductSvcFromDeps(d).Duplicate(newProductCtx(), 7, 9); err == nil {
		t.Errorf("Duplicate with repo failure = nil, want err (tx roll back)")
	}
}

// TestNextBatchName 期数递增规则表驱动。
func TestNextBatchName(t *testing.T) {
	cases := []struct{ in, want string }{
		{"星愿卡包 第 3 期", "星愿卡包 第 4 期"},
		{"星愿卡包", "星愿卡包 第 2 期"},
		{"第 9 期", "第 10 期"},
		{"星愿 第12期 特别版", "星愿 第13期 特别版"},
		{"  前后空格  ", "前后空格 第 2 期"},
	}
	for _, c := range cases {
		if got := nextBatchName(c.in); got != c.want {
			t.Errorf("nextBatchName(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
