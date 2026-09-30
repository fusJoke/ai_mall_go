# admin-init Specification

## Purpose
后台初始化能力：管理员登录后跳转到 `/admin/loading` 占位页面，前端在挂载后台根布局时调一次 `GET /admin/init`，由后端返回当前管理员信息、站点基础配置（site name / record number / version）、当前管理员持有的启用菜单规则。前端据此填充状态存储、把菜单规则注册为 `vue-router` 动态路由、再跳转到第一个可用菜单。整个初始化调用对前端必须可幂等重试，不引入持久化副作用。

## Requirements

### Requirement: `GET /admin/init` 端点

服务端 SHALL 提供 `GET /admin/init` 端点，挂 `AdminAuth` 中间件，仅接受合法 admin Bearer token 的请求；token 缺失 / 无效 / 类型不符 MUST 由中间件统一返回 401，本端点不重做校验。

#### Scenario: 合法 admin token 调用成功

- **WHEN** 客户端携带合法 admin Bearer token 请求 `GET /admin/init`
- **THEN** 响应 200，body 为 `{admin, site_config, menus}`
- **AND** `admin` 字段含 id / username / nickname / avatar / last_login_at / last_login_ip / super
- **AND** `site_config` 字段为对象，键为 `name` / `record_number` / `version`，值取自 `config` 表对应行的 `value`（缺失则为空字符串）
- **AND** `menus` 字段为数组，元素为 `admin_rule` 行投影（id / pid / type / title / name / path / icon / open_type / url / component / keepalive / extend / weigh）

#### Scenario: 缺 / 无效 token

- **WHEN** 客户端未携带 Bearer token 或 token 不合法
- **THEN** 响应 401，错误码 `auth.*`（与既有 admin 路由一致）

#### Scenario: 已禁用管理员

- **WHEN** 当前管理员 `status = 0`
- **THEN** 响应 403，错误码 `admin.account_disabled`
- **AND** MUST NOT 返回 menus 或 site_config

### Requirement: 响应包含当前管理员信息

`GET /admin/init` 响应 SHALL 含 `admin` 对象，其字段映射规则：
- `id` → `admin.id`
- `username` → `admin.username`
- `nickname` → `admin.nickname`
- `avatar` → `admin.avatar`（可空字符串）
- `last_login_at` → `admin.last_login_at` 序列化为 RFC 3339 字符串（nil 输出空字符串）
- `last_login_ip` → `admin.last_login_ip`（可空字符串）
- `super` → Permission Manager 的 `IsSuperAdmin(uid)` 结果

`admin` 对象 MUST NOT 包含 `password` / `login_failure` 等敏感字段。

#### Scenario: 超管标记

- **WHEN** 当前管理员属于 `admin_group.rules == "*"` 的分组
- **THEN** `admin.super == true`

#### Scenario: 非超管

- **WHEN** 当前管理员无 wildcard 分组
- **THEN** `admin.super == false`

### Requirement: 响应含站点基础配置

`GET /admin/init` 响应 SHALL 含 `site_config` 对象，键固定为 `name` / `record_number` / `version`，值取自 `config` 表对应 `name` 行的 `value`：
- 若 `name` 行不存在或 `value` 为 NULL，值 MUST 为空字符串（不是 `null`）
- 服务端 MUST 仅查询这三个 name，不返回 `config` 表的其他行

#### Scenario: 三项配置齐全

- **WHEN** `config` 表存在 `name='name'` / `record_number` / `version` 三行且 `value` 均非空
- **THEN** `site_config` 对象三个键均含对应 `value`

#### Scenario: 部分配置缺失

- **WHEN** `config` 表只有 `name='name'` 一行
- **THEN** `site_config.name` 含该行 `value`
- **AND** `site_config.record_number` 与 `site_config.version` 为空字符串

### Requirement: 响应含当前管理员的菜单规则

`GET /admin/init` 响应 SHALL 含 `menus` 数组，元素为当前管理员有权访问的启用 `admin_rule` 行。
- 超管（`admin.super == true`）：返回 `admin_rule` 表中所有 `status = 1` 的行；
- 普通管理员：返回 Permission Manager `GetRules(uid)` 结果（已含 `status = 1` 过滤 + weigh ASC, id ASC 排序）。

`menus` MUST NOT 包含 `status = 0` 的行；MUST NOT 包含管理员无权访问的行（仅超管豁免）。

#### Scenario: 超管返回所有启用规则

- **WHEN** 当前管理员为超管
- **THEN** `menus` 包含 `admin_rule` 表全部 `status = 1` 的行
- **AND** `menus` 不含 `status = 0` 的行

#### Scenario: 普通管理员返回权限合并后的规则

- **WHEN** 当前管理员属于多个非超管分组，分组的 `admin_group.rules` 字段为 `1,2,3` 和 `3,4,5`
- **THEN** `menus` 包含 id ∈ {1, 2, 3, 4, 5} 且 `status = 1` 的行
- **AND** 不重复

#### Scenario: 无权限管理员返回空数组

- **WHEN** 当前管理员不属于任何分组或所属分组 `rules` 均为空
- **THEN** `menus` 为空数组 `[]`（不是 `null`）

#### Scenario: 被禁用的规则不返回

- **WHEN** `admin_rule.id = 7` 在管理员权限集合内但 `status = 0`
- **THEN** `menus` 不含 id=7 的行

### Requirement: 前端 `init()` 调用流程

前端 `web/src/layouts/admin/index.vue` 在 `<script setup>` 的 `onMounted` 钩子内 SHALL 顺序执行：
1. 调 `web/src/api/admin/index.ts` 的 `init()`，请求 `GET /admin/init`；
2. 失败 / 401 → 清空 `useAdminInfo` 后重定向回 `/admin/login`；
3. 成功 → 把 `admin` 字段写入 `useAdminInfo`（不覆盖 token）、把 `site_config` 写入 `useConfig` 的 `siteConfig` 子状态、把 `menus` 写入 `useMenu` 的 `rawData`；
4. 遍历 `menus` 调 `router.addRoute(...)` 注册动态路由；
5. `router.replace` 跳转到排序后的第一个菜单（按 `weigh ASC, id ASC`）的 path；
6. 若 `menus` 为空，`router.replace` 到 `/admin/loading`（保持现状）。

整个流程 MUST 在用户感知层面表现为"等待后端响应 → 跳第一个菜单"的一次性过渡，不在中间状态把页面暴露给用户。

#### Scenario: 登录后跳转首个菜单

- **WHEN** 用户登录成功后 `router.push('/admin')`，布局组件 mount
- **THEN** 前端发起 `GET /admin/init`
- **AND** 收到响应后 `useAdminInfo` 被填充、`useMenu.rawData` 被填充
- **AND** `vue-router` 增加至少一条 `menus` 元素对应的路由
- **AND** 浏览器 URL 变为第一个菜单的 path（不再是 `/admin/loading`）

#### Scenario: token 失效

- **WHEN** `GET /admin/init` 返回 401
- **THEN** 前端清空 `useAdminInfo` 并 `router.replace('/admin/login')`

#### Scenario: 无菜单管理员

- **WHEN** `init()` 响应 `menus` 为空数组
- **THEN** `vue-router` 不新增任何路由
- **AND** URL 保留在 `/admin/loading`（兜底显示）

#### Scenario: 重复 mount 不重复注册

- **WHEN** 后台布局组件因路由切换被重复 mount
- **THEN** 已注册的动态路由不再重复 `addRoute`（按 `name` 去重）
- **AND** `useMenu.rawData` 在重复调用时直接覆盖（不追加）

### Requirement: 转换 `admin_rule` 行到 vue-router 路由对象

后端 `Init` 服务 SHALL 把每个 `AdminRule` 转换为兼容 `vue-router` `RouteRecordRaw` 的结构：
- `path` 取自 `rule.path`
- `name` 取自 `rule.name`（空字符串时退化为 `rule-{id}`）
- `component` 取自 `rule.component`（空字符串时退化到通用占位 `web/src/views/404.vue`）
- `meta.title` 取自 `rule.title`
- `meta.icon` 取自 `rule.icon`
- `meta.keepalive` 取自 `rule.keepalive`
- `meta.menuType` 取自 `rule.type`（dir / menu / node）
- `meta.openType` 取自 `rule.open_type`（tab / link / iframe，可空）
- `meta.weigh` 取自 `rule.weight`
- `meta.id` 取自 `rule.id`
- `meta.pid` 取自 `rule.pid`
- 父规则 → 子规则通过 `meta.id` / `meta.pid` 在前端二次组装为 `children` 树；后端 MUST 输出扁平数组。

`rule.type == "node"`（纯权限节点） MUST NOT 注册为路由（前端不展示），但仍 SHALL 出现在 `menus` 数组中以便前端按需展示权限节点标签。

#### Scenario: 菜单项带 component

- **WHEN** `admin_rule` 行 `type="menu"`, `component="/admin/user/list"`, `name="user.list"`, `path="/admin/user/list"`
- **THEN** 转换产物的 `path == "/admin/user/list"`, `name == "user.list"`, `component == "/admin/user/list"`

#### Scenario: 权限节点不进路由

- **WHEN** `admin_rule` 行 `type="node"`, `path="/api/x"`, `name="api.x"`
- **THEN** 转换产物不调用 `router.addRoute`（前端跳过该行）
- **AND** 该行仍出现在 `menus` 数组中（id / name 可用于前端显示权限点标签）

#### Scenario: 空 component 降级

- **WHEN** `admin_rule` 行 `component == ""`
- **THEN** 转换产物的 `component` 指向通用占位组件（前端 404 页）

### Requirement: 错误码与可观测性

`GET /admin/init` 在以下 MUST 场景返回对应状态码：
- 401：token 缺失 / 无效（中间件处理，本端点不重做）
- 403：`admin.status != 1`
- 500：内部错误（数据库 / 转换器异常），响应 envelope `{code: "admin.init.internal", message: "init failed"}`，不回显 err.Error() 详情

#### Scenario: 内部错误不泄露详情

- **WHEN** 数据库查询失败
- **THEN** 响应 500，message 为固定文案
- **AND** 原始错误仅写入日志（不入响应体）
