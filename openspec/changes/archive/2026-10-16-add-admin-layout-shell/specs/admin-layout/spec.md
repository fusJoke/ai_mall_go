# admin-layout Specification

## Purpose
后台管理壳层布局（5 种布局模式：Default/Classic/Streamline/Double/LeftSplit）：登录落地后呈现「侧边菜单栏 + 顶栏（标签页导航 + 操作区）+ 主内容区」结构（Streamline 为单栏、Double 顶侧双栏、LeftSplit 主次双栏），承担页面间导航、标签页管理、布局个性化配置与账号操作入口。菜单数据来自 `GET /admin/init`，为空时使用前端示例菜单兜底；本 capability 纯前端，不含任何新增后端接口。

## ADDED Requirements

### Requirement: Default 布局容器结构

后台布局 SHALL 呈现：左侧 `el-aside` 菜单栏（含站点 Logo 区、竖向菜单、底部工具栏），右侧纵向排列顶栏与主内容区；主内容区承载 `router-view`。菜单栏宽度与折叠态 SHALL 由 `config.layout` 驱动（menuWidth/menuCollapse）。

#### Scenario: 登录落地呈现壳层

- **WHEN** 登录成功落地 `/admin/dashboard`
- **THEN** 页面呈现侧边菜单栏、顶栏与主内容区三栏结构
- **AND** 主内容区渲染 dashboard 视图

#### Scenario: 折叠菜单栏

- **WHEN** 用户点击底部工具栏的折叠按钮
- **THEN** 菜单栏收窄为图标模式（menuCollapse = true），再点击恢复

### Requirement: 侧边菜单渲染与导航

侧边菜单 SHALL 以 `el-menu` 递归渲染菜单树：有 children 的节点渲染为 `el-sub-menu`，叶子渲染为 `el-menu-item`；叶子点击 SHALL 经路由跳转到其 path 并高亮激活项（含父级展开）。菜单数据 SHALL 来自 `GET /admin/init` 的 `menus`；为空数组时 SHALL 使用前端示例菜单兜底（数据形状与后端 routeFromRule 输出一致）。

#### Scenario: 菜单点击导航

- **WHEN** 用户点击叶子菜单「管理员账号」
- **THEN** 路由跳转到 `/admin/manager` 且该菜单项高亮

#### Scenario: 示例菜单兜底

- **WHEN** init 响应 `menus` 为空数组（如 root 账号无分组权限）
- **THEN** 侧边菜单渲染前端示例菜单树（含目录与叶子）

### Requirement: 标签页导航

顶栏 SHALL 提供标签页导航：访问带菜单属性的页面时自动新增标签（按路由去重、更新 query）；点击标签切换路由；激活标签以浮动色块标记并自动滚动到可见区域；标签右键 SHALL 提供上下文菜单：刷新（重载当前视图）、关闭、关闭其他、关闭全部；关闭激活标签后 SHALL 跳转到最后一个标签或后台主页。

#### Scenario: 访问页面自动开标签

- **WHEN** 用户从侧边菜单进入「管理员账号」再进入「后台主页」
- **THEN** 顶栏出现两个标签，激活态跟随当前路由

#### Scenario: 关闭其他标签

- **WHEN** 用户在某标签上右键选择「关闭其他」
- **THEN** 仅保留该标签与激活标签相关的页面状态

#### Scenario: 刷新当前视图

- **WHEN** 用户在右键菜单选择「刷新」
- **THEN** 主内容区当前视图被重载（组件重新挂载）

### Requirement: 顶栏操作区

顶栏右侧 SHALL 提供：站点主页外链（新窗口）、语言切换（zh-cn/en，写入 config 持久化）、暗色模式开关（html.dark + element 暗色变量）、全屏切换、用户信息弹层（头像/昵称/上次登录时间/退出登录按钮）。退出登录 SHALL 调用既有 logout 接口（幂等）并清空本地登录态后跳转登录页。

#### Scenario: 暗色切换

- **WHEN** 用户点击暗色开关
- **THEN** html 根元素切换 dark class，布局取色切换为暗色对

#### Scenario: 退出登录

- **WHEN** 用户在用户弹层点击「退出登录」
- **THEN** 调用 `POST /admin/logout`，清空 `useAdminInfo`，跳转 `/admin/login`

### Requirement: 多布局模式容器

布局 SHALL 支持 5 种模式，由 `config.layout.layoutMode` 驱动、根布局动态切换容器且不丢标签页状态：Default（侧栏 + 悬浮标签顶栏）、Classic（通栏侧栏 + 通栏标签顶栏）、Streamline（无侧栏，Logo + 横向菜单顶栏）、Double（横向一级菜单顶栏 + 当前一级菜单子树侧栏）、LeftSplit（一级窄主栏 + 次级菜单树侧栏）。菜单树中相对 path SHALL 统一补全为后台绝对路径（`/admin/xxx`）供激活匹配与导航。

#### Scenario: 切换到 Streamline 模式

- **WHEN** 用户在布局设置抽屉选择「单栏」模式
- **THEN** 侧边栏消失，顶栏呈现 Logo + 横向菜单 + 操作区，已打开标签页保留

#### Scenario: Double 模式子菜单派生

- **WHEN** 布局为 Double 且用户导航到「权限管理/管理员账号」
- **THEN** 顶栏激活「权限管理」，侧栏渲染该目录的子菜单树

#### Scenario: LeftSplit 一级菜单点击

- **WHEN** 布局为 LeftSplit 且用户点击主栏中含子菜单的一级菜单
- **THEN** 次级侧栏切换为该菜单的子树；点击叶子菜单则直接导航

#### Scenario: 横向菜单一级目录点击

- **WHEN** 用户在横向菜单（Streamline/Double 顶栏）点击含子菜单的一级目录
- **THEN** 路由跳转到其第一个可导航子菜单；无可导航子菜单时提示无子菜单

### Requirement: 布局设置抽屉

顶栏 SHALL 提供布局配置入口，抽屉内 SHALL 支持：布局模式切换（5 种，缩略图选择）、暗黑模式、主内容区切换动画（slide/fade 五种）、侧边菜单栏与顶栏的全套色位编辑（亮/暗双色对，编辑写入当前主题对应侧）、菜单宽度/折叠/手风琴/顶部栏开关、LeftSplit 专属配色与次级菜单宽度、恢复默认（二次确认）。所有配置 SHALL 直写 config store 并持久化（刷新后保留），布局组件实时联动。

#### Scenario: 修改菜单背景色

- **WHEN** 用户在抽屉中修改「侧边菜单栏背景色」
- **THEN** 侧边菜单立即应用新色；刷新页面后仍保留

#### Scenario: 恢复默认

- **WHEN** 用户点击「恢复默认」并确认
- **THEN** 布局配置回到初始值，界面即时回到默认形态

### Requirement: 账号资料抽屉

用户信息弹层 SHALL 提供账号资料入口：抽屉展示当前管理员概览（头像/用户名/上次登录时间），支持昵称与头像（agUpload 上传）编辑；示例数据约定下提交仅写入本地 adminInfo store，不调用后端资料接口。

#### Scenario: 修改昵称

- **WHEN** 用户在账号资料抽屉修改昵称并保存
- **THEN** 顶栏用户信息弹层显示新昵称（本地状态即时生效）

### Requirement: 标签全屏与 iframe 菜单

标签右键菜单 SHALL 提供「当前标签全屏」（隐藏侧栏与顶栏，主内容区占满，悬浮按钮还原）。menuType 为 `iframe` 的菜单叶子 SHALL 以 iframe 视图渲染（path 编码外链 URL，视图解码后嵌入），注册为后台子路由。

#### Scenario: 当前标签全屏

- **WHEN** 用户在某标签右键选择「当前标签全屏」
- **THEN** 侧边栏与顶栏隐藏，主内容区占满视口，出现还原入口

#### Scenario: iframe 菜单渲染

- **WHEN** 用户点击「iframe 示例」菜单
- **THEN** 主内容区嵌入对应外链页面的 iframe

### Requirement: 主内容区 keep-alive

主内容区 SHALL 以 keep-alive 缓存现存标签页对应的视图（include 名单 = 现存标签的路由 name，标签关闭时同步移除）；视图组件名与路由 name 对齐时缓存生效，未对齐的视图退化为不缓存（无害）。

#### Scenario: 标签间切换保留状态

- **WHEN** 用户在「后台主页」与「管理员账号」标签间来回切换
- **THEN** 各视图的本地状态（如表单输入）被保留

#### Scenario: 关闭标签释放缓存

- **WHEN** 用户关闭某标签
- **THEN** 该视图退出 keep-alive 名单，再次打开时重新挂载
