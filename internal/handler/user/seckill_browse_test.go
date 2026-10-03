// Package user — seckill_browse_test.go 覆盖秒杀浏览端点
// （GET /user/seckill/list 与 /user/seckill/detail）。
package user

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"ai-go-mall/internal/repository"
	userSvc "ai-go-mall/internal/service/user"
)

// mockSeckillBrowseService 实现 userSvc.SeckillBrowseService。
type mockSeckillBrowseService struct {
	listFunc   func(c *gin.Context, opts repository.ListOptions) ([]userSvc.SeckillListItem, int64, error)
	detailFunc func(c *gin.Context, id int64) (*userSvc.SeckillDetail, error)

	lastListOpts repository.ListOptions
	lastDetailID int64
}

func (m *mockSeckillBrowseService) List(c *gin.Context, opts repository.ListOptions) ([]userSvc.SeckillListItem, int64, error) {
	m.lastListOpts = opts
	if m.listFunc != nil {
		return m.listFunc(c, opts)
	}
	return []userSvc.SeckillListItem{}, 0, nil
}

func (m *mockSeckillBrowseService) Detail(c *gin.Context, id int64) (*userSvc.SeckillDetail, error) {
	m.lastDetailID = id
	if m.detailFunc != nil {
		return m.detailFunc(c, id)
	}
	return &userSvc.SeckillDetail{
		SeckillListItem: userSvc.SeckillListItem{ID: id, SeckillPrice: 79, RemainingStock: 42, PerUserLimit: 1},
	}, nil
}

var _ userSvc.SeckillBrowseService = (*mockSeckillBrowseService)(nil)

func TestSeckillList_HappyPath(t *testing.T) {
	browse := &mockSeckillBrowseService{
		listFunc: func(c *gin.Context, opts repository.ListOptions) ([]userSvc.SeckillListItem, int64, error) {
			if opts.Page != 2 || opts.PageSize != 5 {
				t.Errorf("opts = %+v, want page=2 page_size=5", opts)
			}
			return []userSvc.SeckillListItem{
				{ID: 11, BlindBoxName: "NBA 全明星盲盒", SeckillPrice: 79, RemainingStock: 37, StartAt: time.Now().Add(-time.Hour), EndAt: time.Now().Add(time.Hour)},
			}, 1, nil
		},
	}
	h := NewSeckillHandler(&mockSeckillService{}, browse)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/user/seckill/list", h.List)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/user/seckill/list?page=2&page_size=5", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	var resp struct {
		Items []userSvc.SeckillListItem `json:"items"`
		Total int64                     `json:"total"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Total != 1 || len(resp.Items) != 1 || resp.Items[0].ID != 11 || resp.Items[0].RemainingStock != 37 {
		t.Errorf("resp = %+v", resp)
	}
}

func TestSeckillList_InternalError_500(t *testing.T) {
	browse := &mockSeckillBrowseService{
		listFunc: func(c *gin.Context, opts repository.ListOptions) ([]userSvc.SeckillListItem, int64, error) {
			return nil, 0, errors.New("db down")
		},
	}
	h := NewSeckillHandler(&mockSeckillService{}, browse)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/user/seckill/list", h.List)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/user/seckill/list", nil))

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", w.Code)
	}
	var body map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	if body["code"] != "seckill.internal" {
		t.Errorf("code = %v", body["code"])
	}
}

func TestSeckillDetail_HappyPath(t *testing.T) {
	browse := &mockSeckillBrowseService{
		detailFunc: func(c *gin.Context, id int64) (*userSvc.SeckillDetail, error) {
			if id != 11 {
				t.Errorf("id = %d, want 11", id)
			}
			return &userSvc.SeckillDetail{
				SeckillListItem: userSvc.SeckillListItem{ID: id, SeckillPrice: 79, TotalStock: 100, RemainingStock: 42, PerUserLimit: 1},
				Description:     "含签名卡",
			}, nil
		},
	}
	h := NewSeckillHandler(&mockSeckillService{}, browse)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/user/seckill/detail", h.Detail)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/user/seckill/detail?id=11", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	var resp userSvc.SeckillDetail
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.ID != 11 || resp.RemainingStock != 42 || resp.Description != "含签名卡" {
		t.Errorf("resp = %+v", resp)
	}
}

func TestSeckillDetail_Errors(t *testing.T) {
	t.Run("invalid_id_400", func(t *testing.T) {
		h := NewSeckillHandler(&mockSeckillService{}, &mockSeckillBrowseService{})
		gin.SetMode(gin.TestMode)
		r := gin.New()
		r.GET("/user/seckill/detail", h.Detail)

		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/user/seckill/detail?id=abc", nil))

		if w.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want 400", w.Code)
		}
		var body map[string]any
		_ = json.Unmarshal(w.Body.Bytes(), &body)
		if body["code"] != "seckill.invalid_input" {
			t.Errorf("code = %v", body["code"])
		}
	})
	t.Run("not_available_404", func(t *testing.T) {
		browse := &mockSeckillBrowseService{
			detailFunc: func(c *gin.Context, id int64) (*userSvc.SeckillDetail, error) {
				return nil, userSvc.ErrSeckillNotAvailable
			},
		}
		h := NewSeckillHandler(&mockSeckillService{}, browse)
		gin.SetMode(gin.TestMode)
		r := gin.New()
		r.GET("/user/seckill/detail", h.Detail)

		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/user/seckill/detail?id=99", nil))

		if w.Code != http.StatusNotFound {
			t.Errorf("status = %d, want 404", w.Code)
		}
		var body map[string]any
		_ = json.Unmarshal(w.Body.Bytes(), &body)
		if body["code"] != "seckill.not_available" {
			t.Errorf("code = %v", body["code"])
		}
	})
}
