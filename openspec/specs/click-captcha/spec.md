# click-captcha Specification

## Purpose
为后台管理端登录提供点选验证码能力：服务端随机生成 350x200 背景图与若干元素（含 ICON / 中文文字 / 英文大写字母），用户按顺序点选后客户端提交坐标，服务端按图片原始坐标比对容差半径判定通过。配套 per-key 答错计数与一次性消费机制，防止脚本批量试探。

## Requirements

### Requirement: 生成点选验证码

系统 SHALL 提供 `GET /common/captcha/create` 接口，生成一道点选验证码并返回。响应必须包含：
- `key`：本次验证码唯一标识（uuid），后续 verify 使用
- `elements`：按点击顺序排列的元素名数组（来自启用的元素类型池，长度 = 配置文件中的"长度"）
- `image`：`data:image/png;base64,...` 形式的 PNG 编码
- `width`、`height`：图片原始像素尺寸（固定 350x200）

元素类型池至少包含 `ICON`、`中文文字`、`英文大写字母` 三种，由 `config/captcha.yaml` 中的"元素"配置启用。

#### Scenario: 正常生成
- **WHEN** 客户端调用 `GET /common/captcha/create`
- **THEN** 响应 200，返回包含 `key` / `elements` / `image` / `width=350` / `height=200` 的对象
- **AND** 后端在 `captchas` 表写入一条 key+Info(sha1(elements))+expires_at 的记录

#### Scenario: 配置缺失
- **WHEN** 后端启动时 `config/captcha.yaml` 中"背景图目录"、"ICON目录"或"字体路径"为空
- **THEN** `infra/captcha.GetManager()` 初始化失败并返回 error

### Requirement: 校验点选坐标

系统 SHALL 提供 `POST /common/captcha/verify` 接口，接收 `{key, points: [{x, y}], w, h}`，按图片原始像素坐标判定每个点是否落在对应元素包围盒的容差半径内：
- `text` 类元素容差半径 = 14px
- `icon` 类元素容差半径 = 24px
- points 数组长度 MUST 与 `elements` 长度一致（顺序敏感）
- 图片尺寸 `w`、`h` 与创建时的 `width`、`height` MUST 相等，否则视为篡改

#### Scenario: 校验通过
- **WHEN** 用户按 elements 顺序点击，且每个点落在对应 bbox 中心 ±容差半径内
- **THEN** 响应 200

#### Scenario: 校验失败（坐标偏离）
- **WHEN** 任意一个点偏离对应 bbox 中心超过容差半径
- **THEN** 响应 401，错误码 `captcha.mismatch`

#### Scenario: 校验失败（key 不存在）
- **WHEN** `key` 在 `captchas` 表中查不到
- **THEN** 响应 404，错误码 `captcha.not_found`

#### Scenario: 校验失败（key 已过期）
- **WHEN** `key` 的 `expires_at` 早于当前时间
- **THEN** 响应 410，错误码 `captcha.expired`

#### Scenario: 图片尺寸不一致
- **WHEN** 提交 `w`/`h` 与创建时不一致
- **THEN** 响应 400，错误码 `captcha.invalid_input`

### Requirement: per-key 答错计数与强制刷新

系统 SHALL 在内存中维护 `key → 累计答错次数` 映射。每次 verify 失败 `cnt++`；当 `cnt >= 3` 时 MUST 立即 `DeleteByKey(captchas, key)` 并清除内存计数，使该 key 永久失效（前端必须重新 `getClickCaptcha`）。

#### Scenario: 答错 3 次强制刷新
- **WHEN** 同一 key 在 600s 过期时间内累计 3 次 verify 失败
- **THEN** 第 3 次失败响应后立即 DeleteByKey
- **AND** 该 key 后续 verify 响应 `captcha.not_found`

#### Scenario: 答错次数随 key 删除归零
- **WHEN** key 因强制刷新被 DeleteByKey
- **THEN** 内存中该 key 的答错计数一并清除

### Requirement: 预检不消耗 key

`POST /common/captcha/verify` 的预检调用 MUST 保持 `deleteOnSuccess=false`，不删除 key；只有消费型 verify（由登录链路二次校验调用）才设置 `deleteOnSuccess=true`，实现一次性消费。

#### Scenario: 预检通过后仍可二次校验
- **WHEN** 客户端先调一次 `POST /common/captcha/verify`（预检）成功后，再调一次同样的接口（消费型）
- **THEN** 第一次响应 200 且 key 仍存在
- **AND** 第二次响应 200 后 key 被硬删
- **AND** 第三次同 key 请求响应 `captcha.not_found`

### Requirement: 噪声元素不得与任何正确答案同名

`composeImage` 在选定 `Length` 个正确答案后 SHALL 用正确答案名集合构建 `exclude` 参数，并将其传给噪声 `pickElems`，以确保最终图片中每一个「正确答案元素」在视觉上具备**唯一**实例；噪声名 MUST NOT 与任何已选定的正确答案名重复。

#### Scenario: 给定常见配置下，噪声与正确答案不相交

- **WHEN** 后端以「6 noise / 3 correct / 池 = 26 大写字母 + 20 icon ≈ 46 元素」的配置生成一道验证码
- **THEN** 噪声数组与正确答案数组交集 MUST 为空；任何一张图片都不会出现两个视觉相同的「正确答案元素」

#### Scenario: 排除后噪声池仍足够

- **WHEN** 正确答案已经用掉池中 K 个元素，噪声池 SIZE 仍 MUST ≥ `NoiseLength`，否则应直接报错而非回退到「允许噪声名与正确答案名重复」
