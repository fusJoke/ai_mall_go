package user

import (
	"errors"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"ai-go-mall/internal/model/mall"
	"ai-go-mall/internal/repository"
	mallRepo "ai-go-mall/internal/repository/mall"
)

// =============================================================================
// helpers
// =============================================================================

func newOrderCtx() *gin.Context {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", "/api/v1/user/orders", nil)
	return c
}

// =============================================================================
// Mock DrawOrderRepository
// =============================================================================

type orderDrawOrderRepo struct {
	getByIDFunc          func(c *gin.Context, id int64) (*mall.MallDrawOrder, error)
	listByUserFunc       func(c *gin.Context, uid int64, opts repository.ListOptions) ([]mall.MallDrawOrder, int64, error)
	listItemsByOrderFunc func(c *gin.Context, orderID int64) ([]mall.MallDrawOrderItem, error)

	getByIDCalls          int
	listByUserCalls       int
	listItemsByOrderCalls int
}

func (m *orderDrawOrderRepo) GetByID(c *gin.Context, id int64) (*mall.MallDrawOrder, error) {
	m.getByIDCalls++
	if m.getByIDFunc != nil {
		return m.getByIDFunc(c, id)
	}
	return nil, gorm.ErrRecordNotFound
}
func (m *orderDrawOrderRepo) GetByOrderNo(c *gin.Context, no string) (*mall.MallDrawOrder, error) {
	return nil, gorm.ErrRecordNotFound
}
func (m *orderDrawOrderRepo) CreateOrder(c *gin.Context, order *mall.MallDrawOrder) error {
	return nil
}
func (m *orderDrawOrderRepo) ListByUser(c *gin.Context, uid int64, opts repository.ListOptions) ([]mall.MallDrawOrder, int64, error) {
	m.listByUserCalls++
	if m.listByUserFunc != nil {
		return m.listByUserFunc(c, uid, opts)
	}
	return nil, 0, nil
}
func (m *orderDrawOrderRepo) UpdateStatus(c *gin.Context, id int64, s mall.MallDrawOrderStatus) error {
	return nil
}
func (m *orderDrawOrderRepo) ListItemsByOrderID(c *gin.Context, orderID int64) ([]mall.MallDrawOrderItem, error) {
	m.listItemsByOrderCalls++
	if m.listItemsByOrderFunc != nil {
		return m.listItemsByOrderFunc(c, orderID)
	}
	return nil, nil
}
func (m *orderDrawOrderRepo) CreateItems(c *gin.Context, items []mall.MallDrawOrderItem) error {
	return nil
}
func (m *orderDrawOrderRepo) ListSettledOrdersBySupplierInRange(c *gin.Context, sid int64, s, e time.Time) ([]mall.MallDrawOrder, error) {
	return nil, nil
}
func (m *orderDrawOrderRepo) Create(c *gin.Context, e *mall.MallDrawOrder) error { return nil }
func (m *orderDrawOrderRepo) List(c *gin.Context, opts repository.ListOptions) ([]mall.MallDrawOrder, int64, error) {
	return nil, 0, nil
}
func (m *orderDrawOrderRepo) Update(c *gin.Context, e *mall.MallDrawOrder) error { return nil }
func (m *orderDrawOrderRepo) Delete(c *gin.Context, id int64) error              { return nil }

var _ mallRepo.DrawOrderRepository = (*orderDrawOrderRepo)(nil)

// =============================================================================
// 测试用例
// =============================================================================

func newOrderSvc(repo mallRepo.DrawOrderRepository) OrderService {
	return NewOrderService(OrderServiceDeps{DrawOrderRepo: repo})
}

func TestOrderList_InvalidUserID(t *testing.T) {
	repo := &orderDrawOrderRepo{}
	svc := newOrderSvc(repo)
	items, total, err := svc.List(newOrderCtx(), 0, repository.ListOptions{Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("List err = %v, want nil", err)
	}
	if len(items) != 0 {
		t.Errorf("len(items) = %d, want 0", len(items))
	}
	if total != 0 {
		t.Errorf("total = %d, want 0", total)
	}
	if repo.listByUserCalls != 0 {
		t.Errorf("repo.ListByUser called %d times, want 0 (short-circuited)", repo.listByUserCalls)
	}
}

func TestOrderList_NegativeUserID(t *testing.T) {
	repo := &orderDrawOrderRepo{}
	svc := newOrderSvc(repo)
	items, total, err := svc.List(newOrderCtx(), -1, repository.ListOptions{Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("List err = %v, want nil", err)
	}
	if len(items) != 0 || total != 0 {
		t.Errorf("got (%d, %d), want (0, 0)", len(items), total)
	}
	if repo.listByUserCalls != 0 {
		t.Errorf("repo.ListByUser called %d times, want 0", repo.listByUserCalls)
	}
}

func TestOrderList_DelegatesToRepo(t *testing.T) {
	wantItems := []mall.MallDrawOrder{
		{ID: 1, UserID: 7, OrderNo: "o-1", Price: 100},
		{ID: 2, UserID: 7, OrderNo: "o-2", Price: 80},
	}
	repo := &orderDrawOrderRepo{
		listByUserFunc: func(c *gin.Context, uid int64, opts repository.ListOptions) ([]mall.MallDrawOrder, int64, error) {
			if uid != 7 {
				t.Errorf("repo got uid=%d, want 7", uid)
			}
			if opts.Page != 2 || opts.PageSize != 5 {
				t.Errorf("repo got opts=(%d, %d), want (2, 5)", opts.Page, opts.PageSize)
			}
			return wantItems, 42, nil
		},
	}
	svc := newOrderSvc(repo)
	items, total, err := svc.List(newOrderCtx(), 7, repository.ListOptions{Page: 2, PageSize: 5})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if total != 42 {
		t.Errorf("total = %d, want 42", total)
	}
	if len(items) != 2 {
		t.Fatalf("len(items) = %d, want 2", len(items))
	}
	if items[0].OrderNo != "o-1" || items[1].OrderNo != "o-2" {
		t.Errorf("items = %+v", items)
	}
	if repo.listByUserCalls != 1 {
		t.Errorf("repo.ListByUser called %d times, want 1", repo.listByUserCalls)
	}
}

func TestOrderList_RepoErrorPropagates(t *testing.T) {
	wantErr := errors.New("repo: db down")
	repo := &orderDrawOrderRepo{
		listByUserFunc: func(c *gin.Context, uid int64, opts repository.ListOptions) ([]mall.MallDrawOrder, int64, error) {
			return nil, 0, wantErr
		},
	}
	svc := newOrderSvc(repo)
	_, _, err := svc.List(newOrderCtx(), 1, repository.ListOptions{Page: 1, PageSize: 10})
	if !errors.Is(err, wantErr) {
		t.Errorf("err = %v, want %v", err, wantErr)
	}
}

// =============================================================================
// Detail
// =============================================================================

func TestOrderDetail_InvalidUserID(t *testing.T) {
	repo := &orderDrawOrderRepo{}
	svc := newOrderSvc(repo)
	_, _, err := svc.Detail(newOrderCtx(), 0, 1)
	if !errors.Is(err, ErrOrderNotFound) {
		t.Errorf("userID=0 err = %v, want ErrOrderNotFound", err)
	}
	if repo.getByIDCalls != 0 {
		t.Errorf("repo.GetByID called %d times, want 0", repo.getByIDCalls)
	}
}

func TestOrderDetail_NegativeUserID(t *testing.T) {
	repo := &orderDrawOrderRepo{}
	svc := newOrderSvc(repo)
	_, _, err := svc.Detail(newOrderCtx(), -1, 1)
	if !errors.Is(err, ErrOrderNotFound) {
		t.Errorf("userID=-1 err = %v, want ErrOrderNotFound", err)
	}
}

func TestOrderDetail_InvalidOrderID(t *testing.T) {
	repo := &orderDrawOrderRepo{}
	svc := newOrderSvc(repo)
	_, _, err := svc.Detail(newOrderCtx(), 1, 0)
	if !errors.Is(err, ErrOrderNotFound) {
		t.Errorf("orderID=0 err = %v, want ErrOrderNotFound", err)
	}
	if repo.getByIDCalls != 0 {
		t.Errorf("repo.GetByID called %d times, want 0", repo.getByIDCalls)
	}
}

func TestOrderDetail_NegativeOrderID(t *testing.T) {
	repo := &orderDrawOrderRepo{}
	svc := newOrderSvc(repo)
	_, _, err := svc.Detail(newOrderCtx(), 1, -5)
	if !errors.Is(err, ErrOrderNotFound) {
		t.Errorf("orderID=-5 err = %v, want ErrOrderNotFound", err)
	}
}

// TestOrderDetail_NotFound 验证「订单不存在」统一映射到 ErrOrderNotFound。
func TestOrderDetail_NotFound(t *testing.T) {
	repo := &orderDrawOrderRepo{
		getByIDFunc: func(c *gin.Context, id int64) (*mall.MallDrawOrder, error) {
			return nil, gorm.ErrRecordNotFound
		},
	}
	svc := newOrderSvc(repo)
	_, _, err := svc.Detail(newOrderCtx(), 1, 99)
	if !errors.Is(err, ErrOrderNotFound) {
		t.Errorf("err = %v, want ErrOrderNotFound", err)
	}
	if repo.getByIDCalls != 1 {
		t.Errorf("repo.GetByID called %d times, want 1", repo.getByIDCalls)
	}
}

// TestOrderDetail_OtherRepoError 验证非 gorm.ErrRecordNotFound 的错误透传。
func TestOrderDetail_OtherRepoError(t *testing.T) {
	wantErr := errors.New("repo: timeout")
	repo := &orderDrawOrderRepo{
		getByIDFunc: func(c *gin.Context, id int64) (*mall.MallDrawOrder, error) {
			return nil, wantErr
		},
	}
	svc := newOrderSvc(repo)
	_, _, err := svc.Detail(newOrderCtx(), 1, 99)
	if !errors.Is(err, wantErr) {
		t.Errorf("err = %v, want %v", err, wantErr)
	}
}

// TestOrderDetail_RepoReturnsNilNoErr 防御性兜底：repo 返回 (nil, nil) 时也算 not found。
func TestOrderDetail_RepoReturnsNilNoErr(t *testing.T) {
	repo := &orderDrawOrderRepo{
		getByIDFunc: func(c *gin.Context, id int64) (*mall.MallDrawOrder, error) {
			return nil, nil
		},
	}
	svc := newOrderSvc(repo)
	_, _, err := svc.Detail(newOrderCtx(), 1, 99)
	if !errors.Is(err, ErrOrderNotFound) {
		t.Errorf("err = %v, want ErrOrderNotFound", err)
	}
}

// TestOrderDetail_UnauthorizedAccess 是核心安全测试：
//
// 越权读他人订单 → 必须统一返回 ErrOrderNotFound（不暴露订单存在性，防 ID 枚举）。
func TestOrderDetail_UnauthorizedAccess(t *testing.T) {
	repo := &orderDrawOrderRepo{
		getByIDFunc: func(c *gin.Context, id int64) (*mall.MallDrawOrder, error) {
			// 订单存在，但属于另一个用户。
			return &mall.MallDrawOrder{ID: id, OrderNo: "o-99", UserID: 999, Price: 100}, nil
		},
	}
	svc := newOrderSvc(repo)
	order, items, err := svc.Detail(newOrderCtx(), 1, 99)
	if !errors.Is(err, ErrOrderNotFound) {
		t.Errorf("err = %v, want ErrOrderNotFound (security: hide existence)", err)
	}
	if order != nil {
		t.Errorf("order = %+v, want nil", order)
	}
	if items != nil {
		t.Errorf("items = %+v, want nil", items)
	}
	// 越权读时不应再去查明细。
	if repo.listItemsByOrderCalls != 0 {
		t.Errorf("repo.ListItemsByOrderID called %d times, want 0", repo.listItemsByOrderCalls)
	}
}

// TestOrderDetail_Success 验证 owner 正确读到订单 + 明细。
func TestOrderDetail_Success(t *testing.T) {
	wantItems := []mall.MallDrawOrderItem{
		{ID: 11, OrderID: 7, CardID: 21, SnapshotName: "原始卡名"},
		{ID: 12, OrderID: 7, CardID: 22, SnapshotName: "第二张卡"},
	}
	repo := &orderDrawOrderRepo{
		getByIDFunc: func(c *gin.Context, id int64) (*mall.MallDrawOrder, error) {
			if id != 7 {
				t.Errorf("repo got id=%d, want 7", id)
			}
			return &mall.MallDrawOrder{
				ID: 7, UserID: 1, OrderNo: "o-7",
				BlindBoxID: 100, SupplierID: 200,
				Price: 100, Status: mall.OrderStatusDrawn,
			}, nil
		},
		listItemsByOrderFunc: func(c *gin.Context, orderID int64) ([]mall.MallDrawOrderItem, error) {
			if orderID != 7 {
				t.Errorf("repo got orderID=%d, want 7", orderID)
			}
			return wantItems, nil
		},
	}
	svc := newOrderSvc(repo)
	order, items, err := svc.Detail(newOrderCtx(), 1, 7)
	if err != nil {
		t.Fatalf("Detail: %v", err)
	}
	if order == nil || order.ID != 7 {
		t.Errorf("order = %+v, want ID=7", order)
	}
	if order.UserID != 1 {
		t.Errorf("order.UserID = %d, want 1", order.UserID)
	}
	if len(items) != 2 {
		t.Fatalf("len(items) = %d, want 2", len(items))
	}
	if items[0].SnapshotName != "原始卡名" {
		t.Errorf("items[0].SnapshotName = %q, want 原始卡名", items[0].SnapshotName)
	}
	if repo.getByIDCalls != 1 {
		t.Errorf("repo.GetByID called %d times, want 1", repo.getByIDCalls)
	}
	if repo.listItemsByOrderCalls != 1 {
		t.Errorf("repo.ListItemsByOrderID called %d times, want 1", repo.listItemsByOrderCalls)
	}
}

// TestOrderDetail_ItemsErrorPropagates 验证 owner 命中后拉明细失败时透传错误。
func TestOrderDetail_ItemsErrorPropagates(t *testing.T) {
	wantErr := errors.New("repo: items query failed")
	repo := &orderDrawOrderRepo{
		getByIDFunc: func(c *gin.Context, id int64) (*mall.MallDrawOrder, error) {
			return &mall.MallDrawOrder{ID: id, UserID: 1}, nil
		},
		listItemsByOrderFunc: func(c *gin.Context, orderID int64) ([]mall.MallDrawOrderItem, error) {
			return nil, wantErr
		},
	}
	svc := newOrderSvc(repo)
	_, _, err := svc.Detail(newOrderCtx(), 1, 7)
	if !errors.Is(err, wantErr) {
		t.Errorf("err = %v, want %v", err, wantErr)
	}
}
