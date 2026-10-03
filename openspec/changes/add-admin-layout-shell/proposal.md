# Proposal

## Why

后台目前没有真正的管理布局：`layouts/admin/index.vue` 只有一个 `<div><router-view/></div>`，没有侧边菜单、顶栏、标签页导航。登录落地 dashboard 后用户无法在页面间导航（只能手改 URL）。本项目 stores（config/navTabs/menu/adminInfo）与样式变量（`--ba-*`、element 暗色）此前已按 buildadmin 形态预置，但对应的布局组件从未迁移。

本 change 参考 buildadmin（D:\buildadmin-v2\web\src\layouts，参考站 demo.buildadmin.com）迁移后台壳层。范围经扩展后覆盖 buildadmin 的 **全部 5 种布局模式**（Default/Classic/Streamline/Double/LeftSplit）：左侧菜单栏（Logo + 竖向菜单 + 底部工具栏 + 菜单搜索）、右侧顶栏（标签页导航 + 右侧操作区）+ 主内容区（过渡动画 + 刷新 + keep-alive），以及布局设置抽屉（5 模式切换 + 全套配色）、账号资料抽屉、iframe 菜单视图、标签全屏。服务端数据全部使用示例数据兜底（菜单为空时使用示例菜单树；站点名兜底），不新增任何后端接口。

## What Changes

### 第一阶段（Default 壳层）

- **新增 `web/src/layouts/admin/container/default.vue`**：el-container 结构 = Aside + 右侧（Header + Main），对齐 buildadmin `container/default.vue`。
- **新增布局组件**（`web/src/layouts/admin/components/`）：
  - `aside.vue` / `logo.vue` / `menus/menuVertical.vue` + `menus/menuTree.vue`（el-menu 递归渲染目录/菜单，点击经 `utils/router.ts` 的 routePush 导航）/ `asideToolbar/footer.vue`（折叠切换 + 菜单搜索入口）/ `asideToolbar/menuSearch/dialog.vue`（按标题/路径过滤菜单，回车跳转）。
  - `header.vue`（nav-bar = 标签页 + 右侧操作区）/ `navBar/tabs.vue`（标签页：路由驱动增删、激活态滑块、右键上下文菜单：刷新/关闭/关闭其他/关闭全部）/ `navMenus.vue`（站点主页外链、语言切换、暗色开关、全屏、用户信息弹层含退出登录）。
  - `router-view/main.vue`（el-main + scrollbar + slide 过渡 + 刷新 key）。
- **重构 `web/src/layouts/admin/index.vue`**：模板改为渲染布局容器；init 流程与跳转/防死胡同逻辑保持不变；`menus` 为空时回落示例菜单（`web/src/mock/menus.ts`）；`registerMenuRoutes` 支持目录树递归注册（dir 节点不注册路由，递归注册其 menu 叶子）。
- **扩展 `web/src/stores/navTabs.ts`**：state 提升为模块级单例（原实现每次调用 `useNavTabs()` 都会新建 state，多组件共用必然踩坑）；新增 `tabsViewRoutes`（菜单路由树，来自 useMenu.rawData）、`tabsView`、`activeRoute`、`activeIndex`、`tabFullScreen` 与 addTab/setActiveRoute/closeTab 等标签页操作。
- **新增 `web/src/utils/router.ts`**：`routePush`（导航失败提示）/ `getFirstRoute` / `getMenuKey`（buildadmin utils/router.ts 的裁剪适配版，去掉 memberCenter/siteConfig 依赖）。
- **扩展 `web/src/components/icon/index.vue`**：lucide 图标表补充布局所需图标。
- **新增 i18n**：`web/src/lang/{zh-cn,en}/layouts.yaml`（布局组件文案）。

### 第二阶段（多布局模式扩展）

- **新增 4 个容器**：`container/classic.vue`（通栏）、`container/streamline.vue`（单栏，无侧栏）、`container/double.vue`（顶侧双栏）、`container/leftSplit.vue`（左分双栏）；`layouts/admin/index.vue` 按 `config.layout.layoutMode` 动态切换容器，`header.vue`/`aside.vue` 按模式渲染对应 navBar/菜单变体。
- **新增菜单变体**：`menus/menuHorizontal.vue`（Streamline 顶栏横向菜单）、`menus/menuVerticalChildren.vue`（Double 布局侧边子菜单，由当前路由派生）、`menus/menuLeftSplit.vue` + `menuLeftSplitTree.vue`（LeftSplit 主窄栏 + 次级菜单树）；`menuTree.vue` 扩展横向模式（一级目录点击跳第一个子菜单，`extends.position/level` 区分层级）。
- **新增顶栏变体**：`navBar/default.vue`（悬浮标签）、`navBar/classic.vue`（通栏标签）、`navBar/leftSplit.vue`（悬浮标签）、`navBar/double.vue`（横向菜单）。
- **新增 `components/config.vue` 布局设置抽屉**：5 种布局模式缩略图切换、暗黑模式、页面切换动画（slide/fade 五种）、侧边菜单栏与顶栏全套色位（亮/暗双色对）编辑、菜单宽度/折叠/手风琴/顶部栏开关、LeftSplit 专属配色、恢复默认；入口在 `navMenus.vue` 齿轮图标。
- **新增 `components/baAccount.vue` 账号资料抽屉**：昵称/头像（agUpload）编辑，示例数据约定仅写本地 `useAdminInfo` store。
- **新增 `components/closeFullScreen.vue`**：标签全屏态的还原入口；`navBar/tabs.vue` 右键菜单新增「当前标签全屏」。
- **新增 `router-view/iframe.vue`**：iframe 菜单视图（path 编码为 `/admin/iframe/{encoded}`，视图内解码嵌入）；`index.vue` 注册菜单时 `meta.menuType == 'iframe'` 的叶子改挂该组件。
- **keep-alive 接线**：`navTabs` 新增 `keepAliveTabs`（现存标签的路由 name 列表，增删标签时同步），`main.vue` 的 keep-alive include 消费；dashboard 等视图补 `defineOptions({ name })` 与路由 name 对齐。
- **`navTabs` 扩展**：菜单树相对 path 统一补全为 `/admin/xxx` 绝对路径（对齐 buildadmin handleAdminRoute），修复侧边菜单激活态匹配；新增 `getTabsViewDataByRoute(route, mode)` 菜单树查找（'children' 最深匹配 / 'above' 一级菜单，供 Double/LeftSplit 派生次级菜单）；标签页的路由监听统一上提到 `index.vue`（容器为哑组件）。
- **示例菜单扩充**：`mock/menus.ts` 新增「权限管理」目录（manager/rule/group/log）与「常规管理」目录（agInput + iframe 示例）。
- **样式与杂项**：`styles/app.scss` 补 5 种页面切换动画；`stores/refs.ts` 修正指向 `layouts/admin`；修复 `lang/index.ts` 的 `import.meta.glob` 在 rolldown-vite(v8) 下 `as: 'raw'` 失效导致 yaml 被当 JS 解析的构建错误（改 `query: '?raw', import: 'default'`）；清理 `layouts/backend/` 残留死代码。

### 不改

后端任何代码；路由守卫；登录流程；manager/rule 视图本身。

### 非目标

- **不迁移 buildadmin 专属功能入口**：终端、缓存清理、CRUD 记录、布局漫游引导（tour）、会员中心。
- **上下文菜单为内联简化实现**：不迁移 buildadmin 的通用 contextmenu 组件。
- **iframe 示例仅为前端渲染验证**：真实菜单 open_type='iframe' 的后端链路不在本 change。
- **账号资料不落服务端**：示例数据约定，编辑仅写本地 store；布局设置持久化走 pinia persist（config store 既有机制）。

## Capabilities

### New Capabilities

- `admin-layout`: 后台布局壳层（5 种布局模式）—— 侧边菜单栏（Logo/菜单/折叠/搜索；Double 子菜单/LeftSplit 主次双栏）、顶栏（标签页导航/右键菜单/横向菜单/暗色切换/全屏/语言/布局配置/用户信息与退出登录/账号资料）、主内容区（过渡/刷新/keep-alive/标签全屏）、布局设置抽屉、iframe 菜单视图；菜单数据源为 init 响应、空时使用示例菜单。

### Modified Capabilities

无。admin-init 的「前端 init 流程与登录落地」行为不变（init 仍由布局根组件触发）；本 change 只改 init 完成后的壳层渲染与 menus 为空时的示例数据兜底。

## Impact

- **代码**：新增约 25 个布局组件/容器/视图文件 + `utils/router.ts` + `mock/menus.ts` + 2 个 i18n 文件；修改 `layouts/admin/index.vue`、`stores/navTabs.ts`、`stores/refs.ts`、`components/icon/index.vue`、`styles/app.scss`、`lang/index.ts`。
- **规格**：新增 `admin-layout` capability spec。
- **验证**：`pnpm typecheck`（仅既存 random.ts 遗留报错）；ESLint 新增文件零告警；`pnpm build` 成功；GUI 黑盒——五种布局模式切换生效，各模式菜单/标签页/操作区可用，布局设置抽屉配色与开关实时联动且持久化，标签全屏与还原可用，iframe 示例菜单可嵌入渲染，keep-alive 在标签间切换时保留视图状态。
