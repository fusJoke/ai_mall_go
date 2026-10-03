# Tasks

## 1. 数据与工具层

- [x] 1.1 新增 `web/src/mock/menus.ts`：示例菜单树（routeFromRule 形状，目录+叶子，叶子指向 dashboard/manager/rule/agInput 真实视图，meta.icon 用 `lucide:` 前缀）。
- [x] 1.2 重构 `web/src/stores/navTabs.ts`：state 提升模块级单例；新增 tabsViewRoutes/tabsView/activeRoute/activeIndex/tabFullScreen 与 addTab/setActiveRoute/closeTab/closeOtherTabs/closeAllTabs。
- [x] 1.3 新增 `web/src/utils/router.ts`：routePush（失败通知）/getFirstRoute/getMenuKey。
- [x] 1.4 `web/src/components/icon/index.vue`：lucide 查表登记布局所需图标。
- [x] 1.5 新增 `web/src/lang/zh-cn/layouts.yaml` 与 en 版。

## 2. 布局组件

- [x] 2.1 `layouts/admin/container/default.vue`：el-container = Aside + 右侧（Header + Main）。
- [x] 2.2 `components/aside.vue` + `components/logo.vue`：el-aside（宽度/折叠/暗色取色）、站点名（siteConfig.name 兜底 'AI GO MALL'）。
- [x] 2.3 `components/menus/menuVertical.vue` + `menuTree.vue`：el-menu 递归渲染目录/叶子，路由变化高亮激活项。
- [x] 2.4 `components/asideToolbar/footer.vue` + `menuSearch/dialog.vue`：折叠切换（含导航条宽度联动）+ 菜单过滤搜索跳转。
- [x] 2.5 `components/header.vue` + `navBar/tabs.vue`：标签页（路由驱动、激活滑块、右键菜单 刷新/关闭/关闭其他/关闭全部）。
- [x] 2.6 `components/navMenus.vue`：站点主页外链、语言切换、暗色开关（useDark）、全屏、用户信息弹层（头像/昵称/上次登录 + 退出登录）。
- [x] 2.7 `router-view/main.vue`：el-main + scrollbar + slide 过渡 + provide 刷新 key。
- [x] 2.8 重构 `layouts/admin/index.vue`：渲染布局容器；menus 为空回落示例菜单；registerMenuRoutes 递归注册目录树叶子。

## 3. 多布局模式扩展

- [x] 3.1 `stores/navTabs.ts` 扩展：菜单相对 path 绝对化（补 `/admin` 前缀）；`getTabsViewDataByRoute(route, mode)` 菜单树查找；`keepAliveTabs` 名单（addTab/closeTab/closeOtherTabs/closeAllTabs 同步）。
- [x] 3.2 新增容器 `container/{classic,streamline,double,leftSplit}.vue`；容器统一为哑组件（tabFullScreen 时隐藏 aside/header）。
- [x] 3.3 新增顶栏变体 `components/navBar/{default,classic,leftSplit,double}.vue`；`header.vue` 按 layoutMode 选 navBar 组件（Streamline → `menus/menuHorizontal.vue`）。
- [x] 3.4 新增菜单变体：`menus/menuHorizontal.vue`（横向菜单 + 激活滚动定位）、`menus/menuVerticalChildren.vue`（Double 子菜单派生）、`menus/menuLeftSplit.vue` + `menuLeftSplitTree.vue`（主窄栏 + 次级树）；`menuTree.vue` 横向模式（一级目录点击跳第一个子菜单）。
- [x] 3.5 `aside.vue` 多模式适配（Double→子菜单、LeftSplit→主次双栏、其余→竖向菜单）；`index.vue` 按 layoutMode 动态选容器 + 标签页路由监听上提。
- [x] 3.6 新增 `components/config.vue` 布局设置抽屉：模式切换（缩略图）、暗黑、切换动画、菜单栏/顶栏全套色位（双色对、写当前主题侧）、宽度/折叠/手风琴/顶部栏开关、LeftSplit 配色、恢复默认；`navMenus.vue` 增加齿轮入口。
- [x] 3.7 新增 `components/baAccount.vue` 账号资料抽屉（昵称/头像 agUpload，示例数据约定仅写本地 store）；`navMenus.vue` 用户弹层增加「账号资料」入口。
- [x] 3.8 新增 `components/closeFullScreen.vue`；`navBar/tabs.vue` 右键菜单增加「当前标签全屏」。
- [x] 3.9 新增 `router-view/iframe.vue`（path 解码嵌入）；`index.vue` 注册 iframe 菜单叶子（meta.menuType=='iframe' 挂 iframe 视图）。
- [x] 3.10 keep-alive 接线：`main.vue` include 消费 `keepAliveTabs`；`views/admin/dashboard.vue` 补 `defineOptions({ name: 'adminDashboard' })`。
- [x] 3.11 `styles/app.scss` 补 5 种页面切换动画（slide-right/left/top/bottom/fade）。
- [x] 3.12 `stores/refs.ts` 修正指向 `layouts/admin`；删除 `layouts/backend/` 残留；`mock/menus.ts` 扩充权限管理（manager/rule/group/log）与常规管理（agInput + iframe 示例）目录。
- [x] 3.13 i18n 补充：layouts.yaml（zh-cn/en）补 LeftSplit 配色、账号资料等 key。

## 4. 验证与收尾

- [x] 4.1 `pnpm typecheck` 通过（仅既存 random.ts 遗留报错）。
- [x] 4.2 ESLint：本次新增/修改文件零错误零告警（prettier 格式自动修复）。
- [x] 4.3 修复 `lang/index.ts` yaml raw 加载（rolldown-vite 下 `as: 'raw'` 失效 → `query: '?raw', import: 'default'`），`pnpm build` 成功。
- [x] 4.4 GUI 黑盒：五种布局模式切换生效；各模式菜单导航 + 激活高亮；标签页随路由增删与右键操作（含全屏/还原）；布局设置抽屉配色与开关实时联动且刷新后保留；iframe 示例菜单嵌入渲染；keep-alive 标签间切换保留视图状态；示例菜单在 root（menus 为空）下可见。
- [x] 4.5 `openspec validate add-admin-layout-shell` 通过；勾选本文件所有任务。
