package user

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"ai-go-mall/internal/infra/cache"
	"ai-go-mall/internal/infra/cache/hotspot"
	"ai-go-mall/internal/model/mall"
	"ai-go-mall/internal/repository"
	mallRepo "ai-go-mall/internal/repository/mall"
	supplierRepo "ai-go-mall/internal/repository/supplier"
)

// withinJitter 断言 ttl 落在 base ±10% 区间内（D22：cache stampede 防护测试用）。
//
// 复用 cache.JitterTTL 的常量口径（jitterFraction = 0.10）—— 测试改此文件后
// 仍能跟上 JitterTTL 行为变化（不会因常量改动而漂移）。
func withinJitter(ttl, base time.Duration) bool {
	if base <= 0 {
		return ttl == base
	}
	min := time.Duration(float64(base) * (1 - 0.10))
	max := time.Duration(float64(base) * (1 + 0.10))
	return ttl >= min && ttl <= max
}

// =============================================================================
// mocks
// =============================================================================

// mockBlindBoxRepo 实现 mallRepo.BlindBoxRepository。
//
// 业务路径只用到 GetByID / ListFeaturedOnSale / ListByIDs；其余 CRUD 方法 no-op。
type mockBlindBoxRepo struct {
	getByIDFunc                       func(c *gin.Context, id int64) (*mall.MallBlindBox, error)
	listFeaturedOnSaleFunc            func(c *gin.Context, opts repository.ListOptions) ([]mall.MallBlindBox, int64, error)
	listActiveOnSaleBySupplierIDsFunc func(c *gin.Context, ids []int64, opts repository.ListOptions) ([]mall.MallBlindBox, int64, error)
	listByIDsFunc                     func(c *gin.Context, ids []int64) ([]mall.MallBlindBox, error)
	listBySupplierFunc                func(c *gin.Context, supplierID int64, opts repository.ListOptions) ([]mall.MallBlindBox, int64, error)
	toggleStatusFunc                  func(c *gin.Context, id int64, status mall.MallBlindBoxStatus) error
	toggleOnSaleFunc                  func(c *gin.Context, id int64, onSale bool) error
	toggleFeaturedFunc                func(c *gin.Context, id int64, featured bool) error

	getByIDCalls   int
	listByIDsCalls int
	lastListByIDs  []int64

	createCalls          int
	listCalls            int
	getByIDCallsByMethod int
	updateCalls          int
	deleteCalls          int
}

func (m *mockBlindBoxRepo) GetByID(c *gin.Context, id int64) (*mall.MallBlindBox, error) {
	m.getByIDCalls++
	if m.getByIDFunc != nil {
		return m.getByIDFunc(c, id)
	}
	return nil, gorm.ErrRecordNotFound
}
func (m *mockBlindBoxRepo) ListFeaturedOnSale(c *gin.Context, opts repository.ListOptions) ([]mall.MallBlindBox, int64, error) {
	if m.listFeaturedOnSaleFunc != nil {
		return m.listFeaturedOnSaleFunc(c, opts)
	}
	return nil, 0, nil
}
func (m *mockBlindBoxRepo) ListActiveOnSaleBySupplierIDs(c *gin.Context, ids []int64, opts repository.ListOptions) ([]mall.MallBlindBox, int64, error) {
	if m.listActiveOnSaleBySupplierIDsFunc != nil {
		return m.listActiveOnSaleBySupplierIDsFunc(c, ids, opts)
	}
	return nil, 0, nil
}
func (m *mockBlindBoxRepo) ListByIDs(c *gin.Context, ids []int64) ([]mall.MallBlindBox, error) {
	m.listByIDsCalls++
	m.lastListByIDs = ids
	if m.listByIDsFunc != nil {
		return m.listByIDsFunc(c, ids)
	}
	return nil, nil
}
func (m *mockBlindBoxRepo) ListBySupplier(c *gin.Context, supplierID int64, opts repository.ListOptions) ([]mall.MallBlindBox, int64, error) {
	if m.listBySupplierFunc != nil {
		return m.listBySupplierFunc(c, supplierID, opts)
	}
	return nil, 0, nil
}
func (m *mockBlindBoxRepo) ToggleStatus(c *gin.Context, id int64, status mall.MallBlindBoxStatus) error {
	if m.toggleStatusFunc != nil {
		return m.toggleStatusFunc(c, id, status)
	}
	return nil
}
func (m *mockBlindBoxRepo) ToggleOnSale(c *gin.Context, id int64, onSale bool) error {
	if m.toggleOnSaleFunc != nil {
		return m.toggleOnSaleFunc(c, id, onSale)
	}
	return nil
}
func (m *mockBlindBoxRepo) ToggleFeatured(c *gin.Context, id int64, featured bool) error {
	if m.toggleFeaturedFunc != nil {
		return m.toggleFeaturedFunc(c, id, featured)
	}
	return nil
}

// CRUD no-op 占位。
func (m *mockBlindBoxRepo) Create(c *gin.Context, entity *mall.MallBlindBox) error { return nil }
func (m *mockBlindBoxRepo) List(c *gin.Context, opts repository.ListOptions) ([]mall.MallBlindBox, int64, error) {
	return nil, 0, nil
}
func (m *mockBlindBoxRepo) Update(c *gin.Context, entity *mall.MallBlindBox) error { return nil }
func (m *mockBlindBoxRepo) Delete(c *gin.Context, id int64) error                  { return nil }

var _ mallRepo.BlindBoxRepository = (*mockBlindBoxRepo)(nil)

// mockCardPoolRepo 实现 mallRepo.CardPoolRepository。
type mockCardPoolRepo struct {
	getPoolByBlindBoxIDFunc func(c *gin.Context, blindBoxID int64) (*mall.MallCardPool, error)
	listItemsByPoolIDFunc   func(c *gin.Context, poolID int64) ([]mall.MallCardPoolItem, error)
	listItemsForUpdateFunc  func(c *gin.Context, poolID int64) ([]mall.MallCardPoolItem, error)
	getPoolCalls            int
	listItemsCalls          int
	lastListItemsPoolID     int64
}

func (m *mockCardPoolRepo) GetPoolByBlindBoxID(c *gin.Context, blindBoxID int64) (*mall.MallCardPool, error) {
	m.getPoolCalls++
	if m.getPoolByBlindBoxIDFunc != nil {
		return m.getPoolByBlindBoxIDFunc(c, blindBoxID)
	}
	return nil, gorm.ErrRecordNotFound
}
func (m *mockCardPoolRepo) ListItemsByPoolID(c *gin.Context, poolID int64) ([]mall.MallCardPoolItem, error) {
	m.listItemsCalls++
	m.lastListItemsPoolID = poolID
	if m.listItemsByPoolIDFunc != nil {
		return m.listItemsByPoolIDFunc(c, poolID)
	}
	return nil, nil
}
func (m *mockCardPoolRepo) ListItemsForUpdateByPoolID(c *gin.Context, poolID int64) ([]mall.MallCardPoolItem, error) {
	if m.listItemsForUpdateFunc != nil {
		return m.listItemsForUpdateFunc(c, poolID)
	}
	// 默认 fallback：与 production SQL `WHERE pool_id = ? AND stock > 0` 行为一致。
	all, err := m.ListItemsByPoolID(c, poolID)
	if err != nil {
		return nil, err
	}
	filtered := make([]mall.MallCardPoolItem, 0, len(all))
	for _, it := range all {
		if it.Stock > 0 {
			filtered = append(filtered, it)
		}
	}
	return filtered, nil
}
func (m *mockCardPoolRepo) ListItemsByPoolIDs(c *gin.Context, poolIDs []int64) ([]mall.MallCardPoolItem, error) {
	return nil, nil
}
func (m *mockCardPoolRepo) CreatePool(c *gin.Context, pool *mall.MallCardPool) error     { return nil }
func (m *mockCardPoolRepo) CreateItem(c *gin.Context, item *mall.MallCardPoolItem) error { return nil }
func (m *mockCardPoolRepo) CreateItems(c *gin.Context, items []mall.MallCardPoolItem) error {
	return nil
}
func (m *mockCardPoolRepo) DecrementStock(c *gin.Context, itemID int64) error            { return nil }
func (m *mockCardPoolRepo) IncrementStock(c *gin.Context, itemID int64, delta int) error { return nil }
func (m *mockCardPoolRepo) SumWeightByPoolID(c *gin.Context, poolID int64) (int64, error) {
	return 0, nil
}
func (m *mockCardPoolRepo) SubtotalStockByPoolID(c *gin.Context, poolID int64) (int64, error) {
	return 0, nil
}

var _ mallRepo.CardPoolRepository = (*mockCardPoolRepo)(nil)

// mockPromotionRepo 实现 mallRepo.PromotionRepository。
type mockPromotionRepo struct {
	findActiveFunc  func(c *gin.Context, blindBoxID int64, now time.Time) (*mall.MallPromotion, error)
	findActiveCalls int
}

func (m *mockPromotionRepo) FindActiveByBlindBoxID(c *gin.Context, blindBoxID int64, now time.Time) (*mall.MallPromotion, error) {
	m.findActiveCalls++
	if m.findActiveFunc != nil {
		return m.findActiveFunc(c, blindBoxID, now)
	}
	return nil, nil
}
func (m *mockPromotionRepo) GetByID(c *gin.Context, id int64) (*mall.MallPromotion, error) {
	return nil, gorm.ErrRecordNotFound
}
func (m *mockPromotionRepo) ListBySupplier(c *gin.Context, supplierID int64, opts repository.ListOptions) ([]mall.MallPromotion, int64, error) {
	return nil, 0, nil
}
func (m *mockPromotionRepo) ListTimeOverlap(c *gin.Context, blindBoxID int64, startAt, endAt time.Time) ([]mall.MallPromotion, error) {
	return nil, nil
}
func (m *mockPromotionRepo) ToggleStatus(c *gin.Context, id int64, status mall.MallBlindBoxStatus) error {
	return nil
}
func (m *mockPromotionRepo) Create(c *gin.Context, entity *mall.MallPromotion) error { return nil }
func (m *mockPromotionRepo) List(c *gin.Context, opts repository.ListOptions) ([]mall.MallPromotion, int64, error) {
	return nil, 0, nil
}
func (m *mockPromotionRepo) Update(c *gin.Context, entity *mall.MallPromotion) error { return nil }
func (m *mockPromotionRepo) Delete(c *gin.Context, id int64) error                   { return nil }

var _ mallRepo.PromotionRepository = (*mockPromotionRepo)(nil)

// mockSupplierRepo 实现 supplierRepo.SupplierRepository。
type mockSupplierRepo struct {
	listByIDsFunc func(c *gin.Context, ids []int64) ([]mall.MallSupplier, error)
	getByIDFunc   func(c *gin.Context, id int64) (*mall.MallSupplier, error)

	listByIDsCalls int
	lastListByIDs  []int64
	getByIDCalls   int
	lastGetByID    int64
}

func (m *mockSupplierRepo) ListByIDs(c *gin.Context, ids []int64) ([]mall.MallSupplier, error) {
	m.listByIDsCalls++
	m.lastListByIDs = ids
	if m.listByIDsFunc != nil {
		return m.listByIDsFunc(c, ids)
	}
	return nil, nil
}
func (m *mockSupplierRepo) GetByID(c *gin.Context, id int64) (*mall.MallSupplier, error) {
	m.getByIDCalls++
	m.lastGetByID = id
	if m.getByIDFunc != nil {
		return m.getByIDFunc(c, id)
	}
	return nil, gorm.ErrRecordNotFound
}
func (m *mockSupplierRepo) ListWithFilter(c *gin.Context, opts supplierRepo.SupplierListOptions) ([]mall.MallSupplier, int64, error) {
	return nil, 0, nil
}
func (m *mockSupplierRepo) ToggleStatus(c *gin.Context, id int64, status mall.MallBlindBoxStatus) error {
	return nil
}
func (m *mockSupplierRepo) ToggleFeatured(c *gin.Context, id int64, featured bool) error {
	return nil
}
func (m *mockSupplierRepo) UpdateBalanceAndSales(c *gin.Context, id int64, amount float64) error {
	return nil
}
func (m *mockSupplierRepo) Create(c *gin.Context, entity *mall.MallSupplier) error { return nil }
func (m *mockSupplierRepo) List(c *gin.Context, opts repository.ListOptions) ([]mall.MallSupplier, int64, error) {
	return nil, 0, nil
}
func (m *mockSupplierRepo) Update(c *gin.Context, entity *mall.MallSupplier) error { return nil }
func (m *mockSupplierRepo) Delete(c *gin.Context, id int64) error                  { return nil }

var _ supplierRepo.SupplierRepository = (*mockSupplierRepo)(nil)

// mockCache 实现 cache.Cache（map-backed）。
type mockCache struct {
	data     map[string]string
	getErr   error
	setErr   error
	delErr   error
	setNXErr error

	getCalls   int
	setCalls   int
	delCalls   int
	setNXCalls int

	lastGetKey string
	lastSetKey string
	lastSetVal string
	lastSetTTL time.Duration
	lastDelKey string
}

func newMockCache() *mockCache {
	return &mockCache{data: make(map[string]string)}
}

func (m *mockCache) Get(ctx context.Context, key string) (string, bool, error) {
	m.getCalls++
	m.lastGetKey = key
	if m.getErr != nil {
		return "", false, m.getErr
	}
	v, ok := m.data[key]
	if !ok {
		return "", false, cache.ErrCacheMiss
	}
	if cache.IsNotFoundValue(v) {
		return "", false, nil
	}
	return v, true, nil
}

func (m *mockCache) Set(ctx context.Context, key, value string, ttl time.Duration) error {
	m.setCalls++
	m.lastSetKey = key
	m.lastSetVal = value
	m.lastSetTTL = ttl
	if m.setErr != nil {
		return m.setErr
	}
	m.data[key] = value
	return nil
}

func (m *mockCache) Del(ctx context.Context, key string) error {
	m.delCalls++
	m.lastDelKey = key
	if m.delErr != nil {
		return m.delErr
	}
	delete(m.data, key)
	return nil
}

func (m *mockCache) SetNX(ctx context.Context, key, value string, ttl time.Duration) (bool, error) {
	m.setNXCalls++
	if m.setNXErr != nil {
		return false, m.setNXErr
	}
	if _, ok := m.data[key]; ok {
		return false, nil
	}
	m.data[key] = value
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
// helpers
// =============================================================================

// newSvcCtx 构造一个有 Request 的 *gin.Context，供 service 用 c.Request.Context()。
func newSvcCtx() *gin.Context {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", "/api/v1/blindboxes/1", nil)
	return c
}

// newSvc 把全部 mock 拼成 BlindBoxService。
func newSvc(bb *mockBlindBoxRepo, cp *mockCardPoolRepo, p *mockPromotionRepo, s *mockSupplierRepo, ca cache.Cache) BlindBoxService {
	return NewBlindBoxService(BlindBoxServiceDeps{
		BlindBoxRepo:  bb,
		CardPoolRepo:  cp,
		PromotionRepo: p,
		SupplierRepo:  s,
		Cache:         ca,
	})
}

// =============================================================================
// List tests
// =============================================================================

func TestBlindBoxList_FiltersOutDisabledSupplier(t *testing.T) {
	// 1 个盲盒属于 supplier=1(active)，1 个属于 supplier=2(disabled)。
	// ListFeaturedOnSale 返回 2 条；ListByIDs 返回 1 条 disabled。
	// 最终 List 应只返回 supplier=1 的那一条。
	bbRepo := &mockBlindBoxRepo{
		listFeaturedOnSaleFunc: func(c *gin.Context, opts repository.ListOptions) ([]mall.MallBlindBox, int64, error) {
			return []mall.MallBlindBox{
				{ID: 10, SupplierID: 1, Name: "Active Box", Status: mall.StatusActive, OnSale: true, IsFeatured: true, Price: 100},
				{ID: 20, SupplierID: 2, Name: "Disabled Supplier Box", Status: mall.StatusActive, OnSale: true, IsFeatured: true, Price: 200},
			}, 2, nil
		},
	}
	supRepo := &mockSupplierRepo{
		listByIDsFunc: func(c *gin.Context, ids []int64) ([]mall.MallSupplier, error) {
			if len(ids) != 2 {
				t.Errorf("supplier ListByIDs called with %d ids, want 2", len(ids))
			}
			return []mall.MallSupplier{
				{ID: 1, Name: "Active", Status: mall.StatusActive},
				{ID: 2, Name: "Disabled", Status: mall.StatusDisabled},
			}, nil
		},
	}

	svc := newSvc(bbRepo, &mockCardPoolRepo{}, &mockPromotionRepo{}, supRepo, nil)
	items, total, err := svc.List(newSvcCtx(), repository.ListOptions{Page: 1, PageSize: 20})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if total != 2 {
		t.Errorf("total = %d, want 2 (page-level)", total)
	}
	if len(items) != 1 {
		t.Fatalf("items = %d, want 1 (disabled supplier filtered out)", len(items))
	}
	if items[0].SupplierID != 1 {
		t.Errorf("items[0].SupplierID = %d, want 1", items[0].SupplierID)
	}
}

func TestBlindBoxList_EmptyPage(t *testing.T) {
	bbRepo := &mockBlindBoxRepo{
		listFeaturedOnSaleFunc: func(c *gin.Context, opts repository.ListOptions) ([]mall.MallBlindBox, int64, error) {
			return []mall.MallBlindBox{}, 0, nil
		},
	}
	svc := newSvc(bbRepo, &mockCardPoolRepo{}, &mockPromotionRepo{}, &mockSupplierRepo{}, nil)
	items, total, err := svc.List(newSvcCtx(), repository.ListOptions{Page: 1, PageSize: 20})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(items) != 0 || total != 0 {
		t.Errorf("got items=%d total=%d, want 0/0", len(items), total)
	}
	if bbRepo.listByIDsCalls != 0 {
		t.Errorf("supplier ListByIDs called %d times on empty page, want 0", bbRepo.listByIDsCalls)
	}
}

func TestBlindBoxList_AllActive(t *testing.T) {
	// 所有供应商 active → 不应过滤掉任何条目。
	bbRepo := &mockBlindBoxRepo{
		listFeaturedOnSaleFunc: func(c *gin.Context, opts repository.ListOptions) ([]mall.MallBlindBox, int64, error) {
			return []mall.MallBlindBox{
				{ID: 10, SupplierID: 1, Status: mall.StatusActive, OnSale: true, IsFeatured: true, Price: 100},
				{ID: 20, SupplierID: 2, Status: mall.StatusActive, OnSale: true, IsFeatured: true, Price: 200},
			}, 2, nil
		},
	}
	supRepo := &mockSupplierRepo{
		listByIDsFunc: func(c *gin.Context, ids []int64) ([]mall.MallSupplier, error) {
			return []mall.MallSupplier{
				{ID: 1, Status: mall.StatusActive},
				{ID: 2, Status: mall.StatusActive},
			}, nil
		},
	}
	svc := newSvc(bbRepo, &mockCardPoolRepo{}, &mockPromotionRepo{}, supRepo, nil)
	items, _, err := svc.List(newSvcCtx(), repository.ListOptions{Page: 1, PageSize: 20})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(items) != 2 {
		t.Errorf("items = %d, want 2", len(items))
	}
}

func TestBlindBoxList_RepoError(t *testing.T) {
	bbRepo := &mockBlindBoxRepo{
		listFeaturedOnSaleFunc: func(c *gin.Context, opts repository.ListOptions) ([]mall.MallBlindBox, int64, error) {
			return nil, 0, errors.New("db down")
		},
	}
	svc := newSvc(bbRepo, &mockCardPoolRepo{}, &mockPromotionRepo{}, &mockSupplierRepo{}, nil)
	_, _, err := svc.List(newSvcCtx(), repository.ListOptions{Page: 1, PageSize: 20})
	if err == nil {
		t.Errorf("List err = nil, want error")
	}
}

// =============================================================================
// Detail tests
// =============================================================================

func TestBlindBoxDetail_NotFound(t *testing.T) {
	bbRepo := &mockBlindBoxRepo{
		getByIDFunc: func(c *gin.Context, id int64) (*mall.MallBlindBox, error) {
			return nil, gorm.ErrRecordNotFound
		},
	}
	svc := newSvc(bbRepo, &mockCardPoolRepo{}, &mockPromotionRepo{}, &mockSupplierRepo{}, nil)
	_, err := svc.Detail(newSvcCtx(), 99)
	if !errors.Is(err, ErrBlindBoxNotFound) {
		t.Errorf("Detail err = %v, want ErrBlindBoxNotFound", err)
	}
}

func TestBlindBoxDetail_NotForSale(t *testing.T) {
	bbRepo := &mockBlindBoxRepo{
		getByIDFunc: func(c *gin.Context, id int64) (*mall.MallBlindBox, error) {
			return &mall.MallBlindBox{
				ID:     id,
				Status: mall.StatusActive,
				OnSale: false, // 供应商下架
				Price:  100,
			}, nil
		},
	}
	svc := newSvc(bbRepo, &mockCardPoolRepo{}, &mockPromotionRepo{}, &mockSupplierRepo{}, nil)
	_, err := svc.Detail(newSvcCtx(), 1)
	if !errors.Is(err, ErrBlindBoxNotFound) {
		t.Errorf("Detail (OnSale=false) err = %v, want ErrBlindBoxNotFound", err)
	}
}

func TestBlindBoxDetail_DisabledStatus(t *testing.T) {
	bbRepo := &mockBlindBoxRepo{
		getByIDFunc: func(c *gin.Context, id int64) (*mall.MallBlindBox, error) {
			return &mall.MallBlindBox{
				ID:     id,
				Status: mall.StatusDisabled, // admin 强制下架
				OnSale: true,
			}, nil
		},
	}
	svc := newSvc(bbRepo, &mockCardPoolRepo{}, &mockPromotionRepo{}, &mockSupplierRepo{}, nil)
	_, err := svc.Detail(newSvcCtx(), 1)
	if !errors.Is(err, ErrBlindBoxNotFound) {
		t.Errorf("Detail (status=disabled) err = %v, want ErrBlindBoxNotFound", err)
	}
}

func TestBlindBoxDetail_SupplierDisabled(t *testing.T) {
	bbRepo := &mockBlindBoxRepo{
		getByIDFunc: func(c *gin.Context, id int64) (*mall.MallBlindBox, error) {
			return &mall.MallBlindBox{
				ID:         id,
				Status:     mall.StatusActive,
				OnSale:     true,
				SupplierID: 7,
			}, nil
		},
	}
	supRepo := &mockSupplierRepo{
		getByIDFunc: func(c *gin.Context, id int64) (*mall.MallSupplier, error) {
			return &mall.MallSupplier{ID: id, Status: mall.StatusDisabled}, nil
		},
	}
	svc := newSvc(bbRepo, &mockCardPoolRepo{}, &mockPromotionRepo{}, supRepo, nil)
	_, err := svc.Detail(newSvcCtx(), 1)
	if !errors.Is(err, ErrBlindBoxNotFound) {
		t.Errorf("Detail (supplier disabled) err = %v, want ErrBlindBoxNotFound", err)
	}
}

func TestBlindBoxDetail_Success_NoPromo(t *testing.T) {
	bbRepo := &mockBlindBoxRepo{
		getByIDFunc: func(c *gin.Context, id int64) (*mall.MallBlindBox, error) {
			return &mall.MallBlindBox{
				ID: 1, Name: "Box A", SupplierID: 7,
				Status: mall.StatusActive, OnSale: true,
				IsFeatured: true, Price: 100,
			}, nil
		},
	}
	supRepo := &mockSupplierRepo{
		getByIDFunc: func(c *gin.Context, id int64) (*mall.MallSupplier, error) {
			return &mall.MallSupplier{ID: 7, Name: "Supplier7", Status: mall.StatusActive}, nil
		},
	}
	cpRepo := &mockCardPoolRepo{
		getPoolByBlindBoxIDFunc: func(c *gin.Context, blindBoxID int64) (*mall.MallCardPool, error) {
			return &mall.MallCardPool{ID: 100, BlindBoxID: blindBoxID}, nil
		},
		listItemsByPoolIDFunc: func(c *gin.Context, poolID int64) ([]mall.MallCardPoolItem, error) {
			return []mall.MallCardPoolItem{
				{ID: 1, PoolID: poolID, CardID: 11, Rarity: mall.RaritySSR, Weight: 100, Stock: 5},
				{ID: 2, PoolID: poolID, CardID: 12, Rarity: mall.RarityN, Weight: 9900, Stock: 95},
			}, nil
		},
	}
	promoRepo := &mockPromotionRepo{
		findActiveFunc: func(c *gin.Context, blindBoxID int64, now time.Time) (*mall.MallPromotion, error) {
			return nil, nil // 无活动
		},
	}
	ca := newMockCache()

	svc := newSvc(bbRepo, cpRepo, promoRepo, supRepo, ca)
	detail, err := svc.Detail(newSvcCtx(), 1)
	if err != nil {
		t.Fatalf("Detail: %v", err)
	}
	if detail == nil {
		t.Fatalf("Detail returned nil")
	}
	if detail.BlindBox.ID != 1 {
		t.Errorf("detail.BlindBox.ID = %d, want 1", detail.BlindBox.ID)
	}
	if detail.Supplier.ID != 7 {
		t.Errorf("detail.Supplier.ID = %d, want 7", detail.Supplier.ID)
	}
	if detail.Pool.ID != 100 {
		t.Errorf("detail.Pool.ID = %d, want 100", detail.Pool.ID)
	}
	if len(detail.Items) != 2 {
		t.Errorf("detail.Items len = %d, want 2", len(detail.Items))
	}
	if detail.PoolTotalStock != 100 {
		t.Errorf("detail.PoolTotalStock = %d, want 100", detail.PoolTotalStock)
	}
	if detail.ActivePromotion != nil {
		t.Errorf("detail.ActivePromotion = %+v, want nil", detail.ActivePromotion)
	}
	if detail.EffectivePrice != 100 {
		t.Errorf("detail.EffectivePrice = %v, want 100 (no promo)", detail.EffectivePrice)
	}
	// 缓存写回：1 次 Set
	if ca.setCalls != 1 {
		t.Errorf("cache.Set called %d times, want 1", ca.setCalls)
	}
	// D22：业务侧 TTL 经 JitterTTL ±10%，断言允许落在区间内而非精确值。
	if !withinJitter(ca.lastSetTTL, defaultCacheTTL) {
		t.Errorf("cache.Set TTL = %v, want in jitter range of %v", ca.lastSetTTL, defaultCacheTTL)
	}
}

func TestBlindBoxDetail_Success_WithPromo(t *testing.T) {
	bbRepo := &mockBlindBoxRepo{
		getByIDFunc: func(c *gin.Context, id int64) (*mall.MallBlindBox, error) {
			return &mall.MallBlindBox{
				ID: 1, Name: "Box A", SupplierID: 7,
				Status: mall.StatusActive, OnSale: true,
				IsFeatured: true, Price: 100,
			}, nil
		},
	}
	supRepo := &mockSupplierRepo{
		getByIDFunc: func(c *gin.Context, id int64) (*mall.MallSupplier, error) {
			return &mall.MallSupplier{ID: 7, Status: mall.StatusActive}, nil
		},
	}
	cpRepo := &mockCardPoolRepo{
		getPoolByBlindBoxIDFunc: func(c *gin.Context, blindBoxID int64) (*mall.MallCardPool, error) {
			return &mall.MallCardPool{ID: 100, BlindBoxID: blindBoxID}, nil
		},
		listItemsByPoolIDFunc: func(c *gin.Context, poolID int64) ([]mall.MallCardPoolItem, error) {
			return []mall.MallCardPoolItem{{ID: 1, PoolID: poolID, Stock: 1}}, nil
		},
	}
	now := time.Now()
	promoRepo := &mockPromotionRepo{
		findActiveFunc: func(c *gin.Context, blindBoxID int64, n time.Time) (*mall.MallPromotion, error) {
			return &mall.MallPromotion{
				ID: 99, BlindBoxID: blindBoxID,
				OriginalPrice: 100, PromoPrice: 80,
				StartAt: now.Add(-time.Hour), EndAt: now.Add(time.Hour),
				Status: mall.StatusActive,
			}, nil
		},
	}

	svc := newSvc(bbRepo, cpRepo, promoRepo, supRepo, newMockCache())
	detail, err := svc.Detail(newSvcCtx(), 1)
	if err != nil {
		t.Fatalf("Detail: %v", err)
	}
	if detail.ActivePromotion == nil || detail.ActivePromotion.ID != 99 {
		t.Errorf("detail.ActivePromotion = %+v, want ID=99", detail.ActivePromotion)
	}
	if detail.EffectivePrice != 80 {
		t.Errorf("detail.EffectivePrice = %v, want 80 (promo)", detail.EffectivePrice)
	}
}

func TestBlindBoxDetail_CacheHit(t *testing.T) {
	// 把预先构造好的 JSON 放进 cache，Detail 应直接返回缓存版本，不再调盲盒 repo。
	cached := BlindBoxDetail{
		BlindBox:       mall.MallBlindBox{ID: 1, Name: "FromCache", SupplierID: 7, Status: mall.StatusActive, OnSale: true, Price: 42},
		Supplier:       mall.MallSupplier{ID: 7, Name: "Cached Supplier"},
		Pool:           mall.MallCardPool{ID: 100, BlindBoxID: 1},
		Items:          []mall.MallCardPoolItem{{ID: 1, PoolID: 100, Stock: 7}},
		PoolTotalStock: 7,
		EffectivePrice: 42,
		CachedAt:       time.Now().Add(-time.Minute),
	}
	payload, _ := json.Marshal(cached)
	ca := newMockCache()
	ca.data[cacheKey(1)] = string(payload)

	// 这些 mock 即使被调也会因签名错误而不通过 —— Detail 走缓存路径不应触发它们。
	bbRepo := &mockBlindBoxRepo{
		getByIDFunc: func(c *gin.Context, id int64) (*mall.MallBlindBox, error) {
			t.Errorf("GetByID called on cache hit, should be skipped")
			return nil, nil
		},
	}
	svc := newSvc(bbRepo, &mockCardPoolRepo{}, &mockPromotionRepo{}, &mockSupplierRepo{}, ca)
	detail, err := svc.Detail(newSvcCtx(), 1)
	if err != nil {
		t.Fatalf("Detail: %v", err)
	}
	if detail.BlindBox.Name != "FromCache" {
		t.Errorf("Detail returned name=%q, want FromCache", detail.BlindBox.Name)
	}
	if bbRepo.getByIDCalls != 0 {
		t.Errorf("GetByID called %d times on cache hit, want 0", bbRepo.getByIDCalls)
	}
}

func TestBlindBoxDetail_CacheSetFailureDoesNotBlock(t *testing.T) {
	// cache.Set 失败不应阻塞详情返回。
	bbRepo := &mockBlindBoxRepo{
		getByIDFunc: func(c *gin.Context, id int64) (*mall.MallBlindBox, error) {
			return &mall.MallBlindBox{ID: 1, SupplierID: 7, Status: mall.StatusActive, OnSale: true, Price: 100}, nil
		},
	}
	supRepo := &mockSupplierRepo{
		getByIDFunc: func(c *gin.Context, id int64) (*mall.MallSupplier, error) {
			return &mall.MallSupplier{ID: 7, Status: mall.StatusActive}, nil
		},
	}
	cpRepo := &mockCardPoolRepo{
		getPoolByBlindBoxIDFunc: func(c *gin.Context, blindBoxID int64) (*mall.MallCardPool, error) {
			return &mall.MallCardPool{ID: 100, BlindBoxID: blindBoxID}, nil
		},
		listItemsByPoolIDFunc: func(c *gin.Context, poolID int64) ([]mall.MallCardPoolItem, error) {
			return nil, nil
		},
	}
	ca := newMockCache()
	ca.setErr = errors.New("redis down")

	svc := newSvc(bbRepo, cpRepo, &mockPromotionRepo{}, supRepo, ca)
	detail, err := svc.Detail(newSvcCtx(), 1)
	if err != nil {
		t.Fatalf("Detail with cache.Set error: %v", err)
	}
	if detail == nil {
		t.Errorf("Detail returned nil on cache.Set error")
	}
}

func TestBlindBoxDetail_NoPoolIsGraceful(t *testing.T) {
	// 卡池缺失不阻断详情，返回空卡池即可。
	bbRepo := &mockBlindBoxRepo{
		getByIDFunc: func(c *gin.Context, id int64) (*mall.MallBlindBox, error) {
			return &mall.MallBlindBox{ID: 1, SupplierID: 7, Status: mall.StatusActive, OnSale: true, Price: 100}, nil
		},
	}
	supRepo := &mockSupplierRepo{
		getByIDFunc: func(c *gin.Context, id int64) (*mall.MallSupplier, error) {
			return &mall.MallSupplier{ID: 7, Status: mall.StatusActive}, nil
		},
	}
	cpRepo := &mockCardPoolRepo{
		getPoolByBlindBoxIDFunc: func(c *gin.Context, blindBoxID int64) (*mall.MallCardPool, error) {
			return nil, gorm.ErrRecordNotFound
		},
	}
	svc := newSvc(bbRepo, cpRepo, &mockPromotionRepo{}, supRepo, newMockCache())
	detail, err := svc.Detail(newSvcCtx(), 1)
	if err != nil {
		t.Fatalf("Detail: %v", err)
	}
	if detail.Pool.ID != 0 {
		t.Errorf("detail.Pool.ID = %d, want 0 (missing pool)", detail.Pool.ID)
	}
	if detail.Items == nil || len(detail.Items) != 0 {
		t.Errorf("detail.Items = %v, want empty", detail.Items)
	}
}

func TestBlindBoxDetail_InvalidID(t *testing.T) {
	svc := newSvc(&mockBlindBoxRepo{}, &mockCardPoolRepo{}, &mockPromotionRepo{}, &mockSupplierRepo{}, nil)
	_, err := svc.Detail(newSvcCtx(), 0)
	if !errors.Is(err, ErrBlindBoxNotFound) {
		t.Errorf("Detail(id=0) err = %v, want ErrBlindBoxNotFound", err)
	}
	_, err = svc.Detail(newSvcCtx(), -1)
	if !errors.Is(err, ErrBlindBoxNotFound) {
		t.Errorf("Detail(id=-1) err = %v, want ErrBlindBoxNotFound", err)
	}
}

func TestInvalidateDetail(t *testing.T) {
	ca := newMockCache()
	ca.data[cacheKey(1)] = "x"

	// nil hotspot：只失效 L1/L2。
	if err := InvalidateDetail(context.Background(), ca, nil, 1); err != nil {
		t.Fatalf("InvalidateDetail: %v", err)
	}
	if _, ok := ca.data[cacheKey(1)]; ok {
		t.Errorf("cache key not deleted")
	}
	if ca.delCalls != 1 {
		t.Errorf("cache.Del called %d times, want 1", ca.delCalls)
	}

	// nil cache + nil hotspot 是 no-op
	if err := InvalidateDetail(context.Background(), nil, nil, 1); err != nil {
		t.Errorf("InvalidateDetail with nil caches returned err: %v", err)
	}

	// hotspot 路径：热点 key 一起失效。
	hp := &fakeHotspotCache{}
	if err := InvalidateDetail(context.Background(), nil, hp, 1); err != nil {
		t.Fatalf("InvalidateDetail with hotspot: %v", err)
	}
	if !hp.invalidated[hotspot.BlindBoxKey(1)] {
		t.Errorf("hotspot key %q not invalidated", hotspot.BlindBoxKey(1))
	}
}

// fakeHotspotCache 是 hotspot.HotspotCache 的最小 stub（只记录 Invalidate）。
type fakeHotspotCache struct {
	invalidated map[string]bool
}

func (f *fakeHotspotCache) Get(ctx context.Context, key string, loader hotspot.HotspotLoader, staleAfter time.Duration) (any, error) {
	return nil, errors.New("fakeHotspotCache: Get not supported")
}

func (f *fakeHotspotCache) Invalidate(ctx context.Context, key string) error {
	if f.invalidated == nil {
		f.invalidated = map[string]bool{}
	}
	f.invalidated[key] = true
	return nil
}

// =============================================================================
// D5.1 防穿透：Detail 的 notFound 占位流（任务 17.5）
// =============================================================================

// TestDetail_LegacyCache_NotFoundPlaceholder 验证 TTL 缓存路径:
// loader(ErrBlindBoxNotFound,已包装 cache.ErrNotFound) → 写 30s 占位 →
// 第二次请求占位命中,不再打 DB。
func TestDetail_LegacyCache_NotFoundPlaceholder(t *testing.T) {
	ca := newMockCache()
	bb := &mockBlindBoxRepo{} // GetByID 默认 gorm.ErrRecordNotFound
	cp := &mockCardPoolRepo{}
	promo := &mockPromotionRepo{}
	sup := &mockSupplierRepo{}
	svc := newSvc(bb, cp, promo, sup, ca)

	c := newSvcCtx()
	// 第一次:miss → DB not found → ErrBlindBoxNotFound + 写占位。
	if _, err := svc.Detail(c, 999); !errors.Is(err, ErrBlindBoxNotFound) {
		t.Fatalf("first Detail err = %v, want ErrBlindBoxNotFound", err)
	}
	if ca.data[cacheKey(999)] != cache.NotFoundPlaceholderValue() {
		t.Errorf("placeholder not written: %q", ca.data[cacheKey(999)])
	}
	if !withinJitter(ca.lastSetTTL, cache.NotFoundPlaceholderTTL) {
		t.Errorf("placeholder TTL = %v, want in jitter range of %v", ca.lastSetTTL, cache.NotFoundPlaceholderTTL)
	}
	firstCalls := bb.getByIDCalls

	// 第二次:占位命中 → ErrBlindBoxNotFound 且不再打 DB。
	if _, err := svc.Detail(c, 999); !errors.Is(err, ErrBlindBoxNotFound) {
		t.Fatalf("second Detail err = %v, want ErrBlindBoxNotFound", err)
	}
	if bb.getByIDCalls != firstCalls {
		t.Errorf("DB hit on placeholder: calls %d → %d", firstCalls, bb.getByIDCalls)
	}
}

// TestDetail_Hotspot_NotFoundPlaceholder 验证 Hotspot 路径的占位流(17.5):
// 占位命中翻译回 ErrBlindBoxNotFound;业务创建数据 + Invalidate 后读到真实数据。
func TestDetail_Hotspot_NotFoundPlaceholder(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	hot := hotspot.NewRedis(client)

	exists := atomic.Bool{}
	bb := &mockBlindBoxRepo{
		getByIDFunc: func(c *gin.Context, id int64) (*mall.MallBlindBox, error) {
			if !exists.Load() {
				return nil, gorm.ErrRecordNotFound
			}
			return &mall.MallBlindBox{ID: id, SupplierID: 3, Name: "新盲盒", Price: 99, Status: mall.StatusActive, OnSale: true}, nil
		},
	}
	sup := &mockSupplierRepo{
		getByIDFunc: func(c *gin.Context, id int64) (*mall.MallSupplier, error) {
			return &mall.MallSupplier{ID: id, Name: "卡商", Status: mall.StatusActive}, nil
		},
	}
	cp := &mockCardPoolRepo{
		getPoolByBlindBoxIDFunc: func(c *gin.Context, id int64) (*mall.MallCardPool, error) {
			return nil, gorm.ErrRecordNotFound
		},
	}
	promo := &mockPromotionRepo{}
	svc := NewBlindBoxService(BlindBoxServiceDeps{
		BlindBoxRepo:  bb,
		CardPoolRepo:  cp,
		PromotionRepo: promo,
		SupplierRepo:  sup,
		Hotspot:       hot,
	})

	c := newSvcCtx()
	// 第一次:DB 查无 → 占位写入 → ErrBlindBoxNotFound。
	if _, err := svc.Detail(c, 777); !errors.Is(err, ErrBlindBoxNotFound) {
		t.Fatalf("first Detail err = %v, want ErrBlindBoxNotFound", err)
	}
	// 第二次:占位命中 → ErrBlindBoxNotFound,不打 DB。
	if _, err := svc.Detail(c, 777); !errors.Is(err, ErrBlindBoxNotFound) {
		t.Fatalf("placeholder Detail err = %v, want ErrBlindBoxNotFound", err)
	}
	dbCallsAfterPlaceholder := bb.getByIDCalls

	// 业务创建数据 + 写路径失效占位(17.8 的 service 侧语义)。
	exists.Store(true)
	if err := hot.Invalidate(c.Request.Context(), hotspot.BlindBoxKey(777)); err != nil {
		t.Fatalf("Invalidate: %v", err)
	}
	detail, err := svc.Detail(c, 777)
	if err != nil {
		t.Fatalf("Detail after create+invalidate: %v", err)
	}
	if detail.BlindBox.Name != "新盲盒" {
		t.Errorf("detail = %+v", detail.BlindBox)
	}
	if bb.getByIDCalls <= dbCallsAfterPlaceholder {
		t.Errorf("expected a fresh cold-start DB hit after invalidate (calls=%d)", bb.getByIDCalls)
	}
}
