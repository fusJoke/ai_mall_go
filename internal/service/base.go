// Package service 业务编排层。本文件实现任意模型 T 的最小 CRUD 集合。
//
// 设计要点：
//   - CRUDService[T any] 是接口，对外只暴露接口；BaseCRUDService[T any]
//     是默认实现，调用方不应直接持有结构体。
//   - BaseCRUDService 嵌入 repository.CRUDRepository[T]，方法直接转发到 repo，
//     业务 service 可继续内嵌后再叠加专属校验 / 编排。
//   - 通用 CRUD 不含业务逻辑；如需校验 / 钩子，写业务 service 而不是改这里。
package service

import (
	"errors"

	"github.com/gin-gonic/gin"

	"ai-go-mall/internal/model"
	"ai-go-mall/internal/repository"
)

// CRUDService 是任意模型 T 的最小业务接口。
//
// handler 通过这个接口调用，service 之间也以接口组合，避免耦合到具体结构体。
type CRUDService[T any] interface {
	Create(c *gin.Context, entity *T) error
	List(c *gin.Context, opts repository.ListOptions) (items []T, total int64, err error)
	GetByID(c *gin.Context, id int64) (*T, error)
	Update(c *gin.Context, entity *T) error
	Delete(c *gin.Context, id int64) error
}

// BaseCRUDService 是 CRUDService[T] 的默认实现。
//
// 通过嵌入 repository.CRUDRepository[T] 直接转发所有数据访问，
// 因此 BaseCRUDService 自动满足 CRUDService 接口。
//
// 业务 service 的典型写法：
//
//	type UserService struct {
//	    *service.BaseCRUDService[model.User]
//	}
//
//	func (s *UserService) Register(user *model.User) error {
//	    // 在这里写业务校验
//	    return s.BaseCRUDService.Create(ctx, user)
//	}
type BaseCRUDService[T any] struct {
	repository.CRUDRepository[T]
}

// NewBaseCRUDService 接收一个 CRUDRepository[T]，返回 CRUDService[T] 接口。
//
// 构造方：cmd 启动期把 *BaseRepository[T] 注入；调用方：handler 只看到接口。
func NewBaseCRUDService[T any](repo repository.CRUDRepository[T]) CRUDService[T] {
	return &BaseCRUDService[T]{CRUDRepository: repo}
}

// Create 在转发前做一道 nil 检查，避免 GORM 拿到 nil 时返回模糊错误。
//
// 其余方法直接走嵌入接口的转发，不做二次包装 —— 业务校验属于业务 service 的职责。
func (s *BaseCRUDService[T]) Create(c *gin.Context, entity *T) error {
	if entity == nil {
		return errors.New("service: nil entity")
	}
	return s.CRUDRepository.Create(c, entity)
}

// Update 同样做 nil 检查后转发。
func (s *BaseCRUDService[T]) Update(c *gin.Context, entity *T) error {
	if entity == nil {
		return errors.New("service: nil entity")
	}
	return s.CRUDRepository.Update(c, entity)
}

// 编译期断言：BaseCRUDService[T] 必须实现 CRUDService[T]。
var _ CRUDService[model.User] = (*BaseCRUDService[model.User])(nil)
