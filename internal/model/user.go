package model

import (
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

// User 测试用用户表，演示模型自注册与 AutoMigrate 的写法。
//
// 字段命名遵循 GORM 约定：
//   - ID        → 主键，自增
//   - CreatedAt / UpdatedAt → 自动维护
//   - DeletedAt → 软删除（gorm.DeletedAt 是 sql.NullTime 子类型，
//     实现 DeleteClauses / QueryClauses，GORM 据此自动加 deleted_at IS NULL 过滤）
type User struct {
	ID        int64          `gorm:"primaryKey;autoIncrement"`
	Name      string         `gorm:"size:64;not null"`
	Email     string         `gorm:"size:128;uniqueIndex;not null"`
	CreatedAt time.Time      `gorm:"not null"`
	UpdatedAt time.Time      `gorm:"not null"`
	DeletedAt gorm.DeletedAt `gorm:"index"`
}

// TableName 实现 schema.TablerWithNamer 接口 —— 由 GORM 传入带前缀配置的 namer，
// 我们把 "users" 交给它处理，这样 database 侧设置的 NamingStrategy.TablePrefix
// 才能真正落到表名上。若只实现 Tabler（TableName() string），前缀会被绕过。
func (User) TableName(namer schema.Namer) string {
	return namer.TableName("users")
}

// init 在包加载时把 User 注册到模型注册表。
//
// 注意：不要在这里直接调 AutoMigrate，迁移逻辑统一由 database.Init() 在
// GORM 实例准备好之后执行。
func init() {
	Register(User{})
}
