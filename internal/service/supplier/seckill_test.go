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
// helpers / fakes
// =============================================================================

func newSeckillCtx() *gin.Context {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/api/v1/supplier/seckill", nil)
	return c
}

// fakeSeckillRepo 是 mallRepo.SeckillRepository 的可编程 stub。
type fakeSeckillRepo struct {
	mallRepo.SeckillRepository

	createFunc         func(*gin.Context, *mall.MallSeckillActivity) (int64, error)
	getByIDFunc        func(*gin.Context, int64) (*mall.MallSeckillActivity, error)
	updateFunc         func(*gin.Context, *mall.MallSeckillActivity) error
	markRedisInitFunc  func(*gin.Context, int64) error
	listBySupplierFunc func(*gin.Context, int64, repository.ListOptions) ([]mall.MallSeckillActivity, int64, error)
	toggleStatusFunc   func(*gin.Context, int64, mall.MallBlindBoxStatus) error
	sumPoolStockFunc   func(*gin.Context, int64) (int, error)
}

func (f *fakeSeckillRepo) Create(c *gin.Context, s *mall.MallSeckillActivity) (int64, error) {
	if f.createFunc != nil {
		return f.createFunc(c, s)
	}
	s.ID = 100
	return 100, nil
}

func (f *fakeSeckillRepo) GetByID(c *gin.Context, id int64) (*mall.MallSeckillActivity, error) {
	if f.getByIDFunc != nil {
		return f.getByIDFunc(c, id)
	}
	return nil, nil
}

func (f *fakeSeckillRepo) Update(c *gin.Context, s *mall.MallSeckillActivity) error {
	if f.updateFunc != nil {
		return f.updateFunc(c, s)
	}
	return nil
}

func (f *fakeSeckillRepo) MarkRedisInitialized(c *gin.Context, id int64) error {
	if f.markRedisInitFunc != nil {
		return f.markRedisInitFunc(c, id)
	}
	return nil
}

func (f *fakeSeckillRepo) ListBySupplier(c *gin.Context, sid int64, opts repository.ListOptions) ([]mall.MallSeckillActivity, int64, error) {
	if f.listBySupplierFunc != nil {
		return f.listBySupplierFunc(c, sid, opts)
	}
	return []mall.MallSeckillActivity{}, 0, nil
}

func (f *fakeSeckillRepo) ToggleStatus(c *gin.Context, id int64, st mall.MallBlindBoxStatus) error {
	if f.toggleStatusFunc != nil {
		return f.toggleStatusFunc(c, id, st)
	}
	return nil
}

func (f *fakeSeckillRepo) SumPoolStock(c *gin.Context, bbID int64) (int, error) {
	if f.sumPoolStockFunc != nil {
		return f.sumPoolStockFunc(c, bbID)
	}
	return 1000, nil
}

var _ mallRepo.SeckillRepository = (*fakeSeckillRepo)(nil)

// fakeBlindBoxRepo 是 mallRepo.BlindBoxRepository 的最小 stub。
type fakeBlindBoxRepo struct {
	mallRepo.BlindBoxRepository
	getByIDFunc func(*gin.Context, int64) (*mall.MallBlindBox, error)
}

func (f *fakeBlindBoxRepo) GetByID(c *gin.Context, id int64) (*mall.MallBlindBox, error) {
	if f.getByIDFunc != nil {
		return f.getByIDFunc(c, id)
	}
	return nil, nil
}

var _ mallRepo.BlindBoxRepository = (*fakeBlindBoxRepo)(nil)

// seckillMockCache 复用 product_test.go 中的 mockCache（同一包直接可见）。
// 这里新增 seckill 特定的 SetNX 行为记录。
type seckillMockCache struct {
	*mockCache
	setNXFunc  func(ctx context.Context, key, value string, ttl time.Duration) (bool, error)
	setNXCalls int
	lastKey    string
	lastValue  string
}

func (m *seckillMockCache) SetNX(ctx context.Context, key, value string, ttl time.Duration) (bool, error) {
	m.setNXCalls++
	m.lastKey = key
	m.lastValue = value
	if m.setNXFunc != nil {
		return m.setNXFunc(ctx, key, value, ttl)
	}
	return true, nil
}
func (m *seckillMockCache) Incr(ctx context.Context, key string) (int64, error) {
	return 0, nil
}
func (m *seckillMockCache) Decr(ctx context.Context, key string) (int64, error) {
	return 0, nil
}

var _ cache.Cache = (*seckillMockCache)(nil)

// =============================================================================
// List
// =============================================================================

func TestSeckillList_InvalidSupplierID(t *testing.T) {
	svc := NewSeckillService(SeckillServiceDeps{SeckillRepo: &fakeSeckillRepo{}})
	items, total, err := svc.List(newSeckillCtx(), 0, repository.ListOptions{Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(items) != 0 || total != 0 {
		t.Errorf("got (%d, %d), want (0, 0)", len(items), total)
	}
}

func TestSeckillList_DelegatesToRepo(t *testing.T) {
	want := []mall.MallSeckillActivity{{ID: 1, SupplierID: 7}}
	repo := &fakeSeckillRepo{
		listBySupplierFunc: func(c *gin.Context, sid int64, opts repository.ListOptions) ([]mall.MallSeckillActivity, int64, error) {
			if sid != 7 {
				t.Errorf("supplierID = %d, want 7", sid)
			}
			return want, 1, nil
		},
	}
	svc := NewSeckillService(SeckillServiceDeps{SeckillRepo: repo, TxRunner: &mockTxRunner{}})
	items, total, err := svc.List(newSeckillCtx(), 7, repository.ListOptions{Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if total != 1 || len(items) != 1 {
		t.Errorf("got (%d, %d), want (1, 1)", total, len(items))
	}
}

// =============================================================================
// Create
// =============================================================================

func TestSeckillCreate_HappyPath(t *testing.T) {
	repo := &fakeSeckillRepo{
		getByIDFunc: func(c *gin.Context, id int64) (*mall.MallSeckillActivity, error) {
			return &mall.MallSeckillActivity{ID: 100, SupplierID: 7, Status: mall.StatusActive}, nil
		},
	}
	bb := &fakeBlindBoxRepo{
		getByIDFunc: func(c *gin.Context, id int64) (*mall.MallBlindBox, error) {
			return &mall.MallBlindBox{ID: 2, SupplierID: 7, Price: 99.0}, nil
		},
	}
	mc := &seckillMockCache{mockCache: &mockCache{}}
	svc := NewSeckillService(SeckillServiceDeps{SeckillRepo: repo, BlindBoxRepo: bb, Cache: mc, TxRunner: &mockTxRunner{}})

	in := &mall.MallSeckillActivity{
		BlindBoxID:   2,
		SeckillPrice: 79.0,
		TotalStock:   100,
		PerUserLimit: 1,
		StartAt:      time.Now().Add(time.Hour),
		EndAt:        time.Now().Add(2 * time.Hour),
		Status:       mall.StatusActive,
	}

	out, err := svc.Create(newSeckillCtx(), 7, in)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if out.ID != 100 {
		t.Errorf("out.ID = %d, want 100", out.ID)
	}
	if !out.RedisInitialized {
		t.Errorf("out.RedisInitialized = false, want true")
	}
	if mc.setNXCalls != 1 {
		t.Errorf("setNXCalls = %d, want 1", mc.setNXCalls)
	}
	if mc.lastKey != "seckill:stock:100" {
		t.Errorf("lastKey = %q, want seckill:stock:100", mc.lastKey)
	}
	if mc.lastValue != "100" {
		t.Errorf("lastValue = %q, want 100", mc.lastValue)
	}
}

func TestSeckillCreate_InvalidTimeWindow(t *testing.T) {
	svc := NewSeckillService(SeckillServiceDeps{SeckillRepo: &fakeSeckillRepo{}})
	now := time.Now()
	in := &mall.MallSeckillActivity{
		BlindBoxID:   2,
		SeckillPrice: 79.0,
		TotalStock:   100,
		StartAt:      now.Add(time.Hour),
		EndAt:        now, // before start
		Status:       mall.StatusActive,
	}
	_, err := svc.Create(newSeckillCtx(), 7, in)
	if !errors.Is(err, ErrInvalidSeckillWindow) {
		t.Errorf("err = %v, want ErrInvalidSeckillWindow", err)
	}
}

func TestSeckillCreate_InvalidPrice(t *testing.T) {
	bb := &fakeBlindBoxRepo{
		getByIDFunc: func(c *gin.Context, id int64) (*mall.MallBlindBox, error) {
			return &mall.MallBlindBox{ID: 2, SupplierID: 7, Price: 99.0}, nil
		},
	}
	svc := NewSeckillService(SeckillServiceDeps{SeckillRepo: &fakeSeckillRepo{}, BlindBoxRepo: bb, Cache: &seckillMockCache{mockCache: &mockCache{}}, TxRunner: &mockTxRunner{}})
	now := time.Now()
	in := &mall.MallSeckillActivity{
		BlindBoxID:   2,
		SeckillPrice: 99.0, // 等于原价，非法
		TotalStock:   100,
		StartAt:      now.Add(time.Hour),
		EndAt:        now.Add(2 * time.Hour),
		Status:       mall.StatusActive,
	}
	_, err := svc.Create(newSeckillCtx(), 7, in)
	if !errors.Is(err, ErrInvalidSeckillPrice) {
		t.Errorf("err = %v, want ErrInvalidSeckillPrice", err)
	}
}

func TestSeckillCreate_BlindBoxNotOwned(t *testing.T) {
	bb := &fakeBlindBoxRepo{
		getByIDFunc: func(c *gin.Context, id int64) (*mall.MallBlindBox, error) {
			return &mall.MallBlindBox{ID: 2, SupplierID: 99, Price: 99.0}, nil // 不属于 supplier 7
		},
	}
	svc := NewSeckillService(SeckillServiceDeps{SeckillRepo: &fakeSeckillRepo{}, BlindBoxRepo: bb, Cache: &seckillMockCache{mockCache: &mockCache{}}, TxRunner: &mockTxRunner{}})
	now := time.Now()
	in := &mall.MallSeckillActivity{
		BlindBoxID:   2,
		SeckillPrice: 79.0,
		TotalStock:   100,
		StartAt:      now.Add(time.Hour),
		EndAt:        now.Add(2 * time.Hour),
	}
	_, err := svc.Create(newSeckillCtx(), 7, in)
	if !errors.Is(err, ErrSeckillForbidden) {
		t.Errorf("err = %v, want ErrSeckillForbidden", err)
	}
}

func TestSeckillCreate_BlindBoxNotFound(t *testing.T) {
	bb := &fakeBlindBoxRepo{
		getByIDFunc: func(c *gin.Context, id int64) (*mall.MallBlindBox, error) {
			return nil, gorm.ErrRecordNotFound
		},
	}
	svc := NewSeckillService(SeckillServiceDeps{SeckillRepo: &fakeSeckillRepo{}, BlindBoxRepo: bb, Cache: &seckillMockCache{mockCache: &mockCache{}}, TxRunner: &mockTxRunner{}})
	now := time.Now()
	in := &mall.MallSeckillActivity{
		BlindBoxID:   999,
		SeckillPrice: 79.0,
		TotalStock:   100,
		StartAt:      now.Add(time.Hour),
		EndAt:        now.Add(2 * time.Hour),
	}
	_, err := svc.Create(newSeckillCtx(), 7, in)
	if !errors.Is(err, ErrSeckillForbidden) {
		t.Errorf("err = %v, want ErrSeckillForbidden", err)
	}
}

func TestSeckillCreate_StockExceedsPool(t *testing.T) {
	bb := &fakeBlindBoxRepo{
		getByIDFunc: func(c *gin.Context, id int64) (*mall.MallBlindBox, error) {
			return &mall.MallBlindBox{ID: 2, SupplierID: 7, Price: 99.0}, nil
		},
	}
	repo := &fakeSeckillRepo{
		sumPoolStockFunc: func(c *gin.Context, bbID int64) (int, error) {
			return 50, nil // 卡池只有 50
		},
	}
	svc := NewSeckillService(SeckillServiceDeps{SeckillRepo: repo, BlindBoxRepo: bb, Cache: &seckillMockCache{mockCache: &mockCache{}}, TxRunner: &mockTxRunner{}})
	now := time.Now()
	in := &mall.MallSeckillActivity{
		BlindBoxID:   2,
		SeckillPrice: 79.0,
		TotalStock:   100, // 超过卡池
		StartAt:      now.Add(time.Hour),
		EndAt:        now.Add(2 * time.Hour),
	}
	_, err := svc.Create(newSeckillCtx(), 7, in)
	if !errors.Is(err, ErrSeckillStockExceedsPool) {
		t.Errorf("err = %v, want ErrSeckillStockExceedsPool", err)
	}
}

func TestSeckillCreate_RedisInitFailed(t *testing.T) {
	bb := &fakeBlindBoxRepo{
		getByIDFunc: func(c *gin.Context, id int64) (*mall.MallBlindBox, error) {
			return &mall.MallBlindBox{ID: 2, SupplierID: 7, Price: 99.0}, nil
		},
	}
	mc := &seckillMockCache{mockCache: &mockCache{}}
	mc.setNXFunc = func(ctx context.Context, key, value string, ttl time.Duration) (bool, error) {
		return false, nil // 模拟 SETNX 已存在（拒绝覆盖）
	}
	svc := NewSeckillService(SeckillServiceDeps{SeckillRepo: &fakeSeckillRepo{}, BlindBoxRepo: bb, Cache: mc, TxRunner: &mockTxRunner{}})
	now := time.Now()
	in := &mall.MallSeckillActivity{
		BlindBoxID:   2,
		SeckillPrice: 79.0,
		TotalStock:   100,
		StartAt:      now.Add(time.Hour),
		EndAt:        now.Add(2 * time.Hour),
	}
	_, err := svc.Create(newSeckillCtx(), 7, in)
	if !errors.Is(err, ErrRedisInitFailed) {
		t.Errorf("err = %v, want ErrRedisInitFailed", err)
	}
}

// =============================================================================
// Update
// =============================================================================

func TestSeckillUpdate_NotOwned(t *testing.T) {
	repo := &fakeSeckillRepo{
		getByIDFunc: func(c *gin.Context, id int64) (*mall.MallSeckillActivity, error) {
			return &mall.MallSeckillActivity{ID: 1, SupplierID: 99}, nil // 不是 supplier 7
		},
	}
	svc := NewSeckillService(SeckillServiceDeps{SeckillRepo: repo})
	newPrice := 69.0
	err := svc.Update(newSeckillCtx(), 7, 1, SeckillPatch{SeckillPrice: &newPrice})
	if !errors.Is(err, ErrSeckillForbidden) {
		t.Errorf("err = %v, want ErrSeckillForbidden", err)
	}
}

func TestSeckillUpdate_HappyPath(t *testing.T) {
	repo := &fakeSeckillRepo{
		getByIDFunc: func(c *gin.Context, id int64) (*mall.MallSeckillActivity, error) {
			return &mall.MallSeckillActivity{ID: 1, SupplierID: 7, BlindBoxID: 2, SeckillPrice: 80.0}, nil
		},
		updateFunc: func(c *gin.Context, s *mall.MallSeckillActivity) error {
			if s.SeckillPrice != 69.0 {
				t.Errorf("SeckillPrice = %v, want 69", s.SeckillPrice)
			}
			return nil
		},
	}
	bb := &fakeBlindBoxRepo{
		getByIDFunc: func(c *gin.Context, id int64) (*mall.MallBlindBox, error) {
			return &mall.MallBlindBox{ID: 2, Price: 99.0}, nil
		},
	}
	svc := NewSeckillService(SeckillServiceDeps{SeckillRepo: repo, BlindBoxRepo: bb})
	newPrice := 69.0
	if err := svc.Update(newSeckillCtx(), 7, 1, SeckillPatch{SeckillPrice: &newPrice}); err != nil {
		t.Fatalf("Update: %v", err)
	}
}

// =============================================================================
// Toggle
// =============================================================================

func TestSeckillToggle_HappyPath(t *testing.T) {
	repo := &fakeSeckillRepo{
		getByIDFunc: func(c *gin.Context, id int64) (*mall.MallSeckillActivity, error) {
			return &mall.MallSeckillActivity{ID: 1, SupplierID: 7, Status: mall.StatusActive}, nil
		},
		toggleStatusFunc: func(c *gin.Context, id int64, st mall.MallBlindBoxStatus) error {
			if st != mall.StatusDisabled {
				t.Errorf("toggleStatus st = %v, want disabled", st)
			}
			return nil
		},
	}
	svc := NewSeckillService(SeckillServiceDeps{SeckillRepo: repo})
	if err := svc.Toggle(newSeckillCtx(), 7, 1); err != nil {
		t.Fatalf("Toggle: %v", err)
	}
}

func TestSeckillToggle_NotFound(t *testing.T) {
	repo := &fakeSeckillRepo{
		getByIDFunc: func(c *gin.Context, id int64) (*mall.MallSeckillActivity, error) {
			return nil, gorm.ErrRecordNotFound
		},
	}
	svc := NewSeckillService(SeckillServiceDeps{SeckillRepo: repo})
	if err := svc.Toggle(newSeckillCtx(), 7, 999); !errors.Is(err, ErrSeckillNotFound) {
		t.Errorf("err = %v, want ErrSeckillNotFound", err)
	}
}
