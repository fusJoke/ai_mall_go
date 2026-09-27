# Proposal

## Why

管理端登录当前只校验账号密码，缺少针对脚本/爬虫的图形验证手段。`internal/infra/captcha` 已经具备点选验证码的核心能力（图片合成、碰撞检测、sha1 摘要校验），但缺少 HTTP 层暴露与登录链路集成。本次 change 把点选验证码接成完整的登录前置验证能力，并按"前端预检 + 后端二次校验"的模式落进管理端登录。图片尺寸固定 350x200（与 `asset/captcha/click/background/*.png` 一致）。

## What Changes

- 新增 HTTP 层：包装 `internal/infra/captcha.Manager` 为 handler + service，注册 `/common/captcha/create`（GET）与 `/common/captcha/verify`（POST）两条路由。
- 改造 `infra/captcha.Manager.VerifyClick`：入参改为 `{key, points: [{x, y}, ...], w, h}`，按图片原始 350x200 坐标比对每个点的 bbox 中心距离是否在容差半径内（text=14px、icon=24px）。
- 引入 per-captchaKey 答错计数：每次 verify 失败 `cnt++`（用 `atomic.Int64` 保证并发 RMW 不丢失更新，避免并发请求绕过 M=3），`cnt >= 3` 立即 `DeleteByKey`，要求前端重新 `getClickCaptcha`。计数器内存维护（key 删除即归零），但 `cleanupLazy` 会按 `expiresAt` 同步回收 failCnt，避免 1-2 次失败后放任过期 key 永久驻留内存（被 Codex review 指出后修复）。不持久化到 DB。
- `LoginRequest` 新增 `captchaKey` 字段；`Login` service 改为顺序门控：先 `verifyClick(captchaKey, deleteOnSuccess=true)` 通过、再 bcrypt 比对密码、再签 token。
- 前端 clickCaptcha 组件按新契约改造（响应 `{key, elements[], image, w, h}`、请求 `{key, points, w, h}`），callback 透传 `captchaKey` 而非坐标串。
- `login.vue` 接入 clickCaptcha：每次提交先开弹窗，用户点完后回调拿到 `captchaKey`，登录请求带上。
- 预检 `/common/captcha/verify` 始终 `deleteOnSuccess=false`；只有登录内的二次 verify 才是 `deleteOnSuccess=true`（一次性消费）。

## Capabilities

### New Capabilities

- `click-captcha`: 点选验证码能力 —— 服务端图片生成、坐标精度校验、per-key 答错计数（M=3 强制刷新）、通用 create/verify 接口。
- `admin-login`: 管理端登录 —— 账号密码校验 + 点选验证码二次校验顺序门控；登录链路唯一性由本 capability 固化。

### Modified Capabilities

<!-- 当前 openspec/specs/ 为空，无既有 capability；登录由 admin-login 这一新 capability 引入并固化 -->
- （无）

## Impact

- **后端**
  - 新增：`internal/handler/common/`（captcha handler）、`internal/service/captcha/`（wrapper）、`internal/router/common/`（init 自注册路由）
  - 改造：`internal/infra/captcha/click.go`（VerifyReq 形态、VerifyClick 入参、答错计数）
  - 改造：`internal/handler/admin/admin.go`（LoginRequest 新字段）、`internal/service/admin/admin.go`（Login 顺序门控 + captcha 二次校验）
- **前端**
  - 改造：`web/src/components/clickCaptcha/{index.vue,index.ts}`（响应/请求契约对齐）
  - 改造：`web/src/views/admin/login.vue`（接入 clickCaptcha，登录请求带 captchaKey）
- **配置**
  - 无新增 yaml；M 与容差半径暂以常量固化在 `infra/captcha`（后续若需可配置化再扩展 yaml）
- **依赖**
  - 无新增 Go 模块；无新增 npm 包
- **数据**
  - 复用既有 `captchas` 表（`internal/model/captcha.go`），schema 不变