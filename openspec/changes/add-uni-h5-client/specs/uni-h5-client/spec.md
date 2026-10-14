## Purpose

以 uni-app（Vue 3 + Vite）提供 C 端移动 H5 客户端，独立工程落于仓库 `uni-h5/` 目录，只覆盖「首页商品浏览 → 商品详情 → 抽卡下单 → 开卡结果 → 订单查看」主链路，并复用既有 Go 后端 `/user` 接口。本能力只描述客户端工程结构、路由、请求层、页面行为与视觉规范，不含服务端实现。

## ADDED Requirements

### Requirement: 独立前端工程与依赖隔离

`uni-h5/` SHALL 是一个自包含的 uni-app 工程，拥有独立的 `package.json`、`node_modules` 与构建配置。

`uni-h5/package.json` MUST NOT 引用、继承或以 workspace 形式链接 `web/` 的任何依赖或源码。两端依赖区间存在硬冲突（官方 uni-app 模板锁定 Vite 5.x / TypeScript 4.x，而 `web/` 使用 Vite 8.x / TypeScript 6.x），MUST 各自独立安装依赖。

`@dcloudio/*` 依赖版本 MUST 锁定为 `3.0.0-5020420260813003`，并在 `package.json` 中使用精确版本号（该版本号为非语义化日期串，使用 `^` 范围可能被解析到 alpha 线）。

#### Scenario: 独立安装依赖

- **WHEN** 在 `uni-h5/` 下执行依赖安装
- **THEN** 安装结果 MUST NOT 修改 `web/package.json` 或 `web/node_modules`
- **AND** 两端依赖树不存在共享的锁定文件

#### Scenario: 版本号精确锁定

- **WHEN** 检查 `uni-h5/package.json` 中的 `@dcloudio/*` 依赖
- **THEN** 版本值 MUST 为 `3.0.0-5020420260813003`，不带 `^` 或 `~` 前缀

### Requirement: 仅启用 H5 平台

工程本期 SHALL 仅构建与运行 `h5` 平台。

`manifest.json` MUST NOT 配置 App 打包（iOS / Android）与小程序相关节点；构建脚本 MUST NOT 包含 `build:app` 与 `build:mp-*` 目标；代码中 MUST NOT 出现针对小程序或原生 App 的条件编译特化分支。

#### Scenario: 构建产物为 H5

- **WHEN** 执行构建脚本
- **THEN** 产物 MUST 为 H5 静态资源（HTML + JS + CSS）
- **AND** MUST NOT 产出 `.apk` / `.ipa` / 小程序包

#### Scenario: 无跨端条件编译残留

- **WHEN** 审查源码
- **THEN** MUST NOT 出现以 `MP-WEIXIN` / `APP-PLUS` 为条件的编译指令

### Requirement: 声明式路由与三 TabBar 结构

页面路由 MUST 通过 `pages.json` 声明，MUST NOT 引入 `vue-router`。

底部 TabBar MUST 恰好包含三个页签，顺序为：首页、订单、我的。非 Tab 页面（商品详情、抽卡结果、登录）MUST NOT 出现在 TabBar 中。

`pages.json` 的 `pages` 数组第一项 MUST 为首页，作为应用启动页。

#### Scenario: TabBar 项与顺序

- **WHEN** 应用启动并渲染底部导航
- **THEN** MUST 显示三个页签，顺序为首页、订单、我的
- **AND** 每个页签 MUST 有选中态与未选中态两套图标

#### Scenario: 非 Tab 页不显示底部导航

- **WHEN** 用户从首页进入商品详情页
- **THEN** 详情页 MUST NOT 渲染底部 TabBar

### Requirement: 页面清单与职责

客户端 SHALL 实现以下 8 个页面，路径与鉴权要求如下：

| 页面 | 路径 | 鉴权 | 职责 |
|---|---|---|---|
| 首页 | `pages/home/index` | 否 | 商品瀑布流 |
| 商品详情 | `pages/blindbox/detail` | 否 | 库存、概率公示、下单 |
| 开卡动画 | 页内组件 | 否 | 全屏开卡动效 |
| 抽卡结果 | `pages/draw/result` | 否 | 本次抽中卡牌 |
| 订单列表 | `pages/orders/index` | 是 | 订单分档列表 |
| 订单详情 | `pages/orders/detail` | 是 | 订单与卡牌明细 |
| 登录 | `pages/login/index` | 否 | 手机号 + 短信码（本期无滑块，见 `slider-captcha` 能力） |
| 我的 | `pages/mine/index` | 是 | 用户信息与余额 |

商品详情与抽卡结果 MUST 允许游客访问；订单列表、订单详情与我的 MUST 要求已登录。

#### Scenario: 首页为启动页

- **WHEN** 打开 H5 入口
- **THEN** MUST 直接进入首页并开始拉取商品数据

#### Scenario: 游客可浏览详情

- **WHEN** 未登录用户进入商品详情页
- **THEN** MUST 正常渲染库存与概率公示，MUST NOT 强制跳转登录

#### Scenario: 受保护页面拦截

- **WHEN** 未登录用户直接访问订单列表
- **THEN** MUST 跳转登录页并携带回跳地址

### Requirement: 范围外页面禁止

客户端 MUST NOT 实现以下页面或入口：秒杀列表与详情、关注供应商、搜索、分类筛选、卡册、充值、地址管理、直播拆卡、拼盒、客服。

#### Scenario: 不存在范围外页面

- **WHEN** 审查 `pages.json` 中注册的页面清单
- **THEN** MUST NOT 存在上述范围外的页面

#### Scenario: 无客服入口

- **WHEN** 审查全部页面模板
- **THEN** MUST NOT 出现联系客服的按钮、浮标或跳转

### Requirement: 统一请求层封装

客户端 MUST 提供单一请求封装模块（`src/utils/request.ts`），业务代码 MUST NOT 直接调用 `uni.request`。

封装 MUST 处理：基础地址拼接、`Authorization: Bearer <token>` 注入、JSON 解析、HTTP 状态码到业务错误的映射、网络异常兜底。基础地址 MUST 由环境配置提供，MUST NOT 硬编码在业务文件中。

对需鉴权接口，请求层 MUST 从本地存储读取 token 自动注入，MUST NOT 要求调用方逐个手工传入——此项为对现有 Vue3 Web 端「每个调用点手写 Authorization 头」缺陷的修正。

#### Scenario: 自动注入鉴权头

- **WHEN** 已登录用户调用 `POST /user/blindbox/draw`
- **THEN** 请求 MUST 自动携带 `Authorization: Bearer <token>`
- **AND** 业务调用代码 MUST NOT 出现手工拼接 Authorization 的语句

#### Scenario: 无 token 时公开接口仍可调用

- **WHEN** 游客调用 `GET /user/blindbox/detail`
- **THEN** 请求 MUST 正常发出且 MUST NOT 因缺少 token 而中断

### Requirement: 鉴权失败与限流的统一处理

请求层 MUST 对 401 响应统一处理：清除本地 token、跳转登录页并携带当前页面完整路径作为 `redirect` 参数。

请求层 MUST 对 429 响应统一处理：提示「操作过于频繁」并通知调用方禁用触发按钮至少 3 秒。

登录成功后，登录页 MUST 读取 `redirect` 参数并回跳；`redirect` MUST 仅接受站内路径，非法值 MUST 回退首页，MUST NOT 出现开放重定向。

#### Scenario: 401 触发登录并回跳

- **WHEN** 用户在商品详情页点击抽卡且 token 已过期（服务端返回 401）
- **THEN** 客户端 MUST 清除本地 token 并跳转登录页
- **AND** 登录页 URL MUST 携带 `redirect` 指向该商品详情页
- **AND** 登录成功后 MUST 自动回跳该商品详情页

#### Scenario: 非法 redirect 被拒绝

- **WHEN** 登录页收到的 `redirect` 指向站外地址
- **THEN** 登录成功后 MUST 回退到首页
- **AND** MUST NOT 跳转到站外地址

#### Scenario: 429 禁用按钮

- **WHEN** 抽卡接口返回 429
- **THEN** 客户端 MUST 提示操作过于频繁
- **AND** 抽卡按钮 MUST 在至少 3 秒内保持禁用

### Requirement: 抽卡错误码到用户文案的映射

客户端 MUST 按下表把 `POST /user/blindbox/draw` 的错误码映射为文案与动作，MUST NOT 直接展示后端 `message`（后端统一返回 `"draw failed"`，对用户无意义）：

| HTTP | code | 客户端行为 |
|---|---|---|
| 400 | `draw.invalid_input` | 提示参数异常 |
| 401 | `draw.unauthorized` | 跳登录并回跳 |
| 402 | `draw.insufficient_balance` | 提示余额不足 |
| 403 | `draw.user_not_available` | 提示账号不可用 |
| 404 | `draw.blindbox_not_available` | 提示已下架 |
| 409 | `draw.sold_out` | 提示售罄并刷新 |
| 429 | `rate_limited` | 禁用按钮 3 秒 |
| 500 | `draw.internal` | 通用失败可重试 |

#### Scenario: 余额不足引导

- **WHEN** 抽卡返回 402 `draw.insufficient_balance`
- **THEN** 客户端 MUST 展示余额不足提示
- **AND** MUST 提供跳转「我的」页面的入口

#### Scenario: 售罄后刷新库存

- **WHEN** 抽卡返回 409 `draw.sold_out`
- **THEN** 客户端 MUST 展示售罄提示
- **AND** MUST 重新请求当前商品详情以刷新库存与概率数据

### Requirement: 首页瀑布流渲染

首页 SHALL 以双列瀑布流展示商品卡片，数据源为 `GET /user/home/feed`。

本期 MUST 使用原生 `<scroll-view>` 实现双列瀑布流并配合图片懒加载，MUST NOT 引入虚拟列表组件。瀑布流 MUST 封装为独立组件，对外仅暴露列表数据与加载更多事件，以便后续替换内部实现时不影响调用方。

商品卡片 MUST 展示封面、名称、价格（存在活动价时优先展示活动价并保留原价删除线）、稀有度摘要。

#### Scenario: 双列布局渲染

- **WHEN** 首页获取到商品列表
- **THEN** MUST 以两列瀑布流渲染，卡片宽度约为容器一半且随内容高度自适应

#### Scenario: 图片懒加载

- **WHEN** 商品卡片尚未进入视口
- **THEN** MUST NOT 加载其封面图；进入视口附近后再加载

#### Scenario: 活动价展示

- **WHEN** 某商品存在 `promo_price` 且低于 `price`
- **THEN** 卡片 MUST 展示 `promo_price` 作为主价格，并以删除线展示 `price`

### Requirement: 商品详情与概率公示

详情页 SHALL 展示从 `GET /user/blindbox/detail` 获取的数据：商品封面与名称、供应商名称、库存进度、概率公示表、卡池预览，并在底部固定下单按钮。

概率公示表 MUST 由 `items[]` 的 `weight` 换算（`weight / 10000`）得到百分比展示；库存进度 MUST 基于 `pool_total_stock` 展示；下单按钮 MUST 展示 `effective_price` 而非 `blind_box.price`。

#### Scenario: 概率公示展示

- **WHEN** 详情接口返回若干卡池条目
- **THEN** 页面 MUST 按稀有度分组展示每个条目的中奖概率百分比
- **AND** 同一卡池内所有条目概率之和 MUST 展示为 100%

#### Scenario: 价格取值

- **WHEN** 详情接口返回的 `effective_price` 与 `blind_box.price` 不同（存在活动价）
- **THEN** 下单按钮 MUST 展示 `effective_price`

### Requirement: 抽卡下单与开卡动画

用户点击下单按钮 SHALL 触发 `POST /user/blindbox/draw`，请求体为 `{ blind_box_id }`。

请求进行中按钮 MUST 进入加载态且不可重复点击。请求成功后 MUST 播放全屏开卡动画，动画结束 MUST 跳转抽卡结果页；请求失败 MUST 停止加载态并按错误码映射处理，MUST NOT 播放开卡动画。

#### Scenario: 下单成功播放动画

- **WHEN** 抽卡接口返回 200
- **THEN** 客户端 MUST 播放全屏开卡动画
- **AND** 动画结束后 MUST 跳转抽卡结果页

#### Scenario: 重复点击防护

- **WHEN** 用户已点击下单且请求未返回
- **THEN** 下单按钮 MUST 处于禁用状态，再次点击 MUST NOT 发起新请求

#### Scenario: 下单失败不播动画

- **WHEN** 抽卡接口返回非 200
- **THEN** MUST NOT 播放开卡动画，且 MUST 按错误码映射给出提示

### Requirement: 抽卡结果页数据传递与兜底

抽卡结果页 SHALL 直接使用下单接口响应中的数据渲染，MUST NOT 为展示结果额外发起请求。

下单响应 MUST 先写入本地存储再跳转结果页；结果页读取后 MUST 清除该存储项以防重复消费。结果页 MUST 在无有效数据时（刷新、直接访问、存储已被清除）自动重定向到订单列表，MUST NOT 出现白屏。

结果页 MUST 按 `cards[].rarity` 以四档配色（SSR / SR / R / N）渲染卡牌边框，并展示 `snapshot_name` 与 `snapshot_image`。

#### Scenario: 正常渲染

- **WHEN** 下单成功后跳转结果页
- **THEN** 页面 MUST 展示本次抽中的全部卡牌及稀有度边框
- **AND** MUST 展示订单号与实付金额

#### Scenario: 刷新后兜底

- **WHEN** 用户在结果页执行刷新，本地存储中已无结果数据
- **THEN** 页面 MUST 自动重定向到订单列表
- **AND** MUST NOT 出现空白页

#### Scenario: 快照字段优先

- **WHEN** 渲染结果页卡牌名称与图片
- **THEN** MUST 使用 `snapshot_name` 与 `snapshot_image`
- **AND** MUST NOT 使用卡牌主数据的当前值

### Requirement: 订单列表与订单详情

订单列表 SHALL 调用 `GET /user/orders/list`（参数 `page` / `page_size`）并以分页方式渲染，支持上拉加载更多。

订单详情 SHALL 调用 `GET /user/orders/detail?id=` 渲染订单基础信息与卡牌明细。

列表与详情 MUST 仅依赖白名单字段（`id` / `order_no` / `blind_box_id` / `price` / `status` / `source` / `created_at` 及明细的 `rarity` / `snapshot_name` / `snapshot_image`），MUST NOT 依赖后端直接返回的 GORM 模型中的内部字段（如 `deleted_at`）。

订单状态 MUST 按下表映射为文案：`pending` → 待支付、`paid` → 已支付、`drawn` → 已开卡、`failed` → 已失败。

#### Scenario: 状态文案映射

- **WHEN** 订单列表中某订单 `status` 为 `drawn`
- **THEN** 客户端 MUST 展示为「已开卡」并采用成功态配色

#### Scenario: 分页加载

- **WHEN** 用户上拉至列表底部且仍有更多数据
- **THEN** 客户端 MUST 请求下一页并追加渲染

#### Scenario: 越权订单按不存在处理

- **WHEN** 订单详情接口返回 404 `order.not_found`
- **THEN** 客户端 MUST 提示订单不存在并返回列表
- **AND** MUST NOT 暗示该订单属于其他用户

### Requirement: 视觉令牌与稀有度配色

客户端 SHALL 统一使用深色卡牌主题，颜色 MUST 通过集中定义的 CSS 变量使用，MUST NOT 在页面样式中散落硬编码色值。

主题 MUST 至少定义：背景色 `#0b0d12`、卡片面 `#161a22`、主文字 `#e9ecf2`、次要文字 `#8b94a6`、主强调色 `#f5b544`。

稀有度配色 MUST 固定为四档：SSR `#ffd479`、SR `#b98cff`、R `#5aa9ff`、N `#8b94a6`，并 MUST 在结果页、详情页卡池预览、订单详情三处保持一致。

#### Scenario: 色值集中定义

- **WHEN** 审查页面样式
- **THEN** 颜色 MUST 引用集中定义的变量，MUST NOT 出现重复的十六进制字面量
- **AND** 本约束限于页面与组件样式；SVG data URI（无法解析 CSS 变量）与 `pages.json` 构建期 JSON（无法引用变量）属平台强制例外，MUST NOT 含稀有度色值

#### Scenario: 稀有度配色一致

- **WHEN** 同一张 SSR 卡牌分别出现在结果页与订单详情页
- **THEN** 两处边框颜色 MUST 相同

### Requirement: 本地 Mock 层开关与边界

客户端 MUST 提供统一的 mock 开关（默认开启），用于在真实登录接口就绪前替代登录页交互。

mock 覆盖范围 MUST 严格限于登录相关的界面交互及其配套接口：发送短信验证码、滑块验证码创建与校验、手机号登录。商品 feed、盲盒详情、抽卡下单、订单列表、订单详情与当前用户信息 MUST 走真实后端，MUST NOT 被 mock 拦截。

mock MUST NOT 产出被用于鉴权的令牌。用户在 mock 模式下完成登录后，客户端 MUST 通过真实登录接口换取有效令牌，以保证后续受保护接口与抽卡链路真实可达。

该边界的判据：即便 mock 开关被误开在生产环境，受影响的也仅限于登录页交互，商品与订单数据 MUST NOT 失真。

mock 的响应结构 MUST 与真实接口契约一致，切换开关后业务代码 MUST 无需改动。

#### Scenario: mock 模式下登录可用

- **WHEN** mock 开关为开启状态，用户输入合法手机号与固定验证码 `000000`
- **THEN** MUST 登录成功并写入本地用户信息

#### Scenario: mock 不参与鉴权

- **WHEN** mock 开关为开启状态，用户完成一次 mock 登录
- **THEN** 客户端 MUST 通过真实登录接口换取令牌
- **AND** 随后的受保护接口请求 MUST 通过服务端鉴权，MUST NOT 返回 401

#### Scenario: 业务接口走真实后端

- **WHEN** mock 开关为开启状态，用户进入首页
- **THEN** `/user/home/feed` MUST 走真实后端请求，MUST NOT 被 mock 拦截

#### Scenario: mock 注册范围受限

- **WHEN** 审查 mock 层注册的接口清单
- **THEN** 清单 MUST 恰好为登录相关 4 条
- **AND** MUST NOT 包含 feed、detail、draw、orders、profile 相关路径

#### Scenario: 关闭开关后零改动切换

- **WHEN** mock 开关改为关闭，且后端 `/user/login/mobile` 已实现
- **THEN** 登录页业务代码 MUST 无需修改即可对接真实接口

### Requirement: 余额接口不参与 mock

`GET /user/profile` MUST NOT 由 mock 承接，MUST 始终请求真实后端。

判据：该接口的唯一目的是返回下单后的**最新余额**，而 mock 只能返回登录时刻的快照，两者直接矛盾。后端实现落地后（见 `user-mobile-login` 的「当前用户信息接口」需求）MUST 同步从 mock 注册表中移除，MUST NOT 保留「后端已实现但仍被 mock 拦截」的中间态。

#### Scenario: 余额接口走真实后端

- **WHEN** mock 开关为开启状态，用户进入「我的」页面触发余额刷新
- **THEN** `GET /user/profile` MUST 走真实后端请求，MUST NOT 被 mock 拦截
- **AND** 返回的 `balance` MUST 为服务端实时值，而非登录时刻的快照

### Requirement: 文案内联

本期客户端为单语言（简体中文）应用，不引入国际化框架。界面文案 MUST 直接书写于模板或组件脚本中，无需任何词条映射层。

状态、来源、抽卡错误码等需要「后端枚举值 → 文案」映射的场景，MUST 通过集中定义的常量表完成，MUST NOT 在模板中散落条件判断。

#### Scenario: 无词条映射层

- **WHEN** 审查客户端源码
- **THEN** MUST NOT 存在语言资源目录或 i18n 装配代码
- **AND** MUST NOT 依赖 `vue-i18n` 或任何词条解析库

#### Scenario: 枚举值文案集中映射

- **WHEN** 审查订单状态、下单来源或抽卡错误码的文案来源
- **THEN** 映射关系 MUST 定义在单一常量表中
