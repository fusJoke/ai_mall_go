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
// 本包额外加一个按用户名查询的方法（登录场景需要）+ 4 个"仅改指定列"的专用方法
// （管理页场景：批量删除 / 改密 / 启停 / 解锁）。
type Repository interface {
	repository.CRUDRepository[model.Admin]

	// GetByUsername 按用户名精确查一行。
	// 记录不存在返回 gorm.ErrRecordNotFound，由上层决定渲染 401 / 404。
	GetByUsername(c *gin.Context, username string) (*model.Admin, error)

	// DeleteBatch 软删一组 id。空切片走短路返回 nil；ids 含不存在的行 / 已软删的行
	// GORM 会自动按 deleted_at IS NULL 过滤，不报错。
	DeleteBatch(c *gin.Context, ids []uint) error

	// UpdateStatus 仅更新 Status 列（admin 管理页"启停"场景）。
	// 故意不读 entity 后 Save —— 专用列更新省一次 SELECT + 锁。
	UpdateStatus(c *gin.Context, id uint, status int8) error

	// UpdatePassword 仅更新 Password 列（admin 管理页"改密"场景）。
	UpdatePassword(c *gin.Context, id uint, hashed string) error

	// ResetLoginFailure 重置登录失败计数 + 恢复启用状态（admin 管理页"解锁"场景）。
	// 一次性写两列：login_failure=0, status=1。
	ResetLoginFailure(c *gin.Context, id uint) error
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

// DeleteBatch 软删一组 id。
//
// GORM 自动加 deleted_at IS NULL 过滤；不存在的行 / 已软删的行不会报错，
// 软删效果是"按 deleted_at 列更新"，幂等。
//
// 用 Updates(map[string]any{"deleted_at": gorm.DeletedAt{...}}) 比 Delete(&T{}) 更明确：
// Delete 会把所有列都 SET 一遍（含 updated_at），map 只更新指定列。
func (r *baseRepository) DeleteBatch(c *gin.Context, ids []uint) error {
	if len(ids) == 0 {
		return nil
	}
	return repository.DB(c).
		Model(&model.Admin{}).
		Where("id IN ?", ids).
		Delete(&model.Admin{}).Error
}

// UpdateStatus 仅更新 Status 一列。
//
// 走 Updates(map[string]any{"status": status})：map 形式的 Updates
// 会自动跳过零值、且只更新 map 中指定的列，不会覆盖 UpdatedAt 以外的其他业务字段。
func (r *baseRepository) UpdateStatus(c *gin.Context, id uint, status int8) error {
	return repository.DB(c).
		Model(&model.Admin{}).
		Where("id = ?", id).
		Updates(map[string]any{"status": status}).Error
}

// UpdatePassword 仅更新 Password 一列。
func (r *baseRepository) UpdatePassword(c *gin.Context, id uint, hashed string) error {
	return repository.DB(c).
		Model(&model.Admin{}).
		Where("id = ?", id).
		Updates(map[string]any{"password": hashed}).Error
}

// ResetLoginFailure 一次性写两列：login_failure=0, status=1。
//
// 同 Updates(map[string]any{...})：只更新指定列，不会动其他字段（如 Password / Nickname）。
func (r *baseRepository) ResetLoginFailure(c *gin.Context, id uint) error {
	return repository.DB(c).
		Model(&model.Admin{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"login_failure": 0,
			"status":        int8(1),
		}).Error
}

// 编译期断言：baseRepository 必须实现 Repository。
var _ Repository = (*baseRepository)(nil)
