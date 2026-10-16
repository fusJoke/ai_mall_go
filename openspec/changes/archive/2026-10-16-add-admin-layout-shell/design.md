# Design

## Context

后台缺壳层布局。buildadmin 参考实现的 stores/config 样式配置、`--ba-*` 样式变量、暗色 css-vars 在本项目已预置，缺的只是布局组件本体；init 流程（ layouts/admin/index.vue）已能产出菜单数据（root 账号为空数组）。

第一阶段先落 Default 模式；第二阶段扩展到全部 5 种布局模式（Classic/Streamline/Double/LeftSplit）+ 布局设置抽屉 + 账号资料抽屉 + iframe 菜单 + keep-alive + 标签全屏。

## Goals / Non-Goals

- Goals：5 种布局模式壳层可用、可切换、可配置（抽屉实时联动 + 持久化）；标签页导航含全屏与 keep-alive；iframe 菜单可渲染；menus 为空时示例菜单兜底（含 iframe 示例叶子）；不写任何后端代码。
- Non-Goals：终端、缓存清理、CRUD 记录、漫游引导等 buildadmin 专属功能；iframe 的后端菜单链路；账号资料落库。

## Decisions

### D1. 组件目录落在 `layouts/admin/` 而非 buildadmin 的 `layouts/backend/`

项目对后台一贯命名 `admin`（路由 /admin/*、views/admin/、stores/adminInfo）；`layouts/admin/` 目录已存在。`stores/refs.ts` 里预置的 `/@/layouts/backend/...` import 是迁移前的笔误（该文件至今是无人引用的死桩），第二阶段已修正指向 `layouts/admin` 并删除 `layouts/backend/` 残留。

### D2. navTabs 状态提升为模块级单例

现实现每次 `useNavTabs()` 都 `reactive({...})` 新建 state——aside/header/tabs/main 五个组件共用时必然各持一份。改为模块级 `reactive` + 模块级方法，保留 `useNavTabs()` 调用形态。字段对齐 buildadmin 命名（tabsView/tabsViewRoutes/activeRoute/activeIndex/tabFullScreen），便于后续对照移植。第二阶段补充 `keepAliveTabs` 与 `childrenMenus`。

### D3. 示例菜单在 init 兜底层注入，而非组件层 mock

`menus` 为空时（root 无分组即空）在 init 流程第 3 步回落到 `web/src/mock/menus.ts` 的示例菜单树——数据形状与后端 `routeFromRule` 输出一致（path/name/component/meta.menuType），组件层无感知。示例叶子全部指向真实存在的视图（dashboard/manager/rule/group/log/agInput），目录节点仅用于演示 el-sub-menu 层级；iframe 示例叶子（menuType='iframe'，path 为外链 URL）验证 iframe 渲染链路。

### D4. 标签页右键菜单内联实现

buildadmin 用通用 contextmenu 组件 + 事件总线；本次在 tabs.vue 内联一个固定定位菜单（刷新/关闭/关闭其他/关闭全部/当前标签全屏），刷新通过 main.vue 暴露的刷新 key 事件达成（provide/inject），避免移植事件总线与通用组件。

### D5. 图标统一走 lucide 前缀

项目 `Icon` 组件支持 `lucide:` 查表与本地 svg 雪碧图两类；buildadmin 的 `fa fa-*` / `el-icon-*` / `local-*` 名称体系不通用。示例菜单 meta.icon 与布局组件图标全部使用 `lucide:` 名称，按项目注释约定在 `components/icon/index.vue` 的查表里登记新增图标。

### D6. 容器为哑组件，路由监听与容器选择上提

5 个 container/*.vue 只做结构排布（el-aside/el-header/el-main 组合 + tabFullScreen 隐藏 aside/header）；标签页的路由 watch（addTab/setActiveRoute）统一放 `layouts/admin/index.vue`，容器切换（布局模式变更）不丢标签状态；容器本身由 `index.vue` 按 `config.layout.layoutMode` 从 containerMap 动态选择。`header.vue` 同理按模式选 navBar 变体（Streamline 模式的 navBar 即 `menus/menuHorizontal.vue`，与 buildadmin 一致）。

### D7. 菜单 path 绝对化 + 菜单树查找收口在 navTabs

后端菜单叶子 path 是相对形式（`rule`、`manager`），而 vue-router 注册后为 `/admin/rule`。在 navTabs 生成 tabsViewRoutes 时统一补前缀（对齐 buildadmin handleAdminRoute），侧边/横向菜单的激活匹配、叶子点击 routePush 均依赖绝对路径。Double/LeftSplit 的次级菜单派生统一走 `getTabsViewDataByRoute(route, 'above')`（返回一级菜单节点），`'children'` 模式返回最深匹配节点供激活态使用。

### D8. keep-alive 以「路由 name = 视图组件名」为契约

`keepAliveTabs` 由 tabsView 现存标签的 name 派生（增删/closeOther/closeAll 时同步重建），main.vue 的 `<keep-alive :include>` 消费；视图需 `defineOptions({ name })` 与路由 name 一致（dashboard 已对齐，动态注册的菜单路由 name 即路由 name，但对应视图尚未统一补名——include 匹配不到时退化为不缓存，无害）。

### D9. 布局设置抽屉直写 config store

抽屉不设本地中间态，全部 `config.setLayout()` 直写（pinia persist 自动持久化），布局组件经响应式取色即时联动；颜色项是 `[亮色, 暗色]` 双色对，编辑时只写当前主题对应侧（`config.getColorVal` 取实际生效值）。暗黑开关与 navMenus 的开关共用 `config.layout.isDark` 数据源（html.dark class 切换）。恢复默认 = 重放一组与 store 初始值一致的默认值。

### D10. 账号资料抽屉走示例数据约定

`baAccount.vue` 展示 adminInfo 概览 + 昵称/头像（agUpload）编辑，提交仅 `adminInfo.dataFill` 写本地 store（不调后端）；头像上传复用既有 `POST /admin/ajax/upload`。

### D11. iframe 菜单的 path 编码约定

iframe 菜单叶子注册时改挂 `router-view/iframe.vue`（懒加载），path 编码为 `/admin/iframe/{encodeURIComponent(url)}`（menuType=='iframe' 时菜单数据层处理），视图从路由 path 解码出原始 url 嵌入。外链菜单在示例数据中不注册路由、仅 menuType 区分。

### D13. lang yaml 以 raw 方式加载的构建修复

`import.meta.glob('**/*.yaml', { as: 'raw' })` 在 rolldown-vite(v8) 下失效（构建时 yaml 被当 JS 解析直接报语法错），改为 `{ query: '?raw', import: 'default' }`；dev 与 build 均验证通过。该修复是本 change 构建验证的前置条件（否则 `pnpm build` 必失败，且波及所有既有 yaml）。

## Risks / Trade-offs

- tabsViewRoutes 直接来自菜单树（含目录节点）：buildadmin 同构；目录在 menuTree 里渲染为 el-sub-menu，不参与标签页（仅 menu 叶子入 tabs）。
- 动态注册的菜单路由对应视图未统一 defineOptions name → keep-alive 对这些页不生效（匹配不到 include 名单），退化为不缓存，无功能损坏；后续在各视图补名即可启用。
- 示例菜单注册的路由与静态子路由路径重合（同名不同 name）：静态先注册优先生效，动态注册冗余无害（与 login 落地 change 的结论一致）。
- 布局设置的颜色项为双色对数组直写：绕过了 buildadmin 的 baInput 颜色组件封装，抽屉内用 el-color-picker 原生交互，功能等价、代码更少。

## Migration Plan

纯前端：新增文件 + 少量修改；vite HMR 即时生效；无 schema / 后端面。第二阶段顺带修复 `lang/index.ts` 的 yaml raw 加载（影响面：全部语言包，dev/build 双验证通过）。

## Open Questions

无。
