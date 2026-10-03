// Package user — seckill_browse_test.go 覆盖 C 端秒杀浏览 service
// （List 过滤 + 剩余名额 Redis/DB 双来源 + Detail 可见性校验）。
package user

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
	supplierRepo "ai-go-mall/internal/repository/supplier"
)

// --- fakes ---

type fakeSeckillBrowseRepo struct {
	mallRepo.SeckillRepository // 未实现的方法 panic 即未用到

	active      []mall.MallSeckillActivity
	activeTotal int64
	byID        map[int64]*mall.MallSeckillActivity
	counts      map[int64]int64
	countErr    error
}

func (f *fakeSeckillBrowseRepo) ListActive(c *gin.Context, now time.Time, opts repository.ListOptions) ([]mall.MallSeckillActivity, int64, error) {
	return f.active, f.activeTotal, nil
}

func (f *fakeSeckillBrowseRepo) GetByID(c *gin.Context, id int64) (*mall.MallSeckillActivity, error) {
	if s, ok := f.byID[id]; ok {
		return s, nil
	}
	return nil, gorm.ErrRecordNotFound
}

func (f *fakeSeckillBrowseRepo) CountDeductionLogsBySeckillIDs(c *gin.Context, ids []int64) (map[int64]int64, error) {
	if f.countErr != nil {
		return nil, f.countErr
	}
	out := make(map[int64]int64, len(ids))
	for _, id := range ids {
		if n, ok := f.counts[id]; ok {
			out[id] = n
		}
	}
	return out, nil
}

type fakeBrowseBlindBoxRepo struct {
	mallRepo.BlindBoxRepository
	byID map[int64]*mall.MallBlindBox
}

func (f *fakeBrowseBlindBoxRepo) GetByID(c *gin.Context, id int64) (*mall.MallBlindBox, error) {
	if b, ok := f.byID[id]; ok {
		return b, nil
	}
	return nil, gorm.ErrRecordNotFound
}

func (f *fakeBrowseBlindBoxRepo) ListByIDs(c *gin.Context, ids []int64) ([]mall.MallBlindBox, error) {
	out := make([]mall.MallBlindBox, 0, len(ids))
	for _, id := range ids {
		if b, ok := f.byID[id]; ok {
			out = append(out, *b)
		}
	}
	return out, nil
}

type fakeBrowseSupplierRepo struct {
	supplierRepo.SupplierRepository
	byID map[int64]*mall.MallSupplier
}

func (f *fakeBrowseSupplierRepo) GetByID(c *gin.Context, id int64) (*mall.MallSupplier, error) {
	if s, ok := f.byID[id]; ok {
		return s, nil
	}
	return nil, gorm.ErrRecordNotFound
}

func (f *fakeBrowseSupplierRepo) ListByIDs(c *gin.Context, ids []int64) ([]mall.MallSupplier, error) {
	out := make([]mall.MallSupplier, 0, len(ids))
	for _, id := range ids {
		if s, ok := f.byID[id]; ok {
			out = append(out, *s)
		}
	}
	return out, nil
}

type browseMockCache struct {
	cache.Cache
	values map[string]string // key → decimal string
	getErr error             // 非 nil 时 Get 一律失败（模拟 Redis 故障）
}

func (m *browseMockCache) Get(ctx context.Context, key string) (string, bool, error) {
	if m.getErr != nil {
		return "", false, m.getErr
	}
	if v, ok := m.values[key]; ok {
		return v, true, nil
	}
	return "", false, cache.ErrCacheMiss
}

// --- helpers ---

func newBrowseContext() *gin.Context {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", "/api/v1/user/seckill/list", nil)
	return c
}

func browseFixture() (*fakeSeckillBrowseRepo, *fakeBrowseBlindBoxRepo, *fakeBrowseSupplierRepo) {
	now := time.Now()
	seckillRepo := &fakeSeckillBrowseRepo{
		activeTotal: 2,
		active: []mall.MallSeckillActivity{
			{ID: 11, SupplierID: 3, BlindBoxID: 5, SeckillPrice: 79, TotalStock: 100, PerUserLimit: 1, StartAt: now.Add(-time.Hour), EndAt: now.Add(time.Hour), Status: mall.StatusActive, RedisInitialized: true},
			{ID: 12, SupplierID: 3, BlindBoxID: 6, SeckillPrice: 59, TotalStock: 50, PerUserLimit: 2, StartAt: now.Add(-30 * time.Minute), EndAt: now.Add(2 * time.Hour), Status: mall.StatusActive, RedisInitialized: true},
		},
		byID: map[int64]*mall.MallSeckillActivity{},
		counts: map[int64]int64{
			12: 3, // id 12 已扣 3
		},
	}
	for i := range seckillRepo.active {
		seckillRepo.byID[seckillRepo.active[i].ID] = &seckillRepo.active[i]
	}
	bbRepo := &fakeBrowseBlindBoxRepo{byID: map[int64]*mall.MallBlindBox{
		5: {ID: 5, SupplierID: 3, Name: "NBA 全明星盲盒", Cover: "https://cdn/5.jpg", Price: 99, Status: mall.StatusActive, OnSale: true, Description: "含签名卡"},
		6: {ID: 6, SupplierID: 3, Name: "新秀盲盒", Cover: "https://cdn/6.jpg", Price: 69, Status: mall.StatusActive, OnSale: true},
	}}
	supRepo := &fakeBrowseSupplierRepo{byID: map[int64]*mall.MallSupplier{
		3: {ID: 3, Name: "球星卡旗舰店", Status: mall.StatusActive},
	}}
	return seckillRepo, bbRepo, supRepo
}

func newBrowseService(sr *fakeSeckillBrowseRepo, br *fakeBrowseBlindBoxRepo, pr *fakeBrowseSupplierRepo, cc cache.Cache) SeckillBrowseService {
	return NewSeckillBrowseService(SeckillBrowseServiceDeps{
		SeckillRepo:  sr,
		BlindBoxRepo: br,
		SupplierRepo: pr,
		Cache:        cc,
	})
}

// --- List ---

func TestSeckillBrowseList_HappyPath(t *testing.T) {
	sr, br, pr := browseFixture()
	// id 11 剩余 37 在 Redis；id 12 miss → DB 兜底 50 - 3 = 47。
	cc := &browseMockCache{values: map[string]string{"seckill:stock:11": "37"}}
	svc := newBrowseService(sr, br, pr, cc)

	items, total, err := svc.List(newBrowseContext(), repository.ListOptions{Page: 1, PageSize: 20})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if total != 2 || len(items) != 2 {
		t.Fatalf("total = %d, len(items) = %d, want 2/2", total, len(items))
	}
	first := items[0]
	if first.ID != 11 || first.BlindBoxName != "NBA 全明星盲盒" || first.SupplierName != "球星卡旗舰店" {
		t.Errorf("first = %+v", first)
	}
	if first.RemainingStock != 37 || first.TotalStock != 100 || first.OriginalPrice != 99 || first.SeckillPrice != 79 {
		t.Errorf("first stock/price = %+v", first)
	}
	if items[1].RemainingStock != 47 {
		t.Errorf("items[1].RemainingStock = %d, want 47 (DB fallback)", items[1].RemainingStock)
	}
}

func TestSeckillBrowseList_FiltersUnavailable(t *testing.T) {
	sr, br, pr := browseFixture()
	// 盲盒 6 下架 → 活动 12 不可见；供应商禁用 → 全部不可见。
	br.byID[6].OnSale = false
	t.Run("offSaleBox", func(t *testing.T) {
		svc := newBrowseService(sr, br, pr, &browseMockCache{})
		items, total, err := svc.List(newBrowseContext(), repository.ListOptions{Page: 1, PageSize: 20})
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if total != 2 || len(items) != 1 || items[0].ID != 11 {
			t.Errorf("total = %d, items = %+v, want only id 11", total, items)
		}
	})
	t.Run("disabledSupplier", func(t *testing.T) {
		pr.byID[3].Status = mall.StatusDisabled
		svc := newBrowseService(sr, br, pr, &browseMockCache{})
		items, total, _ := svc.List(newBrowseContext(), repository.ListOptions{Page: 1, PageSize: 20})
		if total != 2 || len(items) != 0 {
			t.Errorf("total = %d, items = %d, want 0 visible", total, len(items))
		}
	})
}

func TestSeckillBrowseList_RedisDownAndDBFallbackFails_UnknownRemaining(t *testing.T) {
	sr, br, pr := browseFixture()
	sr.countErr = errors.New("db down")
	// Cache 为 nil → 全部 miss；DB 兜底也挂 → remaining = -1。
	svc := newBrowseService(sr, br, pr, nil)

	items, _, err := svc.List(newBrowseContext(), repository.ListOptions{Page: 1, PageSize: 20})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	for _, it := range items {
		if it.RemainingStock != remainingUnknown {
			t.Errorf("id %d RemainingStock = %d, want -1", it.ID, it.RemainingStock)
		}
	}
}

// --- Detail ---

func TestSeckillBrowseDetail_HappyPath(t *testing.T) {
	sr, br, pr := browseFixture()
	cc := &browseMockCache{values: map[string]string{"seckill:stock:11": "42"}}
	svc := newBrowseService(sr, br, pr, cc)

	detail, err := svc.Detail(newBrowseContext(), 11)
	if err != nil {
		t.Fatalf("Detail: %v", err)
	}
	if detail.ID != 11 || detail.BlindBoxName != "NBA 全明星盲盒" || detail.Description != "含签名卡" {
		t.Errorf("detail = %+v", detail)
	}
	if detail.RemainingStock != 42 {
		t.Errorf("RemainingStock = %d, want 42", detail.RemainingStock)
	}
}

func TestSeckillBrowseDetail_NotFoundAndDisabled(t *testing.T) {
	sr, br, pr := browseFixture()
	svc := newBrowseService(sr, br, pr, &browseMockCache{})

	if _, err := svc.Detail(newBrowseContext(), 999); !errors.Is(err, ErrSeckillNotAvailable) {
		t.Errorf("missing id err = %v, want ErrSeckillNotAvailable", err)
	}

	sr.byID[11].Status = mall.StatusDisabled
	if _, err := svc.Detail(newBrowseContext(), 11); !errors.Is(err, ErrSeckillNotAvailable) {
		t.Errorf("disabled err = %v, want ErrSeckillNotAvailable", err)
	}
}

func TestSeckillBrowseDetail_BlindBoxUnavailable(t *testing.T) {
	sr, br, pr := browseFixture()
	svc := newBrowseService(sr, br, pr, &browseMockCache{})

	br.byID[5].OnSale = false
	if _, err := svc.Detail(newBrowseContext(), 11); !errors.Is(err, ErrSeckillNotAvailable) {
		t.Errorf("off-sale err = %v, want ErrSeckillNotAvailable", err)
	}

	br.byID[5].OnSale = true
	pr.byID[3].Status = mall.StatusDisabled
	if _, err := svc.Detail(newBrowseContext(), 11); !errors.Is(err, ErrSeckillNotAvailable) {
		t.Errorf("disabled supplier err = %v, want ErrSeckillNotAvailable", err)
	}
}
