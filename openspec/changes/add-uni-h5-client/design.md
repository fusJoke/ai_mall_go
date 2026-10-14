# Design: uni-app H5 客户端

## Context

C 端后端能力已由 `add-trading-card-blindbox-mvp` 落地完毕，接口与数据表齐备。本 change 只做客户端。核心约束：**仅 H5**、**仅首页到下单**、**登录先 mock**、**与 `web/` 物理隔离**。

---

## D1 工程隔离：为什么必须独立

现有 `web/` 与 uni-app 官方模板的依赖区间存在**不可调和的冲突**：

| 依赖 | `web/` 现状 | uni-app 官方模板 | 冲突 |
|---|---|---|---|
| Vite | 8.0.10 | 5.2.8 | 主版本差 3 |
| TypeScript | 6.0.3 | 4.9.4 | 主版本差 2 |
| Vue | 3.5.33 | ^3.4.21 | 兼容 |

> 官方模板另含 `vue-i18n 9.1.9`，本期客户端不使用国际化，已不引入该依赖（见 D14）。

`@dcloudio/*` 的构建插件强绑定 Vite 5 的插件 API（`enforce`、`transformIndexHtml` 钩子签名在 Vite 6+ 有破坏性变更），在 Vite 8 下无法运行。因此：

- **不采用 monorepo / workspace**：workspace 会提升依赖到根 `node_modules`，恰好制造版本冲突。
- **不采用路径别名引用**：`uni-h5` 不 `import` 任何 `web/src/**`。
- **唯一共享的是 API 契约**，以文档形式（`docs/接口与数据契约.md`）而非代码形式共享。

`@dcloudio/*` 版本号形如 `3.0.0-5020420260813003`（日期串），**必须写精确版本**。该包在 npm 上同时存在 `latest`（Vue2 线 `2.0.2-*`）与 `vue3` tag（`3.0.0-alpha-*`），使用 `^` 前缀有被解析到 alpha 线的风险。

---

## D2 目录结构

```
uni-h5/
├── package.json              # 独立依赖树，@dcloudio/* 精确版本
├── vite.config.ts
├── tsconfig.json
├── index.html
├── manifest.json             # 仅填 h5 节点
├── pages.json                # 声明式路由 + TabBar
├── src/
│   ├── main.ts               # createSSRApp
│   ├── App.vue
│   ├── pages/
│   │   ├── home/index.vue
│   │   ├── blindbox/detail.vue
│   │   ├── draw/result.vue
│   │   ├── orders/index.vue
│   │   ├── orders/detail.vue
│   │   ├── login/index.vue
│   │   └── mine/index.vue
│   ├── components/
│   │   ├── waterfall-list.vue
│   │   ├── card-pool-table.vue
│   │   ├── stock-bar.vue
│   │   ├── draw-animation.vue
│   │   └── slider-captcha.vue
│   ├── api/
│   │   ├── auth.ts
│   │   ├── blindbox.ts
│   │   ├── order.ts
│   │   └── profile.ts
│   ├── mock/
│   │   ├── index.ts
│   │   ├── sms.ts
│   │   ├── slider.ts
│   │   └── login.ts
│   ├── store/
│   │   ├── user.ts
│   │   └── draw.ts
│   ├── utils/
│   │   ├── request.ts
│   │   ├── storage.ts
│   │   └── format.ts
│   ├── config/index.ts       # baseURL / USE_MOCK
│   ├── styles/
│   │   ├── tokens.scss
│   │   └── common.scss
└── docs/                     # 需求、原型、契约
```

`pages.json` 中 `pages` 第一项必须是 `pages/home/index`，作为启动页。

原设计中的 `src/lang/`（含 `zh-cn` 与 `en` 两份语言资源）已于本期删除 —— 见 D14。

---

## D3 请求层设计

`uni.request` 是唯一跨端可用的网络 API（axios 依赖 `XMLHttpRequest`，小程序端无此对象）。封装要点：

```ts
// 伪代码，示意职责
async function request<T>(options): Promise<T> {
  const token = storage.get('user_token')
  const res = await uni.request({
    url: BASE_URL + options.url,
    header: token ? { Authorization: `Bearer ${token}` } : {},
    ...
  })
  // 401 → 清 token + 跳登录（带 redirect）
  // 429 → 提示 + 抛出带 retryAfter 的错误
  // 2xx → 直接返回裸 JSON
  // 其他 → 抛 { http, code, message }
}
```

**关键设计**：

1. **默认 resolve 裸 JSON**。后端成功响应绝大多数不带 `{code,message,data}` 信封，请求层不做拆包，业务层直接拿到 DTO。
2. **`logout` / `sms` 是例外**，返回 `{code,message}`，这两个接口的调用方自行处理。
3. **错误统一为结构化异常**，携带 `{ http, code, message }`，业务层按 `code` 分支，不解析字符串。
4. **401 拦截集中在一处**，避免每个页面重复写跳登录逻辑。
5. **`redirect` 只接受站内路径**，防开放重定向。

---

## D4 Mock 层设计

```ts
// config/index.ts
export const USE_MOCK = true
```

`src/mock/index.ts` 导出一个 `mockAdapter(url, options)`，在 `request.ts` 中于真正发起 `uni.request` 之前做一次匹配：命中则直接返回构造好的响应，未命中则透传。

**覆盖范围（只覆盖 UI，严格限定）**：

| 接口 | mock 行为 | 性质 |
|---|---|---|
| `POST /user/sms/send` | 校验手机号正则，返回成功；`console.log` 打印固定码 `000000` | 登录页 UI |
| `GET /common/captcha/slider` | 返回内置 base64 图 + 随机 key | 登录页 UI |
| `POST /common/captcha/slider/verify` | 无条件通过 | 登录页 UI |
| `POST /user/login/mobile` | 校验 `sms_code === '000000'`，返回假 user + 假 token | 登录页 UI |
| `GET /user/profile` | 返回登录时缓存的 user | 登录页 UI |

**其余接口一律走真实后端**（feed / detail / draw / orders）。

**边界原则**：mock 的存在只为让「登录页」这一屏在服务端接口未实现时可用；**业务数据一律真实**。判据是——mock 若被误开在生产，影响的只能是登录页交互，绝不会让商品或订单数据失真。

**关于 token 的取舍**：mock 产生的假 token 与后端数据库中的任何记录都不匹配，无法通过 `UserAuth()` 中间件。这会导致「登录成功但抽卡 401」。

采用的处理方式：**mock 模式下不注入 Authorization 头绕过，而是用真实方式取 token** —— 登录页在 mock 校验通过后，后台静默调用真实的 `POST /user/login`（用户名 + 密码 + 点选验证码），用一个预置的测试账号换取**真 token**，同时用 mock 的 user 数据做界面展示。

这样做的好处：抽卡会**真实扣库存、真实产生订单**，主链路是端到端可验证的，而不是演示性质的假数据流。

---

## D5 瀑布流：为什么不用虚拟列表

**结论：原生 `<scroll-view>` 双列瀑布流。**

| 维度 | 原生双列 | 虚拟列表 |
|---|---|---|
| 适用规模 | ≤ 数千节点 | 1 万+ 节点 |
| 本场景实际规模 | 一次 feed 返回，量级数十到一两百卡 | 相同 |
| 卡片高度 | 动态（封面比例 + 标题 1~2 行 + 价格行） | 动态高度虚拟列表实现复杂度高 |
| uni-app H5 端已知问题 | 无 | 滚动位置错乱、图片复用闪烁 |
| 首屏性能 | 足够（配合懒加载） | 优势体现在大列表滚动帧率 |

**风险对冲**：封装为 `<waterfall-list>`，对外只暴露 `:list` 与 `@load-more`。若后续商品量级上来，替换组件内部实现即可，调用方无需改动。

**升级触发信号**：单次渲染卡片稳定超过 500，或中低端安卓机滚动出现可感知掉帧。

**实现要点**：用左右两列各自维护一个数组，新数据按当前两列累计高度（用 `uni.createSelectorQuery` 或预估高度）分派到较矮一侧，避免整体重排。

---

## D6 登录链路时序

```
用户点击「获取验证码」
  ├─ 校验手机号格式（前端正则）
  ├─ POST /user/sms/send { mobile, scene: 'login' }
  └─ 进入 60 秒倒计时，按钮禁用

用户拖动滑块
  ├─ GET /common/captcha/slider  → { key, bg_image, slider_image, y }
  ├─ 拖动松手
  └─ POST /common/captcha/slider/verify { key, x, w, h }
       ├─ 通过 → 标记 captchaReady，保留 key 与 x
       └─ 失败 → 复位滑块；连续 3 次失败 → 重新创建挑战

用户点击「登录」
  ├─ POST /user/login/mobile { mobile, sms_code, captcha_key, captcha_x, remember }
  │    ├─ 200 → 存 token + user → 回跳 redirect（或首页）
  │    ├─ 401 login.invalid_captcha → 重置滑块，提示重新验证
  │    ├─ 401 login.invalid_sms_code → 提示验证码错误（不重置滑块）
  │    └─ 403 login.account_disabled → 提示账号封禁
  └─ mock 模式下：先本地校验 sms_code，再静默调真实 /user/login 换真 token
```

**首登自动建号**（已确认采纳）：手机号未注册时，服务端在同一事务内建号 —— `username` = 手机号、`password` = `''`、`balance` = 0、`status` = 1。以 `users.mobile` 唯一索引作幂等保证，并发重复建号由唯一约束拦截且 MUST NOT 导致登录失败。

**回跳规则**：`redirect` 参数只接受以 `/` 开头的站内路径，其余一律回退首页。

---

## D7 状态管理

不引入 Pinia（uni-app 对 Vuex/Pinia 的支持需要额外配置，而本期状态极少）。使用轻量方案：

- `store/user.ts`：`ref` 持有 `token` 与 `user`，读写同步到 `uni.setStorageSync`
- `store/draw.ts`：抽卡结果的中转，`set(cards) → 跳结果页 → 结果页 take() 并清空`

抽卡结果传递复用现有 `user-draw-result` spec 的思路（先写入存储再跳转、读取后清除、无数据兜底重定向），但存储 API 换为 `uni.setStorageSync` / `getStorageSync`（跨端可用），键名独立，不与 Web 版共用。

---

## D8 视觉令牌

```scss
// styles/tokens.scss
--bg:        #0b0d12;   // 页面底色
--surface:   #161a22;   // 卡片面
--surface-2: #1d222c;   // 卡片面（高一层）
--text:      #e9ecf2;   // 主文字
--muted:     #8b94a6;   // 次要文字
--dim:       #5d6678;   // 弱化文字
--gold:      #f5b544;   // 主强调（价格、CTA）
--gold-2:    #ffd479;   // 强调渐亮

// 稀有度四档
--ssr: #ffd479;
--sr:  #b98cff;
--r:   #5aa9ff;
--n:   #8b94a6;
```

稀有度配色必须在**三处保持一致**：抽卡结果页、详情页卡池预览、订单详情卡牌明细。集中定义在 `tokens.scss`，页面只引用变量。

---

## D9 错误处理矩阵

| 场景 | 表现 |
|---|---|
| 网络不可达 | Toast「网络异常，请稍后重试」+ 页面可下拉重试 |
| 401（任意受保护接口） | 清 token → 跳登录页（带 redirect） |
| 429 | Toast「操作过于频繁」+ 触发按钮禁用 3 秒 |
| 402 余额不足 | Modal 确认框 → 「去充值/去我的」 |
| 404 商品不存在 | Toast + 返回首页 |
| 409 售罄 | Toast + 重新拉取详情刷新库存 |
| 5xx | Toast「服务异常」+ 保留当前页，允许重试 |

原则：**不留白屏**。任何接口失败都要有可见反馈与出路。

---

## D10 数据表依赖

本期**新增 1 个迁移（1 列）、不修改任何既有列**。链路涉及 8 张表：

| 表 | 读/写 | H5 用途 |
|---|---|---|
| `users` | 读/写 | 登录、余额展示、下单前余额校验；建号时写 `register_source`（写入逻辑属后续 change） |
| `mall_blind_boxes` | 读 | 首页卡片、详情页 |
| `mall_suppliers` | 读 | 详情页供应商信息 |
| `mall_card_pools` | 读 | 详情页卡池 |
| `mall_card_pool_items` | 读 | 概率公示、库存进度 |
| `mall_draw_orders` | 读 | 订单列表、订单详情 |
| `mall_draw_order_items` | 读 | 订单卡牌明细、结果页 |
| `tokens` | 读/写 | 登录签发、鉴权校验 |

注意：用户表名是 `users`（非 `mall_users`），迁移文件名 `000006_mall_users` 内部执行的是 `ALTER TABLE users`。

**首登建号的来源字段（用户决策：本期就加）**：`users.register_source`（`varchar(16) NULL`）由迁移 `000017_users_register_source` 引入。

| 决策点 | 取值 | 理由 |
|---|---|---|
| 生成方式 | `migrate create users_register_source` | `db-migration` spec 明确要求新迁移 MUST 用 CLI 生成，不允许手敲文件名 |
| 列位置 | `AFTER password`（表尾） | 不打扰既有列顺序与 ORM 字段假设，最小侵入 |
| 可空性 | `DEFAULT NULL`，不加 `NOT NULL` | 存量用户来源不可考，`NULL` 语义为「未知或历史数据」，比回填猜测值更诚实 |
| 存量回填 | **不做** | 回填属数据决策且无可靠判据；本期只保证字段存在 |
| 取值 | `h5_mobile` / `admin` / `import` | 长度 16 留有余量；不设 ENUM 或 CHECK，避免新增渠道时再改表 |
| 索引 | 暂不建 | 仅用于后台低频统计；统计查询变频繁后再补 `idx_users_register_source` |

**边界（重要）**：本期只落**表结构与 model 字段**，**不含写入逻辑**。建号时写 `h5_mobile` 依赖 `POST /user/login/mobile` 的服务端实现，属后续 change。本期结束时该列在所有库上都应为全 NULL —— 这是预期状态，不是缺陷。

---

## D11 与既有 openspec 能力的关系

| 既有能力 | 关系 | 处理 |
|---|---|---|
| `add-trading-card-blindbox-mvp` | **前置依赖** | 消费其接口与表，不做改动 |
| `user-draw-result` | **并存** | 该 spec 描述 Vue3 PC Web 版结果页。H5 版行为写入 `uni-h5-client`，不修改主 spec，避免 Web 契约被 H5 实现覆盖 |
| `click-captcha` | **并存** | 点选式（后台用）与滑块式（C 端用）是两种独立能力，`key` 空间隔离 |
| `homepage` | **无冲突** | 该能力是营销门面页 `/`，引导进后台；C 端商品首页是 `/user/home` |
| `upload` / `ag-upload` | **无关** | 本期无上传需求 |

`tokens` 表共享靠 `type` 字段隔离（`user` / `admin` / `supplier`），已在既有实现中验证。

---

## D12 已知风险

| 风险 | 影响 | 缓解 |
|---|---|---|
| `GET /user/home/feed` 强依赖 ES，不可用时返回 500 且无 MySQL 降级 | 首页白屏 | **本 change 内解决**：新增 MySQL 降级通道，见 D13 |
| mock token 无法通过 `UserAuth()` | 抽卡 401 | mock 模式下静默调真实 `/user/login` 换真 token（见 D4） |
| `orders/list` 与 `orders/detail` 裸返回 GORM 模型 | 暴露 `deleted_at` 等内部字段 | H5 侧只取白名单字段，不依赖内部字段 |
| uni-app H5 端在部分安卓 WebView 的 `scroll-view` 吸顶抖动 | 详情页底部 CTA 位置异常 | 用 `position: fixed` 而非 `sticky` |

---

## D13 首页 Feed 的 MySQL 降级

### 现状

`internal/service/user/home.go` 的 `loadFeed` 调两次 ES（`mall_blind_box_index` + `mall_supplier_index`），任一失败即 `wrapSearchErr` 包装 `ErrFeedSearchUnavailable`，由 handler 映射为 `500 search_unavailable`。文件头注释曾明确写着「不 fallback 到 MySQL（spec 明确）」——**该注释与 spec 表述已在本次改造中一并修订**（见下方「降级触发条件」与 capability `home-feed-fallback`）。

**这是当前线上的白屏隐患**，不只是 H5 的问题：ES 抖动或索引重建期间，PC Web 首页同样白屏。

### 降级触发条件

| ES 结果 | 行为 |
|---|---|
| 返回错误（连接失败 / 超时 / 5xx） | **触发降级** |
| 返回 0 条 | **不降级** —— 0 条是合法业务结果，`Feed` 返回空数组 |
| 反序列化失败（schema 漂移） | **触发降级** —— 属于 ES 侧不可用，不是单条数据问题 |

### DTO 同构约束（硬要求）

降级响应 MUST 与 ES 路径**结构完全一致**：

- `BlindBoxFeedItem` 的 10 个字段、`SupplierFeedItem` 的 5 个字段、`Feed` 的 3 个字段全部保留
- 客户端 MUST NOT 需要感知 `X-Feed-Source` 响应头才能正确渲染
- `fetched_at` MUST 为降级时刻的时间戳

### 活动价字段的处理

ES 文档中的 `promo_price` 是 `cmd/es-sync` 预计算写库的，MySQL 降级路径没有现成值。约定：

- MUST 尽力填充；无法确定时 MUST 置为 `null`
- **MUST NOT 返回未经校验的错误价格** —— 前端对 `null` 有成熟处理（展示原价），而显示错误价格是资损风险

### 缓存回填约束

ES 路径命中后会回填 `home:feed:v1`（或 hotspot）。降级结果：

- MUST NOT 按正常 TTL 回填
- 允许两种实现：① 完全不回填；② 回填但使用显著更短的 TTL（≤ 30 秒）
- 原因：若降级数据按正常 TTL 驻留，**ES 恢复后用户仍会长时间看到降级结果**

### 超时

ES 查询 MUST 设置短超时（建议 800ms 量级），避免降级前长时间阻塞用户请求。降级路径的 MySQL 查询 MUST NOT 再引入长等待。

### 可观测性

- 降级 MUST 记录 WARN 日志（含触发原因、索引名、耗时）
- 响应头 MUST 携带 `X-Feed-Source: es | mysql`，供排障与灰度观察
- **MUST NOT 静默降级** —— 否则 ES 长期故障会被掩盖成「首页一直正常」

### 双层兜底后的最终失败

MySQL 也失败时才返回 500，错误码沿用既有 `search_unavailable`，客户端无需新增分支。

### 与既有约束的优先级

`add-trading-card-blindbox-mvp` 的 spec 含「ES 不可用时不查 MySQL 兜底」的表述。该约束**被本能力修订**：ES 不可用时 SHALL 降级到 MySQL。该 change 归档时 MUST 同步修订该表述，否则两个 spec 将互相矛盾。

---

## D14 文案策略：内联，不引入国际化

### 决策

本期客户端为**单语言（简体中文）**应用，删除原设计的 i18n 层：移除 `vue-i18n` 与 `yaml` 两个依赖，删除 `src/lang/` 目录，撤销 `main.ts` 中的 `app.use(i18n)`，全部界面文案直接书写于模板或组件脚本。

### 理由

- **没有真实需求**：C 端只有简体中文一种语言，`en` 语言资源从设计到收尾始终只有 `zh-cn` 一份被实现，多语言是纯粹的预留。
- **成本大于收益**：词条层要求开发者「先想一个 key、再去 YAML 找、再回模板填」，改一句文案要跨两个文件；而收益（多语言、词条复用）在本期为零。
- **删掉两类依赖**：`vue-i18n` 与 `yaml`（后者仅用于构建期解析 YAML 词条），后者不出现在运行时产物中，前者直接被移出依赖树。
- **不违反工程隔离**：原 D1 把 `vue-i18n` 列为两端冲突项，删除后该冲突自然消失，D1 的结论（仍需独立依赖树）由 Vite / TypeScript 的版本差继续支撑。

### 代价与约束

- 文案散落在各页面，**没有集中查阅的地方**。这是本决策的已知代价，接受。
- 需要「后端枚举 → 文案」映射的场景 MUST 保留集中常量表，避免把条件判断散进模板：
  - `config/draw-errors.ts` 的 8 条抽卡错误码规则（字段由 `messageKey` 改为 `message`）
  - 订单状态（`pending` / `paid` / `drawn` / `failed`）与下单来源（`normal` / `seckill`）
- 将来若真要接入多语言，需重新引入词条层 —— 属于独立 change，不在本期范围。

### 被删除的词条

原 `zh-cn/index.yaml` 中有一批**从未被引用**的词条，一并消失，不再保留：`app.name`、`common.empty` / `confirm` / `cancel` / `unitCard`、整段 `tabbar.*`、`home.sectionSupplier` / `hotScore`、`detail.stockTotal` / `poolCol*` / `poolEmpty` / `previewTitle`、`draw.emptyTitle` / `emptyDesc` / `emptyAction`。（其中 `app.name`、`tabbar.*` 的实际文案一直由 `pages.json` 的中文字面量提供，本就与语言资源重复。）
