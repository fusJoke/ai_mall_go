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

func newPromotionCtx() *gin.Context {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", "/api/v1/supplier/promotions", nil)
	return c
}

// =============================================================================
// Mock PromotionRepository
// =============================================================================

type promoPromotionRepo struct {
	getByIDFunc         func(c *gin.Context, id int64) (*mall.MallPromotion, error)
	listBySupplierFunc  func(c *gin.Context, sid int64, opts repository.ListOptions) ([]mall.MallPromotion, int64, error)
	listTimeOverlapFunc func(c *gin.Context, bbID int64, s, e time.Time) ([]mall.MallPromotion, error)
	createFunc          func(c *gin.Context, e *mall.MallPromotion) error
	updateFunc          func(c *gin.Context, e *mall.MallPromotion) error
	deleteFunc          func(c *gin.Context, id int64) error
	toggleStatusFunc    func(c *gin.Context, id int64, s mall.MallBlindBoxStatus) error

	createCalls, updateCalls, deleteCalls, toggleStatusCalls int
	lastCreated, lastUpdated                                 *mall.MallPromotion
	lastDelete                                               int64
	lastToggleID                                             int64
	lastToggleStatus                                         mall.MallBlindBoxStatus
}

func (m *promoPromotionRepo) GetByID(c *gin.Context, id int64) (*mall.MallPromotion, error) {
	if m.getByIDFunc != nil {
		return m.getByIDFunc(c, id)
	}
	return nil, gorm.ErrRecordNotFound
}
func (m *promoPromotionRepo) FindActiveByBlindBoxID(c *gin.Context, bbID int64, now time.Time) (*mall.MallPromotion, error) {
	return nil, nil
}
func (m *promoPromotionRepo) ListBySupplier(c *gin.Context, sid int64, opts repository.ListOptions) ([]mall.MallPromotion, int64, error) {
	if m.listBySupplierFunc != nil {
		return m.listBySupplierFunc(c, sid, opts)
	}
	return nil, 0, nil
}
func (m *promoPromotionRepo) ListTimeOverlap(c *gin.Context, bbID int64, s, e time.Time) ([]mall.MallPromotion, error) {
	if m.listTimeOverlapFunc != nil {
		return m.listTimeOverlapFunc(c, bbID, s, e)
	}
	return nil, nil
}
func (m *promoPromotionRepo) Create(c *gin.Context, e *mall.MallPromotion) error {
	m.createCalls++
	m.lastCreated = e
	e.ID = int64(m.createCalls) * 10
	if m.createFunc != nil {
		return m.createFunc(c, e)
	}
	return nil
}
func (m *promoPromotionRepo) Update(c *gin.Context, e *mall.MallPromotion) error {
	m.updateCalls++
	m.lastUpdated = e
	if m.updateFunc != nil {
		return m.updateFunc(c, e)
	}
	return nil
}
func (m *promoPromotionRepo) Delete(c *gin.Context, id int64) error {
	m.deleteCalls++
	m.lastDelete = id
	if m.deleteFunc != nil {
		return m.deleteFunc(c, id)
	}
	return nil
}
func (m *promoPromotionRepo) ToggleStatus(c *gin.Context, id int64, s mall.MallBlindBoxStatus) error {
	m.toggleStatusCalls++
	m.lastToggleID = id
	m.lastToggleStatus = s
	if m.toggleStatusFunc != nil {
		return m.toggleStatusFunc(c, id, s)
	}
	return nil
}
func (m *promoPromotionRepo) List(c *gin.Context, opts repository.ListOptions) ([]mall.MallPromotion, int64, error) {
	return nil, 0, nil
}

var _ mallRepo.PromotionRepository = (*promoPromotionRepo)(nil)

// =============================================================================
// Mock BlindBoxRepository（只需 GetByID；其他给 no-op 满足接口）
// =============================================================================

type promoBlindBoxRepo struct {
	getByIDFunc func(c *gin.Context, id int64) (*mall.MallBlindBox, error)
}

func (m *promoBlindBoxRepo) GetByID(c *gin.Context, id int64) (*mall.MallBlindBox, error) {
	if m.getByIDFunc != nil {
		return m.getByIDFunc(c, id)
	}
	return nil, gorm.ErrRecordNotFound
}
func (m *promoBlindBoxRepo) List(c *gin.Context, opts repository.ListOptions) ([]mall.MallBlindBox, int64, error) {
	return nil, 0, nil
}
func (m *promoBlindBoxRepo) ListFeaturedOnSale(c *gin.Context, opts repository.ListOptions) ([]mall.MallBlindBox, int64, error) {
	return nil, 0, nil
}
func (m *promoBlindBoxRepo) ListByIDs(c *gin.Context, ids []int64) ([]mall.MallBlindBox, error) {
	return nil, nil
}
func (m *promoBlindBoxRepo) ListBySupplier(c *gin.Context, sid int64, opts repository.ListOptions) ([]mall.MallBlindBox, int64, error) {
	return nil, 0, nil
}
func (m *promoBlindBoxRepo) ListActiveOnSaleBySupplierIDs(c *gin.Context, ids []int64, opts repository.ListOptions) ([]mall.MallBlindBox, int64, error) {
	return nil, 0, nil
}
func (m *promoBlindBoxRepo) ToggleStatus(c *gin.Context, id int64, s mall.MallBlindBoxStatus) error {
	return nil
}
func (m *promoBlindBoxRepo) ToggleOnSale(c *gin.Context, id int64, onSale bool) error { return nil }
func (m *promoBlindBoxRepo) ToggleFeatured(c *gin.Context, id int64, f bool) error    { return nil }
func (m *promoBlindBoxRepo) Create(c *gin.Context, e *mall.MallBlindBox) error        { return nil }
func (m *promoBlindBoxRepo) Update(c *gin.Context, e *mall.MallBlindBox) error        { return nil }
func (m *promoBlindBoxRepo) Delete(c *gin.Context, id int64) error                    { return nil }

var _ mallRepo.BlindBoxRepository = (*promoBlindBoxRepo)(nil)

// =============================================================================
// Mock cache
// =============================================================================

type promoMockCache struct {
	delCalls int
	lastKey  string
}

func (m *promoMockCache) Get(ctx context.Context, key string) (string, bool, error) { return "", false, nil }
func (m *promoMockCache) Set(ctx context.Context, key string, value string, ttl time.Duration) error {
	return nil
}
func (m *promoMockCache) Del(ctx context.Context, key string) error {
	m.delCalls++
	m.lastKey = key
	return nil
}
func (m *promoMockCache) SetNX(ctx context.Context, key string, value string, ttl time.Duration) (bool, error) {
	return true, nil
}
func (m *promoMockCache) Incr(ctx context.Context, key string) (int64, error) {
	return 0, nil
}
func (m *promoMockCache) Decr(ctx context.Context, key string) (int64, error) {
	return 0, nil
}

var _ cache.Cache = (*promoMockCache)(nil)

// =============================================================================
// 默认依赖 + 工厂
// =============================================================================

type promoDeps struct {
	promo *promoPromotionRepo
	bb    *promoBlindBoxRepo
	cache *promoMockCache
}

func defaultPromoDeps() promoDeps {
	return promoDeps{
		promo: &promoPromotionRepo{},
		bb: &promoBlindBoxRepo{
			getByIDFunc: func(c *gin.Context, id int64) (*mall.MallBlindBox, error) {
				return &mall.MallBlindBox{ID: id, SupplierID: 7}, nil
			},
		},
		cache: &promoMockCache{},
	}
}

func newPromoSvcFromDeps(d promoDeps) PromotionService {
	return NewPromotionService(PromotionServiceDeps{
		PromotionRepo: d.promo,
		BlindBoxRepo:  d.bb,
		Cache:         d.cache,
	})
}

func validPromo(bbID int64) *mall.MallPromotion {
	return &mall.MallPromotion{
		BlindBoxID:    bbID,
		OriginalPrice: 100,
		PromoPrice:    79,
		StartAt:       time.Now(),
		EndAt:         time.Now().Add(24 * time.Hour),
		Status:        mall.StatusActive,
	}
}

// =============================================================================
// List
// =============================================================================

func TestPromoList_InvalidSupplierID(t *testing.T) {
	d := defaultPromoDeps()
	svc := newPromoSvcFromDeps(d)
	items, total, err := svc.List(newPromotionCtx(), 0, repository.ListOptions{Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(items) != 0 || total != 0 {
		t.Errorf("got (%d, %d), want (0, 0)", len(items), total)
	}
}

func TestPromoList_DelegatesToRepo(t *testing.T) {
	d := defaultPromoDeps()
	d.promo.listBySupplierFunc = func(c *gin.Context, sid int64, opts repository.ListOptions) ([]mall.MallPromotion, int64, error) {
		return []mall.MallPromotion{{ID: 1, SupplierID: 7}}, 1, nil
	}
	svc := newPromoSvcFromDeps(d)
	items, total, err := svc.List(newPromotionCtx(), 7, repository.ListOptions{Page: 1, PageSize: 10})
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

func TestPromoCreate_InvalidSupplierID(t *testing.T) {
	d := defaultPromoDeps()
	svc := newPromoSvcFromDeps(d)
	_, err := svc.Create(newPromotionCtx(), 0, validPromo(1))
	if !errors.Is(err, ErrPromotionForbidden) {
		t.Errorf("err = %v, want ErrPromotionForbidden", err)
	}
}

func TestPromoCreate_NilPromo(t *testing.T) {
	d := defaultPromoDeps()
	svc := newPromoSvcFromDeps(d)
	_, err := svc.Create(newPromotionCtx(), 7, nil)
	if !errors.Is(err, ErrPromotionNotFound) {
		t.Errorf("err = %v, want ErrPromotionNotFound", err)
	}
}

func TestPromoCreate_InvalidTimeWindow(t *testing.T) {
	d := defaultPromoDeps()
	svc := newPromoSvcFromDeps(d)
	p := validPromo(1)
	p.StartAt = time.Now()
	p.EndAt = p.StartAt // 同时刻 → 不 After
	_, err := svc.Create(newPromotionCtx(), 7, p)
	if !errors.Is(err, ErrInvalidTimeWindow) {
		t.Errorf("err = %v, want ErrInvalidTimeWindow", err)
	}
}

func TestPromoCreate_InvalidPrice(t *testing.T) {
	d := defaultPromoDeps()
	svc := newPromoSvcFromDeps(d)
	p := validPromo(1)
	p.PromoPrice = 100 // == 原价，违反 PromoPrice < OriginalPrice
	_, err := svc.Create(newPromotionCtx(), 7, p)
	if !errors.Is(err, ErrInvalidPrice) {
		t.Errorf("err = %v, want ErrInvalidPrice", err)
	}
}

func TestPromoCreate_BlindBoxNotOwned(t *testing.T) {
	d := defaultPromoDeps()
	d.bb.getByIDFunc = func(c *gin.Context, id int64) (*mall.MallBlindBox, error) {
		// 盲盒属于 supplier 999，不是当前 supplier 7。
		return &mall.MallBlindBox{ID: id, SupplierID: 999}, nil
	}
	svc := newPromoSvcFromDeps(d)
	_, err := svc.Create(newPromotionCtx(), 7, validPromo(1))
	if !errors.Is(err, ErrPromotionForbidden) {
		t.Errorf("err = %v, want ErrPromotionForbidden", err)
	}
}

func TestPromoCreate_BlindBoxNotFound(t *testing.T) {
	d := defaultPromoDeps()
	d.bb.getByIDFunc = func(c *gin.Context, id int64) (*mall.MallBlindBox, error) {
		return nil, gorm.ErrRecordNotFound
	}
	svc := newPromoSvcFromDeps(d)
	_, err := svc.Create(newPromotionCtx(), 7, validPromo(1))
	if !errors.Is(err, ErrPromotionForbidden) {
		t.Errorf("err = %v, want ErrPromotionForbidden (复用: 不在我范围)", err)
	}
}

func TestPromoCreate_TimeOverlap(t *testing.T) {
	d := defaultPromoDeps()
	d.promo.listTimeOverlapFunc = func(c *gin.Context, bbID int64, s, e time.Time) ([]mall.MallPromotion, error) {
		return []mall.MallPromotion{{ID: 99, BlindBoxID: bbID}}, nil
	}
	svc := newPromoSvcFromDeps(d)
	_, err := svc.Create(newPromotionCtx(), 7, validPromo(1))
	if !errors.Is(err, ErrPromotionTimeOverlap) {
		t.Errorf("err = %v, want ErrPromotionTimeOverlap", err)
	}
	if d.promo.createCalls != 0 {
		t.Errorf("repo.Create called %d times, want 0 (overlap 时不入库)", d.promo.createCalls)
	}
}

func TestPromoCreate_Success(t *testing.T) {
	d := defaultPromoDeps()
	svc := newPromoSvcFromDeps(d)
	in := validPromo(1)
	in.SupplierID = 999 // 应被覆盖
	res, err := svc.Create(newPromotionCtx(), 7, in)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if res == nil {
		t.Fatalf("Create returned nil")
	}
	if res.SupplierID != 7 {
		t.Errorf("SupplierID = %d, want 7 (override)", res.SupplierID)
	}
	if res.ID == 0 {
		t.Errorf("ID = 0, want > 0")
	}
	if d.promo.createCalls != 1 {
		t.Errorf("repo.Create called %d times, want 1", d.promo.createCalls)
	}
}

// =============================================================================
// Update
// =============================================================================

func TestPromoUpdate_InvalidSupplierID(t *testing.T) {
	d := defaultPromoDeps()
	svc := newPromoSvcFromDeps(d)
	err := svc.Update(newPromotionCtx(), 0, 1, PromotionPatch{})
	if !errors.Is(err, ErrPromotionForbidden) {
		t.Errorf("err = %v, want ErrPromotionForbidden", err)
	}
}

// TestPromoUpdate_ZeroID 验证非法 ID 直接返回 ErrPromotionNotFound。
//
// patch 化之后「传 nil 实体」在类型上已不可表达（这本身就是收益），
// 因此这里只覆盖「ID <= 0」这一条路径。
func TestPromoUpdate_ZeroID(t *testing.T) {
	d := defaultPromoDeps()
	svc := newPromoSvcFromDeps(d)
	if err := svc.Update(newPromotionCtx(), 7, 0, PromotionPatch{}); !errors.Is(err, ErrPromotionNotFound) {
		t.Errorf("zero id err = %v, want ErrPromotionNotFound", err)
	}
}

func TestPromoUpdate_NotFound(t *testing.T) {
	d := defaultPromoDeps()
	d.promo.getByIDFunc = func(c *gin.Context, id int64) (*mall.MallPromotion, error) {
		return nil, gorm.ErrRecordNotFound
	}
	svc := newPromoSvcFromDeps(d)
	err := svc.Update(newPromotionCtx(), 7, 99, PromotionPatch{PromoPrice: ptrF64(50), OriginalPrice: ptrF64(100)})
	if !errors.Is(err, ErrPromotionNotFound) {
		t.Errorf("err = %v, want ErrPromotionNotFound", err)
	}
}

func TestPromoUpdate_Forbidden(t *testing.T) {
	d := defaultPromoDeps()
	d.promo.getByIDFunc = func(c *gin.Context, id int64) (*mall.MallPromotion, error) {
		return &mall.MallPromotion{ID: id, SupplierID: 999}, nil
	}
	svc := newPromoSvcFromDeps(d)
	err := svc.Update(newPromotionCtx(), 7, 1, PromotionPatch{PromoPrice: ptrF64(50), OriginalPrice: ptrF64(100)})
	if !errors.Is(err, ErrPromotionForbidden) {
		t.Errorf("err = %v, want ErrPromotionForbidden", err)
	}
	if d.promo.updateCalls != 0 {
		t.Errorf("repo.Update called %d times, want 0 (forbidden)", d.promo.updateCalls)
	}
}

func TestPromoUpdate_InvalidPrice(t *testing.T) {
	d := defaultPromoDeps()
	d.promo.getByIDFunc = func(c *gin.Context, id int64) (*mall.MallPromotion, error) {
		return &mall.MallPromotion{
			ID: id, SupplierID: 7, BlindBoxID: 1,
			OriginalPrice: 100, PromoPrice: 80,
			StartAt: time.Now(), EndAt: time.Now().Add(time.Hour),
		}, nil
	}
	svc := newPromoSvcFromDeps(d)
	err := svc.Update(newPromotionCtx(), 7, 1, PromotionPatch{PromoPrice: ptrF64(100), OriginalPrice: ptrF64(100)})
	if !errors.Is(err, ErrInvalidPrice) {
		t.Errorf("err = %v, want ErrInvalidPrice", err)
	}
}

func TestPromoUpdate_Success_PriceAndCacheInvalidation(t *testing.T) {
	d := defaultPromoDeps()
	d.promo.getByIDFunc = func(c *gin.Context, id int64) (*mall.MallPromotion, error) {
		return &mall.MallPromotion{
			ID: id, SupplierID: 7, BlindBoxID: 5,
			OriginalPrice: 100, PromoPrice: 80,
			StartAt: time.Now(), EndAt: time.Now().Add(time.Hour),
			Status: mall.StatusActive,
		}, nil
	}
	svc := newPromoSvcFromDeps(d)
	// 只传 PromoPrice：OriginalPrice / 归属 / 周期 / 状态结构上无法被传入，
	// 必须原样保留 existing 值（existing: 100 / 7 / 5 / active）。
	patch := PromotionPatch{PromoPrice: ptrF64(70)}
	if err := svc.Update(newPromotionCtx(), 7, 1, patch); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if d.promo.updateCalls != 1 {
		t.Errorf("repo.Update called %d times, want 1", d.promo.updateCalls)
	}
	upd := d.promo.lastUpdated
	if upd.ID != 1 {
		t.Errorf("ID = %d, want 1", upd.ID)
	}
	if upd.OriginalPrice != 100 {
		t.Errorf("OriginalPrice = %v, want 100 (preserve)", upd.OriginalPrice)
	}
	if upd.SupplierID != 7 {
		t.Errorf("SupplierID = %d, want 7 (preserve)", upd.SupplierID)
	}
	if upd.BlindBoxID != 5 {
		t.Errorf("BlindBoxID = %d, want 5 (preserve)", upd.BlindBoxID)
	}
	if upd.Status != mall.StatusActive {
		t.Errorf("Status = %q, want active (preserve)", upd.Status)
	}
	if upd.PromoPrice != 70 {
		t.Errorf("PromoPrice = %v, want 70 (update)", upd.PromoPrice)
	}
	// 缓存失效：BlindBoxID=5
	if d.cache.delCalls != 1 {
		t.Errorf("cache.Del called %d times, want 1", d.cache.delCalls)
	}
}

// =============================================================================
// Toggle
// =============================================================================

func TestPromoToggle_InvalidSupplierID(t *testing.T) {
	d := defaultPromoDeps()
	svc := newPromoSvcFromDeps(d)
	err := svc.Toggle(newPromotionCtx(), 0, 1)
	if !errors.Is(err, ErrPromotionForbidden) {
		t.Errorf("err = %v, want ErrPromotionForbidden", err)
	}
}

func TestPromoToggle_InvalidPromoID(t *testing.T) {
	d := defaultPromoDeps()
	svc := newPromoSvcFromDeps(d)
	err := svc.Toggle(newPromotionCtx(), 7, 0)
	if !errors.Is(err, ErrPromotionNotFound) {
		t.Errorf("err = %v, want ErrPromotionNotFound", err)
	}
}

func TestPromoToggle_NotFound(t *testing.T) {
	d := defaultPromoDeps()
	d.promo.getByIDFunc = func(c *gin.Context, id int64) (*mall.MallPromotion, error) {
		return nil, gorm.ErrRecordNotFound
	}
	svc := newPromoSvcFromDeps(d)
	err := svc.Toggle(newPromotionCtx(), 7, 99)
	if !errors.Is(err, ErrPromotionNotFound) {
		t.Errorf("err = %v, want ErrPromotionNotFound", err)
	}
}

func TestPromoToggle_Forbidden(t *testing.T) {
	d := defaultPromoDeps()
	d.promo.getByIDFunc = func(c *gin.Context, id int64) (*mall.MallPromotion, error) {
		return &mall.MallPromotion{ID: id, SupplierID: 999, BlindBoxID: 1}, nil
	}
	svc := newPromoSvcFromDeps(d)
	err := svc.Toggle(newPromotionCtx(), 7, 1)
	if !errors.Is(err, ErrPromotionForbidden) {
		t.Errorf("err = %v, want ErrPromotionForbidden", err)
	}
}

func TestPromoToggle_ActiveToDisabled(t *testing.T) {
	d := defaultPromoDeps()
	d.promo.getByIDFunc = func(c *gin.Context, id int64) (*mall.MallPromotion, error) {
		return &mall.MallPromotion{
			ID: id, SupplierID: 7, BlindBoxID: 5, Status: mall.StatusActive,
		}, nil
	}
	svc := newPromoSvcFromDeps(d)
	if err := svc.Toggle(newPromotionCtx(), 7, 1); err != nil {
		t.Fatalf("Toggle: %v", err)
	}
	if d.promo.toggleStatusCalls != 1 {
		t.Errorf("ToggleStatus called %d times, want 1", d.promo.toggleStatusCalls)
	}
	if d.promo.lastToggleStatus != mall.StatusDisabled {
		t.Errorf("newStatus = %q, want disabled (active→disabled)", d.promo.lastToggleStatus)
	}
	if d.cache.delCalls != 1 {
		t.Errorf("cache.Del called %d times, want 1", d.cache.delCalls)
	}
}

func TestPromoToggle_DisabledToActive(t *testing.T) {
	d := defaultPromoDeps()
	d.promo.getByIDFunc = func(c *gin.Context, id int64) (*mall.MallPromotion, error) {
		return &mall.MallPromotion{
			ID: id, SupplierID: 7, BlindBoxID: 5, Status: mall.StatusDisabled,
		}, nil
	}
	svc := newPromoSvcFromDeps(d)
	if err := svc.Toggle(newPromotionCtx(), 7, 1); err != nil {
		t.Fatalf("Toggle: %v", err)
	}
	if d.promo.lastToggleStatus != mall.StatusActive {
		t.Errorf("newStatus = %q, want active (disabled→active)", d.promo.lastToggleStatus)
	}
}

// =============================================================================
// Delete
// =============================================================================

func TestPromoDelete_InvalidSupplierID(t *testing.T) {
	d := defaultPromoDeps()
	svc := newPromoSvcFromDeps(d)
	err := svc.Delete(newPromotionCtx(), 0, 1)
	if !errors.Is(err, ErrPromotionForbidden) {
		t.Errorf("err = %v, want ErrPromotionForbidden", err)
	}
}

func TestPromoDelete_NotFound(t *testing.T) {
	d := defaultPromoDeps()
	d.promo.getByIDFunc = func(c *gin.Context, id int64) (*mall.MallPromotion, error) {
		return nil, gorm.ErrRecordNotFound
	}
	svc := newPromoSvcFromDeps(d)
	err := svc.Delete(newPromotionCtx(), 7, 99)
	if !errors.Is(err, ErrPromotionNotFound) {
		t.Errorf("err = %v, want ErrPromotionNotFound", err)
	}
}

func TestPromoDelete_Forbidden(t *testing.T) {
	d := defaultPromoDeps()
	d.promo.getByIDFunc = func(c *gin.Context, id int64) (*mall.MallPromotion, error) {
		return &mall.MallPromotion{ID: id, SupplierID: 999}, nil
	}
	svc := newPromoSvcFromDeps(d)
	err := svc.Delete(newPromotionCtx(), 7, 1)
	if !errors.Is(err, ErrPromotionForbidden) {
		t.Errorf("err = %v, want ErrPromotionForbidden", err)
	}
}

func TestPromoDelete_Success(t *testing.T) {
	d := defaultPromoDeps()
	d.promo.getByIDFunc = func(c *gin.Context, id int64) (*mall.MallPromotion, error) {
		return &mall.MallPromotion{
			ID: id, SupplierID: 7, BlindBoxID: 5,
		}, nil
	}
	svc := newPromoSvcFromDeps(d)
	if err := svc.Delete(newPromotionCtx(), 7, 1); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if d.promo.deleteCalls != 1 || d.promo.lastDelete != 1 {
		t.Errorf("Delete calls = (%d, id=%d), want (1, id=1)", d.promo.deleteCalls, d.promo.lastDelete)
	}
	if d.cache.delCalls != 1 {
		t.Errorf("cache.Del called %d times, want 1", d.cache.delCalls)
	}
}
