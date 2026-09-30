# Design

## Context

背景与动机见 [proposal.md](proposal.md) - Why。约束摘要：

- 后端已有 `AdminAuth` 中间件（仅做 token 校验，未把 admin 实体放进 gin context）。
- `internal/service/admin/permission.go` 已落地 `Manager`（5 个方法：`IsSuperAdmin` / `Check` / `GetGroups` / `GetRules` / `GetRuleIds`），init 端点直接复用。
- `internal/model/admin.go` / `internal/model/config.go` 已存在；`admin_rule` 表已带 `type`（dir/menu/node）/ `open_type` / `component` / `keepalive` / `weigh` 等字段。
- 前端 `web/src/layouts/admin/index.vue` 当前是空占位；登录跳转已能落到 `/admin` → `/admin/loading`，但 `/admin/loading` 仅渲染"加载中..."静态文案，没有任何后续动作。
- 前端 `useAdminInfo`（已持久化）/ `useConfig` / `useMenu` 三个 pinia store 已有，需要扩展字段并接入 init 数据。
- `web/src/router/static/adminBase.ts` 已配置 `/admin` 父路由 + `loading/:to?` 子路由 + 通配 fallback。本 change 不动静态路由配置。

## Goals / Non-Goals

**Goals:**

- 后端 `Init` 端点用一次 GET 拉齐"管理员 / 站点配置 / 菜单规则"三个数据维度，减少前端初始化请求次数。
- 后端负责把 `admin_rule` 行转换为 vue-router `RouteRecordRaw` 兼容结构，前端不再做"行 → 路由"的转换逻辑。
- 前端 init 流程串起"调接口 → 填 store → 注册路由 → 跳首个菜单"，并对 401 / 无菜单 / 重复 mount 有明确兜底。
- 全部错误兜底幂等：失败可重试，不留半成品 store / 路由。

**Non-Goals:**

- 不实现"管理员按角色完全不同的首屏"（所有登录管理员都跳第一个菜单）。
- 不在 init 端点返回菜单层级（树状结构），由前端把扁平 `menus` 按 `pid` 组装为树。
- 不修改 `AdminAuth` 已有的 401 错误码体系（401 直接复用）。
- 不在 init 端点实现"获取当前管理员可访问的所有权限节点"用于按钮级判定（按钮级 `Check` 由前端后续单独走 `permissionManager.Check()`，本 change 不暴露 `/admin/check`）。
- 不做菜单国际化（`meta.title` 直接吐原文字符串）。

## Decisions

### D1. `AdminAuth` 中间件扩展：把 admin 实体挂到 context

现状：`middleware/auth.go` 的 `AdminAuth` 只校验 token（存在 / 过期 / 类型），不读 admin。Init 端点需要 admin 实体的字段（nickname / avatar / last_login_*），因此在中间件查一次 DB。

实现：token 校验通过后，`c.Set(adminContextKey, adminRecord)` + `c.Next()`。引入 `adminRepo.NewAdminRepository(db)`（已有）做单点查询。

**替代方案**：handler 单独查 admin。
- 拒绝原因：handler 再查一次意味着 init 端点需要拿 repo；且后续其它 admin 路由（如 `/admin/profile`）也要拿 admin。把 admin 挂到 context 是公共契约，所有 admin 路由都能直接复用。

**风险**：每次 admin 路由多一次 DB 读。Mitigation：admin 主键查询走 dbresolver 的读副本（`First` 默认路由到读库）；加 `cacheKey(uid)` 的内存缓存可选，本 change 不做。

### D2. Init 端点拆为独立 `InitService`，不复用现有 admin `Service`

新增 `internal/service/admin/init.go`，与现有 `admin.Service`（Login / Logout 业务）并列。

**替代方案**：把 Init 方法加到现有 `Service` 接口。
- 拒绝原因：`Service` 已嵌入 `service.CRUDService[model.Admin]` 用于通用 CRUD，Init 业务调用 Permission Manager + Config Repository，跟 CRUD 不在一个抽象层级。混在一起会让接口臃肿且难测。

InitService 依赖：
- `adminRepo.AdminRepository`（拿 admin）
- `adminSvc.PermissionManager`（拿 menus + super 标记）
- `configRepo.ConfigRepository`（拿 site_config）

### D3. 菜单规则 → vue-router 路由的转换放在 service 层

`initService.routeFromRule(*model.AdminRule) RouteRecordRaw` 是纯函数，方便测试。转换规则按 spec ADDED Requirement "转换 admin_rule 行到 vue-router 路由对象" 实现：

| 字段 | 来源 | 退化 |
| --- | --- | --- |
| `path` | `rule.path` | 空 → `"rule-{id}"` |
| `name` | `rule.name` | 空 → `"rule-{id}"` |
| `component` | `rule.component` | 空 → 固定占位字符串 `"404"`（前端 router 用此走 404 页） |
| `meta.title` | `rule.title` | — |
| `meta.icon` | `rule.icon` | — |
| `meta.keepalive` | `rule.keepalive` | — |
| `meta.menuType` | `rule.type` | — |
| `meta.openType` | `rule.open_type` | nil → 不写字段 |
| `meta.weigh` | `rule.weight` | — |
| `meta.id` | `rule.id` | — |
| `meta.pid` | `rule.pid` | — |
| `meta.extend` | `rule.extend` | — |

`rule.type == "node"`：service 层产出 `RouteRecordRaw` 仍照常返回给前端（前端按 spec 不注册为路由，但保留行数据用于权限点展示），由前端按 `meta.menuType == 'node'` 跳过 `addRoute`。

**替代方案**：转换逻辑下沉到前端。
- 拒绝原因：后端转好后，前端只是 `addRoute` 调用，不用关心字段映射；前端逻辑更轻。后端也好测试（纯函数）。

### D4. 站点配置查询走 `ConfigRepository.ListByNames(names []string)`

新增 `internal/repository/admin/config_repository.go`。单条 `SELECT * FROM config WHERE name IN ? AND deleted_at IS NULL`，返回 `[]model.Config`。

**替代方案**：直接用 `database.Get()` 在 InitService 里写 SQL。
- 拒绝原因：违反 CLAUDE.md 分层（Repository 层不掺业务，但 service 层不该写 SQL）。InitService 只调 `ConfigRepository.ListByNames(["name", "record_number", "version"])`。

返回字段映射在 service 层：把 `[]Config` → `map[string]string`，缺失键 → 空串。

### D5. 菜单规则的两种数据来源

- 超管 → `RuleRepository.ListActiveAsMenu()`（新增方法）：返回全量启用规则，按 `weigh ASC, id ASC`。
- 普通管理员 → `PermissionManager.GetRules(uid)`：已含 status=1 过滤 + weigh/id 排序。

超管走专属 Repository 方法，普通走 Permission Manager，**不**统一为 `Manager.GetRules`（Manager 的"超管返回全部"用 `ListActiveIDs` 实现，需要二次 `ListByIDs` 才能拿到完整字段，多一次 DB 读）。直接走 Repository 一次拿全量更省。

### D6. 前端 init 流程的位置：放在 `web/src/layouts/admin/index.vue` 而非 `loading.vue`

现状：`loading.vue` 是占位组件，没有路由挂载钩子。把 init 逻辑放进 `layouts/admin/index.vue` 的 `onMounted`，因为 `router-view` 在该 layout 内部，layout 本身只 mount 一次（login 跳过来后），后续 route 切换不会再 mount 一次。

**替代方案**：放进 `loading.vue`。
- 拒绝原因：loading 是"占位提示"语义组件，业务初始化应放 layout 层；且未来切换 loading 样式不影响 init 逻辑。

### D7. 动态路由注册的幂等性

前端按 `route.name` 去重：注册前先 `if (router.hasRoute(name)) router.removeRoute(name)`；然后 `router.addRoute`。配合 useMenu store 的覆盖式赋值（`setRawData` 整体覆盖，不追加），保证重复 mount 不残留脏路由。

**替代方案**：用 `router.getRoutes()` 比对。
- 拒绝原因：vue-router 没有 `hasRoute` 的官方 API（v4 实际有 `hasRoute(name)`），但通过 name 维护去重表最简单。本设计直接用 `hasRoute`。

### D8. Init 端点的路由注册位置

`internal/router/admin/admin.go` 现有 `init()` 已注册 3 条路由（`/admin/ping` / `/admin/login` / `/admin/logout`）。本 change 在同一 `init()` 内追加：

```go
registry.Register("/admin", http.MethodGet, "/init", func(c *gin.Context) {
    ensureDeps()
    adminInitHandlerInst.Init(c)
}, middleware.AdminAuth())
```

新增包级 `adminInitHandlerInst`（与 `adminHandlerInst` 同级），构造时复用已有的 `permissionManager`（Permission Manager.Default()）。

### D9. error 分类

- 中间件层（401）：由 `AdminAuth` 处理，init 端点不再判 token。
- service 层（403）：管理员 `status != 1` → `ErrAdminDisabled`（复用 `adminSvc.ErrAccountDisabled`，因为语义相同）。
- service 层（500）：DB / 转换器异常 → 直接 `return err`，handler 统一 envelope `{code: "admin.init.internal", message: "init failed"}`。

### D10. 前端 `useConfig` store 扩展

现状 `useConfig` 是 composition store，新增：

```ts
const siteConfig = reactive<{ name: string; record_number: string; version: string }>({
    name: '',
    record_number: '',
    version: '',
})
function setSiteConfig(data: Partial<typeof siteConfig>) { Object.assign(siteConfig, data) }
```

**替代方案**：新建独立 `useSiteConfig` store。
- 拒绝原因：site config 是"配置"语义，与现有 layout/lang/crud 同源，且只有 3 个字段，扩 `useConfig` 比新建 store 成本低。

### D11. 类型定义位置

`InitResponse` / `SiteConfig` / `MenuRule` 三个类型放在 `web/src/stores/interface/index.ts`（已有 `AdminInfo` / `LoginRequest` / `LoginResponse` 风格一致）。

## Risks / Trade-offs

- **Init 端点每次都查 DB**（admin + config + rule）→ 单次请求 3 条 SELECT。Mitigation：均为走主键 / IN 查询，索引已建；上线后视情况加缓存。
- **前端 init 失败时已有动态路由残留**（用户刷新页面）→ Mitigaion：layout 的 `onMounted` 入口先 `router.getRoutes().filter(r => r.meta?.menuType).forEach(r => router.removeRoute(r.name))` 清空上一次的菜单路由再注册。
- **超管场景返回全量启用规则**（可能很大）→ Mitigation：`admin_rule` 是菜单规则，规模 ≤ 数百；JSON 响应在 KB 量级，不构成性能瓶颈。
- **404 兜底组件在前端**用 `web/src/views/404.vue` 已有文件，不需新增视图文件。
- **`init` 函数命名冲突**：前端已有 `init()` 含义在 vite / vue 生态里指"应用启动"。本 change 的 `init()` 是 api 层函数，命名遵循 `web/src/api/admin/index.ts` 已有 `login()` / `logout()` 模式，保留。
- **middleware AdminAuth 加 DB 查询**：增加所有 admin 路由的 latency。Mitigation：后续如果加 cache 可以收敛；当前阶段 admin 路由数量少，影响可控。

## Migration Plan

无 schema 变更、无数据迁移。本 change 是纯新增 + 扩展：

1. 部署后端：合并 `internal/middleware/auth.go` + `internal/handler/admin/init.go` + `internal/service/admin/init.go` + `internal/repository/admin/config_repository.go` + `internal/repository/admin/rule_repository.go` + `internal/router/admin/admin.go`。
2. 部署前端：合并 `web/src/api/admin/index.ts` + `web/src/stores/interface/index.ts` + `web/src/stores/config.ts` + `web/src/layouts/admin/index.vue`。
3. 回滚：纯代码变更，revert 即可，无破坏性。

## Open Questions

- 前端"动态路由的 component 路径"目前用 `rule.component` 字段直接当 component path。如果 `rule.component` 存的是后端组件标识（如 `user/list`）而非真实前端路径，需要一个 component 路径映射表。本 change 假设 `rule.component` 就是前端真实路径（与 `path` 同源），与现有 `web/src/router/static/adminBase.ts` 的 `import('/@/views/admin/login.vue')` 风格兼容。如有偏差，需要在 spec 里加 component 映射表 — 但当前没有原始表 schema，待实现时若发现冲突再回头修订。