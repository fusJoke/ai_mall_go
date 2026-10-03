package user

import (
	"errors"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"ai-go-mall/internal/model"
	"ai-go-mall/internal/model/mall"
	"ai-go-mall/internal/repository"
	mallRepo "ai-go-mall/internal/repository/mall"
	supplierRepo "ai-go-mall/internal/repository/supplier"
	userRepo "ai-go-mall/internal/repository/user"
	"ai-go-mall/internal/service/user/draw_check"
)

// =============================================================================
// helpers
// =============================================================================

func newDrawCtx() *gin.Context {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/api/v1/user/draw", nil)
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

// =============================================================================
// Mocks（满足各 repo 接口）
// =============================================================================

type drawBlindBoxRepo struct {
	getByIDFunc  func(c *gin.Context, id int64) (*mall.MallBlindBox, error)
	getByIDCalls int
}

func (m *drawBlindBoxRepo) GetByID(c *gin.Context, id int64) (*mall.MallBlindBox, error) {
	m.getByIDCalls++
	if m.getByIDFunc != nil {
		return m.getByIDFunc(c, id)
	}
	return nil, gorm.ErrRecordNotFound
}
func (m *drawBlindBoxRepo) ListByIDs(c *gin.Context, ids []int64) ([]mall.MallBlindBox, error) {
	return nil, nil
}
func (m *drawBlindBoxRepo) ListBySupplier(c *gin.Context, sid int64, opts repository.ListOptions) ([]mall.MallBlindBox, int64, error) {
	return nil, 0, nil
}
func (m *drawBlindBoxRepo) ListFeaturedOnSale(c *gin.Context, opts repository.ListOptions) ([]mall.MallBlindBox, int64, error) {
	return nil, 0, nil
}
func (m *drawBlindBoxRepo) ListActiveOnSaleBySupplierIDs(c *gin.Context, ids []int64, opts repository.ListOptions) ([]mall.MallBlindBox, int64, error) {
	return nil, 0, nil
}
func (m *drawBlindBoxRepo) ToggleStatus(c *gin.Context, id int64, s mall.MallBlindBoxStatus) error {
	return nil
}
func (m *drawBlindBoxRepo) ToggleOnSale(c *gin.Context, id int64, onSale bool) error { return nil }
func (m *drawBlindBoxRepo) ToggleFeatured(c *gin.Context, id int64, f bool) error    { return nil }
func (m *drawBlindBoxRepo) Create(c *gin.Context, e *mall.MallBlindBox) error        { return nil }
func (m *drawBlindBoxRepo) List(c *gin.Context, opts repository.ListOptions) ([]mall.MallBlindBox, int64, error) {
	return nil, 0, nil
}
func (m *drawBlindBoxRepo) Update(c *gin.Context, e *mall.MallBlindBox) error { return nil }
func (m *drawBlindBoxRepo) Delete(c *gin.Context, id int64) error             { return nil }

var _ mallRepo.BlindBoxRepository = (*drawBlindBoxRepo)(nil)

type drawCardPoolRepo struct {
	getPoolByBlindBoxIDFunc func(c *gin.Context, blindBoxID int64) (*mall.MallCardPool, error)
	listItemsByPoolIDFunc   func(c *gin.Context, poolID int64) ([]mall.MallCardPoolItem, error)
	listItemsForUpdateFunc  func(c *gin.Context, poolID int64) ([]mall.MallCardPoolItem, error)
	decrementStockFunc      func(c *gin.Context, itemID int64) error

	getPoolCalls    int
	listItemsCalls  int
	decrementCalls  int
	lastDecrementID int64
	// decrementSucceedsOn 控制特定 item id 的剩余成功次数；<=0 时返回 ErrStockEmpty。
	decrementSucceedsOn map[int64]int
	// decrementFailFirst 控制"先失败 N 次再成功"的策略，模拟"极端并发下别人抽走"。
	// 优先级高于 decrementSucceedsOn。
	decrementFailFirst map[int64]int
}

func (m *drawCardPoolRepo) GetPoolByBlindBoxID(c *gin.Context, blindBoxID int64) (*mall.MallCardPool, error) {
	m.getPoolCalls++
	if m.getPoolByBlindBoxIDFunc != nil {
		return m.getPoolByBlindBoxIDFunc(c, blindBoxID)
	}
	return nil, gorm.ErrRecordNotFound
}
func (m *drawCardPoolRepo) ListItemsByPoolID(c *gin.Context, poolID int64) ([]mall.MallCardPoolItem, error) {
	m.listItemsCalls++
	if m.listItemsByPoolIDFunc != nil {
		return m.listItemsByPoolIDFunc(c, poolID)
	}
	return nil, nil
}
func (m *drawCardPoolRepo) ListItemsForUpdateByPoolID(c *gin.Context, poolID int64) ([]mall.MallCardPoolItem, error) {
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
func (m *drawCardPoolRepo) ListItemsByPoolIDs(c *gin.Context, poolIDs []int64) ([]mall.MallCardPoolItem, error) {
	return nil, nil
}
func (m *drawCardPoolRepo) CreatePool(c *gin.Context, pool *mall.MallCardPool) error { return nil }
func (m *drawCardPoolRepo) CreateItem(c *gin.Context, item *mall.MallCardPoolItem) error {
	return nil
}
func (m *drawCardPoolRepo) CreateItems(c *gin.Context, items []mall.MallCardPoolItem) error {
	return nil
}
func (m *drawCardPoolRepo) DecrementStock(c *gin.Context, itemID int64) error {
	m.decrementCalls++
	m.lastDecrementID = itemID
	if m.decrementFailFirst != nil {
		left := m.decrementFailFirst[itemID]
		if left > 0 {
			m.decrementFailFirst[itemID] = left - 1
			return mallRepo.ErrStockEmpty
		}
	}
	if m.decrementSucceedsOn != nil {
		left := m.decrementSucceedsOn[itemID]
		if left <= 0 {
			return mallRepo.ErrStockEmpty
		}
		m.decrementSucceedsOn[itemID] = left - 1
	}
	if m.decrementStockFunc != nil {
		return m.decrementStockFunc(c, itemID)
	}
	return nil
}
func (m *drawCardPoolRepo) IncrementStock(c *gin.Context, itemID int64, delta int) error { return nil }
func (m *drawCardPoolRepo) SumWeightByPoolID(c *gin.Context, poolID int64) (int64, error) {
	return 0, nil
}
func (m *drawCardPoolRepo) SubtotalStockByPoolID(c *gin.Context, poolID int64) (int64, error) {
	return 0, nil
}

var _ mallRepo.CardPoolRepository = (*drawCardPoolRepo)(nil)

type drawPromoRepo struct {
	findActiveFunc  func(c *gin.Context, blindBoxID int64, now time.Time) (*mall.MallPromotion, error)
	findActiveCalls int
}

func (m *drawPromoRepo) FindActiveByBlindBoxID(c *gin.Context, blindBoxID int64, now time.Time) (*mall.MallPromotion, error) {
	m.findActiveCalls++
	if m.findActiveFunc != nil {
		return m.findActiveFunc(c, blindBoxID, now)
	}
	return nil, nil
}
func (m *drawPromoRepo) GetByID(c *gin.Context, id int64) (*mall.MallPromotion, error) {
	return nil, gorm.ErrRecordNotFound
}
func (m *drawPromoRepo) ListBySupplier(c *gin.Context, sid int64, opts repository.ListOptions) ([]mall.MallPromotion, int64, error) {
	return nil, 0, nil
}
func (m *drawPromoRepo) ListTimeOverlap(c *gin.Context, bbID int64, s, e time.Time) ([]mall.MallPromotion, error) {
	return nil, nil
}
func (m *drawPromoRepo) ToggleStatus(c *gin.Context, id int64, s mall.MallBlindBoxStatus) error {
	return nil
}
func (m *drawPromoRepo) Create(c *gin.Context, e *mall.MallPromotion) error { return nil }
func (m *drawPromoRepo) List(c *gin.Context, opts repository.ListOptions) ([]mall.MallPromotion, int64, error) {
	return nil, 0, nil
}
func (m *drawPromoRepo) Update(c *gin.Context, e *mall.MallPromotion) error { return nil }
func (m *drawPromoRepo) Delete(c *gin.Context, id int64) error              { return nil }

var _ mallRepo.PromotionRepository = (*drawPromoRepo)(nil)

type drawSupplierRepo struct {
	getByIDFunc               func(c *gin.Context, id int64) (*mall.MallSupplier, error)
	updateBalanceAndSalesFunc func(c *gin.Context, id int64, amount float64) error
	listByIDsFunc             func(c *gin.Context, ids []int64) ([]mall.MallSupplier, error)

	getByIDCalls       int
	updateBalanceCalls int
	lastUpdateAmount   float64
	lastUpdateID       int64
	listByIDsCalls     int
}

func (m *drawSupplierRepo) GetByID(c *gin.Context, id int64) (*mall.MallSupplier, error) {
	m.getByIDCalls++
	if m.getByIDFunc != nil {
		return m.getByIDFunc(c, id)
	}
	return nil, gorm.ErrRecordNotFound
}
func (m *drawSupplierRepo) ListByIDs(c *gin.Context, ids []int64) ([]mall.MallSupplier, error) {
	m.listByIDsCalls++
	if m.listByIDsFunc != nil {
		return m.listByIDsFunc(c, ids)
	}
	return nil, nil
}
func (m *drawSupplierRepo) ListWithFilter(c *gin.Context, opts supplierRepo.SupplierListOptions) ([]mall.MallSupplier, int64, error) {
	return nil, 0, nil
}
func (m *drawSupplierRepo) ToggleStatus(c *gin.Context, id int64, s mall.MallBlindBoxStatus) error {
	return nil
}
func (m *drawSupplierRepo) ToggleFeatured(c *gin.Context, id int64, f bool) error { return nil }
func (m *drawSupplierRepo) UpdateBalanceAndSales(c *gin.Context, id int64, amount float64) error {
	m.updateBalanceCalls++
	m.lastUpdateAmount = amount
	m.lastUpdateID = id
	if m.updateBalanceAndSalesFunc != nil {
		return m.updateBalanceAndSalesFunc(c, id, amount)
	}
	return nil
}
func (m *drawSupplierRepo) Create(c *gin.Context, e *mall.MallSupplier) error { return nil }
func (m *drawSupplierRepo) List(c *gin.Context, opts repository.ListOptions) ([]mall.MallSupplier, int64, error) {
	return nil, 0, nil
}
func (m *drawSupplierRepo) Update(c *gin.Context, e *mall.MallSupplier) error { return nil }
func (m *drawSupplierRepo) Delete(c *gin.Context, id int64) error             { return nil }

var _ supplierRepo.SupplierRepository = (*drawSupplierRepo)(nil)

type drawUserRepo struct {
	getByIDFunc          func(c *gin.Context, id int64) (*model.User, error)
	decrementBalanceFunc func(c *gin.Context, userID int64, amount float64) error
	incrementBalanceFunc func(c *gin.Context, userID int64, amount float64) error

	getByIDCalls      int
	decrementCalls    int
	lastDecrementUser int64
	lastDecrementAmt  float64
}

func (m *drawUserRepo) GetByUsername(c *gin.Context, username string) (*model.User, error) {
	return nil, gorm.ErrRecordNotFound
}
func (m *drawUserRepo) GetByID(c *gin.Context, id int64) (*model.User, error) {
	m.getByIDCalls++
	if m.getByIDFunc != nil {
		return m.getByIDFunc(c, id)
	}
	return nil, gorm.ErrRecordNotFound
}
func (m *drawUserRepo) DecrementBalance(c *gin.Context, userID int64, amount float64) error {
	m.decrementCalls++
	m.lastDecrementUser = userID
	m.lastDecrementAmt = amount
	if m.decrementBalanceFunc != nil {
		return m.decrementBalanceFunc(c, userID, amount)
	}
	return nil
}
func (m *drawUserRepo) IncrementBalance(c *gin.Context, userID int64, amount float64) error {
	return nil
}
func (m *drawUserRepo) UpdateLoginSuccess(c *gin.Context, id int64, ip string, at time.Time) error {
	return nil
}
func (m *drawUserRepo) IncrementLoginFailure(c *gin.Context, id int64) (int, error) {
	return 0, nil
}
func (m *drawUserRepo) UpdateStatus(c *gin.Context, id int64, status int8) error { return nil }
func (m *drawUserRepo) Create(c *gin.Context, e *model.User) error               { return nil }
func (m *drawUserRepo) List(c *gin.Context, opts repository.ListOptions) ([]model.User, int64, error) {
	return nil, 0, nil
}
func (m *drawUserRepo) Update(c *gin.Context, e *model.User) error { return nil }
func (m *drawUserRepo) Delete(c *gin.Context, id int64) error      { return nil }

var _ userRepo.UserRepository = (*drawUserRepo)(nil)

type drawOrderRepo struct {
	createOrderFunc  func(c *gin.Context, order *mall.MallDrawOrder) error
	createItemsFunc  func(c *gin.Context, items []mall.MallDrawOrderItem) error
	getByOrderNoFunc func(c *gin.Context, no string) (*mall.MallDrawOrder, error)
	updateStatusFunc func(c *gin.Context, id int64, s mall.MallDrawOrderStatus) error

	createOrderCalls  int
	createItemsCalls  int
	updateStatusCalls int
	lastOrder         *mall.MallDrawOrder
	lastItems         []mall.MallDrawOrderItem
	statusUpdates     []mall.MallDrawOrderStatus
	orderIDCounter    int64
}

func (m *drawOrderRepo) GetByID(c *gin.Context, id int64) (*mall.MallDrawOrder, error) {
	return nil, gorm.ErrRecordNotFound
}
func (m *drawOrderRepo) GetByOrderNo(c *gin.Context, no string) (*mall.MallDrawOrder, error) {
	if m.getByOrderNoFunc != nil {
		return m.getByOrderNoFunc(c, no)
	}
	return nil, gorm.ErrRecordNotFound
}
func (m *drawOrderRepo) CreateOrder(c *gin.Context, order *mall.MallDrawOrder) error {
	m.createOrderCalls++
	m.orderIDCounter++
	order.ID = m.orderIDCounter
	m.lastOrder = order
	if m.createOrderFunc != nil {
		return m.createOrderFunc(c, order)
	}
	return nil
}
func (m *drawOrderRepo) ListByUser(c *gin.Context, uid int64, opts repository.ListOptions) ([]mall.MallDrawOrder, int64, error) {
	return nil, 0, nil
}
func (m *drawOrderRepo) UpdateStatus(c *gin.Context, id int64, s mall.MallDrawOrderStatus) error {
	m.updateStatusCalls++
	m.statusUpdates = append(m.statusUpdates, s)
	if m.updateStatusFunc != nil {
		return m.updateStatusFunc(c, id, s)
	}
	return nil
}
func (m *drawOrderRepo) ListItemsByOrderID(c *gin.Context, id int64) ([]mall.MallDrawOrderItem, error) {
	return nil, nil
}
func (m *drawOrderRepo) CreateItems(c *gin.Context, items []mall.MallDrawOrderItem) error {
	m.createItemsCalls++
	m.lastItems = items
	if m.createItemsFunc != nil {
		return m.createItemsFunc(c, items)
	}
	return nil
}
func (m *drawOrderRepo) ListSettledOrdersBySupplierInRange(c *gin.Context, sid int64, s, e time.Time) ([]mall.MallDrawOrder, error) {
	return nil, nil
}
func (m *drawOrderRepo) Create(c *gin.Context, e *mall.MallDrawOrder) error { return nil }
func (m *drawOrderRepo) List(c *gin.Context, opts repository.ListOptions) ([]mall.MallDrawOrder, int64, error) {
	return nil, 0, nil
}
func (m *drawOrderRepo) Update(c *gin.Context, e *mall.MallDrawOrder) error { return nil }
func (m *drawOrderRepo) Delete(c *gin.Context, id int64) error              { return nil }

var _ mallRepo.DrawOrderRepository = (*drawOrderRepo)(nil)

type drawCardRepo struct {
	getByIDFunc  func(c *gin.Context, id int64) (*mall.MallCard, error)
	getByIDCalls int
}

func (m *drawCardRepo) GetByID(c *gin.Context, id int64) (*mall.MallCard, error) {
	m.getByIDCalls++
	if m.getByIDFunc != nil {
		return m.getByIDFunc(c, id)
	}
	return nil, gorm.ErrRecordNotFound
}
func (m *drawCardRepo) ListByIDs(c *gin.Context, ids []int64) ([]mall.MallCard, error) {
	return nil, nil
}
func (m *drawCardRepo) ListByTeam(c *gin.Context, team string, limit int) ([]mall.MallCard, error) {
	return nil, nil
}
func (m *drawCardRepo) Create(c *gin.Context, e *mall.MallCard) error { return nil }
func (m *drawCardRepo) List(c *gin.Context, opts repository.ListOptions) ([]mall.MallCard, int64, error) {
	return nil, 0, nil
}
func (m *drawCardRepo) Update(c *gin.Context, e *mall.MallCard) error { return nil }
func (m *drawCardRepo) Delete(c *gin.Context, id int64) error         { return nil }

var _ mallRepo.CardRepository = (*drawCardRepo)(nil)

// drawSettlementRepo 是 SettlementRepository 的最小 mock：仅实现 IncrementLedger。
//
// 抽卡事务只用到 IncrementLedger（spec 8.5 步骤 7.6 累加 platform ledger）；
// 其余方法在画单流程中不会被触发，stub return 即可。
type drawSettlementRepo struct {
	incrementLedgerFunc func(c *gin.Context, counterKey string, delta float64) error

	incrementCalls int
	lastCounterKey string
	lastDelta      float64
}

func (m *drawSettlementRepo) IncrementLedger(c *gin.Context, counterKey string, delta float64) error {
	m.incrementCalls++
	m.lastCounterKey = counterKey
	m.lastDelta = delta
	if m.incrementLedgerFunc != nil {
		return m.incrementLedgerFunc(c, counterKey, delta)
	}
	return nil
}

// UpdateStatus stub（status machine 在抽卡事务内调用）。
func (m *drawSettlementRepo) UpdateStatus(c *gin.Context, id int64, status mall.MallSettlementStatus) error {
	return nil
}

// 其余 SettlementRepository 方法抽卡事务不会调用，stub return。
func (m *drawSettlementRepo) GetByID(c *gin.Context, id int64) (*mall.MallSettlement, error) {
	return nil, gorm.ErrRecordNotFound
}
func (m *drawSettlementRepo) GetByIDWithItems(c *gin.Context, id int64) (*mall.MallSettlement, []mall.MallSettlementItem, error) {
	return nil, nil, nil
}
func (m *drawSettlementRepo) ListBySupplier(c *gin.Context, sid int64, opts repository.ListOptions) ([]mall.MallSettlement, int64, error) {
	return nil, 0, nil
}
func (m *drawSettlementRepo) ListItemsBySettlementID(c *gin.Context, id int64) ([]mall.MallSettlementItem, error) {
	return nil, nil
}
func (m *drawSettlementRepo) ListEligibleOrders(c *gin.Context, sid int64, s, e time.Time) ([]mall.MallDrawOrder, error) {
	return nil, nil
}
func (m *drawSettlementRepo) Generate(c *gin.Context, s *mall.MallSettlement, items []mall.MallSettlementItem) error {
	return nil
}
func (m *drawSettlementRepo) MarkPaid(c *gin.Context, id int64, adminID int64, payout float64) error {
	return nil
}
func (m *drawSettlementRepo) Create(c *gin.Context, e *mall.MallSettlement) error { return nil }
func (m *drawSettlementRepo) List(c *gin.Context, opts repository.ListOptions) ([]mall.MallSettlement, int64, error) {
	return nil, 0, nil
}
func (m *drawSettlementRepo) Update(c *gin.Context, e *mall.MallSettlement) error { return nil }
func (m *drawSettlementRepo) Delete(c *gin.Context, id int64) error               { return nil }

var _ mallRepo.SettlementRepository = (*drawSettlementRepo)(nil)

// =============================================================================
// 默认依赖（happy path）
// =============================================================================

type drawDeps struct {
	bb         *drawBlindBoxRepo
	cp         *drawCardPoolRepo
	promo      *drawPromoRepo
	sup        *drawSupplierRepo
	user       *drawUserRepo
	order      *drawOrderRepo
	card       *drawCardRepo
	settlement *drawSettlementRepo
	txr        txRunner
}

func defaultDeps() drawDeps {
	bb := &drawBlindBoxRepo{
		getByIDFunc: func(c *gin.Context, id int64) (*mall.MallBlindBox, error) {
			return &mall.MallBlindBox{
				ID: id, Name: "Box", SupplierID: 7,
				Status: mall.StatusActive, OnSale: true,
				Price: 100,
			}, nil
		},
	}
	cp := &drawCardPoolRepo{
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
	promo := &drawPromoRepo{
		findActiveFunc: func(c *gin.Context, bbID int64, now time.Time) (*mall.MallPromotion, error) {
			return nil, nil
		},
	}
	sup := &drawSupplierRepo{
		getByIDFunc: func(c *gin.Context, id int64) (*mall.MallSupplier, error) {
			return &mall.MallSupplier{ID: id, Status: mall.StatusActive}, nil
		},
	}
	user := &drawUserRepo{
		getByIDFunc: func(c *gin.Context, id int64) (*model.User, error) {
			return &model.User{ID: id, Username: "u", Balance: 1000, Status: 1}, nil
		},
	}
	order := &drawOrderRepo{}
	card := &drawCardRepo{
		getByIDFunc: func(c *gin.Context, id int64) (*mall.MallCard, error) {
			return &mall.MallCard{ID: id, Name: "CardName", Image: "card.png"}, nil
		},
	}
	settlement := &drawSettlementRepo{}
	return drawDeps{
		bb: bb, cp: cp, promo: promo, sup: sup, user: user, order: order, card: card, settlement: settlement,
		txr: &mockTxRunner{},
	}
}

// newSvcFromDeps 装配 DrawService，连同责任链一起注入。
//
// task 15：原内联 check（user_active + blindbox_buyable + time_window + balance）
// 已被责任链替代。测试场景下，复用 defaultDeps 的 mocks 装配 chain——
// 各 check 节点访问的 repo 与原内联代码访问的 repo 完全一致，行为保持等价。
func newSvcFromDeps(d drawDeps) DrawService {
	checkChain := draw_check.NewDrawCheckChain(draw_check.CheckDeps{
		Users:     d.user,
		BlindBox:  d.bb,
		Promotion: d.promo,
		// Seckill：time_window 在 normal 链路 fast return，balance_check 的
		// seckill 分支也不会触发（Source=normal）。注入 nil 在 NewDrawCheckChain
		// 不会被访问，但 NewTimeWindowCheck 会断言；显式塞一个空 stub 防止 panic。
		Seckill:  nilSafeSeckillRepo{},
		Supplier: d.sup,
	})
	return NewDrawService(DrawServiceDeps{
		BlindBoxRepo:   d.bb,
		CardPoolRepo:   d.cp,
		PromotionRepo:  d.promo,
		SupplierRepo:   d.sup,
		UserRepo:       d.user,
		DrawOrderRepo:  d.order,
		CardRepo:       d.card,
		SettlementRepo: d.settlement,
		CheckChain:     checkChain,
		TxRunner:       d.txr,
	})
}

// nilSafeSeckillRepo 是 SeckillRepository 的零开销 stub：
// 仅当 time_window / balance 节点真的访问它时才 panic（正常测试不会触发）。
//
// 设计：time_window 在 Source=normal 时直接 return nil 不查 seckill；
// balance_check 的 seckill 分支仅在 Source=seckill 时执行——Draw 链路用不到。
type nilSafeSeckillRepo struct{}

func (nilSafeSeckillRepo) GetByID(c *gin.Context, id int64) (*mall.MallSeckillActivity, error) {
	return nil, gorm.ErrRecordNotFound
}
func (nilSafeSeckillRepo) Create(c *gin.Context, s *mall.MallSeckillActivity) (int64, error) {
	return 0, nil
}
func (nilSafeSeckillRepo) Update(c *gin.Context, s *mall.MallSeckillActivity) error { return nil }
func (nilSafeSeckillRepo) MarkRedisInitialized(c *gin.Context, id int64) error      { return nil }
func (nilSafeSeckillRepo) ListActive(c *gin.Context, now time.Time, opts repository.ListOptions) ([]mall.MallSeckillActivity, int64, error) {
	return nil, 0, nil
}
func (nilSafeSeckillRepo) ListBySupplier(c *gin.Context, sid int64, opts repository.ListOptions) ([]mall.MallSeckillActivity, int64, error) {
	return nil, 0, nil
}
func (nilSafeSeckillRepo) List(c *gin.Context, opts repository.ListOptions) ([]mall.MallSeckillActivity, int64, error) {
	return nil, 0, nil
}
func (nilSafeSeckillRepo) ToggleStatus(c *gin.Context, id int64, status mall.MallBlindBoxStatus) error {
	return nil
}
func (nilSafeSeckillRepo) SumPoolStock(c *gin.Context, bbID int64) (int, error) { return 0, nil }
func (nilSafeSeckillRepo) InsertDeductionLog(c *gin.Context, log *mall.MallStockDeductionLog) error {
	return nil
}
func (nilSafeSeckillRepo) CountDeductionLogsBySeckillIDs(c *gin.Context, ids []int64) (map[int64]int64, error) {
	return nil, nil
}
func (nilSafeSeckillRepo) MarkDeductionLogSynced(c *gin.Context, id int64, at time.Time) (int64, error) {
	return 0, nil
}
func (nilSafeSeckillRepo) ListUnsyncedLogs(c *gin.Context, limit int) ([]mall.MallStockDeductionLog, error) {
	return nil, nil
}

var _ mallRepo.SeckillRepository = nilSafeSeckillRepo{}

// =============================================================================
// 测试用例
// =============================================================================

func TestDraw_InvalidArgs(t *testing.T) {
	svc := newSvcFromDeps(defaultDeps())
	_, err := svc.Draw(newDrawCtx(), 0, 1)
	if !errors.Is(err, ErrBlindBoxNotAvailable) {
		t.Errorf("userID=0 err = %v, want ErrBlindBoxNotAvailable", err)
	}
	_, err = svc.Draw(newDrawCtx(), 1, 0)
	if !errors.Is(err, ErrBlindBoxNotAvailable) {
		t.Errorf("blindBoxID=0 err = %v, want ErrBlindBoxNotAvailable", err)
	}
}

func TestDraw_BlindBoxNotFound(t *testing.T) {
	d := defaultDeps()
	d.bb.getByIDFunc = func(c *gin.Context, id int64) (*mall.MallBlindBox, error) {
		return nil, gorm.ErrRecordNotFound
	}
	svc := newSvcFromDeps(d)
	_, err := svc.Draw(newDrawCtx(), 1, 1)
	if !errors.Is(err, ErrBlindBoxNotAvailable) {
		t.Errorf("err = %v, want ErrBlindBoxNotAvailable", err)
	}
}

func TestDraw_BlindBoxDisabled(t *testing.T) {
	d := defaultDeps()
	d.bb.getByIDFunc = func(c *gin.Context, id int64) (*mall.MallBlindBox, error) {
		return &mall.MallBlindBox{ID: id, Status: mall.StatusDisabled, OnSale: true, Price: 100}, nil
	}
	svc := newSvcFromDeps(d)
	_, err := svc.Draw(newDrawCtx(), 1, 1)
	if !errors.Is(err, ErrBlindBoxNotAvailable) {
		t.Errorf("err = %v, want ErrBlindBoxNotAvailable", err)
	}
}

func TestDraw_BlindBoxNotForSale(t *testing.T) {
	d := defaultDeps()
	d.bb.getByIDFunc = func(c *gin.Context, id int64) (*mall.MallBlindBox, error) {
		return &mall.MallBlindBox{ID: id, Status: mall.StatusActive, OnSale: false, Price: 100}, nil
	}
	svc := newSvcFromDeps(d)
	_, err := svc.Draw(newDrawCtx(), 1, 1)
	if !errors.Is(err, ErrBlindBoxNotAvailable) {
		t.Errorf("err = %v, want ErrBlindBoxNotAvailable", err)
	}
}

func TestDraw_SupplierDisabled(t *testing.T) {
	d := defaultDeps()
	d.sup.getByIDFunc = func(c *gin.Context, id int64) (*mall.MallSupplier, error) {
		return &mall.MallSupplier{ID: id, Status: mall.StatusDisabled}, nil
	}
	svc := newSvcFromDeps(d)
	_, err := svc.Draw(newDrawCtx(), 1, 1)
	if !errors.Is(err, ErrBlindBoxNotAvailable) {
		t.Errorf("err = %v, want ErrBlindBoxNotAvailable", err)
	}
}

func TestDraw_UserNotFound(t *testing.T) {
	d := defaultDeps()
	d.user.getByIDFunc = func(c *gin.Context, id int64) (*model.User, error) {
		return nil, gorm.ErrRecordNotFound
	}
	svc := newSvcFromDeps(d)
	_, err := svc.Draw(newDrawCtx(), 1, 1)
	if !errors.Is(err, ErrUserNotAvailable) {
		t.Errorf("err = %v, want ErrUserNotAvailable", err)
	}
}

func TestDraw_UserDisabled(t *testing.T) {
	d := defaultDeps()
	d.user.getByIDFunc = func(c *gin.Context, id int64) (*model.User, error) {
		return &model.User{ID: id, Balance: 1000, Status: 0}, nil
	}
	svc := newSvcFromDeps(d)
	_, err := svc.Draw(newDrawCtx(), 1, 1)
	if !errors.Is(err, ErrUserNotAvailable) {
		t.Errorf("err = %v, want ErrUserNotAvailable", err)
	}
}

func TestDraw_PoolNotFound(t *testing.T) {
	d := defaultDeps()
	d.cp.getPoolByBlindBoxIDFunc = func(c *gin.Context, blindBoxID int64) (*mall.MallCardPool, error) {
		return nil, gorm.ErrRecordNotFound
	}
	svc := newSvcFromDeps(d)
	_, err := svc.Draw(newDrawCtx(), 1, 1)
	if !errors.Is(err, ErrPoolNotFound) {
		t.Errorf("err = %v, want ErrPoolNotFound", err)
	}
}

func TestDraw_EmptyPool(t *testing.T) {
	d := defaultDeps()
	d.cp.listItemsByPoolIDFunc = func(c *gin.Context, poolID int64) ([]mall.MallCardPoolItem, error) {
		return []mall.MallCardPoolItem{
			{ID: 1, PoolID: poolID, Weight: 100, Stock: 0},
		}, nil
	}
	svc := newSvcFromDeps(d)
	_, err := svc.Draw(newDrawCtx(), 1, 1)
	if !errors.Is(err, ErrSoldOut) {
		t.Errorf("err = %v, want ErrSoldOut", err)
	}
}

func TestDraw_InsufficientBalance(t *testing.T) {
	d := defaultDeps()
	d.user.decrementBalanceFunc = func(c *gin.Context, userID int64, amount float64) error {
		return userRepo.ErrInsufficientBalance
	}
	svc := newSvcFromDeps(d)
	_, err := svc.Draw(newDrawCtx(), 1, 1)
	if !errors.Is(err, ErrInsufficientBalance) {
		t.Errorf("err = %v, want ErrInsufficientBalance", err)
	}
}

func TestDraw_Success_NoPromo(t *testing.T) {
	d := defaultDeps()
	svc := newSvcFromDeps(d)
	res, err := svc.Draw(newDrawCtx(), 1, 1)
	if err != nil {
		t.Fatalf("Draw: %v", err)
	}
	if res == nil {
		t.Fatalf("Draw returned nil result")
	}
	if res.OrderID == 0 {
		t.Errorf("OrderID = 0, want > 0")
	}
	if res.OrderNo == "" {
		t.Errorf("OrderNo = \"\", want non-empty")
	}
	if res.ActualPrice != 100 {
		t.Errorf("ActualPrice = %v, want 100", res.ActualPrice)
	}
	if len(res.Cards) != 1 {
		t.Fatalf("len(Cards) = %d, want 1", len(res.Cards))
	}
	if res.Cards[0].SnapshotName != "CardName" {
		t.Errorf("SnapshotName = %q, want CardName", res.Cards[0].SnapshotName)
	}
	if res.Cards[0].Rarity != mall.RaritySSR && res.Cards[0].Rarity != mall.RarityN {
		t.Errorf("Rarity = %q, want SSR or N", res.Cards[0].Rarity)
	}

	// 副作用
	if d.sup.updateBalanceCalls != 1 {
		t.Errorf("sup UpdateBalanceAndSales called %d times, want 1", d.sup.updateBalanceCalls)
	}
	if d.sup.lastUpdateAmount != 100 {
		t.Errorf("sup update amount = %v, want 100", d.sup.lastUpdateAmount)
	}
	if d.sup.lastUpdateID != 7 {
		t.Errorf("sup update id = %d, want 7", d.sup.lastUpdateID)
	}
	if d.user.decrementCalls != 1 {
		t.Errorf("user.DecrementBalance called %d times, want 1", d.user.decrementCalls)
	}
	if d.user.lastDecrementAmt != 100 {
		t.Errorf("user.DecrementBalance amount = %v, want 100", d.user.lastDecrementAmt)
	}
	if d.order.createOrderCalls != 1 {
		t.Errorf("order.CreateOrder called %d times, want 1", d.order.createOrderCalls)
	}
	if d.order.createItemsCalls != 1 {
		t.Errorf("order.CreateItems called %d times, want 1", d.order.createItemsCalls)
	}
	if len(d.order.lastItems) != 1 {
		t.Errorf("len(order items) = %d, want 1", len(d.order.lastItems))
	}
	if d.cp.decrementCalls != 1 {
		t.Errorf("DecrementStock called %d times, want 1", d.cp.decrementCalls)
	}
	// spec 8.5 步骤 7.6：抽卡事务内累加 platform ledger total_revenue。
	if d.settlement.incrementCalls != 1 {
		t.Errorf("Settlement.IncrementLedger called %d times, want 1", d.settlement.incrementCalls)
	}
	if d.settlement.lastCounterKey != "total_revenue" {
		t.Errorf("counter_key = %q, want total_revenue", d.settlement.lastCounterKey)
	}
	if d.settlement.lastDelta != 100 {
		t.Errorf("ledger delta = %v, want 100", d.settlement.lastDelta)
	}
}

// TestDraw_StateMachineFlow spec 14.5：
//
//	抽卡事务内订单应经历 pending → paid → drawn 三段（外部观察最终态 = drawn，
//	前两段是事务内的中间态，主要价值是 catch 编程错误）。
//
// 断言：
//   - CreateOrder 时传入 status=pending（而非直接 drawn）；
//   - UpdateStatus 依次被调 2 次：先 paid，再 drawn；
//   - 中间任何一次 UpdateStatus 失败 → 整笔事务回滚（CreateItems 没调过）。
func TestDraw_StateMachineFlow(t *testing.T) {
	d := defaultDeps()
	svc := newSvcFromDeps(d)

	if _, err := svc.Draw(newDrawCtx(), 1, 1); err != nil {
		t.Fatalf("Draw: %v", err)
	}

	// CreateOrder 入参 status 必须是 pending（状态机起点）。
	if d.order.lastOrder == nil {
		t.Fatal("CreateOrder not called")
	}
	if d.order.lastOrder.Status != mall.OrderStatusPending {
		t.Errorf("CreateOrder status = %q, want pending", d.order.lastOrder.Status)
	}

	// UpdateStatus 被调用 2 次：pending → paid（先），paid → drawn（后）。
	if d.order.updateStatusCalls != 2 {
		t.Fatalf("UpdateStatus called %d times, want 2", d.order.updateStatusCalls)
	}
	if got := d.order.statusUpdates; len(got) != 2 || got[0] != mall.OrderStatusPaid || got[1] != mall.OrderStatusDrawn {
		t.Errorf("status updates = %v, want [paid drawn]", got)
	}
}

// TestDraw_StateMachineFailure_RollsBack spec 14.5 防御性测试：
//
//	若 UpdateStatus 失败（模拟 state machine 副作用后 DB 写失败），
//	整笔事务回滚：CreateItems 不会被调，CreateOrder 已调但随 ROLLBACK 撤销。
func TestDraw_StateMachineFailure_RollsBack(t *testing.T) {
	d := defaultDeps()
	d.order.updateStatusFunc = func(c *gin.Context, id int64, s mall.MallDrawOrderStatus) error {
		// 第一次 UpdateStatus（pending → paid）失败。
		return errors.New("simulated update failure")
	}
	svc := newSvcFromDeps(d)
	_, err := svc.Draw(newDrawCtx(), 1, 1)
	if err == nil {
		t.Fatal("Draw should fail when UpdateStatus fails")
	}
	if d.order.createItemsCalls != 0 {
		t.Errorf("CreateItems called %d times, want 0 (tx rolled back)", d.order.createItemsCalls)
	}
}

func TestDraw_Success_WithPromo(t *testing.T) {
	d := defaultDeps()
	d.promo.findActiveFunc = func(c *gin.Context, bbID int64, now time.Time) (*mall.MallPromotion, error) {
		return &mall.MallPromotion{
			ID:            99,
			BlindBoxID:    bbID,
			OriginalPrice: 100,
			PromoPrice:    80,
			StartAt:       now.Add(-time.Hour),
			EndAt:         now.Add(time.Hour),
			Status:        mall.StatusActive,
		}, nil
	}
	svc := newSvcFromDeps(d)
	res, err := svc.Draw(newDrawCtx(), 1, 1)
	if err != nil {
		t.Fatalf("Draw: %v", err)
	}
	if res.ActualPrice != 80 {
		t.Errorf("ActualPrice = %v, want 80 (promo)", res.ActualPrice)
	}
	if d.sup.lastUpdateAmount != 80 {
		t.Errorf("sup update amount = %v, want 80 (promo)", d.sup.lastUpdateAmount)
	}
	if d.user.lastDecrementAmt != 80 {
		t.Errorf("user.DecrementBalance amount = %v, want 80 (promo)", d.user.lastDecrementAmt)
	}
	if d.settlement.lastDelta != 80 {
		t.Errorf("ledger delta = %v, want 80 (promo)", d.settlement.lastDelta)
	}
}

// TestDraw_RetryOnDecrementStockFailure 验证 D3 步骤 4 的「重试 1 次」机制。
//
// 单 item pool（确定性 pick）→ 第一次 DecrementStock 强制失败 → 重试仍命中该 item → 第二次成功。
func TestDraw_RetryOnDecrementStockFailure(t *testing.T) {
	d := defaultDeps()
	// 单 item，避免随机性。
	d.cp.listItemsByPoolIDFunc = func(c *gin.Context, poolID int64) ([]mall.MallCardPoolItem, error) {
		return []mall.MallCardPoolItem{
			{ID: 1, PoolID: poolID, CardID: 11, Rarity: mall.RaritySSR, Weight: 10000, Stock: 5},
		}, nil
	}
	// 第一次失败，第二次成功。
	d.cp.decrementFailFirst = map[int64]int{1: 1}
	svc := newSvcFromDeps(d)
	res, err := svc.Draw(newDrawCtx(), 1, 1)
	if err != nil {
		t.Fatalf("Draw: %v", err)
	}
	if res == nil {
		t.Fatalf("Draw returned nil")
	}
	if d.cp.decrementCalls < 2 {
		t.Errorf("DecrementStock called %d times, want >= 2 (first fail + retry)", d.cp.decrementCalls)
	}
}

func TestDraw_AllRetriesExhausted(t *testing.T) {
	d := defaultDeps()
	d.cp.decrementSucceedsOn = map[int64]int{1: 0, 2: 0}
	svc := newSvcFromDeps(d)
	_, err := svc.Draw(newDrawCtx(), 1, 1)
	if !errors.Is(err, ErrSoldOut) {
		t.Errorf("err = %v, want ErrSoldOut (all retries failed)", err)
	}
}

func TestPickWeightedItem_Deterministic(t *testing.T) {
	items := []mall.MallCardPoolItem{
		{ID: 1, Weight: 300, Stock: 100, Rarity: mall.RaritySSR},
		{ID: 2, Weight: 700, Stock: 100, Rarity: mall.RarityN},
	}
	cp := &drawCardPoolRepo{
		listItemsByPoolIDFunc: func(c *gin.Context, poolID int64) ([]mall.MallCardPoolItem, error) {
			return items, nil
		},
	}
	cp.decrementSucceedsOn = map[int64]int{1: 100, 2: 100}
	picked, err := pickWeightedItemWithRetry(newDrawCtx(), cp, 100, items)
	if err != nil {
		t.Fatalf("pickWeightedItemWithRetry: %v", err)
	}
	if picked == nil {
		t.Fatalf("picked is nil")
	}
	if picked.ID != 1 && picked.ID != 2 {
		t.Errorf("picked.ID = %d, want 1 or 2", picked.ID)
	}
}

// TestPickWeightedItem_ConcurrentSafety 跑 50 个并发 draw 验证重试逻辑不 panic。
//
// 集成测试应使用真实 MySQL + dbresolver 验证「FOR UPDATE 行锁 + 条件更新」是否真的
// 不会出现超卖；本单测只验证 pickWeightedItemWithRetry 的重试逻辑在并发下稳定。
func TestPickWeightedItem_ConcurrentSafety(t *testing.T) {
	items := []mall.MallCardPoolItem{
		{ID: 1, Weight: 100, Stock: 1000, Rarity: mall.RaritySSR},
	}
	cp := &drawCardPoolRepo{
		listItemsByPoolIDFunc: func(c *gin.Context, poolID int64) ([]mall.MallCardPoolItem, error) {
			return items, nil
		},
	}
	cp.decrementSucceedsOn = map[int64]int{1: 10000}

	var wg sync.WaitGroup
	for range 50 {
		wg.Go(func() {
			_, _ = pickWeightedItemWithRetry(newDrawCtx(), cp, 100, items)
		})
	}
	wg.Wait()
	if cp.decrementCalls < 50 {
		t.Errorf("DecrementStock called %d times, want >= 50", cp.decrementCalls)
	}
}

func TestDraw_TxRunnerErrorPropagates(t *testing.T) {
	d := defaultDeps()
	d.txr = &mockTxRunner{
		runFunc: func(c *gin.Context, fn func(txCtx *gin.Context) error) error {
			return errors.New("tx runner: commit failed")
		},
	}
	svc := newSvcFromDeps(d)
	_, err := svc.Draw(newDrawCtx(), 1, 1)
	if err == nil {
		t.Errorf("Draw err = nil, want tx runner error")
	}
}

func TestResolveActualPrice_NoPromo(t *testing.T) {
	promo := &drawPromoRepo{
		findActiveFunc: func(c *gin.Context, bbID int64, now time.Time) (*mall.MallPromotion, error) {
			return nil, nil
		},
	}
	bb := &mall.MallBlindBox{ID: 1, Price: 100}
	price, err := resolveActualPrice(newDrawCtx(), bb, promo, time.Now())
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if price != 100 {
		t.Errorf("price = %v, want 100", price)
	}
}

func TestResolveActualPrice_WithPromo(t *testing.T) {
	promo := &drawPromoRepo{
		findActiveFunc: func(c *gin.Context, bbID int64, now time.Time) (*mall.MallPromotion, error) {
			return &mall.MallPromotion{PromoPrice: 80}, nil
		},
	}
	bb := &mall.MallBlindBox{ID: 1, Price: 100}
	price, err := resolveActualPrice(newDrawCtx(), bb, promo, time.Now())
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if price != 80 {
		t.Errorf("price = %v, want 80 (promo)", price)
	}
}

// 编译期断言：mockTxRunner 实现 txRunner 接口。
var _ txRunner = (*mockTxRunner)(nil)

// =============================================================================
// spec 8.5 步骤 7.6：抽卡事务内累加 platform ledger
// =============================================================================

// TestDraw_LedgerIncrementFailure_RollsBack 验证 IncrementLedger 失败时整笔事务回滚：
//
//	sup.UpdateBalanceAndSales 已成功 → IncrementLedger 失败 → Draw 应返回 err，
//	且不应有订单写入（mockTxRunner 收到 rollback error 会传到上层）。
//
// 这里 IncrementLedger 抛非 sentinel err → Draw 直接返回；sup/user/order 副作用
// 都会被事务回滚（生产路径下 RunInTx 收到 err 会回滚；测试用 mockTxRunner 同步执行
// 也能保证调用顺序与失败位置正确）。
func TestDraw_LedgerIncrementFailure_RollsBack(t *testing.T) {
	d := defaultDeps()
	d.settlement.incrementLedgerFunc = func(c *gin.Context, counterKey string, delta float64) error {
		return errors.New("ledger write failed")
	}
	svc := newSvcFromDeps(d)

	_, err := svc.Draw(newDrawCtx(), 1, 1)
	if err == nil {
		t.Fatalf("Draw returned nil err, want ledger failure")
	}
	if err.Error() != "ledger write failed" {
		t.Errorf("err = %v, want ledger write failed", err)
	}
	// 副作用顺序断言：
	//  1. supplier balance 应已被累加（在 ledger 之前执行）
	//  2. order 不应落库（在 ledger 之后）
	if d.sup.updateBalanceCalls != 1 {
		t.Errorf("sup.UpdateBalanceAndSales calls = %d, want 1 (before ledger failure)", d.sup.updateBalanceCalls)
	}
	if d.order.createOrderCalls != 0 {
		t.Errorf("order.CreateOrder calls = %d, want 0 (after ledger failure)", d.order.createOrderCalls)
	}
}

// TestDraw_LedgerIncrementArgs 验证抽卡事务传给 IncrementLedger 的参数：
//
//	counterKey 固定 "total_revenue"，delta = actualPrice（活动价或原价）。
//
// 间接验证 spec 8.5「累加 total_revenue」语义。
func TestDraw_LedgerIncrementArgs(t *testing.T) {
	d := defaultDeps()
	svc := newSvcFromDeps(d)
	_, err := svc.Draw(newDrawCtx(), 1, 1)
	if err != nil {
		t.Fatalf("Draw: %v", err)
	}
	if d.settlement.incrementCalls != 1 {
		t.Fatalf("IncrementLedger calls = %d, want 1", d.settlement.incrementCalls)
	}
	if d.settlement.lastCounterKey != "total_revenue" {
		t.Errorf("counter_key = %q, want total_revenue", d.settlement.lastCounterKey)
	}
	if d.settlement.lastDelta != 100 {
		t.Errorf("delta = %v, want 100 (blindbox price)", d.settlement.lastDelta)
	}
}

// TestNewOrderNo_FitsVarchar32 是 order_no 列宽的回归测试。
//
// spec「mall_draw_orders field schema」、迁移 000009 与 model 都把 order_no 定为
// varchar(32)；但 uuid.UUID.String() 是 36 字符（带 4 个连字符），直接落库会
// `Error 1406 Data too long for column 'order_no'`（本次 7.4 抽卡验证即由此暴露，
// 单测因为 mock 不校验列宽而漏掉）。这里把「32 位、无连字符」锁成契约。
func TestNewOrderNo_FitsVarchar32(t *testing.T) {
	seen := make(map[string]struct{}, 64)
	for i := range 64 {
		orderNo, err := newOrderNo()
		if err != nil {
			t.Fatalf("newOrderNo: %v", err)
		}
		if len(orderNo) != 32 {
			t.Fatalf("len(order_no) = %d (%q), want 32 —— 超过 varchar(32) 会 Error 1406", len(orderNo), orderNo)
		}
		if strings.Contains(orderNo, "-") {
			t.Fatalf("order_no = %q 含连字符（36 字符形态），落库会超宽", orderNo)
		}
		if _, dup := seen[orderNo]; dup {
			t.Fatalf("第 %d 次生成重复的 order_no %q", i, orderNo)
		}
		seen[orderNo] = struct{}{}
	}
}
