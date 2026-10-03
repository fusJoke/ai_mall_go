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
	listBySupplierFunc func(c *gin.Context, sid int64, opts repository.ListOptions) ([]mall.MallBlindBox, int64, error)
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
func (m *productBlindBoxRepo) ListBySupplier(c *gin.Context, sid int64, opts repository.ListOptions) ([]mall.MallBlindBox, int64, error) {
	if m.listBySupplierFunc != nil {
		return m.listBySupplierFunc(c, sid, opts)
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
	return nil, nil
}
func (m *productBlindBoxRepo) ToggleStatus(c *gin.Context, id int64, s mall.MallBlindBoxStatus) error {
	return nil
}
func (m *productBlindBoxRepo) ToggleFeatured(c *gin.Context, id int64, f bool) error { return nil }
func (m *productBlindBoxRepo) Delete(c *gin.Context, id int64) error                 { return nil }

var _ mallRepo.BlindBoxRepository = (*productBlindBoxRepo)(nil)

type productCardPoolRepo struct {
	createPoolFunc  func(f *m_unused_anchor) error // 占位避免编译器报 unused-arg
	createItemsFunc func(c *gin.Context, items []mall.MallCardPoolItem) error
	getPoolFunc     func(c *gin.Context, bbID int64) (*mall.MallCardPool, error)

	createPoolCalls, createItemsCalls int
	lastPoolID                        int64
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
func (m *productCardPoolRepo) GetPoolByBlindBoxID(c *gin.Context, bbID int64) (*mall.MallCardPool, error) {
	if m.getPoolFunc != nil {
		return m.getPoolFunc(c, bbID)
	}
	return nil, gorm.ErrRecordNotFound
}

// 其他方法 no-op。
func (m *productCardPoolRepo) ListItemsByPoolID(c *gin.Context, poolID int64) ([]mall.MallCardPoolItem, error) {
	return nil, nil
}
func (m *productCardPoolRepo) ListItemsForUpdateByPoolID(c *gin.Context, poolID int64) ([]mall.MallCardPoolItem, error) {
	return nil, nil
}
func (m *productCardPoolRepo) ListItemsByPoolIDs(c *gin.Context, poolIDs []int64) ([]mall.MallCardPoolItem, error) {
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

var _ cache.Cache = (*mockCache)(nil)

// =============================================================================
// 默认依赖 + 工厂
// =============================================================================

type productDeps struct {
	bb    *productBlindBoxRepo
	cp    *productCardPoolRepo
	cache *mockCache
	txr   *mockTxRunner
}

func defaultProductDeps() productDeps {
	return productDeps{
		bb:    &productBlindBoxRepo{},
		cp:    &productCardPoolRepo{},
		cache: &mockCache{},
		txr:   &mockTxRunner{},
	}
}

func newProductSvcFromDeps(d productDeps) ProductService {
	return NewProductService(ProductServiceDeps{
		BlindBoxRepo: d.bb,
		CardPoolRepo: d.cp,
		Cache:        d.cache,
		TxRunner:     d.txr,
	})
}

// =============================================================================
// List
// =============================================================================

func TestProductList_InvalidSupplierID(t *testing.T) {
	d := defaultProductDeps()
	svc := newProductSvcFromDeps(d)
	items, total, err := svc.List(newProductCtx(), 0, repository.ListOptions{Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(items) != 0 || total != 0 {
		t.Errorf("got (%d, %d), want (0, 0)", len(items), total)
	}
}

func TestProductList_DelegatesToRepo(t *testing.T) {
	d := defaultProductDeps()
	d.bb.listBySupplierFunc = func(c *gin.Context, sid int64, opts repository.ListOptions) ([]mall.MallBlindBox, int64, error) {
		if sid != 7 {
			t.Errorf("supplierID = %d, want 7", sid)
		}
		return []mall.MallBlindBox{
			{ID: 1, SupplierID: 7, Name: "Box A"},
			{ID: 2, SupplierID: 7, Name: "Box B"},
		}, 2, nil
	}
	svc := newProductSvcFromDeps(d)
	items, total, err := svc.List(newProductCtx(), 7, repository.ListOptions{Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if total != 2 || len(items) != 2 {
		t.Errorf("got (%d, %d), want (2, 2)", total, len(items))
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
		return &mall.MallBlindBox{ID: id, SupplierID: 7}, nil
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
