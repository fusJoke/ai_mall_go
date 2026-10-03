// Package supplier 的 supplier_user_repository.go ——
// MallSupplierUser（mall_supplier_users 表）仓储。
//
// 与 supplier_repository.go 同一包，两个仓储语义独立。
package supplier

import (
	"errors"
	"time"

	"github.com/gin-gonic/gin"

	"ai-go-mall/internal/model/mall"
	"ai-go-mall/internal/repository"
)

// SupplierUserRepository 定义 mall_supplier_users 表的数据访问接口。
//
// 设计原则（与 admin.Repository 风格一致）：
//   - 通用 CRUD 由 CRUDRepository[mall.MallSupplierUser] 提供；
//   - 登录专用查询（GetByUsername）走 UK；
//   - 1:1 关系专用查询（GetBySupplierID）走 UK；
//   - Login 记账（UpdateLoginSuccess）一次性写多列且 MUST NOT 携带 password。
type SupplierUserRepository interface {
	repository.CRUDRepository[mall.MallSupplierUser]

	// GetByUsername 按登录用户名 UK 查唯一一条。
	// 记录不存在返回 gorm.ErrRecordNotFound，由上层渲染 401 / 404。
	GetByUsername(c *gin.Context, username string) (*mall.MallSupplierUser, error)

	// GetBySupplierID 按 supplier_id UK 查唯一一条（1:1 关系）。
	GetBySupplierID(c *gin.Context, supplierID int64) (*mall.MallSupplierUser, error)

	// UpdateLoginSuccess 一次性写两列：last_login_at, last_login_ip。
	// MUST NOT 携带 password 列（模型里是哈希，整行更新会二次哈希）；
	// 也 MUST NOT 携带 login_failure —— 该表没有这一列。
	UpdateLoginSuccess(c *gin.Context, id int64, ip string, at time.Time) error

	// UpdateStatus 仅写 status 列（admin 强制启停）。
	UpdateStatus(c *gin.Context, id int64, status int8) error
}

// gormSupplierUserRepository 是 SupplierUserRepository 的 *gorm.DB 实现。
type gormSupplierUserRepository struct {
	repository.CRUDRepository[mall.MallSupplierUser]
}

// NewSupplierUserRepository 返回 SupplierUserRepository 接口。
func NewSupplierUserRepository() SupplierUserRepository {
	return &gormSupplierUserRepository{
		CRUDRepository: repository.NewBaseRepository[mall.MallSupplierUser](),
	}
}

// GetByUsername 按 username UK 查唯一一条。
//
// 登录场景：handler.Login → service.Login → repo.GetByUsername → 比对密码。
func (r *gormSupplierUserRepository) GetByUsername(c *gin.Context, username string) (*mall.MallSupplierUser, error) {
	if username == "" {
		return nil, errors.New("supplier: empty username")
	}
	var u mall.MallSupplierUser
	if err := repository.DB(c).
		Where("username = ?", username).
		First(&u).Error; err != nil {
		return nil, err
	}
	return &u, nil
}

// GetBySupplierID 按 supplier_id UK 查唯一一条（1:1 关系）。
func (r *gormSupplierUserRepository) GetBySupplierID(c *gin.Context, supplierID int64) (*mall.MallSupplierUser, error) {
	var u mall.MallSupplierUser
	if err := repository.DB(c).
		Where("supplier_id = ?", supplierID).
		First(&u).Error; err != nil {
		return nil, err
	}
	return &u, nil
}

// UpdateLoginSuccess 一次性写两列：last_login_at / last_login_ip。
//
// 注意：mall_supplier_users **没有** login_failure 列（spec 与迁移 000007 均无；
// 该表刻意不做「失败 N 次自动锁定」），所以这里不能像 internal/repository/user
// 那样顺带清零 login_failure —— 否则真实库报 Error 1054 Unknown column。
//
// map 形式 Updates 只写指定列；Password 不被覆盖，避免密码失效。
func (r *gormSupplierUserRepository) UpdateLoginSuccess(c *gin.Context, id int64, ip string, at time.Time) error {
	return repository.DB(c).
		Model(&mall.MallSupplierUser{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"last_login_at": at,
			"last_login_ip": ip,
		}).Error
}

// UpdateStatus 仅写 status 列。
func (r *gormSupplierUserRepository) UpdateStatus(c *gin.Context, id int64, status int8) error {
	return repository.DB(c).
		Model(&mall.MallSupplierUser{}).
		Where("id = ?", id).
		Update("status", status).Error
}

// 编译期断言：gormSupplierUserRepository 必须实现 SupplierUserRepository。
var _ SupplierUserRepository = (*gormSupplierUserRepository)(nil)