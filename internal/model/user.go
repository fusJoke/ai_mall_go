package model

import (
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

// User C 端会员模型（对应 migration 000006 扩展后的 users 表）。
//
// 字段命名遵循 GORM 约定（与 admin.go 风格一致）：
//   - ID / CreatedAt / UpdatedAt / DeletedAt 标准三件套
//   - 业务字段在前、UpdatedAt → CreatedAt → DeletedAt 收尾
//
// 关键设计（mall MVP 设计 D2）：
//   - name 字段在 migration 000006 被重命名为 username（语义对齐 Admin.Username）；
//   - email 由 UK 改为普通索引（允许 NULL 与重复），保留查找能力；
//   - balance 用 decimal(12,2)：抽卡扣款原子性条件更新
//     `SET balance = balance - ? WHERE id = ? AND balance >= ?`，
//     防止超额扣款；
//   - status 1=启用 / 0=禁用，与 Admin.Status / AdminRule.Status 风格一致。
//
// 字段顺序：业务字段在前、UpdatedAt → CreatedAt → DeletedAt 收尾。
type User struct {
	// ID 用户主键。
	ID int64 `gorm:"comment:ID;primaryKey;autoIncrement"`

	// Username 用户名，全局唯一。
	Username string `gorm:"comment:用户名;size:64;uniqueIndex;not null"`

	// Nickname 昵称，留空表示未设置。
	Nickname string `gorm:"comment:昵称;size:64"`

	// Avatar 头像 URL，留空表示未设置。
	Avatar string `gorm:"comment:头像URL;size:255"`

	// Mobile 手机号，全局唯一；nil 表示未填写。
	Mobile *string `gorm:"comment:手机号;size:20;uniqueIndex"`

	// Email 邮箱；nil 表示未填写（不再要求唯一，改为普通索引覆盖查找）。
	Email *string `gorm:"comment:邮箱;size:128;index"`

	// Balance 余额（元，decimal(12,2)）。抽卡事务内条件扣减，保证不超额。
	Balance float64 `gorm:"comment:余额(元);type:decimal(12,2);not null;default:0"`

	// Status 账号状态：1 启用 / 0 禁用。
	Status int8 `gorm:"comment:状态(1启用,0禁用);default:1;not null"`

	// LastLoginAt 最后登录时间；nil 表示从未登录。
	LastLoginAt *time.Time `gorm:"comment:最后登录时间"`

	// LastLoginIp 最后登录 IP（兼容 IPv6，最长 45 字符）。
	LastLoginIp string `gorm:"comment:最后登录IP;size:45"`

	// LoginFailure 累计登录失败次数，达到阈值后锁定账号。
	LoginFailure int `gorm:"comment:登录失败次数;default:0;not null"`

	// Password 哈希后的密码（bcrypt / argon2 等，长度预留 255）。
	Password string `gorm:"comment:密码(已哈希);size:255;not null;default:''"`

	// UpdatedAt 更新时间（GORM 自动维护）。
	UpdatedAt time.Time `gorm:"comment:更新时间;not null;autoUpdateTime"`

	// CreatedAt 创建时间（GORM 自动维护）。
	CreatedAt time.Time `gorm:"comment:创建时间;not null;autoCreateTime"`

	// DeletedAt 软删除标记（GORM 自动维护；Valid=true 即已删除）。
	DeletedAt gorm.DeletedAt `gorm:"comment:删除时间;index"`
}

// TableName 实现 schema.TablerWithNamer 接口，让 GORM 把 "users"
// 交给带前缀配置的 namer 处理，NamingStrategy.TablePrefix 才能真正落到表名上。
func (User) TableName(namer schema.Namer) string {
	return namer.TableName("users")
}

// init 在包加载时把 User 注册到模型注册表。
//
// 注意：不要在这里直接调 AutoMigrate，迁移逻辑统一由 database.Init() 在
// GORM 实例准备好之后执行（即使当前 database.Init() 已切到 golang-migrate，
// model.All() 仍然保留给运行时启动期一致性检查使用）。
func init() {
	Register(User{})
}
