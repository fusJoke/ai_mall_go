# Tasks: uni-app H5 客户端

> 范围：仅 H5，仅「首页商品 → 下单」链路。后端 1 处改动（Feed 降级，见第 11 组）+ 1 处表结构改动（见第 12 组）。数据库新增 1 个迁移、无数据回填。
>
> 已确认决策：① 滑块独立新增；② 首登自动建号；③ feed 本期加 MySQL 降级；④ mock 只覆盖 UI。
>
> ---
>
> **进度说明（2026-10-14 全链路联调完成）**
>
> - 第 1~9 组（客户端）**代码全部完成**，`npm run build:h5` 构建通过（EXIT=0）。
> - 第 11 组（Feed MySQL 降级）与第 13 组（`cmd/devtoken`）**已完成并真库验证**；
>   第 12 组（`users.register_source`）迁移已落库（版本 15 → 17）。
> - **全链路真库端到端已跑通**：登录（手机号 + 滑块 + 验证码 000000）→ 注入真实令牌 →
>   首页 6 条真实商品 → 详情页（概率公示 10 项）→ 下单 → `/pages/draw/result` 出订单号与卡牌 →
>   订单列表 5 单 → 库存 2848→2843、余额 500→5.00 精确扣减 → 402 余额不足路径验证。
> - **降级链路已用真停 ES 验证**：`docker stop es` 后首页返回 200 + `X-Feed-Source: mysql`，
>   恢复后自动回到 `es`（无需清缓存）。
> - **本次联调新发现并修复 1 处集成缺陷**：CORS 白名单缺 uni-h5 dev server 端口，
>   导致所有 POST 被 403 —— 详见第 14 组「联调实录」。
> - **2026-10-14（二）补充**：后端补齐 `GET /user/profile` 并将其移出前端 mock 注册表，
>   10.4 与 10.6 转为通过；剩余未决项仅 8.7（产品决策）。
> - **2026-10-15 补充**：8.7 已决策（列表页不加缩略图）；另修复 `store/user.ts` 的
>   `patchUser()` 无快照早退缺口（见第 16 组）。至此本 change 无未决项。
> - **验证方式调整**：本期未引入前端测试框架，tasks 中标注「单测」的**前端**验证项
>   以「构建通过 + 浏览器行为验证」替代（对应条目已注明 `待补单测`）；
>   **后端**降级路径已补 13 例真实单测（`home_fallback_test.go`）。
> - **色值合规例外**：`pages.json`（tabBar 配置必须字面量）与 `src/mock/slider.ts`（内联 SVG 即 mock 图片数据），
>   其余 `*.vue` / `*.scss` 已确认仅 `tokens.scss` 含十六进制色值。

## 1. 工程初始化

- [x] 1.1 基于官方 `uni-preset-vue/vite-ts` 模板结构手工搭建 `uni-h5/` 工程；`@dcloudio/*` 全部锁定精确版本 `3.0.0-5020420260813003`（不含 `^`） —— 实测：`npm ls` 输出 `@dcloudio/uni-app@3.0.0-5020420260813003`、`vite@5.2.8`、`typescript@4.9.5`、`vue@3.4.21`，`package.json` 中无 `^3.0.0-`。**后续修订**：`vue-i18n` 已随 i18n 删除一并移出依赖树（见 15.2）
- [x] 1.2 配置 `manifest.json` 仅保留 h5 节点 —— 实测：文件内无 `app-plus`、`mp-weixin`
- [x] 1.3 配置 `pages.json`：7 个页面（首页居首）+ 三个 TabBar 页签 —— 实测：开发服务器启动默认落首页，底部三个页签正常渲染（含脚本生成的图标）
- [x] 1.4 配置开发期跨域代理指向本地后端（`/api` → `127.0.0.1:8080`），并建立 `src/config/index.ts` —— 实测：浏览器经 `/api/user/home/feed` 打到后端并收到响应（500 = ES 未启，属后端问题）
- [x] 1.5 ~~建立 i18n 骨架（`src/lang/zh-cn/index.yaml` + 挂载）~~ —— **已撤销**：本期为单语言应用，i18n 层已整体删除（`src/lang/` 目录、`vue-i18n` 与 `yaml` 依赖、`main.ts` 挂载），文案改为内联。决策与理由见 design.md D14，执行记录见 15.1~15.4

## 2. 请求层与基础设施

- [x] 2.1 实现 `src/utils/request.ts`：baseURL 拼接、自动注入 `Authorization: Bearer`、默认 resolve 裸 JSON、错误统一为 `{http, code, message, retryAfter}` —— 验证：行为经浏览器 500/错误态验证；**单测待补**（工程未引入测试框架）
- [x] 2.2 实现 401 统一拦截：清除本地 token 并跳转登录页，携带当前页面完整路径作为 `redirect`
- [x] 2.3 实现 429 统一处理：提取 `Retry-After`（缺省 3 秒）抛出 `retryAfter`
- [x] 2.4 实现 `src/utils/storage.ts` 薄封装（JSON 序列化 + 类型提示） —— **单测待补**；读写往返已经登录链路实测（token / user 正确落 localStorage）
- [x] 2.5 实现 `src/store/user.ts`：token 与 user 的响应式状态，读写同步本地存储 —— 实测：登录后写入、刷新可恢复
- [x] 2.6 实现 `src/mock/index.ts` 拦截器骨架与开关（默认 `USE_MOCK = true`） —— 实测：命中 mock 的 URL 短路不发网络；未命中（feed）正常透传

## 3. 视觉系统

- [x] 3.1 编写 `src/styles/tokens.scss` —— 实测：全项目 grep 十六进制色值，`*.vue` / `*.scss` 中仅 tokens.scss 命中（例外见顶部说明）
- [x] 3.2 编写 `src/styles/common.scss`：卡片、按钮、价格、标签、状态视图等基础类 —— 实测：各页面复用类名，无重复定义
- [x] 3.3 实现稀有度配色工具（`src/utils/rarity.ts`，未知值回退 `--n`） —— **单测待补**

## 4. 登录模块（mock 承接）

- [x] 4.1 实现 `src/components/slider-captcha.vue`：拖动、松手自动预检、失败复位、连续 3 次失败自动重建、窗口 resize 重测 —— 实测：浏览器内拖动后自动预检并通过；尺寸经 `getBoundingClientRect` 校验（track 351×176、block 54×54）
- [x] 4.2 实现 `src/mock/slider.ts`：内联 SVG 生成图与随机 key，预检默认无条件通过（`MOCK_SLIDER_STRICT` 可开启严格校验以验证失败分支）
- [x] 4.3 实现 `src/mock/sms.ts`：校验手机号正则，返回成功并打印固定验证码 `000000` —— 实测：发码后按钮进入 60 秒倒计时
- [x] 4.4 实现 `src/mock/login.ts`：校验 `sms_code === '000000'`，返回与真实契约同构的 user 与 token —— 实测：`000000` 登录成功、token/user 正确落库；mock token 不参与鉴权
- [x] 4.5 实现登录页 `pages/login/index.vue`：手机号输入 + 60 秒倒计时发码 + 滑块 + 登录按钮 —— 实测：倒计时期间按钮文案变为「Ns 后重发」且不可重复点击。**2026-10-14（三）更新：滑块已从本页移除**（产品决策），现仅「手机号 + 短信码 + 登录」；滑块能力保留备用，见 14.4
- [x] 4.6 实现 `redirect` 回跳逻辑：仅接受以 `/` 开头且非 `//` 的站内路径，非法值回退；TabBar 页用 `switchTab`
- [x] 4.7 **方案修订**：原设计「静默调用真实 `POST /user/login` 换真 token」**不可行** —— 该接口强制「点选验证码」消费型校验，答案只存服务端、响应不返回。改为 `VITE_DEV_TOKEN` 注入真实令牌（见 `src/utils/session.ts` 与 README）；未配置时会话降级为 mock-only 并在「我的」页明示（实测：控制台警告按预期输出）
- [x] 4.8 实现 `src/api/profile.ts` 与「我的」页余额读取 —— 实现完成；真实余额待 `/user/profile` 服务端落地
- [x] 4.9 mock 边界自检：`src/mock/index.ts` 注册的 URL 恰好 4 条（sms / slider 两条 / login-mobile；profile 已随后端实现移出 mock，见 14.4），不含 `feed` / `detail` / `draw` / `orders`

## 5. 首页

- [x] 5.1 实现 `src/components/waterfall-list.vue`：双列按列高贪心派发、`lazy-load`、IntersectionObserver 触底哨兵抛 `load-more`；封装为独立组件保留虚拟列表升级口
- [x] 5.2 实现 `src/api/blindbox.ts` 的 feed 请求与类型定义（覆盖 `blind_boxes` / `suppliers` / `fetched_at` 全字段）
- [x] 5.3 实现首页 `pages/home/index.vue`：搜索入口（占位不跳转）、分类胶囊（占位）、Banner、瀑布流 —— 实测：页面正常渲染，点击卡片进详情
- [x] 5.4 实现商品卡片：封面、名称、价格（活动价优先 + 原价删除线）、稀有度摘要
- [x] 5.5 实现首页加载态、空态、错误态与下拉刷新 —— 实测：后端 500 时展示错误态 + 重试按钮，**无白屏**
- [x] 5.6 首页服务端降级联调 —— 实测（`docker stop es` 真停）：`GET /user/home/feed` 返回 **200** 且响应头 `X-Feed-Source: mysql`、body `source:"mysql"`，6 条盲盒 + 2 个供应商与 ES 路径同集合，端到端 0.85s；WARN 日志含触发原因（`blind_box index: ... connection refused`）与耗时。恢复 ES 后**无需清缓存**即回到 `es`（证明降级结果未入缓存）

## 6. 商品详情

- [x] 6.1 实现详情请求与类型定义（`blind_box` / `supplier` 白名单投影 / `pool` / `items` / `pool_total_stock` / `effective_price`）
- [x] 6.2 实现 `src/components/stock-bar.vue`：因后端无「初始总量」字段，改为**剩余库存的稀有度构成条**（不做假分母）；总量为 0 时条为空、CTA 置灰
- [x] 6.3 实现 `src/components/card-pool-table.vue` 概率公示表（`weight / 10000` 换算，按稀有度分组，展示权重合计自检）
- [x] 6.4 实现详情页布局与底部固定下单 CTA（展示 `effective_price`，用 `position: fixed` 而非 sticky）
- [x] 6.5 实现游客可浏览：不预检登录态，仅在点击下单时检查
- [x] 6.6 详情页真数据联调 —— 实测（`/pages/blindbox/detail?id=1`，375×812）：渲染「NBA 全明星盲盒 / 供应商 · Kayou 卡游旗舰店 / selling / ¥99.00 / 库存进度 剩余 2848 / 概率公示 10 项（SSR 50 · SR 199 · R 599 · N 2000）/ 立即拆卡」，无 JS 错误（仅 seed 占位图 `cdn.example.com` DNS 不解析）

## 7. 抽卡下单与结果

- [x] 7.1 实现 `src/api/blindbox.ts` 的 draw 请求（`{blind_box_id}`，自动携带 Authorization）
- [x] 7.2 实现下单按钮的加载态与重复点击防护（`drawing` 状态 + `ctaDisabled`）
- [x] 7.3 实现 `src/components/draw-animation.vue` 全屏开卡动画（仅下单成功后播放，动画结束 emit `finish` 由页面跳转）
- [x] 7.4 实现 `src/store/draw.ts` 结果中转（写入 → 跳转 → 结果页读取后清空；同时携带 `blindBoxId` 供「再拆一盒」）
- [x] 7.5 实现抽卡错误码映射（8 条，`src/config/draw-errors.ts`） —— **单测待补**；映射表与 `uni-h5-client` spec 逐条对齐
- [x] 7.6 实现 409 售罄后自动重新拉取详情刷新库存（`refresh-stock` 动作）
- [x] 7.7 实现 429 后下单按钮禁用 `retryAfter` 秒（倒计时展示在 CTA 上）
- [x] 7.8 实现结果页 `pages/draw/result.vue`：卡牌网格 + 稀有度边框 + 订单信息条 + 双动线按钮
- [x] 7.9 实现结果页无数据兜底：直接访问/刷新时重定向订单列表（TabBar 页用 `switchTab`）

## 8. 订单模块

- [x] 8.1 实现订单列表请求与分页（`page_size` 上限 200，与后端一致）
- [x] 8.2 实现订单列表页：状态标签分档 + 上拉加载 + 下拉刷新
- [x] 8.3 实现订单详情页（订单信息 / 支付信息 / 卡牌明细三区块，明细用 `snapshot_name` / `snapshot_image`）
- [x] 8.4 仅依赖白名单字段 —— 实测：全项目 grep `deleted_at` 无命中（仅注释提及）
- [x] 8.5 实现订单模块的加载态、空态、错误态
- [x] 8.6 订单真数据联调 —— 实测：真令牌下订单列表显示「共 5 单」，5 条订单号与 `GET /user/orders/list` 返回逐条一致（`01a13a826de87c...` / `01a13a7fd8017d...` / `01a13a7c72a979...` / `01a13a7ae86370...` / `01a13a7ad40573...`），状态「已开卡」、金额 ¥99.00、时间正确、分页尾部「没有更多了」
- [x] 8.7 订单列表缩略图 —— **已决策（2026-10-15）：不在列表页加缩略图，维持「详情页展示」现状**。`orders/list` 不返回卡牌明细，列表页做缩略图需 N+1 请求，收益不抵成本

## 9. 我的页面

- [x] 9.1 实现「我的」页：用户信息 + 余额卡 + 菜单；余额走 `/user/profile` 而非登录快照
- [x] 9.2 实现退出登录（调用 `POST /user/logout` + 清本地存储 + 跳首页；登出接口幂等，失败不阻断本地清理）
- [x] 9.3 确认页面内无「联系客服」入口 —— 实测：全项目 grep `客服` / `kefu` 无命中（仅注释提及）

## 10. 联调与验收

- [x] 10.1 端到端跑通主链路 —— 实测（真库 + 真 ES）：登录页（手机号 + 滑块 + 验证码 000000）→ `applyDevToken` 注入真令牌到 `uh5:token` → 详情页 → 点「立即拆卡」→ 跳 `/pages/draw/result`，显示订单号 `01a13a826de87c56a6be0551ba979fc5`、实付 ¥99.00、卡牌「Jokic 15 MVP 赛季卡（N）」
- [x] 10.2 验证抽卡真实扣库存 —— 实测：NBA 全明星盲盒卡池合计 2848 → 2843（抽 5 次，其中 N 3 次 / R 2 次），各档 stock 精确递减；余额 500 → 5.00（−99×5）
- [x] 10.3 验证 402 余额不足路径 —— 实测：余额 5.00 时抽 ¥99 盲盒返回 `402 {"code":"draw.insufficient_balance"}`
- [x] 10.4 验证「我的」页余额下单后刷新 —— **通过（2026-10-14 二次验收）**：后端已实现 `GET /user/profile`（`internal/handler/user/profile.go`，复用登录响应的 `userInfo` DTO 保证同构），前端同步从 mock 注册表移除该路由（mock 返回登录快照，与「取最新余额」的目的直接矛盾）。真库证据：① 差分测试 —— 预先种入假快照 `balance=2000.00 / nickname=FAKE`，页面加载后 store 被覆写为真库值 `876.00 / H5 联调账号` 并渲染 `¥876.00`；② 网络层 `performance.getEntriesByType('resource')` 出现真实请求 `http://localhost:5176/api/user/profile`（证明未被 mock 拦截）；③ 活性验证 —— 抽卡（¥129）前后各查一次，`1005.00 → 876.00` 精确递减，与 `users` 表直查结果一致；④ 单测 5 例（含「与登录响应 user 同构」的结构锁定测试）
- [x] 10.5 验证视觉令牌合规 —— 实测：grep 十六进制色值仅命中 `tokens.scss`（例外已登记）
- [x] 10.6 验证稀有度配色三处一致 —— **通过（代码级比对，2026-10-14）**。先修正一处任务口径错误：spec（`uni-h5-client`「视觉令牌与稀有度配色」）要求的三处是**结果页 / 详情页卡池预览 / 订单详情**，本任务此前误写为「首页卡片」——首页卡片只渲染 `rarity_summary` 文本标签，spec 从未要求它配色。实测证据：① 四档稀有度色值 `#ffd479 / #b98cff / #5aa9ff / #8b94a6` 全项目 grep **仅出现在 `src/styles/tokens.scss`**（唯一真源）；② 三处消费点 `draw/result.vue`、`card-pool-table.vue`、`orders/detail.vue` 全部经 `src/utils/rarity.ts` 的 `rarityColor()` → CSS 变量 `--ssr/--sr/--r/--n` 取色，无任何一处直接写色值；③ 另有两处第 4 消费点（`stock-bar.vue`、订单详情边框）同源。已知的两处非稀有度色值字面量（`mock/slider.ts` 的 SVG data URI、`pages.json` 的 tabBar 构建期配置）见 14.2 —— 均为平台强制，与稀有度配色无关
- [x] 10.7 构建产物与移动端视口验收 —— 实测：`build:h5` 通过；375×812 视口下首页 / 登录页无横向滚动、底部 CTA 位置正确
- [x] 10.8 确认无范围外功能残留 —— 实测：`pages.json` 仅 7 页，无秒杀/关注/搜索/充值/地址/卡册/客服

## 11. 后端：首页 Feed MySQL 降级

> 本组是本期唯一的后端改动。改动文件：
> `internal/repository/mall/feed_repository.go`（新增降级数据源）、
> `internal/service/user/home.go`（降级分支 + 短超时 + 来源标识）、
> `internal/handler/user/home.go`（`X-Feed-Source` 响应头）、
> `internal/router/user/user.go`（装配 `FallbackRepo`）、
> `internal/middleware/cors.go`（暴露响应头，见文末「联调实录」）。
> 单测：`internal/service/user/home_fallback_test.go`（13 例）、
> `internal/handler/user/home_test.go`（+3 例）。
>
> **已完成并真库验证**（2026-10-14）。

- [x] 11.1 在 repository 层新增 Feed 的 MySQL 查询方法（盲盒 + 供应商两条） —— 实测：`FeedBlindBoxes` / `FeedRarityWeights` / `FeedActivePromoPrices` / `FeedSuppliers` 四条只读查询落在 `internal/repository/mall/feed_repository.go`，过滤条件与 `cmd/es-sync` 逐字一致（盲盒 `status=active AND on_sale=true AND lifecycle='selling'` + 供应商 `status=active`；**不筛 `is_featured`**，与 `add-batch-settlement` 后的现行口径一致）；真库验证 6 条盲盒 / 2 个供应商，与 ES 路径同集合
- [x] 11.2 在 `loadFeed` 的调用链中接入降级分支 —— 实测：`TestFeedFallback_ESError_UsesMySQL`、`TestFeedFallback_HotspotColdStart_ESError_UsesMySQL` 通过；降级分支位于 hotspot 与 TTL 两条缓存路径的 ES 失败出口
- [x] 11.3 保证降级结果与主路径 DTO 完全同构 —— 实测：`TestFeedFallback_DTOShapeMatchesESPath` 逐层比较顶层 / `blind_boxes[0]` / `suppliers[0]` 的 JSON key 集合，完全一致；真库对拍两条路径均返回 6 盲盒 + 2 供应商
- [x] 11.4 处理降级路径的 `promo_price` —— 实测：`TestFeedFallback_PromoPrice_NilWhenUnknown`（查不到 → 省略该字段、`price` 仍为 99.00）、`TestFeedFallback_PromoPrice_FilledWhenKnown`（确有生效活动 → 填真实值）；代码中无任何估算或默认填充分支。真库当前无 active 活动，实测 6 条全部无 `promo_price` 且原价正确
- [x] 11.5 约束降级结果的缓存回填 —— 实测：`TestFeedFallback_DoesNotWriteCache`（`cache.Set` 调用 0 次）通过；真库复现「停 ES 期间连续两次请求均为 `mysql`」（若写过缓存第二次会命中并返回 `es`），ES 恢复后**无需清缓存**即刻回到 `es`
- [x] 11.6 增加降级可观测性 —— 实测日志：`level=WARN msg="user.home: feed degraded to mysql" trigger="blind_box index: ... connection refused" blind_boxes=6 suppliers=2 elapsed_ms=44 source=mysql`；响应头 `X-Feed-Source: mysql`（主路径为 `es`），并在 `cors.go` 的 `ExposeHeaders` 中暴露以支持跨域读取
- [x] 11.7 为主检索路径配置短超时（800 毫秒量级） —— 实测：`esSearchTimeout = 800ms` 套在两条索引查询上；`TestFeedFallback_ESTimeout_DegradesFast` 用 2s 假死 ES 验证在 ~1.6s（2×800ms）内转入降级
- [x] 11.8 补降级路径单测 —— 实测：13 例全通过，覆盖三组必需用例（① ES error → 走 MySQL 且结构一致；② ES 0 条 → 不降级且 `FallbackRepo` 零调用；③ 双层失败 → `ErrFeedSearchUnavailable`），另覆盖 hotspot 冷启动、`promo_price`、缓存约束、超时、rarity_summary 算法对齐
- [x] 11.9 同步修订 `add-trading-card-blindbox-mvp` 中「ES 不可用时不查 MySQL 兜底」的表述 —— 实测：该 change 的 spec 中「Home feed via Elasticsearch」已改为「仅当 ES 与 MySQL 降级查询**均**失败才 500」，Scenario 由 `ES unavailable fails fast` 改为 `ES unavailable degrades to MySQL`；同时把盲盒收录口径更新为 `lifecycle='selling'`（旧文案的「不 fallback」「is_featured=true 门槛」已清除）
- [x] 11.10 端到端验证降级：停掉 ES 后访问首页接口 —— 实测：`docker stop es` 后 `GET /user/home/feed` → **200** + `X-Feed-Source: mysql` + 6 条商品（0.85s）；`docker start es` 后自动回到 `es`

## 12. 数据库：users.register_source

> 用户决策：字段**本期就加**。本组只落「表结构 + model 字段」，**不含写入逻辑** —— 写入依赖 `POST /user/login/mobile` 的服务端实现，属后续 change。
>
> 关键约束：新迁移 MUST 用 `migrate create` 生成（`db-migration` spec 明确禁止手敲文件名）。
>
> **实测结果（2026-10-14，真库 `ai_go_mall_local`）**：本地库原版本 15 → up 到 **17**（连带应用 16）；`down 1` 回 16 → `up` 再回 17，`dirty` 始终 `false`。列实测为 `varchar(16) NULLABLE=YES`、位置 13、注释正确；存量用户 `COUNT(register_source)` = **0**（未回填，符合预期）。`go build ./...` 通过。

- [x] 12.1 用 `migrate create users_register_source` 生成迁移文件 —— 验证：生成 `000017_users_register_source.up.sql` 与 `.down.sql`，版本号为目录最大版本 +1 且保留 6 位前导零
- [x] 12.2 编写 up 迁移：`ALTER TABLE users ADD COLUMN register_source varchar(16) DEFAULT NULL`，追加在 `password` 之后 —— 验证：`migrate up` 成功且 `schema_migrations.version` 变为 17
- [x] 12.3 编写 down 迁移：`ALTER TABLE users DROP COLUMN register_source` —— 验证：`migrate down 1` 后该列消失且版本号回到 16
- [x] 12.4 同步 model 字段 `User.RegisterSource *string`（`size:16`，可空） —— 验证：`go build ./...` 通过，且 GORM 列名映射为 `register_source`
- [x] 12.5 真库验证迁移可重入：up 到 17 → down 到 16 → 再 up 到 17 —— 验证：三轮均成功、`dirty` 始终为 false
- [x] 12.6 验证存量数据未被回填 —— 验证：迁移后 `SELECT COUNT(*) FROM users WHERE register_source IS NOT NULL` 返回 0

## 13. 开发令牌工具（新增）

> 4.7 方案修订的配套产物。**已完成**。

- [x] 13.1 新增后端开发命令 `cmd/devtoken` —— 实测：`go run ./cmd/devtoken -username h5dev -topup 500` 自动建号（id=2，status=1，`register_source` 按边界留 NULL）→ 签发 `type=user` 真实令牌（uuid v7 明文，库内只落 SHA256）→ 打印 `VITE_DEV_TOKEN=...`；该令牌实测通过 `UserAuth()` 调 `/user/orders/list` 返回 200（无令牌为 401）
- [x] 13.2 用产出的 token 配置 `uni-h5/.env.local` 并重启 —— 实测：`VITE_DEV_TOKEN` 注入 → mock 登录成功后 `applyDevToken` 把真令牌写入 `uh5:token` → 抽卡真实扣库存（2848→2843）、真出订单（`/pages/draw/result` 显示订单号，订单列表可见 5 单）、真实扣款（500→5.00）

## 14. 联调实录与遗留

> 本节记录 2026-10-14 首次全链路真库联调中发现的问题与结论，供归档时一并核对。

### 14.1 已修复：CORS 白名单缺 uni-h5 端口导致 POST 全 403

- **现象**：浏览器点「立即拆卡」→ 跳登录页；抓包为 `POST /api/user/blindbox/draw → 403`，
  且**响应体为空**。直连后端（不带 `Origin`）同一令牌同一请求返回 200，极易误判为鉴权故障。
- **根因**：浏览器对**非简单请求**（POST + `application/json`）即使同源也会带 `Origin` 头；
  经 vite 代理转发时该头被保留 → `gin-contrib/cors` 白名单（原仅 1818/1819）未命中 →
  直接 `AbortWithStatus(403)` 且不写 body。GET 请求不带 `Origin`，因此首页数据正常，
  掩盖了问题。
- **修复**：
  1. `.env.yaml` / `.env.yaml.example` 的 `cors.allow_origins` 增加
     `http://localhost:5176` 与 `http://127.0.0.1:5176`（uni-h5 dev server）；
  2. `uni-h5/vite.config.ts` 设 `strictPort: true`，端口被占用时直接报错而不是静默漂移
     （否则换端口会让白名单再次失效）；
  3. `internal/middleware/cors.go` 增加 `ExposeHeaders: [X-Feed-Source, Retry-After]`，
     让排障头与限流重试头在跨域下可被前端读取。
- **验证**：修复后经代理带 `Origin` 的 POST 返回 200，响应含
  `access-control-allow-origin: http://localhost:5176`。

> **联调提示（清 Feed 缓存）**：hotspot 是永驻缓存（TTL=0 + 逻辑过期 60s），改动 Feed
> 投影逻辑后需显式删除 `hotspot:feed:home`（热点）与 `home:feed:v1`（TTL 缓存）才能观察到
> 新行为，可用 `redis-cli -a <password> DEL hotspot:feed:home home:feed:v1`。
> 注意：**降级结果从不写缓存**，因此验证 ES 恢复时无需清缓存。

### 14.2 已知边界（非缺陷）

| 项 | 说明 |
| --- | --- |
| 商品图片 | seed 数据用 `cdn.example.com` 占位域名，浏览器 DNS 不解析 → 图片 404（非前端缺陷） |
| 订单列表缩略图 | `orders/list` 不返回卡牌明细，缩略图放在详情页（8.7 已决策：列表页不加，避免 N+1） |
| 降级排序 tie-break | `created_at` 完全相同时（seed 同批插入），MySQL 路径顺序与 ES `_doc` 顺序可能不同；不属 spec 的「结构同构」约束（该约束针对字段集合），且 ES `_doc` 顺序本身不稳定 |
| hotspot 命中期间的降级 | ES 故障时若 hotspot 已有 Feed 缓存，会返回缓存值（客户端仍拿到有效数据）；仅冷启动失败才走 MySQL。降级结果不入 hotspot，故 ES 恢复后无需清缓存即可自愈 |
| `mock/slider.ts` 的色值字面量 | 滑块图用内联 SVG 经 `toDataUri()` 生成 **data URI**，SVG 在隔离上下文渲染、**无法解析文档的 CSS 自定义属性**，硬编码是技术必然；且其中不含任何稀有度色。`tokens.scss` 头注释「全项目唯一允许」已改为限定「页面与组件样式」口径 |
| `pages.json` 的 tabBar 色值 | tabBar 颜色由 uni-app **构建期 JSON** 消费，JSON 无法引用 CSS 变量；同理不涉及稀有度色 |

### 14.3 仍未完成的验收项

- （无）—— 8.7 已于 2026-10-15 决策：列表页不加缩略图，维持详情页展示

### 14.4 修订记录

- 2026-10-14（三）：**登录页移除滑块校验**（产品决策，仅登录页，滑块能力保留备用）。
  代码：`pages/login/index.vue` 删滑块组件 / `captcha` 门控 / 提交体 `captcha_*` 字段；
  `mock/login.ts` 删「`captcha_key` 缺失 → 401」分支（否则登录必然失败）；
  `MobileLoginRequest` 的 `captcha_*` 改为预留可选字段。spec 同步：
  `uni-h5-client` 页面清单表；`user-mobile-login` 的「手机号登录接口 / 登录校验顺序与错误码」
  （校验链改为 ① 格式 ② 短信码 ③ 用户状态，新增「不携带滑块字段仍可登录」场景，
  `401 login.invalid_captcha` 本期 MUST NOT 出现）；`slider-captcha` 的「消费型二次校验」
  标注为重新接入后的契约、「客户端滑块组件」新增「登录页不接入」场景。
  实测：浏览器跑通「手机号 + 000000 → 登录 → 跳首页」，页面 `hasSlider=false`、仅 2 个输入框。
- 2026-10-14（二）：后端补齐 `GET /user/profile`（原属「后续 change」），前端将其移出 mock
  注册表（5 条 → 4 条），`uni-h5-client` 的 mock 边界口径同步修订并**新增独立需求**
  「余额接口不参与 mock」；`user-mobile-login` 的「当前用户信息接口」补 2 个 Scenario
  （用户已不存在 → 401、读取失败 → 500）。10.4 与 10.6 据此由「部分通过 / 待人工比对」转为通过。
- `add-trading-card-blindbox-mvp`：修订「ES 不可用 → 500 不 fallback」→「双层失败才 500」，
  并同步盲盒收录口径为 `lifecycle='selling'`（对应 11.9）。
- `add-uni-h5-client`：`home-feed-fallback` 的口径表述修正为与 `cmd/es-sync` 逐字一致
  （去掉 `is_featured` 门槛），`openspec validate` 零 warning 通过。

## 15. 移除 i18n（新增）

产品决策：本期 C 端只有简体中文，多语言是纯粹的预留，i18n 层「改一句文案要跨两个文件」的成本大于收益。原设计中的 `en` 语言资源自始至终未被实现，仅 `zh-cn` 一份在跑。决策与理由见 design.md **D14**。

- [x] 15.1 删除 `src/lang/` 整个目录（`index.ts` + `zh-cn/index.yaml`），撤销 `main.ts` 的 `app.use(i18n)`
- [x] 15.2 `package.json` 移除 `vue-i18n` 与 `yaml` 两个依赖 —— 实测：构建产物 `dist/build/h5/assets/index-*.js` 中 `vue-i18n` / `createI18n` 出现 0 次（其余 `$t(` 命中经核为 Vue 响应式内核的压缩函数名 `function $t(e,t,n)`，非 i18n）
- [x] 15.3 内联全部界面文案：7 个页面（首页 / 商品详情 / 开卡结果 / 订单列表 / 订单详情 / 登录 / 我的）+ 2 个组件（`draw-animation`、`slider-captcha`）—— 实测：全仓库 `$t(`、`translate`、`@/lang` 残留均为 0
- [x] 15.4 处理 3 处动态 key（这是本项唯一的真实风险点）：
  - `config/draw-errors.ts`：8 条抽卡错误码规则的字段由 `messageKey`（i18n key）改为 `message`（中文字面量）；`blindbox/detail.vue` 的 `t(rule.messageKey)` → `rule.message`
  - 订单状态：`orders/index.vue` 与 `orders/detail.vue` 的 `STATUS_KEYS` → `STATUS_TEXT`（值直接为中文）
  - 下单来源：`orders/detail.vue` 的 `SOURCE_KEYS` → `SOURCE_TEXT`
  - 插值：`orders.count` 的 `共 {n} 单` → 模板内 `共 {{ total }} 单`
- [x] 15.5 同步 spec：`uni-h5-client` 的「国际化键约束」需求（原 MUST 文案走 i18n 键、禁止中文硬编码）**替换为「文案内联」**，新增 2 个 Scenario（无词条映射层 / 枚举值文案集中映射）；`uni-h5-client` 依赖隔离段与 `proposal.md`、`design.md` D1 中的 `vue-i18n` 冲突项一并删除
- [x] 15.6 回归：`npx uni build` EXIT=0、`DONE Build complete.`；openspec `validate` 通过

**验证踩坑**：删 `src/lang/` 后 Vite 模块图陈旧，dev server 页面报模块解析错误 —— 必须**重启 dev server**（本项目启动命令见第 14 组），不能只靠 HMR。

**未做**：`node_modules` 中残留的 `vue-i18n` / `yaml` 包目录未手动删除（已不在 `package.json` 依赖树中，下次干净安装自然消失）。uni-app 框架自带的 `@dcloudio/uni-i18n` 仍在（框架内部使用，非本应用文案层，不可移除）。

## 16. 收尾决策与 store 修复（新增，2026-10-15）

- [x] 16.1 8.7 产品决策落地：订单列表**不加**卡牌缩略图，维持「详情页展示」现状（`orders/list` 不返回卡牌明细，列表页做缩略图需 N+1 请求）；spec 无需改动（原 spec 未对列表页提出缩略图要求），仅同步本清单与 14.3
- [x] 16.2 修复 `src/store/user.ts` `patchUser()` 早退缺口：原实现 `if (!state.user) return` 导致「仅有 token、无用户快照」场景（联调时只注入 `VITE_DEV_TOKEN` 而未种用户缓存）下拉到的真实 `/user/profile` 被静默丢弃，「我的」页始终显示「未登录 / ¥0.00」。修复为：无快照时以本次数据建档（`id` 缺失才忽略；`username`/`nickname`/`avatar`/`balance`/`status` 缺省给兜底值），有快照时维持原合并语义
- [x] 16.3 回归验证：`npx uni build` 构建通过、`openspec validate add-uni-h5-client` 零 warning
