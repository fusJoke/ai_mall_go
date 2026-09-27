# admin-login Specification

## Purpose
为后台管理端提供登录能力：客户端先调点选验证码预检通过后回调拿到 `captchaKey`，登录请求必须携带 `captchaKey`、username、password；服务端按"验证码二次校验 → 密码校验 → 签发 token"的顺序门控执行，任一环节失败均返回 401 且不泄露用户名是否存在。

## Requirements

### Requirement: 登录请求体含 captchaKey

`POST /admin/login` 请求体 SHALL 包含必填字段：
- `username`：字符串，必填
- `password`：字符串，必填
- `captchaKey`：字符串，必填，来源于点选验证码预检通过后回调透传的 `key`
- `remember`：布尔值，可选，默认 false

缺 `captchaKey` 或为空 MUST 返回 400。

#### Scenario: 缺 captchaKey
- **WHEN** 客户端提交登录请求但未带 `captchaKey`
- **THEN** 响应 400，错误码 `login.invalid_input`

### Requirement: 登录顺序门控

`Login` 服务 MUST 按以下顺序执行，任一前置环节失败立即返回 401，不进入后续环节：
1. `verifyClick(captchaKey, deleteOnSuccess=true)` —— 必须通过
2. bcrypt 比对 `password` 与 `admin.Password`
3. 校验 `admin.Status == 1`
4. 签发 token

#### Scenario: 验证码错误（前置失败）
- **WHEN** `captchaKey` 不存在 / 已过期 / 已被消费 / 坐标精度不符
- **THEN** 响应 401，错误码 `login.invalid_captcha`
- **AND** MUST NOT 进行 bcrypt 比对、不返回账号是否存在信息

#### Scenario: 密码错误
- **WHEN** 验证码二次校验通过但密码错误
- **THEN** 响应 401，错误码 `login.invalid_credentials`
- **AND** 文案与"用户名不存在"统一为"invalid username or password"，不泄露用户名是否存在

#### Scenario: 账号禁用
- **WHEN** 验证码与密码均通过但 `admin.Status != 1`
- **THEN** 响应 403，错误码 `login.account_disabled`

#### Scenario: 登录成功
- **WHEN** 验证码二次校验 + 密码 + 状态全部通过
- **THEN** 响应 200，返回 `{admin, token}`
- **AND** `admin.LoginFailure` 清零、`last_login_at` / `last_login_ip` 更新
- **AND** 签发 `remember ? 30天 : 3天` 有效期的 token

### Requirement: 失败计数维护

`Login` 服务 MUST 在密码错误时自增 `admin.LoginFailure` 并落库；登录成功时清零。`LoginFailure` 仅由本 capability 维护，不暴露给客户端。

#### Scenario: 密码错误递增失败计数
- **WHEN** 密码错误但用户名存在
- **THEN** `admin.LoginFailure += 1` 并写回 DB
- **AND** 响应仍为 401，不返回当前 `LoginFailure` 值
