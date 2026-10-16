# Proposal

## Why

2026-10-01 的登录 GUI 验证暴露：登录成功后 `router.push('/admin')` → 重定向 `/admin/loading`，此后**永远卡在静态占位页**。根因有两层：

1. 布局 `layouts/admin/index.vue` 的 init 流程第 5 步 `router.replace(第一个菜单 path)`，而后端 `routeFromRule` 返回的 `path` 是**相对路径**（种子数据 `manager` / `rule`），vue-router 解析不到任何路由，导航静默失败；
2. 即使路径能匹配，动态菜单注册 `router.addRoute(route)` 也是顶层注册，`component` 字段是后端给的字符串（`/src/views/admin/manager/index.vue`），从未被解析成真实组件——动态菜单链路本身不可导航。

同时 `/admin/loading` 占位页只有一个静态文案「加载中...」，无任何加载动画；且 `views/admin/dashboard.vue`（欢迎语 + KPI 卡片的现成主页视图）没有挂任何路由。

本 change：让 loading 具备吃豆人（Pac-Man）加载动画（纯 HTML/CSS，不引入 GIF/图片资源），并把登录后的落地页定为后台主页 `/admin/dashboard`（深链 `to` 参数优先）。

## What Changes

- **重写 `web/src/layouts/common/loading.vue`**：纯 CSS 吃豆人动画（上下颚开合 + 豆子被吃进嘴里的循环动画）+ 「加载中...」文案；`prefers-reduced-motion` 下停用动画只保留文案。不引入任何图片 / GIF 资源。
- **新增静态子路由 `/admin/dashboard`**（`web/src/router/static/adminBase.ts`，name `adminDashboard`，component `views/admin/dashboard.vue`），置于兜底路由之前；补 `pageTitles.Dashboard` / `pageTitles.Loading` i18n key（zh-cn + en）。
- **修改 `web/src/layouts/admin/index.vue` 的 init 流程第 5 步**：
  - 跳转目标改为「loading 路由的 `to` 深链参数（存在且解析成功）→ 否则后台主页 `/admin/dashboard`」；
  - `menus` 为空不再停留 loading（dashboard 是静态路由，与菜单无关），移除「空菜单留在 loading」兜底与 `pickFirstMenu`。
- **修复动态菜单注册（同文件，跳转链路的前置依赖）**：
  - `component` 字符串经 `import.meta.glob('/src/views/**/*.vue')` 解析为懒加载组件（精确 key → `+ '.vue'` → `+ '/index.vue'` → 兜底 `404`）；
  - 相对 `path`（不以 `/` 开头）注册为 `admin` 布局的**子路由**（`router.addRoute('admin', route)`，vue-router 语义拼接为 `/admin/manager`）；绝对路径保持顶层注册；同名路由按 `name` 去重逻辑不变。
- **不改**：后端任何代码（`routeFromRule` 的相对路径契约由前端适配）；`useMenu` store 结构；登录页 / login API；既有静态子路由 `manager` / `rule`。

### 非目标

- **不做后台侧边栏 / 顶栏 UI**：布局当前只有 `<router-view/>`，菜单展示与导航高亮留待后续 admin header/sidebar change。
- **不做动态菜单的 iframe / 外链 extend 类型处理**：种子数据无此类行，`extend`/`url` 字段解析留待真实需求。
- **不做 GIF 版吃豆人**：用户允许 html 或 gif 二选一；CSS 动画无资源依赖、可缩放、可控色，选 CSS。
- **不处理 token 失效以外的 init 错误分支**：401/403/5xx → 回登录页、其余静默留 loading 的现状保持。

## Capabilities

### New Capabilities

无。

### Modified Capabilities

- `admin-init`: 「前端 init() 调用流程」的跳转目标从「排序后的第一个菜单」改为「`to` 深链参数优先，否则后台主页 `/admin/dashboard`」；「menus 为空保留 loading」兜底删除；动态菜单注册语义收紧（相对路径 → 布局子路由、component 字符串 → 解析为懒加载组件）。新增「loading 页吃豆人动画」需求。

## Impact

- **代码**：`web/src/layouts/common/loading.vue`（重写）、`web/src/router/static/adminBase.ts`（+1 子路由）、`web/src/layouts/admin/index.vue`（跳转目标 + 组件解析器 + 注册语义）、`web/src/lang/{zh-cn,en}/pageTitles.yaml`（各 +2 key）。
- **规格**：`openspec/specs/admin-init/spec.md` 由本 change delta 修改。
- **数据**：无 schema / 后端变更。
- **验证**：GUI 黑盒——登录 → loading 显示吃豆人动画 → 落地 `/admin/dashboard`（URL 断言 + 截图）；深链 `/admin/rule` 直接可达；`go build ./...` 无涉（纯前端）。
