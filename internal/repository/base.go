// Package repository 数据访问层。本文件实现任意模型 T 的最小 CRUD 集合。
//
// 设计要点：
//   - CRUDRepository[T any] 是接口，对外只暴露接口；BaseRepository[T any]
//     是默认实现，调用方不应直接依赖结构体。
//   - 所有方法接收 *gin.Context，由 DBMiddleware 注入请求作用域 *gorm.DB，
//     因此客户端断开 → 请求 ctx 取消 → GORM 查询自动中止。
//   - Delete 走 GORM 默认行为：若 T 含 gorm.DeletedAt 则软删，否则硬删。
//   - List 同时返回 total + 当前页数据，便于 handler 直接渲染分页。
package repository

import (
	"errors"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"ai-go-mall/internal/model"
)

// CRUDRepository 是任意模型 T 的最小数据访问接口。
//
// 业务仓库可内嵌 BaseRepository[T] 后再叠加业务专属查询；调用方（service 层）
// 只持有接口，避免被具体实现绑死。
type CRUDRepository[T any] interface {
	Create(c *gin.Context, entity *T) error
	List(c *gin.Context, opts ListOptions) (items []T, total int64, err error)
	GetByID(c *gin.Context, id int64) (*T, error)
	Update(c *gin.Context, entity *T) error
	Delete(c *gin.Context, id int64) error
}

// ListOptions 通用分页参数。Page 从 1 计数。
//
// handler 负责把默认值与上限写好再传进来；repository 不再做兜底，便于 service
// 复用同一份参数。
type ListOptions struct {
	Page     int
	PageSize int
}

// BaseRepository 是 CRUDRepository[T] 的默认实现，零依赖、零字段。
//
// 通过 NewBaseRepository[T]() 实例化；亦可直接用作嵌入类型。
type BaseRepository[T any] struct{}

// NewBaseRepository 返回 CRUDRepository[T] 接口，调用方拿到的是接口而非结构体。
func NewBaseRepository[T any]() CRUDRepository[T] {
	return &BaseRepository[T]{}
}

func (r *BaseRepository[T]) db(c *gin.Context) *gorm.DB {
	return DB(c)
}

// Create 插入新行。entity 为 nil 时报错（编程错误，不应让 GORM 拿到 nil）。
func (r *BaseRepository[T]) Create(c *gin.Context, entity *T) error {
	if entity == nil {
		return errors.New("repository: nil entity")
	}
	return r.db(c).Create(entity).Error
}

// List 按 opts 分页查询，返回当前页数据 + 满足条件的总行数。
//
// total 通过不带 Limit/Offset 的 Count 单独取，避免 offset 越大越慢。
func (r *BaseRepository[T]) List(c *gin.Context, opts ListOptions) ([]T, int64, error) {
	db := r.db(c)

	var total int64
	if err := db.Model(new(T)).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	items := make([]T, 0)
	if total == 0 {
		return items, total, nil
	}

	offset := (opts.Page - 1) * opts.PageSize
	if err := db.Model(new(T)).
		Offset(offset).
		Limit(opts.PageSize).
		Find(&items).Error; err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

// GetByID 按主键查一行。记录不存在时返回 gorm.ErrRecordNotFound，
// 由上层决定是渲染 404 还是回落到其它行为。
func (r *BaseRepository[T]) GetByID(c *gin.Context, id int64) (*T, error) {
	var entity T
	if err := r.db(c).First(&entity, id).Error; err != nil {
		return nil, err
	}
	return &entity, nil
}

// Update 全量更新一行。GORM Save 会按主键更新所有字段；
// 调用方在 handler 层保证 entity.ID 已填好。
func (r *BaseRepository[T]) Update(c *gin.Context, entity *T) error {
	if entity == nil {
		return errors.New("repository: nil entity")
	}
	return r.db(c).Save(entity).Error
}

// Delete 按主键删除一行。
//
//   - 当 T 含 gorm.DeletedAt 字段时，GORM 自动软删（UPDATE deleted_at）；
//   - 否则执行硬删（DELETE）。
//
// 故意不调用 Unscoped() —— admin 页"删除"通常希望保留记录以便审计/恢复。
func (r *BaseRepository[T]) Delete(c *gin.Context, id int64) error {
	return r.db(c).Delete(new(T), id).Error
}

// 编译期断言：BaseRepository[T] 必须实现 CRUDRepository[T]。
// 若接口增减方法导致不一致，编译直接失败，最早在 CI 发现。
var _ CRUDRepository[model.User] = (*BaseRepository[model.User])(nil)
