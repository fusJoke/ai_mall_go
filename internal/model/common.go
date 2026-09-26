package model

import (
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

// Token 令牌表，记录 API 调用凭证（登录会话、refresh token 等）。
//
// 字段命名遵循 GORM 约定（与 admin.go 保持一致）：
//   - ID        → 主键，自增
//   - UpdatedAt / CreatedAt → 自动维护
//   - DeletedAt → 软删除（gorm.DeletedAt 是 sql.NullTime 子类型，
//     实现 DeleteClauses / QueryClauses，GORM 据此自动加 deleted_at IS NULL 过滤）
//
// 关键设计：
//   - Token 字段保存的是 SHA256(hex) 之后的哈希值，明文 token 不会落库；
//     校验时用同一算法把入参哈希后去库里查，避免日志 / 备份泄露明文凭证。
//   - Type 用字符串表示"用途 / 身份"，例如 "admin"、"user"、"refresh"，
//     跨身份模块统一为 string，便于后续按业务扩展。
//   - (UserID, Type) 复合索引 + Token 唯一索引 —— Clear(userID, type) 与
//     Get(rawToken) 分别命中这两个索引。
//
// 字段顺序：业务字段在前、UpdatedAt → CreatedAt → DeletedAt 收尾。
type Token struct {
	// ID 令牌主键。
	ID int64 `gorm:"comment:ID;primaryKey;autoIncrement"`

	// Token 哈希后的 token 值（SHA256 hex，64 字符），全局唯一；
	// 明文不会落库，校验时同样哈希入参再 Where 查询。
	Token string `gorm:"comment:令牌哈希(SHA256);size:128;uniqueIndex;not null"`

	// Type token 用途标识，例如 "admin"、"user"、"refresh"；
	// 配合 UserID 形成复合索引，便于按"用户 + 用途"批量清理。
	Type string `gorm:"comment:令牌类型;size:32;index:idx_user_type,priority:2;not null"`

	// UserID 所属用户主键（admin.id / user.id 等，跨身份统一为 int64）。
	UserID int64 `gorm:"comment:用户ID;index:idx_user_type,priority:1;not null"`

	// ExpiresAt token 过期时间；Manager.Check 在 Get 之后判断是否早于当前时间。
	ExpiresAt time.Time `gorm:"comment:过期时间;not null"`

	// UpdatedAt 更新时间（GORM 自动维护）。
	UpdatedAt time.Time `gorm:"comment:更新时间;not null;autoUpdateTime"`

	// CreatedAt 创建时间（GORM 自动维护）。
	CreatedAt time.Time `gorm:"comment:创建时间;not null;autoCreateTime"`

	// DeletedAt 软删除标记（GORM 自动维护；Valid=true 即已删除）。
	DeletedAt gorm.DeletedAt `gorm:"comment:删除时间;index"`
}

// TableName 实现 schema.TablerWithNamer 接口 —— 由 GORM 传入带前缀配置的 namer，
// 我们把 "tokens" 交给它处理，这样 database 侧设置的 NamingStrategy.TablePrefix
// 才能真正落到表名上。若只实现 Tabler（TableName() string），前缀会被绕过。
func (Token) TableName(namer schema.Namer) string {
	return namer.TableName("tokens")
}

// init 在包加载时把 Token 注册到模型注册表。
//
// 注意：不要在这里直接调 AutoMigrate，迁移逻辑统一由 database.Init() 在
// GORM 实例准备好之后执行。
func init() {
	Register(Token{})
}
