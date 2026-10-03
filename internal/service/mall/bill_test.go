package mall

import (
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	"ai-go-mall/internal/domain/state"
	"ai-go-mall/internal/infra/config"
	"ai-go-mall/internal/infra/database"
	"ai-go-mall/internal/model/mall"
	mallRepo "ai-go-mall/internal/repository/mall"
	supplierRepo "ai-go-mall/internal/repository/supplier"
)

// =============================================================================
// helpers
// =============================================================================

// newBillPlainCtx 构造无 DB 注入的测试 ctx（GenerateForBox / Regenerate 不开事务）。
func newBillPlainCtx() *gin.Context {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", "/", nil)
	return c
}

// newBillTestCtx 构造注入 sqlmock DB 的测试 ctx（MarkPaid 的 RunInTx 需要）。
func newBillTestCtx(t *testing.T) (*gin.Context, sqlmock.Sqlmock) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })

	gdb, err := gorm.Open(mysql.New(mysql.Config{
		Conn:                      sqlDB,
		SkipInitializeWithVersion: true,
	}), &gorm.Config{})
	if err != nil {
		t.Fatalf("gorm.Open: %v", err)
	}

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", "/", nil)
	c.Set(database.CtxKey, gdb)
	return c, mock
}

// withBillTestConfig 设置默认费率 0.10（supplier.CommissionRate=nil 时走 config）。
func withBillTestConfig(t *testing.T) {
	t.Helper()
	config.SetForTest(&config.Config{
		Mall: config.MallConfig{DefaultCommissionRate: 0.10},
	})
}

// =============================================================================
// Fakes（嵌入 nil 接口 + 按需 override；service 调到未 mock 方法会 panic = 测试红）
// =============================================================================

type fakeBillRepo struct {
	mallRepo.SettlementBillRepository

	getByBoxIDFunc   func(c *gin.Context, boxID int64) (*mall.MallSettlementBill, error)
	getByIDFunc      func(c *gin.Context, id int64) (*mall.MallSettlementBill, error)
	createWithItemsF func(c *gin.Context, bill *mall.MallSettlementBill, items []mall.MallSettlementBillItem) error
	markPaidFunc     func(c *gin.Context, id int64, adminID int64) (int64, error)
	deductFunc       func(c *gin.Context, supplierID int64, amount float64) (int64, error)
	setLifecycleFunc func(c *gin.Context, boxID int64, from, to mall.MallBlindBoxLifecycle) (int64, error)
	regenerateFunc   func(c *gin.Context, id int64, amounts mallRepo.BillRegenAmounts, remark string) (int64, error)

	createCalls       int
	lastCreateBill    *mall.MallSettlementBill
	lastCreateItems   []mall.MallSettlementBillItem
	markPaidCalls     int
	deductCalls       int
	setLifecycleCalls int
	lastLifecycleFrom mall.MallBlindBoxLifecycle
	lastLifecycleTo   mall.MallBlindBoxLifecycle
}

func (f *fakeBillRepo) GetByBoxID(c *gin.Context, boxID int64) (*mall.MallSettlementBill, error) {
	if f.getByBoxIDFunc != nil {
		return f.getByBoxIDFunc(c, boxID)
	}
	return nil, mallRepo.ErrBillNotFound
}

func (f *fakeBillRepo) GetByID(c *gin.Context, id int64) (*mall.MallSettlementBill, error) {
	if f.getByIDFunc != nil {
		return f.getByIDFunc(c, id)
	}
	return nil, gorm.ErrRecordNotFound
}

func (f *fakeBillRepo) CreateWithItems(c *gin.Context, bill *mall.MallSettlementBill, items []mall.MallSettlementBillItem) error {
	f.createCalls++
	f.lastCreateBill = bill
	f.lastCreateItems = items
	if f.createWithItemsF != nil {
		return f.createWithItemsF(c, bill, items)
	}
	bill.ID = 77
	return nil
}

func (f *fakeBillRepo) MarkPaid(c *gin.Context, id int64, adminID int64) (int64, error) {
	f.markPaidCalls++
	if f.markPaidFunc != nil {
		return f.markPaidFunc(c, id, adminID)
	}
	return 1, nil
}

func (f *fakeBillRepo) DeductSupplierBalance(c *gin.Context, supplierID int64, amount float64) (int64, error) {
	f.deductCalls++
	if f.deductFunc != nil {
		return f.deductFunc(c, supplierID, amount)
	}
	return 1, nil
}

func (f *fakeBillRepo) SetBoxLifecycle(c *gin.Context, boxID int64, from, to mall.MallBlindBoxLifecycle) (int64, error) {
	f.setLifecycleCalls++
	f.lastLifecycleFrom = from
	f.lastLifecycleTo = to
	if f.setLifecycleFunc != nil {
		return f.setLifecycleFunc(c, boxID, from, to)
	}
	return 1, nil
}

func (f *fakeBillRepo) Regenerate(c *gin.Context, id int64, amounts mallRepo.BillRegenAmounts, remark string) (int64, error) {
	if f.regenerateFunc != nil {
		return f.regenerateFunc(c, id, amounts, remark)
	}
	return 1, nil
}

type fakeBoxRepo struct {
	mallRepo.BlindBoxRepository

	getByIDFunc func(c *gin.Context, id int64) (*mall.MallBlindBox, error)
}

func (f *fakeBoxRepo) GetByID(c *gin.Context, id int64) (*mall.MallBlindBox, error) {
	if f.getByIDFunc != nil {
		return f.getByIDFunc(c, id)
	}
	return nil, gorm.ErrRecordNotFound
}

type fakeSupplierRepo struct {
	supplierRepo.SupplierRepository

	getByIDFunc func(c *gin.Context, id int64) (*mall.MallSupplier, error)
}

func (f *fakeSupplierRepo) GetByID(c *gin.Context, id int64) (*mall.MallSupplier, error) {
	if f.getByIDFunc != nil {
		return f.getByIDFunc(c, id)
	}
	return nil, gorm.ErrRecordNotFound
}

type fakeOrderRepo struct {
	mallRepo.DrawOrderRepository

	listBoxDrawnOrdersFunc func(c *gin.Context, boxID int64) ([]mall.MallDrawOrder, error)

	listBoxCalls int
}

func (f *fakeOrderRepo) ListBoxDrawnOrders(c *gin.Context, boxID int64) ([]mall.MallDrawOrder, error) {
	f.listBoxCalls++
	if f.listBoxDrawnOrdersFunc != nil {
		return f.listBoxDrawnOrdersFunc(c, boxID)
	}
	return nil, nil
}

// newBillService 组装被测 service。
func newBillService(b *fakeBillRepo, x *fakeBoxRepo, s *fakeSupplierRepo, o *fakeOrderRepo) BillService {
	return NewBillService(BillServiceDeps{
		BillRepo:     b,
		BoxRepo:      x,
		SupplierRepo: s,
		OrderRepo:    o,
	})
}

// soldOutBox 返回一个售罄商品 fixture。
func soldOutBox() *mall.MallBlindBox {
	return &mall.MallBlindBox{ID: 9, SupplierID: 3, Lifecycle: mall.LifecycleSoldOut}
}

// supplierWithRate 返回显式费率供应商 fixture。
func supplierWithRate(rate float64) *mall.MallSupplier {
	return &mall.MallSupplier{ID: 3, CommissionRate: &rate}
}

// =============================================================================
// GenerateForBox
// =============================================================================

// TestGenerateForBox_Idempotent 已有账单 → 直接返回，不重查订单、不重建。
func TestGenerateForBox_Idempotent(t *testing.T) {
	existing := &mall.MallSettlementBill{ID: 77, BlindBoxID: 9, Status: mall.BillStatusConfirmed}
	b := &fakeBillRepo{getByBoxIDFunc: func(c *gin.Context, boxID int64) (*mall.MallSettlementBill, error) {
		return existing, nil
	}}
	o := &fakeOrderRepo{}

	got, err := newBillService(b, &fakeBoxRepo{}, &fakeSupplierRepo{}, o).GenerateForBox(newBillPlainCtx(), 9)
	if err != nil {
		t.Fatalf("GenerateForBox: %v", err)
	}
	if got != existing || got.ID != 77 {
		t.Errorf("got = %+v, want existing bill", got)
	}
	if o.listBoxCalls != 0 || b.createCalls != 0 {
		t.Errorf("idempotent skip violated: listBoxCalls=%d createCalls=%d, want 0/0", o.listBoxCalls, b.createCalls)
	}
}

// TestGenerateForBox_BoxNotFound 商品不存在 → 报错。
func TestGenerateForBox_BoxNotFound(t *testing.T) {
	b := &fakeBillRepo{} // GetByBoxID → ErrBillNotFound
	x := &fakeBoxRepo{}  // GetByID → gorm.ErrRecordNotFound

	if _, err := newBillService(b, x, &fakeSupplierRepo{}, &fakeOrderRepo{}).GenerateForBox(newBillPlainCtx(), 9); err == nil {
		t.Errorf("GenerateForBox(box missing) = nil, want err")
	}
}

// TestGenerateForBox_NotSoldOut 商品仍在售 → 拒绝生成账单。
func TestGenerateForBox_NotSoldOut(t *testing.T) {
	b := &fakeBillRepo{}
	box := soldOutBox()
	box.Lifecycle = mall.LifecycleSelling
	x := &fakeBoxRepo{getByIDFunc: func(c *gin.Context, id int64) (*mall.MallBlindBox, error) {
		return box, nil
	}}

	_, err := newBillService(b, x, &fakeSupplierRepo{}, &fakeOrderRepo{}).GenerateForBox(newBillPlainCtx(), 9)
	if !errors.Is(err, ErrBoxNotSoldOut) {
		t.Errorf("GenerateForBox(selling) = %v, want ErrBoxNotSoldOut", err)
	}
}

// TestGenerateForBox_NoOrders 售罄但 0 订单 = 脏数据 → 拒绝生成空账单。
func TestGenerateForBox_NoOrders(t *testing.T) {
	withBillTestConfig(t)
	b := &fakeBillRepo{}
	x := &fakeBoxRepo{getByIDFunc: func(c *gin.Context, id int64) (*mall.MallBlindBox, error) {
		return soldOutBox(), nil
	}}
	s := &fakeSupplierRepo{getByIDFunc: func(c *gin.Context, id int64) (*mall.MallSupplier, error) {
		return supplierWithRate(0.1), nil
	}}
	o := &fakeOrderRepo{} // 0 订单

	_, err := newBillService(b, x, s, o).GenerateForBox(newBillPlainCtx(), 9)
	if !errors.Is(err, ErrNoOrdersForBill) {
		t.Errorf("GenerateForBox(no orders) = %v, want ErrNoOrdersForBill", err)
	}
}

// TestGenerateForBox_Happy 正常路径：3 订单聚合 → 金额快照 → 建账单 + 明细。
//
// orders: 100/200/70，rate=0.1 → total=370, commission=37, payout=333；
// items 分笔快照：10/20/7 手续费，90/180/63 打款。
func TestGenerateForBox_Happy(t *testing.T) {
	withBillTestConfig(t)
	b := &fakeBillRepo{}
	x := &fakeBoxRepo{getByIDFunc: func(c *gin.Context, id int64) (*mall.MallBlindBox, error) {
		return soldOutBox(), nil
	}}
	s := &fakeSupplierRepo{getByIDFunc: func(c *gin.Context, id int64) (*mall.MallSupplier, error) {
		return supplierWithRate(0.1), nil
	}}
	o := &fakeOrderRepo{listBoxDrawnOrdersFunc: func(c *gin.Context, boxID int64) ([]mall.MallDrawOrder, error) {
		return []mall.MallDrawOrder{
			{ID: 101, BlindBoxID: 9, SupplierID: 3, Price: 100, Status: mall.OrderStatusDrawn},
			{ID: 102, BlindBoxID: 9, SupplierID: 3, Price: 200, Status: mall.OrderStatusDrawn},
			{ID: 103, BlindBoxID: 9, SupplierID: 3, Price: 70, Status: mall.OrderStatusDrawn},
		}, nil
	}}

	got, err := newBillService(b, x, s, o).GenerateForBox(newBillPlainCtx(), 9)
	if err != nil {
		t.Fatalf("GenerateForBox: %v", err)
	}

	bill := b.lastCreateBill
	if bill == nil || b.createCalls != 1 {
		t.Fatalf("CreateWithItems calls = %d, bill = %+v", b.createCalls, bill)
	}
	if bill.BlindBoxID != 9 || bill.SupplierID != 3 {
		t.Errorf("bill box/supplier = %d/%d, want 9/3", bill.BlindBoxID, bill.SupplierID)
	}
	if bill.Status != mall.BillStatusGenerated {
		t.Errorf("bill status = %s, want generated", bill.Status)
	}
	if bill.TotalAmount != 370 || bill.CommissionRate != 0.1 || bill.CommissionAmount != 37 || bill.PayoutAmount != 333 {
		t.Errorf("amounts = %v/%v/%v/%v, want 370/0.1/37/333",
			bill.TotalAmount, bill.CommissionRate, bill.CommissionAmount, bill.PayoutAmount)
	}
	if len(b.lastCreateItems) != 3 {
		t.Fatalf("items = %d, want 3", len(b.lastCreateItems))
	}
	want := []struct {
		orderID    int64
		amount     float64
		commission float64
		payout     float64
	}{{101, 100, 10, 90}, {102, 200, 20, 180}, {103, 70, 7, 63}}
	for i, w := range want {
		it := b.lastCreateItems[i]
		if it.DrawOrderID != w.orderID || it.Amount != w.amount || it.CommissionAmount != w.commission || it.PayoutAmount != w.payout {
			t.Errorf("items[%d] = %+v, want %+v", i, it, w)
		}
	}
	if got.ID != 77 {
		t.Errorf("returned bill ID = %d, want 77", got.ID)
	}
}

// TestGenerateForBox_DefaultRate supplier 未配置费率 → 走 config 默认值。
func TestGenerateForBox_DefaultRate(t *testing.T) {
	withBillTestConfig(t)
	b := &fakeBillRepo{}
	x := &fakeBoxRepo{getByIDFunc: func(c *gin.Context, id int64) (*mall.MallBlindBox, error) {
		return soldOutBox(), nil
	}}
	s := &fakeSupplierRepo{getByIDFunc: func(c *gin.Context, id int64) (*mall.MallSupplier, error) {
		return &mall.MallSupplier{ID: 3, CommissionRate: nil}, nil // NULL → config 默认
	}}
	o := &fakeOrderRepo{listBoxDrawnOrdersFunc: func(c *gin.Context, boxID int64) ([]mall.MallDrawOrder, error) {
		return []mall.MallDrawOrder{{ID: 101, Price: 100}}, nil
	}}

	if _, err := newBillService(b, x, s, o).GenerateForBox(newBillPlainCtx(), 9); err != nil {
		t.Fatalf("GenerateForBox: %v", err)
	}
	if bill := b.lastCreateBill; bill == nil || bill.CommissionRate != 0.10 || bill.CommissionAmount != 10 {
		t.Errorf("default rate snapshot = %+v, want rate 0.10 / commission 10", bill)
	}
}

// TestGenerateForBox_Conflict 并发生成 UK 冲突 → 原样上抛 ErrBillConflict。
func TestGenerateForBox_Conflict(t *testing.T) {
	withBillTestConfig(t)
	b := &fakeBillRepo{createWithItemsF: func(c *gin.Context, bill *mall.MallSettlementBill, items []mall.MallSettlementBillItem) error {
		return mallRepo.ErrBillConflict
	}}
	x := &fakeBoxRepo{getByIDFunc: func(c *gin.Context, id int64) (*mall.MallBlindBox, error) {
		return soldOutBox(), nil
	}}
	s := &fakeSupplierRepo{getByIDFunc: func(c *gin.Context, id int64) (*mall.MallSupplier, error) {
		return supplierWithRate(0.1), nil
	}}
	o := &fakeOrderRepo{listBoxDrawnOrdersFunc: func(c *gin.Context, boxID int64) ([]mall.MallDrawOrder, error) {
		return []mall.MallDrawOrder{{ID: 101, Price: 100}}, nil
	}}

	_, err := newBillService(b, x, s, o).GenerateForBox(newBillPlainCtx(), 9)
	if !errors.Is(err, mallRepo.ErrBillConflict) {
		t.Errorf("GenerateForBox conflict = %v, want ErrBillConflict", err)
	}
}

// =============================================================================
// MarkPaid
// =============================================================================

// confirmedBill 返回一个待打款账单 fixture。
func confirmedBill() *mall.MallSettlementBill {
	return &mall.MallSettlementBill{
		ID:           77,
		BlindBoxID:   9,
		SupplierID:   3,
		PayoutAmount: 333,
		Status:       mall.BillStatusConfirmed,
	}
}

// TestMarkPaid_Happy 正常打款：paid CAS + 扣 balance + 商品置 settled 同事务。
func TestMarkPaid_Happy(t *testing.T) {
	c, mock := newBillTestCtx(t)
	mock.ExpectBegin()
	mock.ExpectCommit()

	b := &fakeBillRepo{getByIDFunc: func(c *gin.Context, id int64) (*mall.MallSettlementBill, error) {
		return confirmedBill(), nil
	}}

	if err := newBillService(b, &fakeBoxRepo{}, &fakeSupplierRepo{}, &fakeOrderRepo{}).MarkPaid(c, 77, 1); err != nil {
		t.Fatalf("MarkPaid: %v", err)
	}
	if b.markPaidCalls != 1 || b.deductCalls != 1 || b.setLifecycleCalls != 1 {
		t.Fatalf("calls = %d/%d/%d, want 1/1/1", b.markPaidCalls, b.deductCalls, b.setLifecycleCalls)
	}
	if b.lastLifecycleFrom != mall.LifecycleSoldOut || b.lastLifecycleTo != mall.LifecycleSettled {
		t.Errorf("lifecycle = %s -> %s, want sold_out -> settled", b.lastLifecycleFrom, b.lastLifecycleTo)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}

// TestMarkPaid_NotFound 账单不存在 → ErrBillNotFound。
func TestMarkPaid_NotFound(t *testing.T) {
	c, mock := newBillTestCtx(t)

	b := &fakeBillRepo{} // GetByID → gorm.ErrRecordNotFound
	err := newBillService(b, &fakeBoxRepo{}, &fakeSupplierRepo{}, &fakeOrderRepo{}).MarkPaid(c, 999, 1)
	if !errors.Is(err, ErrBillNotFound) {
		t.Errorf("MarkPaid not found = %v, want ErrBillNotFound", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v (expected zero SQL calls)", err)
	}
}

// TestMarkPaid_InvalidTransition generated 态直接打款 → 状态机拒绝（未确认不可打款）。
func TestMarkPaid_InvalidTransition(t *testing.T) {
	c, mock := newBillTestCtx(t)

	bill := confirmedBill()
	bill.Status = mall.BillStatusGenerated
	b := &fakeBillRepo{getByIDFunc: func(c *gin.Context, id int64) (*mall.MallSettlementBill, error) {
		return bill, nil
	}}

	err := newBillService(b, &fakeBoxRepo{}, &fakeSupplierRepo{}, &fakeOrderRepo{}).MarkPaid(c, 77, 1)
	if !errors.Is(err, state.ErrInvalidStateTransition) {
		t.Errorf("MarkPaid(generated) = %v, want ErrInvalidStateTransition", err)
	}
	if b.markPaidCalls != 0 {
		t.Errorf("markPaidCalls = %d, want 0 (state check must short-circuit)", b.markPaidCalls)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v (expected zero SQL calls)", err)
	}
}

// TestMarkPaid_AlreadyPaid 二次打款 → 状态机拒绝。
func TestMarkPaid_AlreadyPaid(t *testing.T) {
	c, _ := newBillTestCtx(t)

	bill := confirmedBill()
	bill.Status = mall.BillStatusPaid
	b := &fakeBillRepo{getByIDFunc: func(c *gin.Context, id int64) (*mall.MallSettlementBill, error) {
		return bill, nil
	}}

	if err := newBillService(b, &fakeBoxRepo{}, &fakeSupplierRepo{}, &fakeOrderRepo{}).MarkPaid(c, 77, 1); !errors.Is(err, state.ErrInvalidStateTransition) {
		t.Errorf("MarkPaid(paid) = %v, want ErrInvalidStateTransition", err)
	}
}

// TestMarkPaid_CASMisss 并发抢先（paid CAS 0 行）→ ErrBillStatusConflict + 回滚。
func TestMarkPaid_CASMisss(t *testing.T) {
	c, mock := newBillTestCtx(t)
	mock.ExpectBegin()
	mock.ExpectRollback()

	b := &fakeBillRepo{
		getByIDFunc: func(c *gin.Context, id int64) (*mall.MallSettlementBill, error) {
			return confirmedBill(), nil
		},
		markPaidFunc: func(c *gin.Context, id int64, adminID int64) (int64, error) {
			return 0, nil // CAS 未命中
		},
	}

	err := newBillService(b, &fakeBoxRepo{}, &fakeSupplierRepo{}, &fakeOrderRepo{}).MarkPaid(c, 77, 1)
	if !errors.Is(err, ErrBillStatusConflict) {
		t.Errorf("MarkPaid CAS miss = %v, want ErrBillStatusConflict", err)
	}
	if b.deductCalls != 0 || b.setLifecycleCalls != 0 {
		t.Errorf("deduct/lifecycle calls = %d/%d, want 0/0 (must abort before them)", b.deductCalls, b.setLifecycleCalls)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}

// TestMarkPaid_InsufficientBalance 余额不足 → ErrInsufficientSupplierBalance + 回滚。
func TestMarkPaid_InsufficientBalance(t *testing.T) {
	c, mock := newBillTestCtx(t)
	mock.ExpectBegin()
	mock.ExpectRollback()

	b := &fakeBillRepo{
		getByIDFunc: func(c *gin.Context, id int64) (*mall.MallSettlementBill, error) {
			return confirmedBill(), nil
		},
		deductFunc: func(c *gin.Context, supplierID int64, amount float64) (int64, error) {
			return 0, nil // 余额不足
		},
	}

	err := newBillService(b, &fakeBoxRepo{}, &fakeSupplierRepo{}, &fakeOrderRepo{}).MarkPaid(c, 77, 1)
	if !errors.Is(err, ErrInsufficientSupplierBalance) {
		t.Errorf("MarkPaid insufficient = %v, want ErrInsufficientSupplierBalance", err)
	}
	if b.setLifecycleCalls != 0 {
		t.Errorf("setLifecycleCalls = %d, want 0 (must abort)", b.setLifecycleCalls)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}

// TestMarkPaid_ZeroPayout payout=0 → 跳过扣款，仍置 settled。
func TestMarkPaid_ZeroPayout(t *testing.T) {
	c, mock := newBillTestCtx(t)
	mock.ExpectBegin()
	mock.ExpectCommit()

	bill := confirmedBill()
	bill.PayoutAmount = 0
	b := &fakeBillRepo{getByIDFunc: func(c *gin.Context, id int64) (*mall.MallSettlementBill, error) {
		return bill, nil
	}}

	if err := newBillService(b, &fakeBoxRepo{}, &fakeSupplierRepo{}, &fakeOrderRepo{}).MarkPaid(c, 77, 1); err != nil {
		t.Fatalf("MarkPaid: %v", err)
	}
	if b.deductCalls != 0 {
		t.Errorf("deductCalls = %d, want 0 (zero payout)", b.deductCalls)
	}
	if b.setLifecycleCalls != 1 {
		t.Errorf("setLifecycleCalls = %d, want 1", b.setLifecycleCalls)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}

// =============================================================================
// FinalizeSoldOut（draw.go 7.7 售罄落定）
// =============================================================================

// TestFinalizeSoldOut_Happy 正常落定：CAS selling→sold_out + 幂等建账单。
func TestFinalizeSoldOut_Happy(t *testing.T) {
	withBillTestConfig(t)
	b := &fakeBillRepo{}
	x := &fakeBoxRepo{getByIDFunc: func(c *gin.Context, id int64) (*mall.MallBlindBox, error) {
		return soldOutBox(), nil
	}}
	s := &fakeSupplierRepo{getByIDFunc: func(c *gin.Context, id int64) (*mall.MallSupplier, error) {
		return supplierWithRate(0.1), nil
	}}
	o := &fakeOrderRepo{listBoxDrawnOrdersFunc: func(c *gin.Context, boxID int64) ([]mall.MallDrawOrder, error) {
		return []mall.MallDrawOrder{{ID: 101, Price: 100}}, nil
	}}

	if err := newBillService(b, x, s, o).FinalizeSoldOut(newBillPlainCtx(), 9); err != nil {
		t.Fatalf("FinalizeSoldOut: %v", err)
	}
	if b.setLifecycleCalls != 1 || b.lastLifecycleFrom != mall.LifecycleSelling || b.lastLifecycleTo != mall.LifecycleSoldOut {
		t.Errorf("lifecycle CAS = %d call(s), %s -> %s, want 1 call, selling -> sold_out",
			b.setLifecycleCalls, b.lastLifecycleFrom, b.lastLifecycleTo)
	}
	if b.createCalls != 1 {
		t.Errorf("createCalls = %d, want 1 (bill generated)", b.createCalls)
	}
}

// TestFinalizeSoldOut_Idempotent 重复触发：CAS 0 行（已售罄）+ 账单已存在 → 静默幂等。
func TestFinalizeSoldOut_Idempotent(t *testing.T) {
	withBillTestConfig(t)
	existing := &mall.MallSettlementBill{ID: 77, BlindBoxID: 9}
	b := &fakeBillRepo{
		setLifecycleFunc: func(c *gin.Context, boxID int64, from, to mall.MallBlindBoxLifecycle) (int64, error) {
			return 0, nil // CAS 未命中：lifecycle 已是 sold_out
		},
		getByBoxIDFunc: func(c *gin.Context, boxID int64) (*mall.MallSettlementBill, error) {
			return existing, nil // 账单已存在
		},
	}
	o := &fakeOrderRepo{}

	if err := newBillService(b, &fakeBoxRepo{}, &fakeSupplierRepo{}, o).FinalizeSoldOut(newBillPlainCtx(), 9); err != nil {
		t.Fatalf("FinalizeSoldOut idempotent: %v", err)
	}
	if b.createCalls != 0 || o.listBoxCalls != 0 {
		t.Errorf("createCalls/listBoxCalls = %d/%d, want 0/0 (fully idempotent)", b.createCalls, o.listBoxCalls)
	}
}

// TestFinalizeSoldOut_InvalidBox 非法 boxID 短路。
func TestFinalizeSoldOut_InvalidBox(t *testing.T) {
	b := &fakeBillRepo{}
	if err := newBillService(b, &fakeBoxRepo{}, &fakeSupplierRepo{}, &fakeOrderRepo{}).FinalizeSoldOut(newBillPlainCtx(), 0); err == nil {
		t.Errorf("FinalizeSoldOut(boxID=0) = nil, want err")
	}
}

// =============================================================================
// Regenerate
// =============================================================================

// rejectedBill 返回一个被驳回账单 fixture。
func rejectedBill() *mall.MallSettlementBill {
	return &mall.MallSettlementBill{
		ID:           77,
		BlindBoxID:   9,
		SupplierID:   3,
		PayoutAmount: 333,
		Status:       mall.BillStatusRejected,
		Version:      1,
	}
}

// TestRegenerate_Happy 驳回后重核：rejected → generated。
func TestRegenerate_Happy(t *testing.T) {
	b := &fakeBillRepo{getByIDFunc: func(c *gin.Context, id int64) (*mall.MallSettlementBill, error) {
		return rejectedBill(), nil
	}}

	amounts := mallRepo.BillRegenAmounts{TotalAmount: 10, CommissionRate: 0.1, CommissionAmount: 1, PayoutAmount: 9}
	if err := newBillService(b, &fakeBoxRepo{}, &fakeSupplierRepo{}, &fakeOrderRepo{}).Regenerate(newBillPlainCtx(), 77, amounts, "金额修正"); err != nil {
		t.Fatalf("Regenerate: %v", err)
	}
}

// TestRegenerate_NotRejected 非 rejected 态 → 状态机拒绝。
func TestRegenerate_NotRejected(t *testing.T) {
	bill := rejectedBill()
	bill.Status = mall.BillStatusConfirmed
	b := &fakeBillRepo{getByIDFunc: func(c *gin.Context, id int64) (*mall.MallSettlementBill, error) {
		return bill, nil
	}}

	err := newBillService(b, &fakeBoxRepo{}, &fakeSupplierRepo{}, &fakeOrderRepo{}).Regenerate(
		newBillPlainCtx(), 77, mallRepo.BillRegenAmounts{}, "")
	if !errors.Is(err, state.ErrInvalidStateTransition) {
		t.Errorf("Regenerate(confirmed) = %v, want ErrInvalidStateTransition", err)
	}
}

// TestRegenerate_CASMisss 重核期间状态被并发改变 → ErrBillStatusConflict。
func TestRegenerate_CASMisss(t *testing.T) {
	b := &fakeBillRepo{
		getByIDFunc: func(c *gin.Context, id int64) (*mall.MallSettlementBill, error) {
			return rejectedBill(), nil
		},
		regenerateFunc: func(c *gin.Context, id int64, amounts mallRepo.BillRegenAmounts, remark string) (int64, error) {
			return 0, nil // CAS 未命中
		},
	}

	err := newBillService(b, &fakeBoxRepo{}, &fakeSupplierRepo{}, &fakeOrderRepo{}).Regenerate(
		newBillPlainCtx(), 77, mallRepo.BillRegenAmounts{TotalAmount: 10}, "")
	if !errors.Is(err, ErrBillStatusConflict) {
		t.Errorf("Regenerate CAS miss = %v, want ErrBillStatusConflict", err)
	}
}
