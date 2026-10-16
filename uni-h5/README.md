# uni-h5 — C 端移动客户端（uni-app H5）

`ai-go-mall` 的 C 端移动端工程。一期只做 **H5**，只打通 **「首页商品 → 下单」** 主链路。

需求与技术方案见 `docs/`：

| 文件 | 内容 |
|---|---|
| `docs/uni-h5-C端需求文档-v3.docx` | 需求文档（9 章：范围 / 技术选型 / 页面清单 / 流程 / 接口 / 视觉 / 任务 / 验收 / 规划） |
| `docs/接口与数据契约.md` | 接口清单、mock 方案、状态机、表结构 |
| `docs/prototype.html` | 8 屏高保真原型 |

规格（openspec）在仓库根目录：`openspec/changes/add-uni-h5-client/`。

---

## 环境要求

| 项 | 版本 | 说明 |
|---|---|---|
| Node.js | ≥ 18 | 开发时使用 22.x 验证 |
| 包管理器 | npm | |
| 后端 | `ai-go-mall` 本地启动，默认 `127.0.0.1:8080` | 首页 Feed 需要 ES，或后端开启降级 |

> **依赖隔离**：本工程与仓库内的 `web/`（Vue3 PC 端）**必须保持独立**。
> 两边的 Vite / TypeScript 大版本不兼容，`package.json` 不可互相引用。
> `@dcloudio/*` 一律锁**精确版本**（不含 `^`），否则会被解析到 alpha 版本。

---

## 快速开始

```bash
npm install

# 开发（默认 http://localhost:5176，/api 代理到 127.0.0.1:8080）
npm run dev:h5

# 生产构建，产物在 dist/build/h5
npm run build:h5

# 生产构建 + 产物卫生门禁（推荐：发布前跑这一条）
npm run check:prod
```

如果后端不在默认端口，设置代理目标：

```bash
VITE_DEV_PROXY_TARGET=http://127.0.0.1:9090 npm run dev:h5
```

### 环境文件放哪里

| 文件 | 何时加载 | 放什么 |
| --- | --- | --- |
| `.env.development.local` | **仅 dev 模式**（已 gitignore） | 开发令牌、`VITE_USE_MOCK=true` 等开发专用值 |
| `.env.example` | 不加载（模板） | 变量清单与说明 |
| ~~`.env.local`~~ | **production 构建同样加载** | ⚠️ 不要用：Vite 的加载顺序是 `.env → .env.local → .env.<mode> → .env.<mode>.local`，放这里等于每次打生产包都会把内容静态内联进 JS |

`VITE_USE_MOCK` **默认关闭**：只有显式 `true` 才启用（原先是 `!== 'false'` 即默认开启，
漏配变量会静默跑假数据）。生产构建里若仍为 `true`，或 `VITE_DEV_TOKEN` 非空，
`src/config/index.ts` 的 `assertProductionHygiene()` 会在入口直接抛错，
`npm run verify:prod-bundle` 则会在产物里把它们扫出来。

---

## 开发令牌（`VITE_DEV_TOKEN`）

### 为什么需要它

登录页的「手机号 + 短信 + 滑块」本期是**前端 mock**，服务端尚未实现。但抽卡（`POST /user/blindbox/draw`）
与订单（`GET /user/orders/list`）是**真实接口且需要鉴权**。mock 签发的假 token 过不了后端 `UserAuth()`。

而既有的真实登录接口 `POST /user/login` **强制要求「点选验证码」消费型校验**，其答案只存在服务端、
响应中不返回，因此客户端无法在 mock 模式下静默换取真 token。

> 这一点与早期设计（tasks 4.7「静默调用 `/user/login` 换真 token」）不符，属于设计修正 ——
> 原方案在当时未核验该接口的验证码约束，实施阶段确认不可行。

### 怎么做

1. 复制 `.env.example` 为 **`.env.development.local`**（匹配 `.env.*.local`，已被 `.gitignore` 忽略）。
2. 用后端提供的开发令牌工具生成一个**真实用户令牌**，填入 `VITE_DEV_TOKEN`。
3. 重启开发服务器。

> ⚠️ **不要放进 `.env.local`**。该文件在 production 构建里同样会被加载，而
> `VITE_DEV_TOKEN` 会被 Vite 在构建期**静态内联**进产物 —— 任何访客 view-source
> 就能拿到一串能直接读写真实用户数据的令牌（OCR 审查 P2-4）。

配置后：mock 登录成功时会用该真实令牌替换会话，此后抽卡会**真扣库存、真出订单**。

未配置时：会话降级为 mock-only，抽卡 / 订单会返回 401，「我的」页会给出明确提示（不会静默失败）。

---

## 目录结构

```
src/
├── api/            # 接口定义：只描述 URL / 参数 / 返回类型
├── components/     # 通用组件（瀑布流、卡片、滑块、开卡动画…）
├── config/         # 运行时配置、错误码映射表
├── mock/           # mock 层：仅登录相关 4 条接口
├── pages/          # 页面
├── store/          # 响应式状态（用户会话、抽卡结果中转）
├── styles/         # 设计令牌与全局样式
├── types/          # 全项目类型单一来源
└── utils/          # 请求层、存储、格式化、会话
```

---

## 架构要点

**1. 请求层自封装（`src/utils/request.ts`）**
不用 axios（依赖 XMLHttpRequest，小程序端不可用）。统一处理 baseURL、`Authorization: Bearer` 注入、
裸 JSON 解析、错误归一为 `{http, code, message, retryAfter}`、401 拦截跳登录、429 提取 `retryAfter`。

**2. Mock 边界严格受限（`src/mock/index.ts`）**
只覆盖登录页及其配套 5 条接口：`sms/send`、`captcha/slider`、`captcha/slider/verify`、
`login/mobile`、`user/profile`。商品、抽卡、订单一律走真实后端。
mock 注册表以常量 `MOCK_ROUTES` 导出，便于自检。

**3. 设计令牌（`src/styles/tokens.scss`）**
全项目唯一的十六进制色值来源。组件与页面一律用 `var(--xxx)`，
**不要**在 `<style scoped>` 中 `@import` tokens —— 全局 `:root` 被 scoped 化后不会生效。

**4. 稀有度配色三处一致**
结果页 / 详情卡池 / 订单详情均通过 `src/utils/rarity.ts` 取色，取值对齐后端枚举 `SSR / SR / R / N`。

**5. 瀑布流**
原生双列 + 触底哨兵，**未引虚拟列表**（data.md D7 有选型论证）。封装为 `<waterfall-list>`，
将来量级上来替换内部实现即可，调用方零改动。

---

## 已知限制

- **无订单列表缩略图**：`GET /user/orders/list` 只返回订单字段，不含卡牌明细。
  为避免 N+1 请求，列表页不展示缩略图，卡牌图在订单详情页展示。
- **余额取自 mock**：`GET /user/profile` 服务端未实现，暂由 mock 返回本地会话中的用户快照。
- **详情页无卡池图片预览**：`blindbox/detail` 的 `items[]` 只含 `card_id / rarity / weight / stock`，
  不含卡名与卡图，因此「卡池预览」以概率表 + 库存构成条呈现。
- **滑块 `block_w/block_h`**：服务端契约需返回拼图块原始尺寸，缺省时客户端按比例估算（视觉可用、判定精度不保证）。

---

## 相关规格

`openspec/changes/add-uni-h5-client/`

| capability | 内容 |
|---|---|
| `uni-h5-client` | 客户端工程、页面、请求层、mock 边界、错误码映射 |
| `user-mobile-login` | 手机号登录契约（含首登自动建号） |
| `slider-captcha` | 滑块验证码（与现有点选验证码并存） |
| `home-feed-fallback` | 首页 Feed 的 MySQL 降级 |
