// Package admin 是管理员模型的数据访问层。
//
// 路径选择：用 admin/index.go 而非 admin/admin.go 是为了让单文件包能
// 保持包名短（package admin），同时把"实现入口"放到 index.go 与 web 路由层风格一致。
package admin

import (
	"errors"

	"github.com/gin-gonic/gin"

	"ai-go-mall/internal/model"
	"ai-go-mall/internal/repository"
)

// Repository 是管理员实体的数据访问接口：通用 CRUD 由 CRUDRepository 提供，
// 本包额外加一个按用户名查询的方法（登录场景需要）。
type Repository interface {
	repository.CRUDRepository[model.Admin]

	// GetByUsername 按用户名精确查一行。
	// 记录不存在返回 gorm.ErrRecordNotFound，由上层决定渲染 401 / 404。
	GetByUsername(c *gin.Context, username string) (*model.Admin, error)
}

// baseRepository 是 Repository 的默认实现。
//
// 通过嵌入 repository.CRUDRepository[model.Admin] 复用通用 CRUD；
// 业务专属方法（GetByUsername）直接通过 repository.DB(c) 拿请求作用域的 *gorm.DB。
type baseRepository struct {
	repository.CRUDRepository[model.Admin]
}

// NewRepository 返回 Repository 接口，调用方拿到的不是结构体。
func NewRepository() Repository {
	return &baseRepository{
		CRUDRepository: repository.NewBaseRepository[model.Admin](),
	}
}

// GetByUsername 按 username 查唯一一条。
//
// 用 repository.DB(c) 而不是 baseRepository 内暴露 db 方法，保证请求作用域绑定；
// 用户名在 GORM 层就有唯一索引，这里直接 Where + First 命中唯一行。
func (r *baseRepository) GetByUsername(c *gin.Context, username string) (*model.Admin, error) {
	if username == "" {
		return nil, errors.New("admin: empty username")
	}

	var adm model.Admin
	if err := repository.DB(c).
		Where("username = ?", username).
		First(&adm).Error; err != nil {
		return nil, err
	}
	return &adm, nil
}

// 编译期断言：baseRepository 必须实现 Repository。
var _ Repository = (*baseRepository)(nil)
