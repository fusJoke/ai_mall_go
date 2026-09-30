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

// init 在包加载时把 Admin / AdminRule / AdminGroup / AdminGroupAccess 注册到模型注册表。
//
// 注意：不要在这里直接调 AutoMigrate，迁移逻辑统一由 database.Init() 在
// GORM 实例准备好之后执行（即使当前 database.Init() 已切到 golang-migrate，
// model.All() 仍然保留给运行时启动期一致性检查使用）。
func init() {
	Register(Admin{}, AdminRule{}, AdminGroup{}, AdminGroupAccess{})
}

// ============================================================================
// AdminRule 菜单和权限规则表（参考 ba_admin_rule，按项目惯例重塑）。
// ============================================================================

// AdminRuleType 规则类型枚举：dir=规则目录 / menu=菜单项 / node=权限节点。
type AdminRuleType string

// 规则类型枚举值。
const (
	RuleTypeDir  AdminRuleType = "dir"  // 规则目录
	RuleTypeMenu AdminRuleType = "menu" // 菜单项
	RuleTypeNode AdminRuleType = "node" // 权限节点
)

// AdminRuleOpenType 菜单打开方式枚举：tab=选项卡 / link=链接 / iframe=Iframe。
type AdminRuleOpenType string

// 菜单打开方式枚举值。
const (
	RuleOpenTab    AdminRuleOpenType = "tab"    // 选项卡
	RuleOpenLink   AdminRuleOpenType = "link"   // 链接
	RuleOpenIframe AdminRuleOpenType = "iframe" // Iframe
)

// AdminRuleExtend 扩展属性枚举：none=无 / add_route_only=只添加为路由 / add_menu_only=只添加为菜单。
type AdminRuleExtend string

// 扩展属性枚举值。
const (
	RuleExtendNone         AdminRuleExtend = "none"           // 无
	RuleExtendAddRouteOnly AdminRuleExtend = "add_route_only" // 只添加为路由
	RuleExtendAddMenuOnly  AdminRuleExtend = "add_menu_only"  // 只添加为菜单
)

// AdminRule 菜单和权限规则表（PID 自引用，构成规则树）。
//
// 关键设计：
//   - PID 自引用（pid=0 表示顶级），与 ID 同类型（uint）便于外键引用；
//   - Type / OpenType / Extend 用自定义 string 类型 + 常量，编译期拒绝拼写错值；
//     OpenType 用 *AdminRuleOpenType：权限节点 type=node 不需要该项，nil 表示"不适用"，
//     区别于"显式设为某个值"，与 Admin.Email / Admin.Mobile 处理方式一致；
//   - Keepalive 用 bool（严格二值），与 Config.AllowDel 风格一致；
//   - Status 1=启用 / 0=禁用，与 Admin.Status 风格一致；
//   - 时间戳字段按 CLAUDE.md「每张业务表都需包含」补齐：user SQL 用 create_time / update_time，
//     类型 bigint UNSIGNED，与项目惯例不符故舍弃；统一改为 created_at / updated_at，类型
//     datetime(3)，搭配 autoCreateTime / autoUpdateTime。
//
// 字段顺序：业务字段在前、UpdatedAt → CreatedAt → DeletedAt 收尾。
type AdminRule struct {
	// ID 主键（SQL: int UNSIGNED）。
	ID uint `gorm:"comment:ID;primaryKey;autoIncrement"`

	// Pid 上级规则 ID；0 表示顶级，构成"规则树"。
	Pid uint `gorm:"comment:上级规则;default:0;not null;index"`

	// Type 规则类型：RuleTypeDir=规则目录 / RuleTypeMenu=菜单项 / RuleTypeNode=权限节点。
	Type AdminRuleType `gorm:"comment:规则类型(dir=规则目录,menu=菜单项,node=权限节点);size:16;default:'menu';not null"`

	// Title 规则标题，用于后台菜单显示。
	Title string `gorm:"comment:规则标题;size:50;default:'';not null"`

	// Name 规则名称（英文标识 / 权限点 key）。
	Name string `gorm:"comment:规则名称;size:50;default:'';not null"`

	// Path 菜单路由路径（前端 router 配置）。
	Path string `gorm:"comment:菜单路由路径;size:100;default:'';not null"`

	// Icon 图标（lucide 等图标库的图标名）。
	Icon string `gorm:"comment:图标;size:50;default:'';not null"`

	// OpenType 菜单打开方式：RuleOpenTab=选项卡 / RuleOpenLink=链接 / RuleOpenIframe=Iframe；
	// 可空：nil 表示"不适用"（如纯权限节点 type=node 不需要此项）。
	OpenType *AdminRuleOpenType `gorm:"comment:菜单打开方式(tab=选项卡,link=链接,iframe=Iframe);size:16"`

	// Url 菜单 URL（外链 / 实际跳转地址）。
	Url string `gorm:"comment:菜单URL;size:255;default:'';not null"`

	// Component 菜单组件路径（前端组件文件路径）。
	Component string `gorm:"comment:菜单组件路径;size:100;default:'';not null"`

	// Keepalive 是否缓存该路由页面：true=开启 / false=关闭。
	Keepalive bool `gorm:"comment:缓存(0=关闭,1=开启);default:false;not null"`

	// Extend 扩展属性：RuleExtendNone=无 / RuleExtendAddRouteOnly=只添加为路由 / RuleExtendAddMenuOnly=只添加为菜单。
	Extend AdminRuleExtend `gorm:"comment:扩展属性(none=无,add_route_only=只添加为路由,add_menu_only=只添加为菜单);size:32;default:'none';not null"`

	// Remark 备注。
	Remark string `gorm:"comment:备注;size:255;default:'';not null"`

	// Weigh 权重，列表按 weigh DESC 排序（小者靠前）。
	Weigh int `gorm:"comment:权重;default:0;not null"`

	// Status 状态：1=启用 / 0=禁用。
	Status int8 `gorm:"comment:状态(1启用,0禁用);default:1;not null"`

	// UpdatedAt 更新时间（GORM 自动维护）。
	UpdatedAt time.Time `gorm:"comment:更新时间;not null;autoUpdateTime"`

	// CreatedAt 创建时间（GORM 自动维护）。
	CreatedAt time.Time `gorm:"comment:创建时间;not null;autoCreateTime"`

	// DeletedAt 软删除标记（GORM 自动维护；Valid=true 即已删除）。
	DeletedAt gorm.DeletedAt `gorm:"comment:删除时间;index"`
}

// TableName 实现 schema.TablerWithNamer 接口 —— 由 GORM 传入带前缀配置的 namer，
// 我们把 "admin_rule" 交给它处理，这样 database 侧设置的 NamingStrategy.TablePrefix
// 才能真正落到表名上。若只实现 Tabler（TableName() string），前缀会被绕过。
func (AdminRule) TableName(namer schema.Namer) string {
	return namer.TableName("admin_rule")
}

// ============================================================================
// AdminGroup 管理员分组表（参考 ba_admin_group，按项目惯例重塑）。
// ============================================================================

// AdminGroup 管理员分组表，存储分组元信息及该组持有的权限规则 id 列表。
//
// 关键设计：
//   - Pid 用 *uint（可空）：顶级分组 pid=NULL，与 AdminRule.Pid=0 顶级语义对齐
//     但表达方式不同 —— admin_group 的分组树允许 pid=NULL 表达"无父节点"；
//   - Rules 用 *string（可空）：内容是 admin_rule.id 的逗号分隔列表，特殊值 '*'
//     表示该组成员为超管。空字符串 / nil 表达"无规则"。
//   - Status 1=启用 / 0=禁用，与 Admin.Status / AdminRule.Status 风格一致。
//
// 字段顺序：业务字段在前、UpdatedAt → CreatedAt → DeletedAt 收尾。
type AdminGroup struct {
	// ID 主键（SQL: int UNSIGNED）。
	ID uint `gorm:"comment:ID;primaryKey;autoIncrement"`

	// Pid 上级分组 ID；nil 表示顶级（无父节点）。
	Pid *uint `gorm:"comment:上级分组"`

	// Name 组名。
	Name string `gorm:"comment:组名;size:100;default:'';not null"`

	// Rules 权限规则 ID 集：逗号分隔的 admin_rule.id 列表，'*' 表示全权限（超管）；
	// 可空：nil 表示"未配置任何规则"。
	Rules *string `gorm:"comment:权限规则ID集;type:text"`

	// Status 状态：1=启用 / 0=禁用。
	Status int8 `gorm:"comment:状态(1启用,0禁用);default:1;not null"`

	// UpdatedAt 更新时间（GORM 自动维护）。
	UpdatedAt time.Time `gorm:"comment:更新时间;not null;autoUpdateTime"`

	// CreatedAt 创建时间（GORM 自动维护）。
	CreatedAt time.Time `gorm:"comment:创建时间;not null;autoCreateTime"`

	// DeletedAt 软删除标记（GORM 自动维护；Valid=true 即已删除）。
	DeletedAt gorm.DeletedAt `gorm:"comment:删除时间;index"`
}

// TableName 实现 schema.TablerWithNamer 接口 —— 由 GORM 传入带前缀配置的 namer，
// 我们把 "admin_group" 交给它处理，这样 database 侧设置的 NamingStrategy.TablePrefix
// 才能真正落到表名上。
func (AdminGroup) TableName(namer schema.Namer) string {
	return namer.TableName("admin_group")
}

// ============================================================================
// AdminGroupAccess 管理员与分组的多对多关系表。
// ============================================================================

// AdminGroupAccess 管理员与分组的映射表。
//
// 关键设计：
//   - 复合主键 (Uid, GroupID)：天然表达"某个管理员在某个组里"的精确关系，
//     不需要额外的自增 id 列；
//   - 两个主键字段均非自增，与 AdminRule.ID 的自增策略不同；
//   - 时间戳字段按 CLAUDE.md 公共字段规定补齐；DeletedAt 保留以便未来审计
//     "何时把谁移出了哪个组"，但 Permission Manager 实际并不软删该表 ——
//     移除关系时直接走硬删，避免与复合 PK 的唯一性冲突。
//
// 字段顺序：复合主键在前、UpdatedAt → CreatedAt → DeletedAt 收尾。
type AdminGroupAccess struct {
	// Uid 管理员 ID（复合主键之一）。
	Uid uint `gorm:"comment:管理员ID;primaryKey;not null"`

	// GroupID 分组 ID（复合主键之一）。
	GroupID uint `gorm:"comment:分组ID;primaryKey;not null"`

	// UpdatedAt 更新时间（GORM 自动维护）。
	UpdatedAt time.Time `gorm:"comment:更新时间;not null;autoUpdateTime"`

	// CreatedAt 创建时间（GORM 自动维护）。
	CreatedAt time.Time `gorm:"comment:创建时间;not null;autoCreateTime"`

	// DeletedAt 软删除标记（GORM 自动维护；Valid=true 即已删除）。
	DeletedAt gorm.DeletedAt `gorm:"comment:删除时间;index"`
}

// TableName 实现 schema.TablerWithNamer 接口 —— 由 GORM 传入带前缀配置的 namer，
// 我们把 "admin_group_access" 交给它处理，这样 database 侧设置的 NamingStrategy.TablePrefix
// 才能真正落到表名上。
func (AdminGroupAccess) TableName(namer schema.Namer) string {
	return namer.TableName("admin_group_access")
}
