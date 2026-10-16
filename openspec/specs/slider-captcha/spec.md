# slider-captcha Specification

## Purpose
为 C 端移动 H5 提供滑块拼图式人机验证能力。现有 `click-captcha` 面向后台登录，采用 350×200 背景图按序点击坐标的交互，在移动端触控场景下体验不佳；滑块验证码以横向拖动拼图的方式完成验证，作为独立能力与点选验证码并存，互不替换。

## Requirements

### Requirement: 滑块验证码创建接口

系统 SHALL 提供 `GET /common/captcha/slider`（公开，无需鉴权）用于创建一个滑块验证码挑战。

响应 MUST 为裸 JSON，包含：`key`（string，本次挑战唯一标识）、`bg_image`（string，`data:image/png;base64,` 前缀的背景图，含缺口）、`slider_image`（string，拼图块图）、`y`（int，缺口纵坐标）、`width`（int）、`height`（int）、`expires_in`（int，有效期秒数，SHALL 为 120）。

本接口 MUST NOT 与既有的 `GET /common/captcha/create`（点选验证码）共享路径或 `key` 空间。两者的 `key` MUST 相互不可解析，`captcha_key` 混用 MUST 被拒绝。

生成的缺口位置 MUST 为随机值，且在挑战生命周期内保持稳定——同一 `key` 重复查询 MUST NOT 改变答案。

#### Scenario: 创建成功

- **WHEN** 客户端请求创建滑块验证码
- **THEN** 系统 MUST 返回 200 与全部必需字段
- **AND** `bg_image` MUST 以 `data:image/png;base64,` 开头
- **AND** `expires_in` MUST 为 120

#### Scenario: 缺口位置稳定

- **WHEN** 同一 `key` 在有效期内被重复获取
- **THEN** 缺口答案 MUST 保持一致

#### Scenario: key 空间与点选隔离

- **WHEN** 将一个由点选验证码创建的 `key` 提交至滑块校验接口
- **THEN** 系统 MUST 拒绝并返回失败，MUST NOT 误判为通过

### Requirement: 滑块预检接口

系统 SHALL 提供 `POST /common/captcha/slider/verify`（公开，无需鉴权）用于对滑块偏移量做预检。

请求体 MUST 包含 `key`、`x`（滑块最终横向偏移）、`w` 与 `h`（前端渲染尺寸），四者均为必需。

成功响应 MUST 为 `{ code: "captcha.ok", message: "verified" }`；失败 MUST 返回 `400 captcha.invalid_input`、`401 captcha.failed` 或 `410 captcha.expired` 之一。

该接口 MUST 为预检语义，校验通过后 `key` MUST NOT 被消费；消费型校验由登录接口在服务端内部完成（见下一节）。

#### Scenario: 预检通过不消费 key

- **WHEN** 提交的 `x` 在容差范围内且 `w`/`h` 与创建时一致
- **THEN** 系统 MUST 返回 `captcha.ok`
- **AND** 该 `key` MUST 仍然有效，可用于后续登录请求

#### Scenario: key 已过期

- **WHEN** 提交的 `key` 超过 `expires_in` 或不存在
- **THEN** 系统 MUST 返回 410 `captcha.expired`

### Requirement: 滑块容差与尺寸约束

横向偏移容差 SHALL 为 ±8 像素，`y` 方向 MUST NOT 参与判定。

`w` 与 `h` MUST 与创建时一致；不一致 MUST 返回 `400 captcha.invalid_input`，MUST NOT 按缩放比例折算。

#### Scenario: 偏移超容差

- **WHEN** 提交的 `x` 与真实缺口位置相差超过 8 像素
- **THEN** 系统 MUST 返回 401 `captcha.failed`

#### Scenario: 尺寸不一致

- **WHEN** 提交的 `w` 或 `h` 与创建时不同
- **THEN** 系统 MUST 返回 400 `captcha.invalid_input`

#### Scenario: 纵向位置不参与判定

- **WHEN** 客户端提交的拼图块纵向位置与缺口 `y` 不同
- **THEN** 判定的结果 MUST 只取决于 `x` 的偏差

### Requirement: 消费型二次校验

**本期登录链路未接入滑块，本需求描述的是滑块（重新）接入登录后的服务端契约**，服务端实现落地时 MUST 按此执行。当前阶段登录接口 MUST NOT 校验滑块字段（见 `user-mobile-login` 的「登录校验顺序与错误码」）。

登录接口在服务端 MUST 对滑块做一次**消费型**二次校验（`deleteOnSuccess=true`）：校验通过后 `key` MUST 立即失效，同一 `key` MUST NOT 可用于第二次登录。

客户端提交的 `captcha_x` MUST 与预检时的 `x` 一致；服务端 MUST 重新按真实缺口位置判定，MUST NOT 信任客户端「已通过预检」的任何声明。

#### Scenario: 登录时消费 key

- **WHEN** 一次使用某 `key` 的登录请求校验通过
- **THEN** 该 `key` MUST 被立即删除
- **AND** 使用同一 `key` 的第二次登录请求 MUST 返回 `login.invalid_captcha`

#### Scenario: 不信任客户端声明

- **WHEN** 客户端未调用预检接口，直接以错误的 `captcha_x` 发起登录
- **THEN** 服务端 MUST 判定失败并返回 401 `login.invalid_captcha`

### Requirement: 失败计数与强制刷新

系统 MUST 为每个滑块 `key` 维护答错计数。

同一 `key` 连续失败达到 3 次时，系统 MUST 立即失效该 `key` 并清零计数，该 `key` MUST 永久不可再用。此后任何针对该 `key` 的请求 MUST 返回过期或失败，客户端 MUST 重新创建挑战。

#### Scenario: 三次失败后强制刷新

- **WHEN** 同一 `key` 的答错次数达到 3 次
- **THEN** 系统 MUST 使该 `key` 失效
- **AND** 后续针对该 `key` 的校验请求 MUST 失败，客户端 MUST 重新获取新挑战

#### Scenario: 成功后计数清零

- **WHEN** 某 `key` 在失败 2 次后校验通过
- **THEN** 该 `key` 的失败计数 MUST 被清除

### Requirement: 客户端滑块组件

客户端 SHALL 提供可复用的滑块验证组件，负责向创建接口取图、渲染背景与拼图块、处理拖动交互、在拖动结束后调用预检接口。

组件 MUST 在预检通过后向上层暴露有效的 `key` 与偏移量 `x`，供登录请求使用。组件 MUST 在 `key` 失效或失败达到 3 次时自动重新创建挑战并重置滑块位置。

**本期登录页 MUST NOT 挂载本组件**（登录页仅手机号 + 短信码）；组件与其依赖的创建 / 预检接口保留备用。重新接入时把组件挂回登录表单、恢复提交前的 `captcha` 门控、并在登录请求体中带上 `captcha_key` / `captcha_x` / `captcha_w` / `captcha_h` 即可，无需改动组件本身。

#### Scenario: 拖动结束后自动预检

- **WHEN** 用户拖动拼图块并松手
- **THEN** 组件 MUST 自动调用预检接口
- **AND** 通过时 MUST NOT 再要求用户点击确认按钮

#### Scenario: 失败自动重置

- **WHEN** 预检返回失败
- **THEN** 组件 MUST 将拼图块复位并允许重试
- **AND** 连续失败 3 次后 MUST 自动获取新挑战

#### Scenario: 登录页不接入

- **WHEN** 审查登录页（`pages/login/index`）
- **THEN** 页面 MUST NOT 渲染滑块组件，提交前 MUST NOT 要求滑块校验通过

### Requirement: 本期实现边界

在本 change 范围内，滑块验证码的服务端实现与客户端组件均 SHALL 由客户端 mock 承接；服务端实现属后续 change。**登录页本期不接入滑块**，因此 mock 的登录实现（`mockLoginMobile`）MUST NOT 校验 `captcha_key`。

mock MUST 满足：创建接口返回内置图片与随机 `key`；预检接口无条件返回通过；滑块能力（组件、两条接口、mock、类型）保留备用，MUST NOT 因登录页未接入而删除。

mock 的响应结构 MUST 与本能力定义的契约完全一致，以便服务端实现落地后客户端零改动切换。

既有 `click-captcha` 的全部行为 MUST 保持不变，本能力 MUST NOT 修改其接口、容差或失败计数规则。

#### Scenario: mock 下可完成流程

- **WHEN** 客户端处于 mock 模式，用户将滑块拖至最右端
- **THEN** 预检 MUST 返回通过，流程可继续至登录

#### Scenario: 点选验证码不受影响

- **WHEN** 后台登录继续使用点选验证码
- **THEN** 其创建、预检、消费与失败计数行为 MUST 与本 change 前完全一致
