# Spec Delta

## Purpose

为后台管理端提供登出能力：调用方通过 `Authorization: Bearer <token>` 证明对当前 token 的持有，服务端软删除该 token。登出必须幂等 —— 不论调用方是否已登录、token 是否有效，调用都正常完成。

## ADDED Requirements

### Requirement: Logout 端点接收 Bearer token

`POST /admin/logout` SHALL 接受 `Authorization` 请求头（值形如 `Bearer <token>`），并按以下规则解析：

- 缺失 header / 值为空 / 不以 `Bearer ` 开头 → token 为空字符串
- 否则从 `Bearer ` 后截取余下内容，`TrimSpace` 后作为 `rawToken`

无 request body。

#### Scenario: 携带 Bearer token 调用
- **WHEN** 请求带 `Authorization: Bearer abc.def.ghi`
- **THEN** 后端把 `abc.def.ghi` 作为 `rawToken` 解析

#### Scenario: 缺失 Authorization 头
- **WHEN** 请求不带 `Authorization` header
- **THEN** 后端将 `rawToken` 视为空字符串

#### Scenario: Authorization 不以 Bearer 开头
- **WHEN** 请求带 `Authorization: Token abc`（非 `Bearer ` 前缀）
- **THEN** 后端将 `rawToken` 视为空字符串

### Requirement: Logout 软删除调用方持有的 token

服务端 SHALL 对解析得到的 `rawToken` 执行软删除（`tokens` 表上打 `DeletedAt`）。任一情况下调用 SHALL 返回 HTTP 200：

- `rawToken` 为空字符串
- token 不存在于数据库
- token 已软删除
- token 已过期
- token 存在且有效（软删除成功）

唯一返回非 200 的情况是底层存储驱动出错（如数据库不可达），此时 SHALL 返回 HTTP 500。

#### Scenario: 有效 token 被软删除
- **WHEN** 请求携带当前有效的 `rawToken`
- **THEN** HTTP 200，响应体 `{code: "logout.ok", message: "ok"}`
- **AND** 数据库中该 token 行 `deleted_at` 被设置

#### Scenario: token 不存在
- **WHEN** 请求携带格式合法但数据库中不存在的 `rawToken`
- **THEN** HTTP 200，响应体 `{code: "logout.ok", message: "no active session"}`

#### Scenario: token 已被登出
- **WHEN** 请求携带已软删除的 `rawToken`
- **THEN** HTTP 200，响应体 `{code: "logout.ok", message: "no active session"}`

#### Scenario: 未登录访客调用
- **WHEN** 请求不带 Authorization header
- **THEN** HTTP 200，响应体 `{code: "logout.ok", message: "no active session"}`
- **AND** 数据库无任何写入

#### Scenario: 存储驱动错误
- **WHEN** 软删除过程中数据库不可达
- **THEN** HTTP 500，响应体 `{code: "logout.internal", message: <err>}`（`message` 透传驱动错误文本）

### Requirement: Logout 幂等性

重复调用 SHALL 始终返回 HTTP 200，最终状态等价于单次调用。重复调用的副作用 SHALL 仅限一次成功的软删除（首次），后续调用 SHALL 为 no-op。

#### Scenario: 同一 token 连续两次登出
- **WHEN** 同一有效 token 第一次调用返回 200 后，再次调用
- **THEN** 第二次仍返回 HTTP 200
- **AND** 数据库中该 token 行 `deleted_at` 保持不变（不重复写入）

### Requirement: 已登出 token 不可再用

已被登出的 token MUST 不能通过现有任何基于该 token 鉴权的接口（不在本 capability 范围内，但由 `tokens` 表的软删除 + `Manager.Get` 对 `DeletedAt` 的过滤共同保证）。

#### Scenario: 已软删除 token 被其他接口使用
- **WHEN** 该 token 被登出后，再用于任何要求 `Authorization` 的接口
- **THEN** 该接口应返回 401（具体错误码由对应接口 spec 定义）

### Requirement: 前端 Logout 调用与本地清理

前端 `web/src/api/admin/index.ts` 的 `logout()` SHALL 调用 `POST /admin/logout`，并 SHALL 在调用结果（无论成功或失败）后由 Pinia `adminInfo` store 的 `reset()` action 清除 token 之外的所有持久化字段，以避免 `pinia-plugin-persistedstate` 把旧数据留在 `localStorage`。清除的字段 MUST 至少包括：`id`、`username`、`nickname`、`avatar`、`last_login_at`、`last_login_ip`、`super`、`token`。

#### Scenario: 登出成功
- **WHEN** 前端点击"注销"且 `POST /admin/logout` 返回 200
- **THEN** `adminInfo` store 中所有持久化字段被清空
- **AND** `localStorage` 中 `adminInfo` 不再包含旧值（保留初始默认序列化）

#### Scenario: 登出请求失败但本地仍清理
- **WHEN** 前端点击"注销"但 `POST /admin/logout` 返回 500 或网络失败
- **THEN** `adminInfo` store 仍 SHALL 清空所有持久化字段
- **AND** SHALL 不弹全局 toast（前端选择静默失败）
