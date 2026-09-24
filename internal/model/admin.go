package model

import (
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

// Admin 管理员账号模型。
//
// 字段命名遵循 GORM 约定：
//   - ID        → 主键，自增
//   - CreatedAt / UpdatedAt → 自动维护
//   - DeletedAt → 软删除（gorm.DeletedAt 是 sql.NullTime 子类型，
//     实现 DeleteClauses / QueryClauses，GORM 据此自动加 deleted_at IS NULL 过滤）
//
// 字段标签规约：
//   - comment 永远放在第一位（所有字段都有列注释）；
//   - 其余约束按常用度排序：primaryKey / not null / default / size / uniqueIndex / index / autoCreateTime / autoUpdateTime；
//   - 不嵌入 gorm.Model：它的字段没有注释、顺序不可控，无法满足本规约。
//
// 字段顺序按业务字段在前、时间戳字段在后排列，UpdatedAt → CreatedAt → DeletedAt 收尾。
type Admin struct {
	// ID 管理员主键。
	ID int64 `gorm:"comment:ID;primaryKey;autoIncrement"`

	// Username 用户名，全局唯一。
	Username string `gorm:"comment:用户名;size:64;uniqueIndex;not null"`

	// Nickname 昵称，可重复，留空表示未设置。
	Nickname string `gorm:"comment:昵称;size:64"`

	// Avatar 头像 URL，留空表示未设置。
	Avatar string `gorm:"comment:头像URL;size:255"`

	// Email 邮箱，全局唯一；nil 表示未填写（指针类型以支持 NULL）。
	Email *string `gorm:"comment:邮箱;size:128;uniqueIndex"`

	// Mobile 手机号，全局唯一；nil 表示未填写（指针类型以支持 NULL）。
	Mobile *string `gorm:"comment:手机号;size:20;uniqueIndex"`

	// LoginFailure 累计登录失败次数，达到阈值后锁定账号。
	LoginFailure int `gorm:"comment:登录失败次数;default:0;not null"`

	// LastLoginAt 最后登录时间；nil 表示从未登录。
	LastLoginAt *time.Time `gorm:"comment:最后登录时间"`

	// LastLoginIp 最后登录 IP（兼容 IPv6，最长 45 字符）；留空表示从未登录。
	LastLoginIp string `gorm:"comment:最后登录IP;size:45"`

	// Password 哈希后的密码（bcrypt / argon2 等，长度预留 255）。
	Password string `gorm:"comment:密码(已哈希);size:255;not null"`

	// Bio 个人简介，留空表示未填写。
	Bio string `gorm:"comment:个人简介;size:500"`

	// Status 账号状态：1 启用 / 0 禁用。
	Status int8 `gorm:"comment:状态(1启用,0禁用);default:1;not null"`

	// UpdatedAt 更新时间（GORM 自动维护）。
	UpdatedAt time.Time `gorm:"comment:更新时间;not null;autoUpdateTime"`

	// CreatedAt 创建时间（GORM 自动维护）。
	CreatedAt time.Time `gorm:"comment:创建时间;not null;autoCreateTime"`

	// DeletedAt 软删除标记（GORM 自动维护；Valid=true 即已删除）。
	DeletedAt gorm.DeletedAt `gorm:"comment:删除时间;index"`
}

// TableName 实现 schema.TablerWithNamer 接口 —— 由 GORM 传入带前缀配置的 namer，
// 我们把 "admins" 交给它处理，这样 database 侧设置的 NamingStrategy.TablePrefix
// 才能真正落到表名上。若只实现 Tabler（TableName() string），前缀会被绕过。
func (Admin) TableName(namer schema.Namer) string {
	return namer.TableName("admins")
}

// init 在包加载时把 Admin 注册到模型注册表。
//
// 注意：不要在这里直接调 AutoMigrate，迁移逻辑统一由 database.Init() 在
// GORM 实例准备好之后执行。
func init() {
	Register(Admin{})
}
