# Proposal

## Why

后台【菜单 / 权限规则】管理需要持久化层 —— 后台界面要渲染菜单树、API 层要按规则名鉴权、权限分组要把规则 id 列表赋给某个角色，都依赖一张可枚举、可分组、可软删的规则表。本次变更先把规则表的数据模型与 schema 落下来，repository / service / handler / UI 留待后续 change（在 `add-admin-permission-system` 中已规划 Permission Manager 的运行时 API，本 change 不重复）。

> 本 change 实际代码已在分支中落地（`internal/model/admin.go` 追加 `AdminRule` + 3 个枚举类型、`cmd/migrate/migrations/000002_admin_rule.{up,down}.sql`），本次 OpenSpec 文档为事后补齐。

## What Changes

- **新增** `internal/model/admin.go` 中追加 `AdminRule` 结构体 + 3 个枚举类型 `AdminRuleType` / `AdminRuleOpenType` / `AdminRuleExtend` + 9 个常量（`RuleType*` / `RuleOpen*` / `RuleExtend*`）；`init()` 改为一次性 `Register(Admin{}, AdminRule{})`。
- **新增** `cmd/migrate/migrations/000002_admin_rule.up.sql`：建 `admin_rule` 表（17 个字段 + 主键 + pid / deleted_at 索引），与 `AdminRule` 结构体字段一一对齐。
- **新增** `cmd/migrate/migrations/000002_admin_rule.down.sql`：`DROP TABLE IF EXISTS admin_rule;`。
- **不修改**：既有任何 capability 的需求、路由、handler、service、repository、`admins` / `tokens` / `users` / `captchas` / `config` 表结构。

## Capabilities

### New Capabilities

- `admin-rule`：后台【菜单与权限规则】的数据模型。包含三轴字段（结构 / 类别 / 扩展属性）的枚举约束、状态 / 软删 / 启用三个生命周期钩子字段、PID 自引用构造规则树。后续的 Permission Manager 消费这张表。

### Modified Capabilities

无（`admin-login` / `admin-logout` / `click-captcha` / `homepage` / `site-config` 的需求均不变）。

## Impact

- **代码**：新增 1 个模型文件追加段（约 100 行，含类型 / 常量 / 结构体 / TableName）、2 个 migration 文件（约 50 行）。
- **数据库**：新增 `admin_rule` 表（17 列 + 主键 + 2 索引）。
- **依赖**：无新增第三方依赖。
- **后续 change 留口**：Permission Manager（`add-admin-permission-system` 已规划）、admin_rule_repository / service / handler、后台【菜单管理】UI 均不在本 change 范围。