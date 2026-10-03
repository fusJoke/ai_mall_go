package admin

import (
	"errors"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"ai-go-mall/internal/domain/state"
	"ai-go-mall/internal/infra/config"
	"ai-go-mall/internal/model/mall"
	"ai-go-mall/internal/repository"
	mallRepo "ai-go-mall/internal/repository/mall"
	supplierRepo "ai-go-mall/internal/repository/supplier"
)

// =============================================================================
// helpers
// =============================================================================

func newAdminSettlementCtx() *gin.Context {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", "/api/v1/admin/settlements", nil)
	return c
}

// withDefaultConfig 保证 service.Preview / Generate 的 commission_rate 默认值可用。
func withDefaultConfig(t *testing.T) {
	t.Helper()
	config.SetForTest(&config.Config{
		Mall: config.MallConfig{
			DefaultCommissionRate: 0.10,
		},
	})
}

// =============================================================================
// Mock SettlementRepository
// =============================================================================

type adminSettlementRepo struct {
	listFunc          func(c *gin.Context, opts repository.ListOptions) ([]mall.MallSettlement, int64, error)
	getByIDFunc       func(c *gin.Context, id int64) (*mall.MallSettlement, error)
	getByIDWithItemsF func(c *gin.Context, id int64) (*mall.MallSettlement, []mall.MallSettlementItem, error)
	listEligibleFunc  func(c *gin.Context, supplierID int64, periodStart, periodEnd time.Time) ([]mall.MallDrawOrder, error)
	generateFunc      func(c *gin.Context, settlement *mall.MallSettlement, items []mall.MallSettlementItem) error
	markPaidFunc      func(c *gin.Context, settlementID int64, adminID int64, payoutAmount float64) error
	updateStatusFunc  func(c *gin.Context, id int64, status mall.MallSettlementStatus) error

	generateCalls    int
	lastGenerate     *mall.MallSettlement
	lastGenerateItem []mall.MallSettlementItem

	updateStatusCalls int
	lastUpdateID      int64
	lastUpdateStatus  mall.MallSettlementStatus
}

func (m *adminSettlementRepo) List(c *gin.Context, opts repository.ListOptions) ([]mall.MallSettlement, int64, error) {
	if m.listFunc != nil {
		return m.listFunc(c, opts)
	}
	return nil, 0, nil
}
func (m *adminSettlementRepo) GetByID(c *gin.Context, id int64) (*mall.MallSettlement, error) {
	if m.getByIDFunc != nil {
		return m.getByIDFunc(c, id)
	}
	return nil, gorm.ErrRecordNotFound
}
func (m *adminSettlementRepo) GetByIDWithItems(c *gin.Context, id int64) (*mall.MallSettlement, []mall.MallSettlementItem, error) {
	if m.getByIDWithItemsF != nil {
		return m.getByIDWithItemsF(c, id)
	}
	return nil, nil, gorm.ErrRecordNotFound
}
func (m *adminSettlementRepo) ListEligibleOrders(c *gin.Context, supplierID int64, periodStart, periodEnd time.Time) ([]mall.MallDrawOrder, error) {
	if m.listEligibleFunc != nil {
		return m.listEligibleFunc(c, supplierID, periodStart, periodEnd)
	}
	return nil, nil
}
func (m *adminSettlementRepo) Generate(c *gin.Context, settlement *mall.MallSettlement, items []mall.MallSettlementItem) error {
	m.generateCalls++
	m.lastGenerate = settlement
	m.lastGenerateItem = items
	if m.generateFunc != nil {
		return m.generateFunc(c, settlement, items)
	}
	return nil
}
func (m *adminSettlementRepo) MarkPaid(c *gin.Context, settlementID int64, adminID int64, payoutAmount float64) error {
	if m.markPaidFunc != nil {
		return m.markPaidFunc(c, settlementID, adminID, payoutAmount)
	}
	return nil
}

// 其余 CRUD 方法 stub（service 未直接使用，保持编译期断言）。
func (m *adminSettlementRepo) ListBySupplier(c *gin.Context, supplierID int64, opts repository.ListOptions) ([]mall.MallSettlement, int64, error) {
	return nil, 0, nil
}
func (m *adminSettlementRepo) ListItemsBySettlementID(c *gin.Context, settlementID int64) ([]mall.MallSettlementItem, error) {
	return nil, nil
}
func (m *adminSettlementRepo) IncrementLedger(c *gin.Context, counterKey string, delta float64) error {
	return nil
}
func (m *adminSettlementRepo) UpdateStatus(c *gin.Context, id int64, status mall.MallSettlementStatus) error {
	m.updateStatusCalls++
	m.lastUpdateID = id
	m.lastUpdateStatus = status
	if m.updateStatusFunc != nil {
		return m.updateStatusFunc(c, id, status)
	}
	return nil
}
func (m *adminSettlementRepo) Create(c *gin.Context, e *mall.MallSettlement) error { return nil }
func (m *adminSettlementRepo) Update(c *gin.Context, e *mall.MallSettlement) error { return nil }
func (m *adminSettlementRepo) Delete(c *gin.Context, id int64) error               { return nil }

var _ mallRepo.SettlementRepository = (*adminSettlementRepo)(nil)

// =============================================================================
// Mock SupplierRepository
// =============================================================================

type adminSettlementSupplierRepo struct {
	getByIDFunc func(c *gin.Context, id int64) (*mall.MallSupplier, error)
}

func (m *adminSettlementSupplierRepo) GetByID(c *gin.Context, id int64) (*mall.MallSupplier, error) {
	if m.getByIDFunc != nil {
		return m.getByIDFunc(c, id)
	}
	return nil, gorm.ErrRecordNotFound
}

// 其余方法 stub。
func (m *adminSettlementSupplierRepo) ListByIDs(c *gin.Context, ids []int64) ([]mall.MallSupplier, error) {
	return nil, nil
}
func (m *adminSettlementSupplierRepo) ListWithFilter(c *gin.Context, opts supplierRepo.SupplierListOptions) ([]mall.MallSupplier, int64, error) {
	return nil, 0, nil
}
func (m *adminSettlementSupplierRepo) ToggleStatus(c *gin.Context, id int64, status mall.MallBlindBoxStatus) error {
	return nil
}
func (m *adminSettlementSupplierRepo) ToggleFeatured(c *gin.Context, id int64, featured bool) error {
	return nil
}
func (m *adminSettlementSupplierRepo) UpdateBalanceAndSales(c *gin.Context, id int64, amount float64) error {
	return nil
}
func (m *adminSettlementSupplierRepo) Create(c *gin.Context, e *mall.MallSupplier) error { return nil }
func (m *adminSettlementSupplierRepo) List(c *gin.Context, opts repository.ListOptions) ([]mall.MallSupplier, int64, error) {
	return nil, 0, nil
}
func (m *adminSettlementSupplierRepo) Update(c *gin.Context, e *mall.MallSupplier) error { return nil }
func (m *adminSettlementSupplierRepo) Delete(c *gin.Context, id int64) error             { return nil }

var _ supplierRepo.SupplierRepository = (*adminSettlementSupplierRepo)(nil)

// =============================================================================
// Mock DrawOrderRepository（仅用于保持 SettlementServiceDeps 注入完整）
// =============================================================================

type adminSettlementOrderRepo struct{}

func (m *adminSettlementOrderRepo) GetByID(c *gin.Context, id int64) (*mall.MallDrawOrder, error) {
	return nil, nil
}
func (m *adminSettlementOrderRepo) GetByOrderNo(c *gin.Context, orderNo string) (*mall.MallDrawOrder, error) {
	return nil, nil
}
func (m *adminSettlementOrderRepo) CreateOrder(c *gin.Context, order *mall.MallDrawOrder) error {
	return nil
}
func (m *adminSettlementOrderRepo) ListByUser(c *gin.Context, userID int64, opts repository.ListOptions) ([]mall.MallDrawOrder, int64, error) {
	return nil, 0, nil
}
func (m *adminSettlementOrderRepo) UpdateStatus(c *gin.Context, id int64, status mall.MallDrawOrderStatus) error {
	return nil
}
func (m *adminSettlementOrderRepo) ListItemsByOrderID(c *gin.Context, orderID int64) ([]mall.MallDrawOrderItem, error) {
	return nil, nil
}
func (m *adminSettlementOrderRepo) CreateItems(c *gin.Context, items []mall.MallDrawOrderItem) error {
	return nil
}
func (m *adminSettlementOrderRepo) ListSettledOrdersBySupplierInRange(c *gin.Context, supplierID int64, startAt, endAt time.Time) ([]mall.MallDrawOrder, error) {
	return nil, nil
}
func (m *adminSettlementOrderRepo) Create(c *gin.Context, e *mall.MallDrawOrder) error { return nil }
func (m *adminSettlementOrderRepo) List(c *gin.Context, opts repository.ListOptions) ([]mall.MallDrawOrder, int64, error) {
	return nil, 0, nil
}
func (m *adminSettlementOrderRepo) Update(c *gin.Context, e *mall.MallDrawOrder) error { return nil }
func (m *adminSettlementOrderRepo) Delete(c *gin.Context, id int64) error              { return nil }

var _ mallRepo.DrawOrderRepository = (*adminSettlementOrderRepo)(nil)

// =============================================================================
// fixtures
// =============================================================================

func newAdminSettlementSvc(srepo *adminSettlementRepo, suprepo *adminSettlementSupplierRepo) SettlementService {
	return NewSettlementService(SettlementServiceDeps{
		SettlementRepo: srepo,
		SupplierRepo:   suprepo,
		DrawOrderRepo:  &adminSettlementOrderRepo{},
	})
}

func ptr64(v float64) *float64 { return &v }

// =============================================================================
// List
// =============================================================================

func TestAdminSettlementList_DelegatesToRepo(t *testing.T) {
	wantItems := []mall.MallSettlement{
		{ID: 1, SupplierID: 3, Status: mall.SettlementStatusPending},
	}
	wantTotal := int64(1)
	repo := &adminSettlementRepo{
		listFunc: func(c *gin.Context, opts repository.ListOptions) ([]mall.MallSettlement, int64, error) {
			return wantItems, wantTotal, nil
		},
	}
	svc := newAdminSettlementSvc(repo, &adminSettlementSupplierRepo{})
	items, total, err := svc.List(newAdminSettlementCtx(), repository.ListOptions{Page: 1, PageSize: 20})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if total != wantTotal || len(items) != len(wantItems) {
		t.Errorf("got (%d, %d), want (%d, %d)", total, len(items), wantTotal, len(wantItems))
	}
}

func TestAdminSettlementList_RepoErrorPropagates(t *testing.T) {
	wantErr := errors.New("db: timeout")
	repo := &adminSettlementRepo{
		listFunc: func(c *gin.Context, opts repository.ListOptions) ([]mall.MallSettlement, int64, error) {
			return nil, 0, wantErr
		},
	}
	svc := newAdminSettlementSvc(repo, &adminSettlementSupplierRepo{})
	_, _, err := svc.List(newAdminSettlementCtx(), repository.ListOptions{Page: 1, PageSize: 10})
	if !errors.Is(err, wantErr) {
		t.Errorf("err = %v, want %v", err, wantErr)
	}
}

// =============================================================================
// Detail
// =============================================================================

func TestAdminSettlementDetail_InvalidID(t *testing.T) {
	svc := newAdminSettlementSvc(&adminSettlementRepo{}, &adminSettlementSupplierRepo{})
	_, _, err := svc.Detail(newAdminSettlementCtx(), 0)
	if !errors.Is(err, ErrSettlementNotFound) {
		t.Errorf("err = %v, want ErrSettlementNotFound", err)
	}
}

func TestAdminSettlementDetail_NotFound(t *testing.T) {
	srepo := &adminSettlementRepo{
		getByIDWithItemsF: func(c *gin.Context, id int64) (*mall.MallSettlement, []mall.MallSettlementItem, error) {
			return nil, nil, gorm.ErrRecordNotFound
		},
	}
	svc := newAdminSettlementSvc(srepo, &adminSettlementSupplierRepo{})
	_, _, err := svc.Detail(newAdminSettlementCtx(), 99)
	if !errors.Is(err, ErrSettlementNotFound) {
		t.Errorf("err = %v, want ErrSettlementNotFound", err)
	}
}

func TestAdminSettlementDetail_HappyPath(t *testing.T) {
	settlement := &mall.MallSettlement{ID: 7, SupplierID: 3, Status: mall.SettlementStatusPending}
	items := []mall.MallSettlementItem{{ID: 1, SettlementID: 7, DrawOrderID: 101}}
	srepo := &adminSettlementRepo{
		getByIDWithItemsF: func(c *gin.Context, id int64) (*mall.MallSettlement, []mall.MallSettlementItem, error) {
			return settlement, items, nil
		},
	}
	svc := newAdminSettlementSvc(srepo, &adminSettlementSupplierRepo{})
	got, gotItems, err := svc.Detail(newAdminSettlementCtx(), 7)
	if err != nil {
		t.Fatalf("Detail: %v", err)
	}
	if got == nil || got.ID != 7 || len(gotItems) != 1 {
		t.Errorf("got (%+v, %+v)", got, gotItems)
	}
}

// =============================================================================
// Preview
// =============================================================================

func TestAdminSettlementPreview_InvalidPeriod(t *testing.T) {
	withDefaultConfig(t)
	srepo := &adminSettlementRepo{}
	suprepo := &adminSettlementSupplierRepo{
		getByIDFunc: func(c *gin.Context, id int64) (*mall.MallSupplier, error) {
			return &mall.MallSupplier{ID: id}, nil
		},
	}
	svc := newAdminSettlementSvc(srepo, suprepo)
	_, err := svc.Preview(newAdminSettlementCtx(), 3, time.Now(), time.Now()) // end <= start
	if !errors.Is(err, ErrInvalidPeriod) {
		t.Errorf("err = %v, want ErrInvalidPeriod", err)
	}
}

func TestAdminSettlementPreview_SupplierNotFound(t *testing.T) {
	suprepo := &adminSettlementSupplierRepo{
		getByIDFunc: func(c *gin.Context, id int64) (*mall.MallSupplier, error) {
			return nil, gorm.ErrRecordNotFound
		},
	}
	svc := newAdminSettlementSvc(&adminSettlementRepo{}, suprepo)
	_, err := svc.Preview(newAdminSettlementCtx(), 99, time.Now().Add(-time.Hour), time.Now())
	if !errors.Is(err, ErrSupplierNotFound) {
		t.Errorf("err = %v, want ErrSupplierNotFound", err)
	}
}

func TestAdminSettlementPreview_NoOrders(t *testing.T) {
	withDefaultConfig(t)
	suprepo := &adminSettlementSupplierRepo{
		getByIDFunc: func(c *gin.Context, id int64) (*mall.MallSupplier, error) {
			rate := ptr64(0.10)
			return &mall.MallSupplier{ID: id, Name: "Acme", CommissionRate: rate}, nil
		},
	}
	srepo := &adminSettlementRepo{
		listEligibleFunc: func(c *gin.Context, supplierID int64, periodStart, periodEnd time.Time) ([]mall.MallDrawOrder, error) {
			return nil, nil
		},
	}
	svc := newAdminSettlementSvc(srepo, suprepo)
	preview, err := svc.Preview(newAdminSettlementCtx(), 3, time.Now().Add(-time.Hour), time.Now())
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}
	if preview.OrderCount != 0 || preview.TotalAmount != 0 || preview.CommissionAmount != 0 || preview.PayoutAmount != 0 {
		t.Errorf("empty preview got %+v", preview)
	}
	if preview.SupplierName != "Acme" {
		t.Errorf("SupplierName = %q, want Acme", preview.SupplierName)
	}
}

func TestAdminSettlementPreview_HappyPath(t *testing.T) {
	withDefaultConfig(t)
	suprepo := &adminSettlementSupplierRepo{
		getByIDFunc: func(c *gin.Context, id int64) (*mall.MallSupplier, error) {
			rate := ptr64(0.10)
			return &mall.MallSupplier{ID: id, Name: "Acme", CommissionRate: rate}, nil
		},
	}
	srepo := &adminSettlementRepo{
		listEligibleFunc: func(c *gin.Context, supplierID int64, periodStart, periodEnd time.Time) ([]mall.MallDrawOrder, error) {
			return []mall.MallDrawOrder{
				{ID: 1, SupplierID: 3, Price: 100},
				{ID: 2, SupplierID: 3, Price: 200},
			}, nil
		},
	}
	svc := newAdminSettlementSvc(srepo, suprepo)
	start := time.Now().Add(-time.Hour)
	end := time.Now()
	preview, err := svc.Preview(newAdminSettlementCtx(), 3, start, end)
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}
	if preview.OrderCount != 2 {
		t.Errorf("OrderCount = %d, want 2", preview.OrderCount)
	}
	if preview.TotalAmount != 300 {
		t.Errorf("TotalAmount = %v, want 300", preview.TotalAmount)
	}
	if preview.CommissionAmount != 30 {
		t.Errorf("CommissionAmount = %v, want 30", preview.CommissionAmount)
	}
	if preview.PayoutAmount != 270 {
		t.Errorf("PayoutAmount = %v, want 270", preview.PayoutAmount)
	}
	if preview.CommissionRate != 0.10 {
		t.Errorf("CommissionRate = %v, want 0.10", preview.CommissionRate)
	}
}

func TestAdminSettlementPreview_FallsBackToDefault(t *testing.T) {
	withDefaultConfig(t)
	suprepo := &adminSettlementSupplierRepo{
		getByIDFunc: func(c *gin.Context, id int64) (*mall.MallSupplier, error) {
			return &mall.MallSupplier{ID: id, Name: "Acme", CommissionRate: nil}, nil // NULL
		},
	}
	srepo := &adminSettlementRepo{
		listEligibleFunc: func(c *gin.Context, supplierID int64, periodStart, periodEnd time.Time) ([]mall.MallDrawOrder, error) {
			return nil, nil
		},
	}
	svc := newAdminSettlementSvc(srepo, suprepo)
	preview, err := svc.Preview(newAdminSettlementCtx(), 3, time.Now().Add(-time.Hour), time.Now())
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}
	if preview.CommissionRate != 0.10 {
		t.Errorf("CommissionRate = %v, want 0.10 (default)", preview.CommissionRate)
	}
}

// =============================================================================
// Generate
// =============================================================================

func TestAdminSettlementGenerate_NoOrders(t *testing.T) {
	withDefaultConfig(t)
	suprepo := &adminSettlementSupplierRepo{
		getByIDFunc: func(c *gin.Context, id int64) (*mall.MallSupplier, error) {
			rate := ptr64(0.10)
			return &mall.MallSupplier{ID: id, CommissionRate: rate}, nil
		},
	}
	srepo := &adminSettlementRepo{
		listEligibleFunc: func(c *gin.Context, supplierID int64, periodStart, periodEnd time.Time) ([]mall.MallDrawOrder, error) {
			return nil, nil
		},
	}
	svc := newAdminSettlementSvc(srepo, suprepo)
	_, err := svc.Generate(newAdminSettlementCtx(), 3, time.Now().Add(-time.Hour), time.Now())
	if !errors.Is(err, ErrNoOrdersToSettle) {
		t.Errorf("err = %v, want ErrNoOrdersToSettle", err)
	}
	if srepo.generateCalls != 0 {
		t.Errorf("Generate called %d times, want 0", srepo.generateCalls)
	}
}

func TestAdminSettlementGenerate_HappyPath(t *testing.T) {
	withDefaultConfig(t)
	suprepo := &adminSettlementSupplierRepo{
		getByIDFunc: func(c *gin.Context, id int64) (*mall.MallSupplier, error) {
			rate := ptr64(0.10)
			return &mall.MallSupplier{ID: id, CommissionRate: rate}, nil
		},
	}
	srepo := &adminSettlementRepo{
		listEligibleFunc: func(c *gin.Context, supplierID int64, periodStart, periodEnd time.Time) ([]mall.MallDrawOrder, error) {
			return []mall.MallDrawOrder{
				{ID: 1, SupplierID: 3, Price: 100},
				{ID: 2, SupplierID: 3, Price: 200},
			}, nil
		},
	}
	svc := newAdminSettlementSvc(srepo, suprepo)
	settlement, err := svc.Generate(newAdminSettlementCtx(), 3, time.Now().Add(-time.Hour), time.Now())
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if srepo.generateCalls != 1 {
		t.Errorf("Generate called %d times, want 1", srepo.generateCalls)
	}
	if settlement == nil {
		t.Fatalf("settlement is nil")
	}
	if settlement.TotalAmount != 300 || settlement.CommissionAmount != 30 || settlement.PayoutAmount != 270 {
		t.Errorf("settlement amounts = (%v, %v, %v), want (300, 30, 270)",
			settlement.TotalAmount, settlement.CommissionAmount, settlement.PayoutAmount)
	}
	if len(srepo.lastGenerateItem) != 2 {
		t.Errorf("items count = %d, want 2", len(srepo.lastGenerateItem))
	}
	// spec 14.6：Generate 完成后状态机推进 pending → processing；
	// service 返回的 settlement.Status 也同步更新为 processing。
	if settlement.Status != mall.SettlementStatusProcessing {
		t.Errorf("settlement.Status = %v, want processing", settlement.Status)
	}
	// UpdateStatus 也应被调一次（state → DB 持久化）。
	if srepo.updateStatusCalls != 1 {
		t.Errorf("UpdateStatus called %d times, want 1", srepo.updateStatusCalls)
	}
	if srepo.lastUpdateStatus != mall.SettlementStatusProcessing {
		t.Errorf("last UpdateStatus target = %v, want processing", srepo.lastUpdateStatus)
	}
}

func TestAdminSettlementGenerate_ConflictPropagates(t *testing.T) {
	withDefaultConfig(t)
	suprepo := &adminSettlementSupplierRepo{
		getByIDFunc: func(c *gin.Context, id int64) (*mall.MallSupplier, error) {
			rate := ptr64(0.10)
			return &mall.MallSupplier{ID: id, CommissionRate: rate}, nil
		},
	}
	srepo := &adminSettlementRepo{
		listEligibleFunc: func(c *gin.Context, supplierID int64, periodStart, periodEnd time.Time) ([]mall.MallDrawOrder, error) {
			return []mall.MallDrawOrder{{ID: 1, Price: 100}}, nil
		},
		generateFunc: func(c *gin.Context, settlement *mall.MallSettlement, items []mall.MallSettlementItem) error {
			return mallRepo.ErrSettlementConflict
		},
	}
	svc := newAdminSettlementSvc(srepo, suprepo)
	_, err := svc.Generate(newAdminSettlementCtx(), 3, time.Now().Add(-time.Hour), time.Now())
	if !errors.Is(err, ErrSettlementConflict) {
		t.Errorf("err = %v, want ErrSettlementConflict", err)
	}
}

// =============================================================================
// MarkPaid
// =============================================================================

func TestAdminSettlementMarkPaid_InvalidAdminID(t *testing.T) {
	svc := newAdminSettlementSvc(&adminSettlementRepo{}, &adminSettlementSupplierRepo{})
	if err := svc.MarkPaid(newAdminSettlementCtx(), 7, 0); err == nil {
		t.Errorf("err = nil, want error")
	}
}

func TestAdminSettlementMarkPaid_InvalidSettlementID(t *testing.T) {
	svc := newAdminSettlementSvc(&adminSettlementRepo{}, &adminSettlementSupplierRepo{})
	if err := svc.MarkPaid(newAdminSettlementCtx(), 0, 1); !errors.Is(err, ErrSettlementNotFound) {
		t.Errorf("err = %v, want ErrSettlementNotFound", err)
	}
}

func TestAdminSettlementMarkPaid_SettlementNotFound(t *testing.T) {
	srepo := &adminSettlementRepo{
		getByIDFunc: func(c *gin.Context, id int64) (*mall.MallSettlement, error) {
			return nil, gorm.ErrRecordNotFound
		},
	}
	svc := newAdminSettlementSvc(srepo, &adminSettlementSupplierRepo{})
	if err := svc.MarkPaid(newAdminSettlementCtx(), 99, 1); !errors.Is(err, ErrSettlementNotFound) {
		t.Errorf("err = %v, want ErrSettlementNotFound", err)
	}
}

func TestAdminSettlementMarkPaid_HappyPath(t *testing.T) {
	wantPayout := 270.0
	var gotPayout float64
	srepo := &adminSettlementRepo{
		getByIDFunc: func(c *gin.Context, id int64) (*mall.MallSettlement, error) {
			return &mall.MallSettlement{ID: id, PayoutAmount: wantPayout, Status: mall.SettlementStatusProcessing}, nil
		},
		markPaidFunc: func(c *gin.Context, settlementID int64, adminID int64, payoutAmount float64) error {
			gotPayout = payoutAmount
			return nil
		},
	}
	svc := newAdminSettlementSvc(srepo, &adminSettlementSupplierRepo{})
	if err := svc.MarkPaid(newAdminSettlementCtx(), 7, 1); err != nil {
		t.Fatalf("MarkPaid: %v", err)
	}
	if gotPayout != wantPayout {
		t.Errorf("MarkPaid repayout = %v, want %v", gotPayout, wantPayout)
	}
}

func TestAdminSettlementMarkPaid_AlreadyPaidPropagates(t *testing.T) {
	// status=paid → 状态机 transition processing|pending → paid 失败
	// → ErrInvalidStateTransition（在调用 repo 之前 fail-fast）。
	// 与 service 层依赖 repo.MarkPaid 抛 ErrSettlementAlreadyPaid 的旧行为不同：
	// 状态机统一在「写库前」拦截非法转换，handler 仍映射 HTTP 409（code 字符串略变）。
	srepo := &adminSettlementRepo{
		getByIDFunc: func(c *gin.Context, id int64) (*mall.MallSettlement, error) {
			return &mall.MallSettlement{ID: id, Status: mall.SettlementStatusPaid}, nil
		},
		markPaidFunc: func(c *gin.Context, settlementID int64, adminID int64, payoutAmount float64) error {
			return mallRepo.ErrSettlementAlreadyPaid
		},
	}
	svc := newAdminSettlementSvc(srepo, &adminSettlementSupplierRepo{})
	if err := svc.MarkPaid(newAdminSettlementCtx(), 7, 1); !errors.Is(err, state.ErrInvalidStateTransition) {
		t.Errorf("err = %v, want state.ErrInvalidStateTransition", err)
	}
}

// TestAdminSettlementMarkPaid_PendingRejected spec 14.6：
//
//	settlement.status=pending 调 MarkPaid → 状态机拒绝（必须先经 Generate 走到 processing）。
//	错误名 ErrInvalidStateTransition（handler 映射 HTTP 409）。
func TestAdminSettlementMarkPaid_PendingRejected(t *testing.T) {
	var markPaidCalled bool
	srepo := &adminSettlementRepo{
		getByIDFunc: func(c *gin.Context, id int64) (*mall.MallSettlement, error) {
			return &mall.MallSettlement{ID: id, PayoutAmount: 100, Status: mall.SettlementStatusPending}, nil
		},
		markPaidFunc: func(c *gin.Context, settlementID int64, adminID int64, payoutAmount float64) error {
			markPaidCalled = true
			return nil
		},
	}
	svc := newAdminSettlementSvc(srepo, &adminSettlementSupplierRepo{})
	if err := svc.MarkPaid(newAdminSettlementCtx(), 7, 1); !errors.Is(err, state.ErrInvalidStateTransition) {
		t.Errorf("err = %v, want state.ErrInvalidStateTransition", err)
	}
	if markPaidCalled {
		t.Error("repo.MarkPaid should NOT be called when state machine rejects (transition pending → paid)")
	}
}

// TestAdminSettlementMarkPaid_FailedRejected spec 14.6：
//
//	settlement.status=failed（终态）调 MarkPaid → 状态机拒绝。
func TestAdminSettlementMarkPaid_FailedRejected(t *testing.T) {
	var markPaidCalled bool
	srepo := &adminSettlementRepo{
		getByIDFunc: func(c *gin.Context, id int64) (*mall.MallSettlement, error) {
			return &mall.MallSettlement{ID: id, PayoutAmount: 100, Status: mall.SettlementStatusFailed}, nil
		},
		markPaidFunc: func(c *gin.Context, settlementID int64, adminID int64, payoutAmount float64) error {
			markPaidCalled = true
			return nil
		},
	}
	svc := newAdminSettlementSvc(srepo, &adminSettlementSupplierRepo{})
	if err := svc.MarkPaid(newAdminSettlementCtx(), 7, 1); !errors.Is(err, state.ErrInvalidStateTransition) {
		t.Errorf("err = %v, want state.ErrInvalidStateTransition", err)
	}
	if markPaidCalled {
		t.Error("repo.MarkPaid should NOT be called when state machine rejects (terminal failed → paid)")
	}
}

// =============================================================================
// roundHalfUp
// =============================================================================

func TestRoundHalfUp(t *testing.T) {
	cases := []struct {
		v        float64
		decimals int
		want     float64
	}{
		// 用 IEEE 754 精确表示的 half（1.125 = 1+1/8）验证 half-up 语义。
		{1.125, 2, 1.13}, // half-up：远离偶数 → 1.13
		{1.135, 2, 1.14}, // half-up → 1.14
		{1.004, 2, 1.00},
		{100.0 * 0.10, 2, 10.0},
		{0.005, 3, 0.005}, // 边界
	}
	for _, c := range cases {
		if got := roundHalfUp(c.v, c.decimals); got != c.want {
			t.Errorf("roundHalfUp(%v, %d) = %v, want %v", c.v, c.decimals, got, c.want)
		}
	}
}
