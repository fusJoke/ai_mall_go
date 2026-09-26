package model

import (
	"time"

	"gorm.io/gorm/schema"
)

// Captcha 验证码表，记录一次性验证码的查询键与详情，用于登录、注册、找回密码等场景。
//
// 设计要点（与业务表不同，刻意偏离项目惯用的 id+UpdatedAt+DeletedAt 模板）：
//   - Key 作为主键（字符串 PK），而不是自增 ID：key 本身就是查询/提交令牌，自然键即主键；
//   - 无 UpdatedAt：验证码创建后不再修改；
//   - 无软删除 DeletedAt：验证成功立即硬删除，过期清理走 expires_at 批量硬删除；
//   - Code / Info 二选一填一：文本类验证码填 Code（加密后的密文），
//     点选类验证码填 Info（坐标+文字 JSON+图片宽高等），二者都为可空 *string。
//
// 字段标签规约与 admin.go 保持一致：comment 永远第一位；其余约束按常用度排序；
// 业务字段在前，时间戳字段收尾。
type Captcha struct {
	// Key 验证码查询键 / 唯一令牌，提交验证时由客户端携带；主键，字符串类型。
	Key string `gorm:"comment:验证令牌;primaryKey;size:64"`

	// Code 加密后的验证码值；文本类验证码使用，点选类验证码留空；nil 表示未设置。
	Code *string `gorm:"comment:验证码值(已加密,文本类使用);size:255"`

	// Info 验证码详情 JSON；点选类验证码存储元素坐标+文字 JSON+图片宽高，
	// 用于核对点击位置准确性；文本类验证码留空；nil 表示未设置。
	// 用 string 存 JSON 字符串而非 MySQL 原生 JSON 列，便于跨库兼容与 GORM 透传。
	Info *string `gorm:"comment:验证码详情JSON(点选类使用);type:text"`

	// ExpiresAt 过期时间；过期清理 SQL：DELETE FROM captchas WHERE expires_at < NOW()。
	ExpiresAt time.Time `gorm:"comment:过期时间;not null;index"`

	// CreatedAt 创建时间（GORM 自动维护）。
	CreatedAt time.Time `gorm:"comment:创建时间;not null;autoCreateTime"`
}

// TableName 实现 schema.TablerWithNamer 接口 —— 由 GORM 传入带前缀配置的 namer，
// 我们把 "captchas" 交给它处理，这样 database 侧设置的 NamingStrategy.TablePrefix
// 才能真正落到表名上。若只实现 Tabler（TableName() string），前缀会被绕过。
func (Captcha) TableName(namer schema.Namer) string {
	return namer.TableName("captchas")
}

// init 在包加载时把 Captcha 注册到模型注册表。
//
// 注意：不要在这里直接调 AutoMigrate，迁移逻辑统一由 database.Init() 在
// GORM 实例准备好之后执行。
func init() {
	Register(Captcha{})
}
