# Proposal: uni-app H5 客户端（C 端）

## Why

`ai-go-mall` 已通过 `add-trading-card-blindbox-mvp` 落地完整的 C 端后端能力：`/user` 下 14 条业务路由、8 张业务表（迁移 000006~000015）、抽卡事务与校验责任链、限流中间件均已就绪（该 change 完成 150/163，未完成项为缓存预热与 TTL 抖动，与客户端链路无关）。

但 C 端**前端只有 Vue 3 PC Web 版**（`web/src/views/user/`，`layouts/user/` 为 1200px 横向导航布局）。该实现无法在手机上使用：横向导航挤压、瀑布流缺失、无开卡动效、登录页为用户名密码 + 点选验证码（350×200 背景图坐标点击），在移动端的交互体验不可接受。

本 change 新增基于 **uni-app（Vue 3）的 H5 客户端**，独立工程落 `uni-h5/`，只打通「首页商品 → 下单」这一条最小但端到端可演示的链路。UI 参考「嗨卡」App 的深色卡牌形态，登录改为**手机号 + 滑块验证码**（登录与校验本期均由客户端 mock 承接，服务端契约在本 change 中定义清楚，实现留待后续 change）。

同时修复一处**现存线上隐患**：`GET /user/home/feed` 强依赖 Elasticsearch 且无兜底，ES 不可用时直接返回 500，H5 首页将白屏。本 change 为它补上 MySQL 降级通道。

### 已确认的四项决策

| # | 决策 | 落地 |
|---|---|---|
| 1 | 滑块验证码**独立新增** | 不复用 `click-captcha`（点选式），新建 `slider-captcha` 能力，`key` 空间隔离 |
| 2 | 新用户首登**自动建号** | 手机号即 `username`，余额 0；以 `mobile` 唯一索引保证幂等 |
| 3 | feed **本期就加 MySQL 降级** | 新增 `home-feed-fallback` 能力，修订既有 change 的「不兜底」约束 |
| 4 | mock **只覆盖 UI** | mock 仅承接登录页交互与其配套 5 个接口；商品 / 抽卡 / 订单 100% 走真实后端 |

## What Changes

### 新增独立前端工程 `uni-h5/`

- 基于官方 `uni-preset-vue/vite-ts` 模板，`@dcloudio/*` 版本锁定 `3.0.0-5020420260813003`
- **与现有 `web/` 完全隔离**：两端依赖区间硬冲突（官方模板锁 Vite 5.2.8 / TS 4.9.4，`web/` 用 Vite 8.0.10 / TS 6.0.3），`package.json` 不得互相引用，不做 monorepo 共享
- **仅配置 h5 平台**：不打包 App、不发布小程序

### 请求层自封装

- `uni.request` 是唯一跨端 API（axios 依赖 XMLHttpRequest，小程序端不可用），封装 `src/utils/request.ts`
- 统一处理：baseURL、`Authorization: Bearer` 注入、401 拦截、`{code, message}` 错误映射、429 限流提示
- **顺带修复现有缺陷**：当前 Vue3 Web 版需在每个调用点手写 `Authorization` 头，新请求层统一注入

### 页面与路由

- `pages.json` 声明式路由（非 vue-router），3 个 TabBar：首页 / 订单 / 我的
- 8 个页面：首页、商品详情、开卡动画（页内组件）、抽卡结果、订单列表、订单详情、登录、我的
- **不实现**：秒杀、关注、搜索、卡册、充值、地址、直播拼盒、客服入口

### 视觉语言整体替换

- 深色卡牌主题：底色 `#0b0d12`、金色主色 `#f5b544`、稀有度四档（SSR 金 / SR 紫 / R 蓝 / N 灰）
- 首期瀑布流采用**原生 `<scroll-view>` 双列 + 图片懒加载**，封装为独立组件以留出后续替换为虚拟列表的口子

### 登录与验证码（契约新增，本期 mock）

- 登录方式改为 **手机号 + 短信验证码 + 滑块验证码**
- **不改动**现有 `POST /user/login`（用户名 + 密码 + 点选验证码），PC Web 继续使用
- 新增契约：`POST /user/sms/send`、`GET /common/captcha/slider`、`POST /common/captcha/slider/verify`、`POST /user/login/mobile`、`GET /user/profile`
- **滑块不是现有点选验证码的复用**：`click-captcha` 是 350×200 背景图按序点击坐标，滑块是拖动拼图，属独立能力，`key` 空间隔离
- **首登自动建号**：未注册手机号验证通过后自动建号（手机号即 `username`、`password` 置空、`balance` 为 0），以 `users.mobile` 唯一索引保证并发幂等

### 客户端 Mock 层（只覆盖 UI）

- 统一开关 `src/config/index.ts` 的 `USE_MOCK`
- **mock 边界严格限定在登录相关的界面交互**：`/user/sms/send`、`/common/captcha/slider`、`/common/captcha/slider/verify`、`/user/login/mobile`、`/user/profile` 五条由 mock 承接；**商品 feed、盲盒详情、抽卡下单、订单列表与详情全部走真实后端**
- token 通过真实 `POST /user/login` 换取，保证抽卡**真实扣库存、真实产生订单**，主链路端到端可验证，而非演示性质的假数据流
- mock 返回结构严格对齐 §契约，后续切真接口零改动

### 首页 Feed MySQL 降级（后端改动）

- `GET /user/home/feed` 在主路径（ES）失败时回落到 MySQL 查询，**响应结构 MUST 与 ES 路径完全同构**，客户端无感知
- 仅「ES 返回错误」触发降级；「ES 返回 0 条」是合法业务结果，MUST NOT 降级
- 降级原因 MUST 可观测：响应头标识来源（`X-Feed-Source: es | mysql`）+ WARN 日志
- 降级结果 MUST NOT 按正常 TTL 回填缓存，避免 ES 恢复后仍返回降级数据
- 这是对既有 change「ES 不可用时不查 MySQL 兜底」约束的**修订**，约束优先级写在本 change 的 `home-feed-fallback` 能力中

## Capabilities

### New Capabilities

- `uni-h5-client`：uni-app H5 客户端工程（独立依赖树、`pages.json` 路由、3 TabBar、8 页面、`uni.request` 封装、深色卡牌主题、瀑布流组件、开卡动画、结果页数据传递、登录回跳、mock 层开关与边界）。
- `user-mobile-login`：C 端手机号短信验证码登录契约（发码接口 + 登录接口 + 用户信息接口 + 鉴权与错误码 + 首登自动建号规则 + 建号来源落库要求）。
- `slider-captcha`：滑块验证码能力（创建接口 + 预检接口 + 消费型二次校验语义 + 容差与失败计数），与现有点选式 `click-captcha` 并存。
- `home-feed-fallback`：首页 Feed 的 ES → MySQL 降级通道（触发条件、DTO 同构约束、缓存回填约束、可观测性），并修订既有 change 中「ES 不可用时不查 MySQL 兜底」的约束。

### Modified Capabilities

**无。** 有四处需要说明为何不修改：

- `user-draw-result` 描述的是 **Vue 3 PC Web 版**结果页（vue-router + sessionStorage 传参）。H5 版使用 uni-app 路由与 `uni.setStorageSync`，两端共存而非替换，因此 H5 的结果页行为写入 `uni-h5-client` 自己的 spec，**不修改**主 spec，避免 PC Web 的契约被 H5 实现覆盖而失真。
- `homepage` 是面向访客的营销门面页（`/`，引导进后台），与 C 端商品首页（`/user/home`）不是同一能力，无冲突。
- feed 的降级行为**本应修改** `add-trading-card-blindbox-mvp` 的 spec，但该 change 尚未归档（150/163），处于 `changes/` 目录，无法跨 change 修改。因此以**新增 `home-feed-fallback` 能力**的方式定义降级语义，并在其 spec 中写明约束优先级；该 change 归档时 MUST 同步修订其「不 fallback MySQL」表述。
- `db-migration` 定义的是迁移**机制**（文件命名规则、目录、CLI 子命令、启动期阻塞 up、禁用 AutoMigrate）。本期为其新增一个迁移文件属于**使用**该机制，不改变机制本身的任何行为，因此无需 delta。新迁移已按该 spec 的要求用 `migrate create` 生成。

## Impact

- **新增前端工程**：`uni-h5/`，约 40 个文件（8 页面 + 3 TabBar + 请求层 + mock 层 + 组件 + 配置）
- **依赖冲突**：与 `web/` 必须物理隔离，两个 `package.json` 不得合并；CI 需分别安装
- **后端**：**1 处改动** —— `internal/service/user/home.go` 的 `loadFeed` 增加 MySQL 降级分支（含 MySQL 查询方法、降级标识、缓存回填约束）。新增的 5 条登录相关接口契约仅用于前端 mock 对齐，不落服务端代码
- **数据库**：**1 处改动** —— 新增迁移 `000017_users_register_source`，为 `users` 表追加 `register_source varchar(16)`（注册来源标记，可空，NULL 表示未知或历史数据）。链路涉及的其他 8 张表全部已存在、无需改动。**本期只落 schema 与 model 字段**，写入逻辑随 `POST /user/login/mobile` 的服务端实现落地
- **沉淀文档**：`uni-h5/docs/接口与数据契约.md`（接口总览、流程图、状态图、表结构、待确认项）
- **后续 change 依赖**：后端短信真实通道、滑块服务端实现、`POST /user/login/mobile` 实现

## Out of Scope（非目标）

明确不在本期落地：

- 打包 App（iOS / Android）与微信小程序
- 后端短信真实通道接入（继续用 `config/channel.yaml` 的 `mock` driver）
- 后端滑块验证码服务端实现（本期前端 mock；点选验证码保持不动）
- `POST /user/login/mobile` 的后端实现（**含 `register_source` 的写入逻辑**：字段本期落表，但建号时写值依赖该接口的服务端实现）
- 秒杀链路（列表 / 详情 / 秒杀抽卡）
- 关注供应商（`/user/follow*` 三条路由）
- 搜索、分类筛选、卡册、晒卡、二级交易
- 充值、提现、地址管理、真实支付
- 直播拆卡、拼盒、组队
- 联系客服入口（本期明确删除）
- `web/` 现有 Vue3 PC 端的任何改动
