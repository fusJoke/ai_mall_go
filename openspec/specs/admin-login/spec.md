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

`Login` 服务 MUST 在密码错误时自增 `admin.LoginFailure` 并落库；登录成功时清零。落库 MUST 使用仅更新目标列的定向更新（失败路径：`login_failure`，触锁时追加 `status`；成功路径：`login_failure`、`last_login_at`、`last_login_ip`），MUST NOT 经由携带 Password 字段的整行更新。`LoginFailure` 仅由本 capability 维护，不暴露给客户端。

失败计数的自增 MUST 在数据库侧以原子表达式完成（`login_failure = login_failure + 1`），MUST NOT 采用「先读出当前值、在应用层加一、再写回绝对值」的两步写法 —— 后者在并发请求下互相覆盖，会使锁定阈值永不可达，锁定机制可被并发绕过。自增后的新值 MUST 在同一事务内回读，且回读值 MUST 用于判定是否触锁。

#### Scenario: 密码错误递增失败计数

- **WHEN** 密码错误但用户名存在
- **THEN** `admin.LoginFailure += 1` 并经定向列更新写回 DB
- **AND** 该写路径 MUST NOT 产生包含 `password` 列的 UPDATE
- **AND** 响应仍为 401，不返回当前 `LoginFailure` 值
- **AND** 自增 MUST 由数据库侧的原子表达式完成，MUST NOT 由应用层写回绝对值

#### Scenario: 并发失败尝试不丢计数

- **WHEN** 同一管理员账号在阈值内并发提交多个密码错误的登录请求
- **THEN** `admin.LoginFailure` 的最终值 MUST 等于实际失败次数
- **AND** 达到阈值时 MUST 触发账号禁用与令牌清除

#### Scenario: 登录成功清零失败计数

- **WHEN** 验证码、密码、状态校验全部通过
- **THEN** `login_failure` 清零，`last_login_at` / `last_login_ip` 更新，经定向列更新一次写库
- **AND** 该写路径 MUST NOT 产生包含 `password` 列的 UPDATE

### Requirement: 密码哈希不可变性

`Login` 全链路（验证码校验、密码比对、失败计数、登录成功记账、token 签发）MUST NOT 改写 `admins.password` 列。密码哈希的唯一合法写入口是显式改密（管理页 `ChangePassword` 走 `repo.UpdatePassword`）。任何登录路径的落库操作 SHALL 采用不含 `password` 列的定向更新，禁止把从 DB 读出的完整 admin 模型（其 `Password` 为哈希串）交给具备"非空即 bcrypt"语义的通用更新。

#### Scenario: 密码错误后哈希不变

- **WHEN** 一次密码错误的登录尝试完成（含失败计数落库）
- **THEN** `admins.password` 列值与尝试前一致
- **AND** 后续使用正确密码的登录可成功

#### Scenario: 登录成功后哈希不变

- **WHEN** 一次登录成功完成（含 last_login 记账落库）
- **THEN** `admins.password` 列值与登录前一致
- **AND** 登出后使用同一密码再次登录可成功
