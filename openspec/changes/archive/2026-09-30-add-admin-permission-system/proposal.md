# Proposal

## Why

后台管理员权限校验目前没有任何持久化与运行时能力 —— `internal/model.Admin` 只有 `Status` 一个状态字段，任何登录成功的管理员都能访问所有 admin 端点。后续【菜单/权限】管理页、admin API 的 handler 级鉴权都依赖一张「管理员 → 分组 → 权限规则」的数据图。本次变更先把这层底座（数据模型 + Repository + 权限 Manager）落地，handler 鉴权中间件与后台【角色管理】UI 留待后续 change。

## What Changes

- **新增** `internal/model/admin.go` 中追加 `AdminGroup` 与 `AdminGroupAccess` 两个模型（`AdminRule` 已在 `add-admin-rule-model` 中先行落地，作为本 change 的前置依赖之一）。三张表构成：管理员 `admin` 通过 `admin_group_access` 多对多关联到 `admin_group`，`admin_group.rules` 用文本存储 `admin_rule.id` 列表，特殊值 `*` 表示全权限（超管）。
- **新增** `cmd/migrate/migrations/000003_admin_group.{up,down}.sql`：建 `admin_group` 与 `admin_group_access` 两张表（PK 行为：`admin_group.id` 自增；`admin_group_access` 复合 PK `(uid, group_id)`）。`000002_admin_rule` 已在 `add-admin-rule-model` 落地，本 change 不重复。
- **新增** `internal/repository/admin/{rule,group}_repository.go` 与 `internal/repository/admin/access_repository.go`：按 CLAUDE.md 分层 `Handler → Service → Repository → Model`，只做 DB 搬运，不掺业务。
- **新增** `internal/service/admin/permission.go`：权限 Manager，公开方法 `GetRules(uid) []AdminRule`、`Check(uid, name) bool`、`GetGroups(uid) []AdminGroup`、`GetRuleIds(uid) []uint`、`IsSuperAdmin(uid) bool`。`*` 通配作为超管判定唯一机制；不引入 `admin.is_super_admin` 标志位。
- **不修改**：任何现有 capability 的需求、路由、handler、admin login/logout 的运行时行为。

## Capabilities

### New Capabilities

- `admin-permission`：后台管理员权限校验的数据底座 + 运行时 API。包含三张表的数据模型、按用户聚合组与权限规则的查询、规则名/规则 ID 列表的获取、单点权限判定、以及 `*` 通配的超管识别。

### Modified Capabilities

无（`admin-login` / `admin-logout` / `click-captcha` / `homepage` 的需求均不变）。

## Impact

- **代码**：新增 1 个模型文件追加段、3 个 repository 文件、1 个 service 文件、2 个 migration 文件；总增 ~400-500 行。
- **数据库**：新增 `admin_group` 与 `admin_group_access` 两张表；`admin_rule` 表已在 `add-admin-rule-model` 落地（前置依赖，本 change 不再重复）。
- **依赖**：无新增第三方依赖；GORM + 既有 `database.Get()` 即可。
- **后续 change 留口**：handler 级鉴权中间件（基于 `Permission.Check`）、后台【角色管理】/【菜单管理】UI、初始 seed 数据（如 `*` 分组）均不在本 change 范围。