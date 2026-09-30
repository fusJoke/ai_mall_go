# Proposal

## Why

管理员登录后跳到 `/admin/loading` 占位页面，目前没有真正调用后端的"后台初始化"接口——菜单路由靠前端硬编码、配置项用临时 MOCK。后台可用菜单由 `admin_rule` 表管理（运行时可视化配置），需要一次"拉取当前管理员上下文 + 站点配置 + 权限菜单"的合并请求，由前端据此注册 `vue-router` 动态路由并跳转第一个菜单。本 change 落这个初始化能力，串起"登录 → 动态菜单 → 第一个路由"完整闭环。

## What Changes

- 新增后端 `GET /admin/init` 接口（挂 `AdminAuth` 中间件）：
  - 响应当前管理员信息（id / username / nickname / avatar / last_login_at / last_login_ip / super）
  - 响应站点基础配置（取 `config` 表中 `name IN (name|record_number|version)` 三行的 `value`）
  - 响应当前管理员持有的菜单规则（`admin_rule` 中 status=1 且管理员有权访问的行；超管返回所有启用规则，普通管理员按 Permission Manager 聚合去重）
- 后端把 `admin.Rule` 表结构 → 前端 vue-router 路由结构的转换下沉到 service 层，handler 只做参数与响应序列化。
- 前端 `web/src/api/admin/index.ts` 新增 `init()` 请求函数（GET `/admin/init`）。
- 前端 `web/src/layouts/admin/index.vue` 在路由挂载时执行 init：调后端 → 写入 `useAdminInfo` / `useConfig` / `useMenu` 三个 store → 把菜单规则动态 `router.addRoute` → `router.replace` 到第一个可用菜单。
- 扩展 `middleware.AdminAuth`：在请求上下文 `c.Set(adminContextKey, *model.Admin)` 保存当前管理员记录（之前只校验 token，未保存管理员），handler 可从 context 读出。
- `internal/repository/admin/rule_repository.go` 新增 `ListActiveAsMenu()`（返回整张启用规则表），让 init 时一次性拿到菜单树数据；Permission Manager 内部已有的 `ListActiveIDs()` / `ListByIDs()` 复用不变。
- `internal/service/admin/init.go`：新增 `InitService` 接口与 `initService` 实现，串起"读 admin → 读 site config → 读 menu rules → 转换为 vue-router routes"。

## Capabilities

### New Capabilities

- `admin-init`：后台初始化能力。后端 `GET /admin/init` 在登录后被前端调用，聚合返回管理员信息 + 站点配置 + 权限菜单；前端 init 流程据此填充 store + 注册动态路由 + 跳第一个菜单。

### Modified Capabilities

无（admin-login / admin-permission / site-config 的 REQUIREMENTS 不变，仅被 init 复用其实现）。

## Impact

- **后端新增/修改文件**：
  - `internal/middleware/auth.go`：在 AdminAuth 校验 token 通过后查 admin 并 `c.Set` 到上下文（新增 `adminContextKey` 常量与 getter 辅助）。
  - `internal/handler/admin/init.go`：新增 `InitHandler` 与 `Init(c *gin.Context)` 方法。
  - `internal/service/admin/init.go`：新增 `InitService` 接口 + `initService` 实现 + `routeFromRule(*model.AdminRule) RouteRecordRaw` 转换器。
  - `internal/repository/admin/rule_repository.go`：新增 `ListActiveAsMenu()` 方法（返回 `[]AdminRule` 全量启用规则）。
  - `internal/repository/admin/config_repository.go`：新增 `ConfigRepository` 接口与 `ListByNames(names []string) ([]model.Config, error)` 实现（GORM `WHERE name IN ?`）。
  - `internal/router/admin/admin.go`：注册 `GET /admin/init`，挂 `AdminAuth()`。
- **前端新增/修改文件**：
  - `web/src/api/admin/index.ts`：新增 `init()` 函数 + `InitResponse` 类型（移到 `web/src/stores/interface/index.ts`）。
  - `web/src/stores/interface/index.ts`：新增 `InitResponse` / `SiteConfig` / `MenuRule` 类型。
  - `web/src/stores/config.ts`：扩展 store 增加 siteConfig 字段（site name / record number / version）+ `setSiteConfig()` action。
  - `web/src/layouts/admin/index.vue`：从空占位改为完整 init 流程（新增 `<script setup>` 含 `onMounted` 调 init）。
- **数据流**：登录 → `setToken + dataFill(adminInfo)` → `router.push('/admin')` → `/admin` 重定向到 `/admin/loading` → `<router-view>` 渲染 `web/src/layouts/admin/index.vue` → 该 layout 在 `onMounted` 调 `init()` → 拉数据 → 注册路由 → `router.replace(第一个菜单)`。
- **依赖**：复用 `add-admin-permission-system` 的 Permission Manager（`internal/service/admin/permission.go`）、`add-admin-rule-model` 的 AdminRule 模型、`add-config-model` 的 Config 模型。无新依赖。
- **MySQL**：本 change 不改 schema，无新迁移文件。
- **测试影响**：Permission Manager 已有 5 个测试不受影响；新增 init service 的菜单路由转换测试（纯函数）+ handler/service 的 mock 测试。