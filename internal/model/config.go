package model

import (
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

// Config 系统配置表，存储站点级键值型配置（站点名称、备案号、客服联系方式等）。
//
// 字段命名遵循 GORM 约定（与 admin.go / common.go 保持一致）：
//   - ID        → 主键，自增
//   - CreatedAt / UpdatedAt → 自动维护
//   - DeletedAt → 软删除（gorm.DeletedAt 是 sql.NullTime 子类型，
//     实现 DeleteClauses / QueryClauses，GORM 据此自动加 deleted_at IS NULL 过滤）
//
// 关键设计：
//   - Name 是唯一键，长度上限 30 字符；存的是「变量名」语义标识（如 site_name / icp），
//     后台【系统配置】界面通过 name 索引定位配置项。
//   - Type 决定后台表单渲染哪种输入控件（text / textarea / number / select / image 等）；
//     Type 字段命名上撞 Go 的 type 关键字，但 Go 允许结构体字段名为 type，故沿用 SQL 原名。
//   - Value / Content 可空：nil 表达「未设置」语义，与空字符串「用户显式清空」区分。
//   - AllowDel 控制后台 UI 是否允许删除该配置项；系统关键配置（如 site_name）设为 false。
//
// 字段顺序：业务字段在前、UpdatedAt → CreatedAt → DeletedAt 收尾。
type Config struct {
	// ID 主键（SQL: int UNSIGNED）。
	ID uint `gorm:"comment:ID;primaryKey;autoIncrement"`

	// Name 变量名，全局唯一。
	Name string `gorm:"comment:变量名;size:30;uniqueIndex;not null"`

	// Group 分组；用于后台【系统配置】按类目渲染。
	Group string `gorm:"comment:分组;size:30;default:'';not null"`

	// Title 变量标题；后台表单显示名。
	Title string `gorm:"comment:变量标题;size:50;default:'';not null"`

	// Tip 变量描述；后台表单 placeholder / helper。
	Tip string `gorm:"comment:变量描述;size:100;default:'';not null"`

	// Type 变量输入组件类型（text / textarea / number / select / image ...）。
	Type string `gorm:"comment:变量输入组件类型;size:30;default:'';not null"`

	// Value 变量值；nil 表示「未设置」，与空字符串不同。
	Value *string `gorm:"comment:变量值;type:longtext"`

	// Content 字典数据；Type 为 select / radio / checkbox 时存选项 JSON。
	Content *string `gorm:"comment:字典数据;type:longtext"`

	// Rule 验证规则。
	Rule string `gorm:"comment:验证规则;size:100;default:'';not null"`

	// Extend 扩展属性。
	Extend string `gorm:"comment:扩展属性;size:255;default:'';not null"`

	// InputExtend 输入框扩展属性。
	InputExtend string `gorm:"comment:输入框扩展属性;size:255;default:'';not null"`

	// AllowDel 是否允许后台删除：true=是, false=否。
	AllowDel bool `gorm:"comment:允许删除;default:false;not null"`

	// Weigh 权重；后台列表按 weigh DESC 排序。
	Weigh int `gorm:"comment:权重;default:0;not null"`

	// UpdatedAt 更新时间（GORM 自动维护）。
	UpdatedAt time.Time `gorm:"comment:更新时间;not null;autoUpdateTime"`

	// CreatedAt 创建时间（GORM 自动维护）。
	CreatedAt time.Time `gorm:"comment:创建时间;not null;autoCreateTime"`

	// DeletedAt 软删除标记（GORM 自动维护；Valid=true 即已删除）。
	DeletedAt gorm.DeletedAt `gorm:"comment:删除时间;index"`
}

// TableName 实现 schema.TablerWithNamer 接口 —— 由 GORM 传入带前缀配置的 namer，
// 我们把 "config" 交给它处理，这样 database 侧设置的 NamingStrategy.TablePrefix
// 才能真正落到表名上。若只实现 Tabler（TableName() string），前缀会被绕过。
func (Config) TableName(namer schema.Namer) string {
	return namer.TableName("config")
}

// init 在包加载时把 Config 注册到模型注册表。
//
// 注意：不要在这里直接调 AutoMigrate，迁移逻辑统一由 database.Init() 在
// GORM 实例准备好之后执行。
func init() {
	Register(Config{})
}
