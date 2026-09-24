// Package handler HTTP 请求处理层。本文件实现任意模型 T 的最小 CRUD 路由。
//
// 设计要点：
//   - BaseHandler[T] 嵌入 service.CRUDService[T]，方法直接转发，handler 不掺业务判断。
//   - 仅使用 GET / POST 两种请求方法 —— Edit 拆成 GET（取行）+ POST（提交），
//     Create / Delete 统一走 POST。
//   - RegisterRoutes(rg, resource) 把通用 CRUD 路由挂到指定资源路径下，
//     业务 handler 通过嵌入 BaseHandler[T] 复用通用方法，再叠加专属路由。
//
// 路由约定（resource 由调用方传入，如 "users"）：
//
//	POST /resource/create   JSON → 新建实体
//	GET  /resource/list     ?page=&page_size= → 分页列表
//	GET  /resource/edit     ?id= → 取待编辑行
//	POST /resource/edit     JSON → 提交修改（必须含 id）
//	POST /resource/delete   ?id= 或 form id= → 删除
package handler

import (
	"errors"
	"net/http"
	"reflect"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"ai-go-mall/internal/repository"
	"ai-go-mall/internal/service"
)

// BaseHandler 提供任意模型 T 的通用 CRUD 路由处理函数。
//
// 通过嵌入 service.CRUDService[T] 自动获得通用方法的转发；业务 handler
// 只需再写自己专属的方法 / 路由。
type BaseHandler[T any] struct {
	service.CRUDService[T]
}

// NewBaseHandler 接收 service.CRUDService[T]，返回 *BaseHandler[T]。
//
// 调用方拿到的是结构体指针（handler 不强制定义接口；中间件风格的扩展由
// 嵌入 + RegisterRoutes 完成），构造期已经把依赖注入好。
func NewBaseHandler[T any](svc service.CRUDService[T]) *BaseHandler[T] {
	return &BaseHandler[T]{CRUDService: svc}
}

// Create 解析 JSON body → 调 service.Create → 回写实体 JSON。
//
// body 非法 → 400；DB 出错 → 500；成功 → 200 + 新建后的实体（含 ID / 时间戳）。
func (h *BaseHandler[T]) Create(c *gin.Context) {
	var entity T
	if err := c.ShouldBindJSON(&entity); err != nil {
		respondError(c, http.StatusBadRequest, err)
		return
	}
	if err := h.CRUDService.Create(c, &entity); err != nil {
		respondError(c, http.StatusInternalServerError, err)
		return
	}
	c.JSON(http.StatusOK, entity)
}

// List 解析 ?page= &page_size= → 调 service.List → 返回分页结果。
//
// 返回结构：{ items, total, page, page_size }；缺省 page=1 page_size=20，
// 上限 page_size=200 防止有人 -1 把库拖垮。
func (h *BaseHandler[T]) List(c *gin.Context) {
	opts := parseListOptions(c)

	items, total, err := h.CRUDService.List(c, opts)
	if err != nil {
		respondError(c, http.StatusInternalServerError, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"items":     items,
		"total":     total,
		"page":      opts.Page,
		"page_size": opts.PageSize,
	})
}

// EditGet 处理 GET /edit?id= —— 取待编辑行。
//
// id 缺失 / 非法 → 400；记录不存在 → 404；成功 → 200 + 实体 JSON。
func (h *BaseHandler[T]) EditGet(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		respondError(c, http.StatusBadRequest, err)
		return
	}

	entity, err := h.CRUDService.GetByID(c, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			respondError(c, http.StatusNotFound, err)
			return
		}
		respondError(c, http.StatusInternalServerError, err)
		return
	}
	c.JSON(http.StatusOK, entity)
}

// EditPost 处理 POST /edit —— 提交修改（全量更新）。
//
// body 必须含 id 字段；id 缺失 / 零值 / body 非法 → 400；DB 出错 → 500；
// 成功 → 200 + 更新后实体。
//
// 注意：GORM Save 是全量覆盖；主键为零时 Save 会走 INSERT，导致
// POST /edit 静默建一条新行。本方法在调 Update 之前显式校验 ID 非零。
// 调用方若只想改部分字段，请走 PATCH 风格的专用接口
// 或在 service 层叠加 update_columns 逻辑。
func (h *BaseHandler[T]) EditPost(c *gin.Context) {
	var entity T
	if err := c.ShouldBindJSON(&entity); err != nil {
		respondError(c, http.StatusBadRequest, err)
		return
	}
	if err := requireNonZeroID(&entity); err != nil {
		respondError(c, http.StatusBadRequest, err)
		return
	}
	if err := h.CRUDService.Update(c, &entity); err != nil {
		respondError(c, http.StatusInternalServerError, err)
		return
	}
	c.JSON(http.StatusOK, entity)
}

// Delete 处理 POST /delete —— id 可走 query (?id=) 或 form (id=)。
func (h *BaseHandler[T]) Delete(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		respondError(c, http.StatusBadRequest, err)
		return
	}
	if err := h.CRUDService.Delete(c, id); err != nil {
		respondError(c, http.StatusInternalServerError, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// RegisterRoutes 把通用 CRUD 路由挂到 rg/resource 下：
//
//	POST /resource/create
//	GET  /resource/list
//	GET  /resource/edit
//	POST /resource/edit
//	POST /resource/delete
//
// 业务 handler 在自己 RegisterRoutes 里调一次 BaseHandler.RegisterRoutes 即可复用。
func (h *BaseHandler[T]) RegisterRoutes(rg *gin.RouterGroup, resource string) {
	g := rg.Group(resource)
	g.POST("/create", h.Create)
	g.GET("/list", h.List)
	g.GET("/edit", h.EditGet)
	g.POST("/edit", h.EditPost)
	g.POST("/delete", h.Delete)
}

// parseID 优先从 query (?id=) 取，其次从 postForm (id=) 取；都拿不到 / 非法 → 报错。
func parseID(c *gin.Context) (int64, error) {
	raw := c.Query("id")
	if raw == "" {
		raw = c.PostForm("id")
	}
	if raw == "" {
		return 0, errors.New("missing id")
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, errors.New("invalid id")
	}
	return id, nil
}

// parseListOptions 从 query 取 page / page_size，写好默认值与上限。
func parseListOptions(c *gin.Context) repository.ListOptions {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 200 {
		pageSize = 200
	}
	return repository.ListOptions{Page: page, PageSize: pageSize}
}

// respondError 统一 JSON 错误响应。
func respondError(c *gin.Context, status int, err error) {
	c.JSON(status, gin.H{"error": err.Error()})
}

// requireNonZeroID 通过反射校验 entity 的 ID 字段（命名 "ID"、整型）非零。
// 主要用于 EditPost —— GORM Save 在主键为零时会走 INSERT，需要在 handler 层挡住。
//
// 仅支持命名 "ID" 的整型字段（含有符号 / 无符号）；其它主键命名约定（如
// gorm:"primaryKey" tag）暂未覆盖 —— 当前项目所有模型均按 "ID" 主键命名。
func requireNonZeroID(entity any) error {
	v := reflect.ValueOf(entity)
	if v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return errors.New("edit: nil entity")
		}
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return errors.New("edit: entity must be a struct")
	}

	idField := v.FieldByName("ID")
	if !idField.IsValid() {
		return errors.New("edit: entity has no ID field")
	}

	switch idField.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if idField.Int() == 0 {
			return errors.New("edit: missing or zero id")
		}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		if idField.Uint() == 0 {
			return errors.New("edit: missing or zero id")
		}
	default:
		return errors.New("edit: ID field must be an integer")
	}
	return nil
}
