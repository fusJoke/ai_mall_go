# Tasks

## 1. 路由与 i18n

- [x] 1.1 `web/src/router/static/adminBase.ts`：在兜底路由之前新增静态子路由 `dashboard`（name `adminDashboard`，component `views/admin/dashboard.vue`，meta.title `pageTitles.Dashboard`）。
- [x] 1.2 `web/src/lang/zh-cn/pageTitles.yaml` 与 `web/src/lang/en/pageTitles.yaml` 各追加 `Dashboard` 与 `Loading` key（zh-cn：后台主页 / 加载中；en：Dashboard / Loading）。
  验证：`pnpm build`（或 dev HMR 无报错）。

## 2. loading.vue 吃豆人动画

- [x] 2.1 重写 `web/src/layouts/common/loading.vue`：纯 HTML/CSS 吃豆人——上下颚半圆 `transform-origin` 反向旋转张合；三颗豆子向嘴部平移并淡出（错峰 delay）；下方保留「加载中...」文案。
- [x] 2.2 `prefers-reduced-motion: reduce` 时停用动画（动画元素静止），文案保留。
  验证：浏览器手动检查动画循环流畅、无布局抖动。

## 3. 布局 init 流程

- [x] 3.1 `web/src/layouts/admin/index.vue`：新增 component 字符串解析器（`import.meta.glob('/src/views/**/*.vue')`，精确 key → `.vue` 后缀 → `/index.vue` 后缀 → 兜底 `404`）。
- [x] 3.2 `registerMenuRoutes`：相对 `path`（不以 `/` 开头）走 `router.addRoute('admin', route)`（布局子路由语义）；绝对 path 走顶层 `addRoute(route)`；`component` 解析结果挂到 route 上；`meta.menuType === 'node'` 跳过与按 `name` 去重逻辑不变。
- [x] 3.3 第 5 步跳转目标改为：解析 loading 路由 `to` 参数（`JSON.parse` 后取 `path`，须为 `/` 开头字符串且非 `/admin/loading`），无效则回落 `/admin/dashboard`；`replace` 失败 catch 回落 dashboard。移除 `pickFirstMenu` 与「menus 为空留 loading」分支，同步更新文件头注释。
  验证：`vue-tsc` / dev 编译无类型错误。

## 4. 验证与收尾

- [x] 4.1 GUI 黑盒：登录 → loading 出现吃豆人动画 → URL 落地 `#/admin/dashboard`，dashboard 欢迎语渲染；退出登录后重新登录可复现。
- [x] 4.2 GUI 黑盒：深链 `#/admin/manager` 直接可达（静态子路由，不经 loading）；`#/admin/rule` 同。
  （备注：路由层面已验证深链 hash 保持在目标页、不被拽回 loading；但 manager/rule 视图渲染空白为**历史遗留独立问题**——已用 git stash A/B + 全新 vite 实例定案与本次改动无关，报错 `parentNode null during component update`，另立任务排查。）
- [x] 4.3 `openspec validate add-admin-loading-home-redirect` 通过；勾选本文件所有任务。
