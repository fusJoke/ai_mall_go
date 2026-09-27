# Design

## Context

后台 `admin-login` capability 已落地（详见 `openspec/specs/admin-login/spec.md`）：`POST /admin/login` 签发 token，sha256 哈希后写 `tokens` 表，`DeletedAt gorm.DeletedAt` 软删除字段已存在，TTL 默认 3 天 / `remember` 30 天。`internal/infra/token.Manager` 已实现 `Create` / `Get` / `Delete` / `Clear` / `Check`，其中 `Delete(ctx, rawToken)` 由 `internal/infra/token/driver/database.go` 实现，对 `ErrTokenNotFound` 做 no-op —— 天然幂等。

前端 `web/src/stores/adminInfo.ts` Pinia store 持久化 token + admin 资料字段；axios 拦截器（`web/src/utils/request.ts`）在 401 时只清 token 不跳转。`web/src/views/admin/login.vue` 是 `useAdminInfo` 唯一消费者。

`internal/handler/admin/admin.go` 当前只有 `Login` 一个 handler；`internal/service/admin/admin.go` 当前只有 `Login` 一个方法，`tokenIssuer` 接口（`admin.go:28-30`）只有 `Create`。

## Goals / Non-Goals

**Goals:**

- 在 `/admin/logout` 暴露服务端登出能力，复用 `Manager.Delete` 现成实现。
- 登出对调用方幂等（任何状态下调用都成功返回 200）。
- 前端最小集成：调接口 + 清 store + 一个临时 UI 入口供联调。
- 4 个单元测试覆盖 service.Logout 的关键路径。

**Non-Goals:**

- 不引入 auth middleware（强鉴权 token 轮换是独立 change）。
- 不做"踢出所有会话"（需要按 userID 清，清除模型不同）。
- 不做 TTL sweep（接受软删除行留 DB，与现有 `Manager.Delete` 语义一致）。
- 不消费 `web/src/lang/...` 里 `layouts.logout` i18n key（留给后续 UI commit）。
- 不动 router/auth guard、不动 401 拦截器、不动 login 流程。

## Decisions

### D1. 复用 `Manager.Delete` 而非新增 service 层包装

`Manager.Delete` 已经是幂等的（驱动层 `database.go` 对 `ErrTokenNotFound` 是 no-op），service 层不需要再加包装。

- **替代方案**：service 层先 `Get` 再 `Delete` —— 拒绝。原因：多一次 DB 读、且需要新增 `ErrTokenNotFound` 的翻译，复杂度无收益。
- **替代方案**：service 层加 `sentinel error` 如 `ErrTokenAlreadyRevoked` —— 拒绝。原因：登出的语义就是"任意状态下都成功"，错误码无意义。

### D2. Bearer 解析内联在 handler

`Authorization` header 的解析写在 `Logout` handler 顶部，不抽 helper。

- **理由**：单一调用点。将来第二个 bearer-needing 路由出现再抽。
- **替代方案**：抽 `auth.ParseBearer(c) (string, error)` 到 `internal/middleware/auth/` —— 拒绝。原因：当前不需要，且过早抽象会污染 `internal/middleware/` 的接口边界。

### D3. `tokenIssuer` 接口扩展为含 `Delete`

现有 `tokenIssuer`（`internal/service/admin/admin.go:28-30`）只声明 `Create`。`mockIssuer` 也要同步加 `Delete` 实现。

- **理由**：service 通过接口依赖 `Manager`，mock 测试可以无侵入。新增 `Delete` 是 interface 演化，不破坏既有 `TestLogin_*` 测试（它们只调 `Create`）。
- **断言**：`var _ tokenIssuer = (*mockIssuer)(nil)` 编译期保证接口一致性。
- **替代方案**：直接在 `*baseService` 上持有 `*token.Manager` 具体类型 —— 拒绝。原因：丢了 mock 能力，service 测试无法独立验证 `Logout` 行为。

### D4. Service.Logout 接受 `rawToken` 而非 `model.Token`

`Logout(ctx, rawToken string) error` —— 接口签名只接收字符串。

- **理由**：登出语义是"凭持有 token 证明身份"，service 不需要反查 user；handler 已经从 header 拿到 rawToken。
- **替代方案**：`Logout(ctx, t *model.Token)` —— 拒绝。原因：handler 不持有 `*model.Token`，需要先 `Get` 再传，多一次 DB 读 + 引入新错误码。

### D5. handler 在 token 解析失败时空短路返回 200

header 缺失 / 不以 `Bearer ` 开头 / 后面 token 为空 → 不走 `s.svc.Logout`，直接 `c.JSON(200, logout.ok)`。

- **理由**：登出语义幂等 —— "未登录"和"已登录"的调用方期望是一致的（"确认我不在线了"）。
- **替代方案**：解析失败返回 400 —— 拒绝。原因：会让前端必须先 `Get` 再 logout；丢失幂等性；与 REST 惯例不符。

### D6. 前端 `reset()` 不复用 Pinia 内置 `$reset()`

显式 `removeToken()` + `$patch({...零值...})`，不调 `this.$reset()`。

- **理由**：`$reset()` 依赖 Pinia 内部 state factory 初始化；显式列字段更可控、不会留下未来加的新字段的"残留默认值"。
- **替代方案**：`this.$reset()` —— 拒绝。原因：测试覆盖不到所有未来新增字段。

### D7. 临时注销 bar 放在 `login.vue`

`login.vue` 是 `useAdminInfo` 唯一消费者，且没有 auth guard 拦截已登录用户。

- **理由**：已登录用户访问 `/admin/login` 时自然落到这里；不引入新文件。
- **替代方案**：新建 `web/src/components/admin/LoggedInBanner.vue` —— 拒绝。原因：本 PR 只为联调，正式 UI 在后续 commit。
- **明确标记**：template 注释里写 `// TEMP: 待 admin header UI commit 替换`。

## Risks / Trade-offs

| 风险 | 缓解 |
|---|---|
| 软删除让 token 行永久留 DB | 接受。匹配现有 `Manager.Delete` 语义；`tokens` 表小、有 `ExpiresAt`，将来增长再考虑 TTL sweep |
| Token 泄漏 → 攻击者触发受害者登出 | 接受。低成本骚扰；做强认证靠 auth middleware + token 轮换，独立 change |
| 在途请求与并发注销竞态 | 接受。已通过 `Manager.Check` 的请求正常响应；下一请求走 `Get` → `ErrTokenNotFound` → 401 |
| 多端会话 | 用 `Delete(rawToken)` 而非 `Clear(userID, type)`。调用方凭持有 token 证明身份；`Clear` 需要不同授权模型 |
| `mockIssuer` 接口扩展影响既有 login 测试 | 接口加方法不影响老用例；编译期 `var _ tokenIssuer = (*mockIssuer)(nil)` 断言兜底 |
| 前端临时注销 bar 被遗忘 | 在 `login.vue` 顶部加 `TODO(admin-header): replace this banner` 注释，让后续 reviewer 看到 |
| `swag init` 未执行导致 `/swagger/index.html` 不显示新路由 | tasks.md 里强制 `swag init` 验证步骤 |

## Migration Plan

部署步骤：

1. 后端：`go build ./...` → 重启服务（无 DB 迁移）。
2. 后端：`swag init` 在仓库根目录执行，重新生成 `docs/docs.go`。
3. 前端：`pnpm typecheck` → `pnpm build` 走 CI。
4. 无需 feature flag / 灰度 —— 这是新增端点，不修改既有行为。

回滚策略：

- 后端：撤回 `internal/router/admin/admin.go` 的 `registry.Register` 行 + handler/service 的方法即可。`tokens` 表无破坏性变更，软删除行可保留。
- 前端：`logout()` / `reset()` / 临时 bar 都是新增，不存在既有行为被破坏，回滚是纯减法。

## Open Questions

无。设计已收口；所有可能影响 spec 或 task 拆分的决策都已确认。
