# Spec Delta

## Purpose

后台【菜单 / 权限规则】的数据模型。持久化后台界面渲染菜单树与权限分组赋权所需的结构（pid 自引用构成规则树）、类别（dir / menu / node）、打开方式（tab / link / iframe）、扩展属性（none / add_route_only / add_menu_only）、排序权重与启用状态。后续 Permission Manager 与后台【菜单管理】UI 都消费这张表。

## ADDED Requirements

### Requirement: Rule field schema

系统 SHALL 提供 `admin_rule` 表，包含下列字段，类型与默认值如下：

| 字段 | 类型 | 默认值 | 可空 | 说明 |
|---|---|---|---|---|
| `id` | int UNSIGNED | auto increment | 否 | 主键 |
| `pid` | int UNSIGNED | 0 | 否 | 上级规则 id |
| `type` | varchar(16) | 'menu' | 否 | 规则类别，仅取 `dir` / `menu` / `node` |
| `title` | varchar(50) | '' | 否 | 规则标题 |
| `name` | varchar(50) | '' | 否 | 规则名（英文标识 / 权限点 key） |
| `path` | varchar(100) | '' | 否 | 前端路由路径 |
| `icon` | varchar(50) | '' | 否 | 图标名 |
| `open_type` | varchar(16) | NULL | 是 | 打开方式，仅取 `tab` / `link` / `iframe` |
| `url` | varchar(255) | '' | 否 | 菜单 URL |
| `component` | varchar(100) | '' | 否 | 菜单组件路径 |
| `keepalive` | tinyint(1) | 0 | 否 | 是否缓存路由页面（0=关闭 / 1=开启） |
| `extend` | varchar(32) | 'none' | 否 | 扩展属性，仅取 `none` / `add_route_only` / `add_menu_only` |
| `remark` | varchar(255) | '' | 否 | 备注 |
| `weigh` | int | 0 | 否 | 权重（列表排序） |
| `status` | tinyint | 1 | 否 | 状态（0=禁用 / 1=启用） |
| `created_at` | datetime(3) | now(3) | 否 | 创建时间，GORM 自动维护 |
| `updated_at` | datetime(3) | now(3) | 否 | 更新时间，GORM 自动维护 |
| `deleted_at` | datetime(3) | NULL | 是 | 软删除标记，GORM 自动维护 |

#### Scenario: New rule with minimal fields
- **WHEN** 客户端插入一条仅指定 `name='user.read'` 的记录
- **THEN** 数据库写入该条记录，且 `id` 自增；`pid` 为 0；`type` 为 `menu`；`status` 为 1；`keepalive` 为 0；`extend` 为 `none`；`created_at` / `updated_at` 为当前时间；`deleted_at` 为 NULL

#### Scenario: Insert with custom type and extend
- **WHEN** 客户端插入一条 `type='node'`、`extend='add_route_only'`、`pid=2` 的记录
- **THEN** 数据库接受该条记录，字段值按字面存储

### Requirement: PID self-reference for rule tree

`pid` SHALL 引用同表的 `id`，构成规则树。`pid=0` SHALL 表示顶级（无父节点）。

#### Scenario: Top-level rule
- **WHEN** 插入一条 `pid=0` 的记录
- **THEN** 该记录为顶级规则，不存在父节点

#### Scenario: Child rule
- **WHEN** 插入一条 `pid` 等于同表某条已存在记录 `id` 的记录
- **THEN** 该记录视作父规则的子节点

#### Scenario: Index lookup by pid
- **WHEN** 客户端查询 `pid = N` 的全部记录
- **THEN** 系统返回所有以 N 为父节点的记录（含软删之外的记录；软删规则见单独 Requirement）

### Requirement: Enum type for rule category

`type` SHALL 仅接受三个取值：`dir`（规则目录）/ `menu`（菜单项）/ `node`（权限节点）。

#### Scenario: Valid type accepted
- **WHEN** 客户端写入 `type='dir'` / `type='menu'` / `type='node'` 三种值之一
- **THEN** 数据库接受写入

#### Scenario: Unknown type rejected by application
- **WHEN** 调用方代码尝试写入 `type='unknown'`（非三个取值之一）
- **THEN** 编译期拒绝（Go 端 `AdminRuleType` 是自定义 string 类型，仅接受常量 `RuleTypeDir` / `RuleTypeMenu` / `RuleTypeNode` 赋值）；运行期若绕过类型直接写字符串则由 DB 层 `varchar(16)` 容纳，但调用方代码应不出现此场景

### Requirement: Nullable enum for open type

`open_type` SHALL 可为 NULL，表示「不适用」；非 NULL 时 SHALL 仅接受三个取值：`tab`（选项卡）/ `link`（链接）/ `iframe`（Iframe）。

#### Scenario: Null open_type for permission nodes
- **WHEN** 插入一条 `type='node'` 的纯权限节点记录，`open_type` 不指定
- **THEN** 数据库写入 `open_type=NULL`

#### Scenario: Tab open_type for menu
- **WHEN** 插入一条 `type='menu'`、`open_type='tab'` 的菜单项
- **THEN** 数据库接受，菜单以选项卡形式打开

#### Scenario: Link open_type
- **WHEN** 插入一条 `open_type='link'` 的菜单项
- **THEN** 数据库接受，菜单以外链形式打开

### Requirement: Enum type for extend property

`extend` SHALL 仅接受三个取值：`none`（无）/ `add_route_only`（只添加为路由）/ `add_menu_only`（只添加为菜单）。

#### Scenario: Default extend is none
- **WHEN** 客户端插入记录时不指定 `extend`
- **THEN** 数据库写入 `extend='none'`

#### Scenario: Route-only rule
- **WHEN** 插入 `extend='add_route_only'` 的记录
- **THEN** 该规则只注册为前端路由，不出现在菜单树中

### Requirement: Status field gates visibility

`status` SHALL 控制规则是否启用：1=启用、0=禁用。查询接口 SHALL 在默认行为下排除 `status=0` 的记录。

#### Scenario: Disabled rule excluded from default listing
- **WHEN** 某条记录被写入 `status=0`
- **THEN** 该条记录不参与默认列表查询结果，但行本身仍存在

#### Scenario: Enabled rule included
- **WHEN** 某条记录被写入 `status=1`（默认值）
- **THEN** 该条记录参与默认列表查询结果

### Requirement: Soft delete via deleted_at

`deleted_at` SHALL 支持软删除：默认查询 SHALL 排除 `deleted_at IS NOT NULL` 的记录；软删记录 SHALL 保留行以供审计。

#### Scenario: Default query excludes soft-deleted rows
- **WHEN** 客户端执行默认查询（即不带 `Unscoped()`）
- **THEN** 返回结果中不包含 `deleted_at IS NOT NULL` 的行

#### Scenario: Soft-deleted row preserved on disk
- **WHEN** 客户端调用软删（如 GORM `Delete(&rule)`）
- **THEN** 数据库该行的 `deleted_at` 被设为当前时间，行本身不被物理删除

#### Scenario: Unscoped query includes soft-deleted rows
- **WHEN** 客户端执行 `Unscoped()` 查询
- **THEN** 返回结果中包含全部记录（含 `deleted_at IS NOT NULL`）

### Requirement: created_at and updated_at auto-maintained

`created_at` SHALL 在记录创建时由 GORM 自动设置为当前时间，`updated_at` SHALL 在记录每次更新（含软删）时由 GORM 自动刷新为当前时间。客户端 SHALL 不直接写入这两个字段。

#### Scenario: Auto-fill on insert
- **WHEN** 客户端插入一条新记录（不指定 `created_at` / `updated_at`）
- **THEN** 数据库写入当前时间到这两个字段

#### Scenario: updated_at refreshed on update
- **WHEN** 客户端更新一条已存在记录的任意业务字段
- **THEN** 该记录的 `updated_at` 被刷新为更新时刻；`created_at` 保持不变

### Requirement: Default values for optional string fields

`title` / `name` / `path` / `icon` / `url` / `component` / `remark` 在客户端不指定时 SHALL 默认写为空字符串。

#### Scenario: Insert without optional strings
- **WHEN** 客户端插入一条仅指定 `name='x.read'` 的记录，不指定其他字符串字段
- **THEN** 数据库写入 `title=''`、`path=''`、`icon=''`、`url=''`、`component=''`、`remark=''`

### Requirement: Weigh field for ordering

`weigh` SHALL 提供排序权重，调用方 SHALL 按 `weigh` 升序、`id` 升序稳定排序展示规则列表。

#### Scenario: Rules sorted by weigh
- **WHEN** 客户端按 `weigh ASC, id ASC` 拉取规则列表
- **THEN** 系统按权重升序返回，权重相同按 id 升序

### Requirement: Index on pid

`pid` SHALL 建索引以加速子节点查询。

#### Scenario: Index exists on pid
- **WHEN** 客户端检查 `admin_rule` 表的索引
- **THEN** 系统报告 `pid` 字段存在索引