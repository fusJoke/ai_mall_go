# admin-init Specification (Delta)

## REMOVED Requirements

### Requirement: 前端 `init()` 调用流程

**Reason**：登录后的跳转目标从「排序后的第一个菜单」改为「后台主页 `/admin/dashboard`（深链 `to` 参数优先）」，且动态菜单注册语义（相对路径 → 布局子路由、component 字符串 → 解析为懒加载组件）一并收紧，原需求块整体被下方 ADDED 版本替换。

## ADDED Requirements

### Requirement: 前端 init 流程与登录落地

前端 `web/src/layouts/admin/index.vue` 在 `<script setup>` 的 `onMounted` 钩子内 SHALL 顺序执行：
1. 调 `web/src/api/admin/index.ts` 的 `init()`，请求 `GET /admin/init`；
2. 失败 / 401 → 清空 `useAdminInfo` 后重定向回 `/admin/login`；
3. 成功 → 把 `admin` 字段写入 `useAdminInfo`（不覆盖 token）、把 `site_config` 写入 `useConfig` 的 `siteConfig` 子状态、把 `menus` 写入 `useMenu` 的 `rawData`；
4. 遍历 `menus` 注册动态路由：`meta.menuType === 'node'` 跳过；`path` 不以 `/` 开头的菜单 SHALL 注册为 `admin` 布局路由的子路由（`router.addRoute('admin', route)`），以 `/` 开头的保持顶层注册；`component` 字符串 SHALL 经 `import.meta.glob('/src/views/**/*.vue')` 解析为懒加载组件（精确 key → `'/src/views/' + component + '.vue'` → `'/src/views/' + component + '/index.vue'`），解析失败兜底 `/src/views/404.vue`；
5. `router.replace` 跳转目标：loading 路由的 `to` 深链参数（JSON 解析出以 `/` 开头且非 `/admin/loading` 的 path）优先，否则后台主页 `/admin/dashboard`；目标导航失败时回落 `/admin/dashboard`；
6. 主页 `/admin/dashboard` 为静态子路由，MUST NOT 依赖 `menus` 非空。

整个流程 MUST 在用户感知层面表现为"吃豆人动画过渡 → 落地后台主页/深链目标"的一次性过渡，不在中间状态把页面暴露给用户。MUST NOT 出现停留在 `/admin/loading` 的死胡同（init 失败分支跳登录页除外）。

#### Scenario: 登录后落地后台主页

- **WHEN** 用户登录成功后 `router.push('/admin')`，布局组件 mount
- **THEN** loading 页展示吃豆人动画过渡
- **AND** 收到 init 响应后 `useAdminInfo` 被填充、`useMenu.rawData` 被填充
- **AND** 浏览器 URL 变为 `/admin/dashboard`（不再是 `/admin/loading`）

#### Scenario: 深链 to 参数优先

- **WHEN** 用户直接访问一个仅存在于动态菜单的 `/admin/*` 路径，被兜底路由重定向到 `/admin/loading` 且携带 `to` 参数
- **THEN** init 完成后 `router.replace` 到 `to` 参数解析出的 path（该路由已注册）
- **AND** `to` 缺失 / 解析失败时回落 `/admin/dashboard`

#### Scenario: token 失效

- **WHEN** `GET /admin/init` 返回 401
- **THEN** 前端清空 `useAdminInfo` 并 `router.replace('/admin/login')`

#### Scenario: 无菜单管理员

- **WHEN** `init()` 响应 `menus` 为空数组
- **THEN** `vue-router` 不新增任何动态路由
- **AND** URL 仍落地 `/admin/dashboard`（主页为静态路由，不依赖菜单）

#### Scenario: 重复 mount 不重复注册

- **WHEN** 后台布局组件因路由切换被重复 mount
- **THEN** 已注册的动态路由不再重复 `addRoute`（按 `name` 去重）
- **AND** `useMenu.rawData` 在重复调用时直接覆盖（不追加）

## ADDED Requirements

### Requirement: loading 页吃豆人动画

`web/src/layouts/common/loading.vue` SHALL 以纯 HTML/CSS 实现吃豆人（Pac-Man）加载动画：吃豆人上下颚循环张合，豆子持续被"吃入"嘴中；MUST NOT 引入 GIF / 图片二进制资源。动画下方 SHALL 保留「加载中...」文案。

`prefers-reduced-motion: reduce` 时 SHALL 停用动画（静止展示），文案保留。

#### Scenario: 动画渲染

- **WHEN** 用户进入 `/admin/loading`
- **THEN** 页面居中展示吃豆人动画与「加载中...」文案
- **AND** 页面无 GIF / 图片资源请求（纯 CSS 绘制）

#### Scenario: 减弱动态效果

- **WHEN** 系统开启 `prefers-reduced-motion: reduce`
- **THEN** 动画静止，文案正常显示
