# Proposal

## Why

后台登录流程（`admin-login` capability）已落地：`POST /admin/login` 签发 token，前端拿到 token 之后写进 Pinia `adminInfo` store。但**没有任何登出接口** —— `logout` / `注销` / `signout` / `Invalidate` / `Revoke` 在路由、handler、service 中零命中。前端 `adminInfo` 只有 `removeToken()`（清 token 字段），axios 401 拦截器也只是清 token 不跳转；项目里没有任何登出 UI。

后果：用户在前端点"注销"目前没有任何作用 —— 服务端的 `tokens` 表里那行记录（sha256(token)）依然存在且有效，直到自然过期。token 实际上从前端 `localStorage` 被清掉后仍然在数据库里有效 3 天 / 30 天，留下了潜在的安全敞口。

`internal/infra/token.Manager.Delete(rawToken)` 已实现（`internal/infra/token/driver/database.go` 对 `ErrTokenNotFound` 是 no-op，天然幂等）。本次变更只是把这条已存在的能力接到 HTTP 层。

## What Changes

- 后端：`internal/service/admin.Service` 新增 `Logout(ctx, rawToken)`；`internal/handler/admin/admin.go` 新增 `Logout` handler，从 `Authorization: Bearer <token>` header 解析 token；`internal/router/admin/admin.go` 注册 `POST /admin/logout`。新增 4 个单元测试。
- 前端：`web/src/api/admin/index.ts` 新增 `logout()`；`web/src/stores/adminInfo.ts` 新增 `reset()` action（清 token + 清其他字段，避免 `pinia-plugin-persistedstate` 把旧数据留在 localStorage）；`web/src/views/admin/login.vue` 加一个**临时**的注销 bar（"已登录为 xxx [注销]"）作为联调入口 —— 后续独立 UI commit 会替换为正式 admin header。
- 不修改：login 流程、`tokens` 表结构、`Manager.Delete` 实现、axios 拦截器、router/auth guard。

## Capabilities

### New Capabilities

- `admin-logout`：管理员登出 —— 调用方凭 `Authorization: Bearer <token>` 证明身份，服务端软删除该 token，幂等返回 200。

### Modified Capabilities

<!-- 无：login 自身的需求不变；登出是独立 lifecycle 事件。 -->

## Impact

- **代码**：后端 4 个文件（`internal/handler/admin/admin.go`、`internal/service/admin/admin.go`、`internal/service/admin/admin_test.go`、`internal/router/admin/admin.go`），前端 4 个文件（`web/src/api/admin/index.ts`、`web/src/stores/adminInfo.ts`、`web/src/stores/interface/index.ts`、`web/src/views/admin/login.vue`）。
- **API 契约**：新增 `POST /admin/logout`（无 request body，token 取自 header）。响应信封遵循现有 handler 约定：`{code, message}`。状态码 200 / 500。错误码 `logout.ok` / `logout.internal`。
- **依赖**：无新第三方依赖；`swag init` 需重新生成 `docs/docs.go`。
- **数据库**：无迁移 —— 复用 `tokens` 表已有的 `DeletedAt gorm.DeletedAt` 软删除字段。
- **前端 UX**：临时注销 bar 复用现有 `login.vue`，未登录时不显示。后续会被独立 PR 替换为 admin 正式 header。
