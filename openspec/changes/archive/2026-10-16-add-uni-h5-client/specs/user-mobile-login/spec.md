## Purpose

为 C 端提供手机号短信验证码登录能力，替代现有「用户名 + 密码 + 点选验证码」的 PC 场景登录方式，以适配移动 H5 客户端的输入习惯。本能力定义短信发码、手机号登录、当前用户信息三条接口契约与首登建号规则；服务端实现留待后续 change，本期由客户端 mock 承接。

## ADDED Requirements

### Requirement: 短信验证码发送接口

系统 SHALL 提供 `POST /user/sms/send`（公开，无需鉴权）用于向指定手机号下发登录验证码。

请求体 MUST 包含：`mobile`（string，必填，中国大陆手机号，格式 `^1[3-9]\d{9}$`）、`scene`（string，必填，本期固定为 `login`）。

成功响应 MUST 为 `{ code: "sms.ok", message: "sent", expires_in: <秒数> }`，其中 `expires_in` 为验证码有效期，SHALL 为 300 秒。

失败响应 MUST 使用：`400 sms.invalid_mobile`（手机号格式非法）、`429 sms.too_frequent`（触发频率限制）、`500 sms.internal`。

该接口 MUST 施加频率限制：同一手机号 60 秒内至多 1 次、每自然日至多 10 次；同一来源 IP 每小时至多 20 次。超限 MUST 返回 429。

验证码 MUST NOT 出现在响应体中。

#### Scenario: 正常下发

- **WHEN** 调用方提交格式合法的手机号与 `scene=login`
- **THEN** 系统 MUST 返回 200 与 `code="sms.ok"`
- **AND** 响应体 MUST NOT 包含验证码原文

#### Scenario: 手机号格式非法

- **WHEN** 提交的手机号不匹配 `^1[3-9]\d{9}$`
- **THEN** 系统 MUST 返回 400 `sms.invalid_mobile`

#### Scenario: 同号 60 秒内重复请求

- **WHEN** 同一手机号在上一次发送成功的 60 秒内再次请求发送
- **THEN** 系统 MUST 返回 429 `sms.too_frequent`

#### Scenario: 当日次数超限

- **WHEN** 同一手机号当日发送次数已达 10 次
- **THEN** 系统 MUST 拒绝并返回 429 `sms.too_frequent`

### Requirement: 手机号登录接口

系统 SHALL 提供 `POST /user/login/mobile`（公开，无需鉴权）以「手机号 + 短信验证码」完成登录。

请求体 MUST 包含：`mobile`（string，必填）、`sms_code`（string，必填，6 位数字）、`remember`（bool，可选，默认 `false`）。**本期滑块字段不参与本接口**（登录页已移除滑块，滑块能力与接口保留备用，见 `slider-captcha` 能力）。

成功响应 MUST 为裸 JSON（不含 `{code,message,data}` 信封），结构为 `{ user: {...}, token: "<令牌>" }`。`user` 对象 MUST 包含 `id`、`username`、`nickname`、`avatar`、`mobile`、`balance`（**字符串**，保留两位小数）、`status`、`last_login_at`、`last_login_ip`。

#### Scenario: 登录成功

- **WHEN** 提交正确的手机号与未过期的短信验证码
- **THEN** 系统 MUST 返回 200 与 `{ user, token }`
- **AND** 响应 MUST 为裸 JSON，不含 `code` 字段
- **AND** `user.balance` MUST 为字符串类型

### Requirement: 登录校验顺序与错误码

服务端 MUST 按以下顺序校验，任一环节失败立即终止：① 请求体格式校验（`mobile` 格式）；② 短信验证码校验；③ 用户状态校验。

本期登录链路 MUST NOT 校验滑块（滑块已从登录页移除）；重新接入滑块时，MUST 在上述顺序最前恢复「滑块消费型二次校验」，契约见 `slider-captcha` 能力。

短信验证码一经校验成功 MUST 立即失效，MUST NOT 可重复使用。

失败响应 MUST 使用 `400 login.invalid_input`、`401 login.invalid_sms_code`、`403 login.account_disabled`、`500 login.internal`（`401 login.invalid_captcha` 保留给滑块重新接入后的场景，本期 MUST NOT 出现）。

在自动建号策略下，「手机号未注册」MUST NOT 成为拒绝登录的理由，系统 MUST NOT 以任何形式向客户端暴露注册状态。

#### Scenario: 不携带滑块字段仍可登录

- **WHEN** 登录请求不携带 `captcha_key` / `captcha_x` / `captcha_w` / `captcha_h`
- **THEN** 系统 MUST 正常执行后续校验，MUST NOT 返回 `login.invalid_captcha`
- **AND** 这四个字段在本期 MUST NOT 参与校验，仅作为重新接入滑块时的预留字段

#### Scenario: 短信验证码错误

- **WHEN** 提交的 `sms_code` 与下发值不一致或已过期
- **THEN** 系统 MUST 返回 401 `login.invalid_sms_code`

#### Scenario: 验证码一次性消费

- **WHEN** 同一短信验证码在其首次成功登录后再次被用于登录
- **THEN** 系统 MUST 返回 401 `login.invalid_sms_code`

#### Scenario: 账号被禁用

- **WHEN** 手机号对应账号 `status != 1`
- **THEN** 系统 MUST 返回 403 `login.account_disabled`

#### Scenario: 错误提示不暴露账号存在性

- **WHEN** 登录因短信验证码错误而失败
- **THEN** 系统 MUST 返回 401 `login.invalid_sms_code`
- **AND** 该提示 MUST NOT 以任何形式暗示该手机号是否已注册

#### Scenario: 账号被禁用优先于建号

- **WHEN** 手机号对应账号存在但 `status != 1`
- **THEN** 系统 MUST 返回 403 `login.account_disabled`
- **AND** 系统 MUST NOT 走自动建号路径

### Requirement: 登录限流与接口并存约束

该接口 MUST 施加频率限制：同一手机号每分钟至多 10 次，超限 MUST 返回 `429 rate_limited`。

本接口 MUST 与既有 `POST /user/login` 并存，MUST NOT 修改或替换后者；既有接口继续服务 PC Web 端。

#### Scenario: 触发限流

- **WHEN** 同一手机号在一分钟内第 11 次请求登录
- **THEN** 系统 MUST 返回 429 `rate_limited`

#### Scenario: 不影响既有登录接口

- **WHEN** PC Web 端继续使用用户名与密码登录
- **THEN** `POST /user/login` 的请求契约、校验顺序与错误码 MUST 与本 change 前完全一致

### Requirement: 首次登录自动建号

用户以未注册手机号首次登录时，系统 SHALL **自动创建账号**（已确认采纳该策略），MUST 在同一事务内完成建号与登录。

新建账号的字段约束：`username` MUST 等于手机号，`password` MUST 置为空字符串，`balance` MUST 初始化为 0，`status` MUST 初始化为 1，`register_source` MUST 置为 `h5_mobile`。

建号 MUST 以 `users.mobile` 的唯一索引作为幂等保证；并发重复建号 MUST 由唯一约束拦截，且 MUST NOT 导致登录失败。

关于建号来源字段：`users.register_source`（`varchar(16) NULL`）由本 change 的迁移 `000017_users_register_source` 引入。**表结构随本 change 落地**；**写入逻辑随本接口的服务端实现落地**（属后续 change）。存量用户该列 MUST 保持 `NULL`（表示来源未知），系统 MUST NOT 为其臆测回填。

#### Scenario: 新建账号来源标记

- **WHEN** 一个新手机号首次登录并触发自动建号
- **THEN** 新建记录的 `register_source` MUST 为 `h5_mobile`

#### Scenario: 存量用户来源保持未知

- **WHEN** 迁移 `000017_users_register_source` 被应用到已有数据的库上
- **THEN** 存量用户的 `register_source` MUST 为 `NULL`
- **AND** 系统 MUST NOT 为其回填任何猜测值

#### Scenario: 新手机号首登

- **WHEN** 一个未被任何账号占用的合法手机号完成验证码校验
- **THEN** 系统 MUST 创建用户记录并返回 token
- **AND** 新建用户的 `username` MUST 等于该手机号、`balance` MUST 为 0
- **AND** 新建用户的 `password` MUST 为空字符串

#### Scenario: 并发首登幂等

- **WHEN** 同一新手机号的两个登录请求并发到达
- **THEN** 系统 MUST 只创建一个用户记录
- **AND** 两个请求 MUST 均能正常返回 token 或其中一个返回登录成功

#### Scenario: 自动建号 MUST NOT 覆盖既有账号

- **WHEN** 手机号已存在对应账号
- **THEN** 系统 MUST NOT 新建或重置该账号
- **AND** 系统 MUST 直接以既有账号完成登录

### Requirement: 当前用户信息接口

系统 SHALL 提供 `GET /user/profile`，需 `UserAuth()` 中间件鉴权，返回当前登录用户的信息。

响应 MUST 为裸 JSON，结构与登录响应的 `user` 对象**同构**：`id`、`username`、`nickname`、`avatar`、`mobile`、`balance`（字符串）、`status`、`last_login_at`、`last_login_ip`。MUST NOT 返回 `password`、`login_failure`、`deleted_at` 等内部字段。

该接口解决的是：登录响应的 `balance` 只是登录时刻的快照，而客户端在下单失败（余额不足）后或进入「我的」页面时需要拿到实时值。

#### Scenario: 查询余额

- **WHEN** 已登录用户进入「我的」页面
- **THEN** 系统 MUST 返回 200 与最新 `balance`
- **AND** 响应 MUST NOT 包含 `password` 字段

#### Scenario: 未携带令牌

- **WHEN** 请求未携带 `Authorization` 头
- **THEN** 系统 MUST 返回 401 `auth.missing_token`

#### Scenario: 令牌合法但用户已不存在

- **WHEN** 请求携带的令牌有效，但对应账号已不存在
- **THEN** 系统 MUST 返回 401 `profile.unauthorized`
- **AND** MUST NOT 返回 200

#### Scenario: 读取用户记录失败

- **WHEN** 处理过程中读取用户记录失败
- **THEN** 系统 MUST 返回 500 `user.internal`
- **AND** MUST NOT 在响应中暴露驱动层错误细节

### Requirement: 令牌语义与端间隔离

登录签发的令牌 MUST 复用既有 token 基础设施，类型 MUST 为 `user`。令牌为不透明字符串（MUST NOT 为 JWT），服务端 MUST 以哈希形式存储。

`user` 类型令牌 MUST NOT 通过 `admin` 与 `supplier` 类型路由的鉴权校验，反之亦然。令牌有效期 MUST 与既有约定一致：`remember=true` 为 30 天，否则为 3 天。

#### Scenario: 端间令牌不互通

- **WHEN** 使用手机号登录获得的令牌访问 admin 路由
- **THEN** 系统 MUST 返回 401 `auth.token_type_mismatch`

#### Scenario: 记忆登录延长有效期

- **WHEN** 登录请求携带 `remember=true`
- **THEN** 签发令牌的有效期 MUST 为 30 天
