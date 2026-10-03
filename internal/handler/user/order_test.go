package user

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"ai-go-mall/internal/model"
	"ai-go-mall/internal/model/mall"
	"ai-go-mall/internal/repository"
	userSvc "ai-go-mall/internal/service/user"
)

// mockOrderService 实现 userSvc.OrderService。
type mockOrderService struct {
	listFunc   func(c *gin.Context, userID int64, opts repository.ListOptions) ([]mall.MallDrawOrder, int64, error)
	detailFunc func(c *gin.Context, userID, orderID int64) (*mall.MallDrawOrder, []mall.MallDrawOrderItem, error)

	listCalls   int
	detailCalls int
	lastUserID  int64
	lastOrderID int64
}

func (m *mockOrderService) List(c *gin.Context, userID int64, opts repository.ListOptions) ([]mall.MallDrawOrder, int64, error) {
	m.listCalls++
	m.lastUserID = userID
	if m.listFunc != nil {
		return m.listFunc(c, userID, opts)
	}
	return []mall.MallDrawOrder{}, 0, nil
}

func (m *mockOrderService) Detail(c *gin.Context, userID, orderID int64) (*mall.MallDrawOrder, []mall.MallDrawOrderItem, error) {
	m.detailCalls++
	m.lastUserID = userID
	m.lastOrderID = orderID
	if m.detailFunc != nil {
		return m.detailFunc(c, userID, orderID)
	}
	return &mall.MallDrawOrder{ID: orderID}, []mall.MallDrawOrderItem{}, nil
}

var _ userSvc.OrderService = (*mockOrderService)(nil)

// withUser 模拟 UserAuth 中间件写 context（跳过真实 token 校验）。
func withUser(userID int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		if userID > 0 {
			c.Set("user.current", &model.User{ID: userID, Username: "alice", Status: 1})
		}
		c.Next()
	}
}

// doOrderGET 在新 engine 上注册路由并执行 GET。
func doOrderGET(t *testing.T, path string, register func(r *gin.Engine)) (int, map[string]any) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	register(r)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	return w.Code, resp
}

func TestOrderList_HappyPath(t *testing.T) {
	svc := &mockOrderService{
		listFunc: func(c *gin.Context, userID int64, opts repository.ListOptions) ([]mall.MallDrawOrder, int64, error) {
			if userID != 42 {
				t.Errorf("userID = %d, want 42", userID)
			}
			if opts.Page != 1 || opts.PageSize != 20 {
				t.Errorf("opts = %+v", opts)
			}
			return []mall.MallDrawOrder{
				{ID: 100, OrderNo: "D1", Price: 79, Status: "drawn"},
			}, 1, nil
		},
	}
	h := NewOrderHandler(svc)

	code, resp := doOrderGET(t, "/user/orders/list", func(r *gin.Engine) {
		r.GET("/user/orders/list", withUser(42), h.List)
	})

	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200", code)
	}
	if svc.lastUserID != 42 {
		t.Errorf("svc userID = %d, want 42", svc.lastUserID)
	}
	if resp["total"] != float64(1) {
		t.Errorf("total = %v", resp["total"])
	}
	items := resp["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("items len = %d", len(items))
	}
	first := items[0].(map[string]any)
	if first["order_no"] != "D1" {
		t.Errorf("order_no = %v", first["order_no"])
	}
}

func TestOrderList_NoUser_401(t *testing.T) {
	svc := &mockOrderService{}
	h := NewOrderHandler(svc)

	code, body := doOrderGET(t, "/user/orders/list", func(r *gin.Engine) {
		r.GET("/user/orders/list", withUser(0), h.List)
	})
	if code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", code)
	}
	if body["code"] != "order.unauthorized" {
		t.Errorf("code = %v", body["code"])
	}
	if svc.listCalls != 0 {
		t.Errorf("List should NOT be called without user")
	}
}

func TestOrderList_ServiceError_500(t *testing.T) {
	svc := &mockOrderService{
		listFunc: func(c *gin.Context, userID int64, opts repository.ListOptions) ([]mall.MallDrawOrder, int64, error) {
			return nil, 0, errors.New("db: down")
		},
	}
	h := NewOrderHandler(svc)

	code, _ := doOrderGET(t, "/user/orders/list", func(r *gin.Engine) {
		r.GET("/user/orders/list", withUser(42), h.List)
	})
	if code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", code)
	}
}

func TestOrderDetail_HappyPath(t *testing.T) {
	svc := &mockOrderService{
		detailFunc: func(c *gin.Context, userID, orderID int64) (*mall.MallDrawOrder, []mall.MallDrawOrderItem, error) {
			if userID != 42 || orderID != 100 {
				t.Errorf("userID=%d orderID=%d, want 42/100", userID, orderID)
			}
			return &mall.MallDrawOrder{ID: 100, OrderNo: "D1"}, []mall.MallDrawOrderItem{
				{ID: 1, CardID: 5, Rarity: mall.RaritySSR, SnapshotName: "签名卡"},
			}, nil
		},
	}
	h := NewOrderHandler(svc)

	code, resp := doOrderGET(t, "/user/orders/detail?id=100", func(r *gin.Engine) {
		r.GET("/user/orders/detail", withUser(42), h.Detail)
	})

	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200", code)
	}
	if svc.lastOrderID != 100 {
		t.Errorf("svc orderID = %d", svc.lastOrderID)
	}
	order, ok := resp["order"].(map[string]any)
	if !ok || order["order_no"] != "D1" {
		t.Fatalf("order missing or wrong: %v", resp["order"])
	}
	items, ok := resp["items"].([]any)
	if !ok || len(items) != 1 {
		t.Fatalf("items missing: %v", resp["items"])
	}
	first := items[0].(map[string]any)
	if first["snapshot_name"] != "签名卡" || first["rarity"] != "SSR" {
		t.Errorf("item = %v", first)
	}
}

func TestOrderDetail_InvalidID_400(t *testing.T) {
	svc := &mockOrderService{}
	h := NewOrderHandler(svc)

	code, _ := doOrderGET(t, "/user/orders/detail?id=abc", func(r *gin.Engine) {
		r.GET("/user/orders/detail", withUser(42), h.Detail)
	})
	if code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", code)
	}
	if svc.detailCalls != 0 {
		t.Errorf("Detail should NOT be called")
	}
}

func TestOrderDetail_NotFound_404(t *testing.T) {
	svc := &mockOrderService{
		detailFunc: func(c *gin.Context, userID, orderID int64) (*mall.MallDrawOrder, []mall.MallDrawOrderItem, error) {
			return nil, nil, userSvc.ErrOrderNotFound
		},
	}
	h := NewOrderHandler(svc)

	code, body := doOrderGET(t, "/user/orders/detail?id=999", func(r *gin.Engine) {
		r.GET("/user/orders/detail", withUser(42), h.Detail)
	})
	if code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", code)
	}
	if body["code"] != "order.not_found" {
		t.Errorf("code = %v, want order.not_found", body["code"])
	}
}

func TestOrderDetail_ServiceError_500(t *testing.T) {
	svc := &mockOrderService{
		detailFunc: func(c *gin.Context, userID, orderID int64) (*mall.MallDrawOrder, []mall.MallDrawOrderItem, error) {
			return nil, nil, errors.New("db: down")
		},
	}
	h := NewOrderHandler(svc)

	code, _ := doOrderGET(t, "/user/orders/detail?id=1", func(r *gin.Engine) {
		r.GET("/user/orders/detail", withUser(42), h.Detail)
	})
	if code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", code)
	}
}
