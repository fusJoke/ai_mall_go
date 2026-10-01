# Design

## Context

登录后卡死 `/admin/loading` 的两层根因：布局 init 第 5 步 `router.replace(相对路径 'manager')` 解析不到路由；动态菜单 `component` 是字符串从未解析。`views/admin/dashboard.vue` 主页视图已存在但无路由。loading 占位页无动画。

## Goals / Non-Goals

- Goals：登录 → loading（吃豆人动画）→ 落地 `/admin/dashboard`；`to` 深链参数优先生效；动态菜单注册真正可导航。
- Non-Goals：侧边栏 / 顶栏 UI、iframe 菜单类型、后端改动、GIF 资源。

## Decisions

### D1. 主页定为静态子路由 `/admin/dashboard`，而非「第一个菜单」

候选：A. 修好路径拼接后落地第一个菜单（`/admin/manager`）；B. 新增 `/admin/dashboard` 静态子路由作为主页（采纳）。

- 用户语义「后台主页」指向 dashboard——`views/admin/dashboard.vue` 是现成的欢迎语 + KPI 视图，只是从未挂路由；
- 「第一个菜单」随 `weigh` 数据变化，作为登录落地页不稳定；dashboard 与菜单数据解耦，`menus` 为空也能落地（顺带删掉「空菜单留 loading」兜底）；
- dashboard 不进 `admin_rule` 菜单表：主页是产品结构不是权限点，后续侧边栏可将其固定为首项。

### D2. init + 跳转逻辑保持在布局组件，loading.vue 只管动画

spec「前端 init() 调用流程」的责任边界本就在 `layouts/admin/index.vue` 的 `onMounted`；loading.vue 是纯展示组件。跳转目标从布局内读当前路由的 `to` 参数（`loading/:to?` 的 JSON 深链），不让 loading.vue 参与流程编排。

### D3. 相对路径菜单注册为 `admin` 布局的子路由

后端契约（`routeFromRule` 返回相对 path，如 `manager`）不改，前端适配：`path` 不以 `/` 开头 → `router.addRoute('admin', route)`，vue-router 子路由语义拼接为 `/admin/manager`；以 `/` 开头 → 顶层 `addRoute(route)`。这比「前端拼前缀后顶层注册」更贴 vue-router 模型，且嵌套在 admin 布局内渲染（顶层注册会脱离布局）。

### D4. component 字符串用 `import.meta.glob` 解析，失败兜底 404

后端 `component` 形如 `/src/views/admin/manager/index.vue`（buildadmin 惯例）。解析顺序：精确 glob key → `/src/views/${component}.vue` → `/src/views/${component}/index.vue` → `/src/views/404.vue`。`import.meta.glob('/src/views/**/*.vue')` 让 Vite 静态分析全量懒加载，无运行时拼接 import 风险。

### D5. 吃豆人用纯 CSS，不用 GIF

上下两片半圆以 `transform-origin` 在嘴角反向旋转实现「张嘴闭合」，三颗豆子向嘴部平移并在 mouth 处淡出，`infinite` 循环。相对 GIF：无二进制资源与版权顾虑、随主题色可调、无加载闪烁；`prefers-reduced-motion: reduce` 下停用动画仅保留文案（可访问性）。

## Risks / Trade-offs

- 动态路由与静态子路由（manager/rule）路径重合：同名不同 name（`admin/manager` vs `adminManager`），vue-router 按注册顺序匹配，静态先注册故动态重复路由不可达——无害冗余，且 adminBase 注释已声明「两条独立链路」是刻意设计。
- `to` 深链解析失败（非法 JSON / 非 `/` 开头）→ 静默回落 dashboard，不抛错。
- 移除「menus 为空留 loading」是行为变更：无权限管理员现在也能落地 dashboard（页面本身无数据请求），后续真实数据接口接入时再按权限收紧。

## Migration Plan

纯前端 4 文件 + spec delta；Vite HMR 下改完即生效，无迁移面。

## Open Questions

无。
