// Package user 是 C 端会员（mall_users / users 表）的数据访问层。
//
// 本文件按 CLAUDE.md 分层：repository 层只做 DB ↔ 内存 的搬运，不掺业务。
//
// 设计要点（与 design D2 / D3 一致）：
//   - 嵌入 repository.CRUDRepository[model.User] 复用通用 CRUD；
//   - GetByUsername 走 username 唯一索引（users.username UK），登录场景专用；
//   - DecrementBalance 走条件更新 `SET balance = balance - ? WHERE id = ? AND balance >= ?`，
//     保证抽卡事务原子扣款不超额（设计 D3 关键约束）。
//
// 调用方（service 层）只持有 UserRepository 接口，避免被具体实现绑死。
package user

import (
	"errors"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"ai-go-mall/internal/model"
	"ai-go-mall/internal/repository"
)

// UserRepository 定义 C 端会员的数据访问接口。
//
// 设计原则（与 admin.Repository 风格一致）：
//   - 通用 CRUD 由 CRUDRepository[model.User] 提供；
//   - 业务专属方法（GetByUsername / DecrementBalance / UpdateLoginSuccess /
//     UpdateLoginFailure）走 GORM 链，由 service 层调用；
//   - DecrementBalance 是"金额方向安全一致"的硬约束：调用方传入 amount，
//     repo 拼 `balance >= amount` 条件；影响行数=0 时返回 ErrInsufficientBalance，
//     由 service 层做错误映射。
type UserRepository interface {
	repository.CRUDRepository[model.User]

	// GetByUsername 按用户名精确查一行（走 users.username UK）。
	// 记录不存在返回 gorm.ErrRecordNotFound，由上层渲染 401 / 404。
	GetByUsername(c *gin.Context, username string) (*model.User, error)

	// DecrementBalance 条件扣减余额。
	//
	// SQL 形态：`UPDATE users SET balance = balance - ? WHERE id = ? AND balance >= ?`。
	//
	// 行为：
	//   - 影响 1 行 → 返回 nil；
	//   - 影响 0 行（用户不存在 / 已软删 / 余额不足）→ 返回 ErrInsufficientBalance。
	//
	// 设计动机：抽卡事务步骤 6 一次性扣款，必须保证不超额；
	// 用「条件 UPDATE」让 DB 自己判定原子性，无需在 Go 层先 SELECT 余额。
	DecrementBalance(c *gin.Context, userID int64, amount float64) error

	// IncrementBalance 条件增加余额（admin 手动加款 / 退款补偿场景）。
	//
	// SQL 形态：`UPDATE users SET balance = balance + ? WHERE id = ?`。
	// id 不存在 / 已软删均影响 0 行 → 返回 ErrUserNotFound。
	IncrementBalance(c *gin.Context, userID int64, amount float64) error

	// UpdateLoginSuccess 一次性写三列：login_failure=0, last_login_at, last_login_ip。
	UpdateLoginSuccess(c *gin.Context, id int64, ip string, at time.Time) error

	// IncrementLoginFailure 原子自增 login_failure 并返回新值。
	//
	// 设计动机（H1 审查修复）：旧的「read-then-write」路径（service 读 u.LoginFailure、
	// 在 Go 层 +1、调 UpdateLoginFailure 写回）在并发错密码场景下两个请求都从
	// 同一份 DB 快照读 4，都 +1 后都写 5，互相覆盖，锁阈值判定失效。
	//
	// 本方法把「读 + 加 + 写」整体下推到 DB：
	//   - `UPDATE users SET login_failure = login_failure + 1 WHERE id = ?` 让 DB 串行化自增；
	//   - 紧接着 `SELECT login_failure` 回读新值返回给 service —— 走 dbresolver 的写库
	//     session（事务内），避免主从延迟读到 stale。
	//
	// service 拿到 newFailure 后即可基于「真实新值」判定锁阈值（>= MaxLoginFailure），
	// 不再做 Go 层计算。
	IncrementLoginFailure(c *gin.Context, id int64) (int, error)

	// UpdateStatus 仅更新 Status 列（admin 强制启停 / 解锁场景）。
	UpdateStatus(c *gin.Context, id int64, status int8) error
}

// ErrInsufficientBalance 余额不足。DecrementBalance 影响 0 行时返回，
// service 层把它映射成 4xx 业务错误。
var ErrInsufficientBalance = errors.New("user: insufficient balance")

// ErrUserNotFound 用户不存在 / 已软删（IncrementBalance 影响 0 行时返回）。
var ErrUserNotFound = errors.New("user: not found")

// gormUserRepository 是 UserRepository 的 *gorm.DB 实现。
//
// 通过嵌入 repository.CRUDRepository[model.User] 复用通用 CRUD；
// 业务专属方法直接通过 repository.DB(c) 拿请求作用域的 *gorm.DB。
type gormUserRepository struct {
	repository.CRUDRepository[model.User]
}

// NewRepository 返回 UserRepository 接口，调用方拿到的是接口而非结构体。
func NewRepository() UserRepository {
	return &gormUserRepository{
		CRUDRepository: repository.NewBaseRepository[model.User](),
	}
}

// GetByUsername 按 username 查唯一一条。
//
// username 在 users 表上有 UK（migration 000006），直接 Where + First 命中唯一行。
// 记录不存在时 GORM 返回 gorm.ErrRecordNotFound，service 层做错误映射。
func (r *gormUserRepository) GetByUsername(c *gin.Context, username string) (*model.User, error) {
	if username == "" {
		return nil, errors.New("user: empty username")
	}

	var u model.User
	if err := repository.DB(c).
		Where("username = ?", username).
		First(&u).Error; err != nil {
		return nil, err
	}
	return &u, nil
}

// DecrementBalance 条件扣减余额。
//
// 用 `gorm.Expr("balance - ?", amount)` 配合 `Where("id = ? AND balance >= ?", ...)`：
// DB 层判定余额足够再扣，影响行数 = 1 即成功；= 0 时（用户不存在 / 余额不足）统一
// 用 ErrInsufficientBalance 上报，由 service 走余额不足分支。
//
// 注：deleted_at IS NULL 由 GORM 自动加（Update / Where 触发软删过滤）。
func (r *gormUserRepository) DecrementBalance(c *gin.Context, userID int64, amount float64) error {
	if amount <= 0 {
		return errors.New("user: decrement amount must be positive")
	}
	res := repository.DB(c).
		Model(&model.User{}).
		Where("id = ? AND balance >= ?", userID, amount).
		Update("balance", gorm.Expr("balance - ?", amount))
	if err := res.Error; err != nil {
		return err
	}
	if res.RowsAffected == 0 {
		return ErrInsufficientBalance
	}
	return nil
}

// IncrementBalance 条件增加余额。
//
// `balance = balance + ?` 走 gorm.Expr 表达式，让 DB 算；
// 不带 balance >= 0 条件（additive 操作总是增加）。id 不存在 / 已软删均影响 0 行 → ErrUserNotFound。
func (r *gormUserRepository) IncrementBalance(c *gin.Context, userID int64, amount float64) error {
	if amount <= 0 {
		return errors.New("user: increment amount must be positive")
	}
	res := repository.DB(c).
		Model(&model.User{}).
		Where("id = ?", userID).
		Update("balance", gorm.Expr("balance + ?", amount))
	if err := res.Error; err != nil {
		return err
	}
	if res.RowsAffected == 0 {
		return ErrUserNotFound
	}
	return nil
}

// UpdateLoginSuccess 一次性写三列：login_failure=0, last_login_at, last_login_ip。
//
// 同 admin baseRepository 的 UpdateLoginSuccess 风格：map 形式 Updates 只写指定列，
// Password / Mobile 等其他字段不被覆盖。
func (r *gormUserRepository) UpdateLoginSuccess(c *gin.Context, id int64, ip string, at time.Time) error {
	return repository.DB(c).
		Model(&model.User{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"login_failure": 0,
			"last_login_at": at,
			"last_login_ip": ip,
		}).Error
}

// IncrementLoginFailure 原子自增 login_failure 并返回新值。
//
// 设计动机（H1 审查修复）：旧的「read-then-write」路径（service 读 u.LoginFailure、
// 在 Go 层 +1、调 UpdateLoginFailure 写回）在并发错密码场景下两个请求都从
// 同一份 DB 快照读 4，都 +1 后都写 5，互相覆盖，锁阈值判定失效。
//
// 本方法把「读 + 加 + 写」整体下推到 DB：
//   - `UPDATE users SET login_failure = login_failure + 1 WHERE id = ?` 让 DB 串行化自增；
//   - 紧接着 `SELECT login_failure` 回读新值返回给 service —— 走 dbresolver 的写库
//     session（事务内），避免主从延迟读到 stale。
//
// service 拿到 newFailure 后即可基于「真实新值」判定锁阈值（>= MaxLoginFailure），
// 不再做 Go 层计算。
//
// 原子性与一致性：
//   - 自增 UPDATE 与回读 SELECT 同处一个事务（Begin/Commit），DB 层保证两者读到的
//     login_failure 是「本事务视角下」的连续状态；
//   - 影响行数 = 0（用户不存在 / 已软删）→ 返回 ErrUserNotFound；
//   - 写库走 repository.DB(c) 的请求作用域 *gorm.DB（事务强制写库，由 dbresolver 保证）。
func (r *gormUserRepository) IncrementLoginFailure(c *gin.Context, id int64) (int, error) {
	var newFailure int
	err := repository.DB(c).Transaction(func(tx *gorm.DB) error {
		// 1) 原子自增。RowsAffected = 0 时用户不存在 / 已软删，由 service 走 404 分支。
		res := tx.Model(&model.User{}).
			Where("id = ?", id).
			Update("login_failure", gorm.Expr("login_failure + 1"))
		if err := res.Error; err != nil {
			return err
		}
		if res.RowsAffected == 0 {
			return ErrUserNotFound
		}
		// 2) 同事务回读 —— 走写库 session，保证读到本事务自增后的新值。
		return tx.Model(&model.User{}).
			Where("id = ?", id).
			Select("login_failure").
			Scan(&newFailure).Error
	})
	if err != nil {
		return 0, err
	}
	return newFailure, nil
}

// UpdateStatus 仅更新 Status 列。
func (r *gormUserRepository) UpdateStatus(c *gin.Context, id int64, status int8) error {
	return repository.DB(c).
		Model(&model.User{}).
		Where("id = ?", id).
		Update("status", status).Error
}

// 编译期断言：gormUserRepository 必须实现 UserRepository。
var _ UserRepository = (*gormUserRepository)(nil)