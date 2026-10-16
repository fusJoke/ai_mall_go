// Package admin 是管理员模型的数据访问层。
//
// 路径选择：用 admin/index.go 而非 admin/admin.go 是为了让单文件包能
// 保持包名短（package admin），同时把"实现入口"放到 index.go 与 web 路由层风格一致。
package admin

import (
	"errors"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"ai-go-mall/internal/model"
	"ai-go-mall/internal/repository"
)

// ErrAdminNotFound 影响 0 行时返回：行不存在 / 已软删。
// 与 user 侧 ErrUserNotFound 同构，供 service 层用 errors.Is 区分「业务不存在」与「DB 故障」。
var ErrAdminNotFound = errors.New("admin: not found")

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

	// IncrementLoginFailure 原子自增 login_failure 并返回**自增后的真实新值**。
	//
	// 设计动机（OCR 审查 P1-1）：旧的 UpdateLoginFailure(c, id, failure, locked) 接收
	// 「最终绝对值」，service 层因此必须做 read-then-write（读 adm.LoginFailure → Go 层 +1 →
	// 写回）。两个并发错密码请求都从同一份快照读到 4、都算出 5、都写 5 → 计数只 +1，
	// 阈值永远打不到，管理员账号**可被并发暴力破解**。
	//
	// 本方法把「读 + 加 + 写」整体下推到 DB：
	//   - `UPDATE admins SET login_failure = login_failure + 1 WHERE id = ?` 让 DB 串行化自增；
	//   - 紧接着 `SELECT login_failure` 回读新值返回给 service —— 与自增同处一个事务，
	//     走写库 session，避免主从延迟读到 stale。
	//
	// service 拿到 newFailure 后基于「真实新值」判定锁阈值（>= MaxLoginFailure），不再做 Go 层计算。
	// 该判定天然安全：计数单调递增且每次自增只被一个请求观察到，因此必有请求观察到 >= 阈值。
	//
	// 与 user 侧 UserRepository.IncrementLoginFailure 完全同构（同一缺陷的两个实例，
	// user 侧已在 H1 修复中改掉，admin 侧本次补齐）。
	IncrementLoginFailure(c *gin.Context, id uint) (int, error)

	// UpdateLoginSuccess 仅更新登录成功记账列（Login 成功分支专用）：
	// 一次性写 login_failure=0, last_login_at=at, last_login_ip=ip。
	// 同样 MUST NOT 携带 password 列。
	UpdateLoginSuccess(c *gin.Context, id uint, ip string, at time.Time) error
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

// IncrementLoginFailure 原子自增 login_failure 并在同事务内回读新值。
//
// SQL 形态（注意 `+1` 是 gorm.Expr 内联字面量，不占位）：
//
//	UPDATE `admins` SET `login_failure`=login_failure + 1,`updated_at`=? WHERE id = ? AND `admins`.`deleted_at` IS NULL
//	SELECT `login_failure` FROM `admins` WHERE id = ? AND `admins`.`deleted_at` IS NULL
//
// 影响行数 = 0（不存在 / 已软删）→ 返回 ErrAdminNotFound，由 service 决定如何响应。
// 刻意只写 login_failure 一列，不携带 password —— 模型里的 Password 是哈希形态，
// 整行更新会被二次哈希（P0，见 openspec/changes/fix-admin-login-password-rehash）。
//
// 触锁（status 置 0）由 service 在拿到返回值后单独发起，见 UpdateStatus：
// 不把 IF(...) 塞进同一条 UPDATE，是因为 MySQL 对 SET 子句是「左到右、使用已更新的值」求值，
// 列顺序一变判定就会 off-by-one，脆弱且难以察觉。
func (r *baseRepository) IncrementLoginFailure(c *gin.Context, id uint) (int, error) {
	var newFailure int
	err := repository.RunInTx(c, func(txCtx *gin.Context) error {
		res := repository.DB(txCtx).
			Model(&model.Admin{}).
			Where("id = ?", id).
			Update("login_failure", gorm.Expr("login_failure + 1"))
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return ErrAdminNotFound
		}
		return repository.DB(txCtx).
			Model(&model.Admin{}).
			Where("id = ?", id).
			Select("login_failure").
			Scan(&newFailure).Error
	})
	if err != nil {
		return 0, err
	}
	return newFailure, nil
}

// UpdateLoginSuccess 一次性写 login_failure=0, last_login_at, last_login_ip。
//
// at 由 service 层传入（与签发 token 的 ExpiresAt 共用同一时刻）。
func (r *baseRepository) UpdateLoginSuccess(c *gin.Context, id uint, ip string, at time.Time) error {
	return repository.DB(c).
		Model(&model.Admin{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"login_failure": 0,
			"last_login_at": at,
			"last_login_ip": ip,
		}).Error
}

// 编译期断言：baseRepository 必须实现 Repository。
var _ Repository = (*baseRepository)(nil)
