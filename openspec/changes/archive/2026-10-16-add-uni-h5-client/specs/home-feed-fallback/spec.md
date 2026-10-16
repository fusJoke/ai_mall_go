## Purpose

为 C 端首页 Feed（`GET /user/home/feed`）提供 Elasticsearch 不可用时的 MySQL 降级通道，消除「ES 抖动即首页白屏」的线上隐患。本能力同时修订 `add-trading-card-blindbox-mvp` 中「ES 不可用时不查 MySQL 兜底」的既有约束。

降级的核心要求是**对客户端完全透明**：降级响应的结构与正常路径同构，客户端无需任何感知或分支处理。

## ADDED Requirements

### Requirement: Feed 降级触发条件

系统 SHALL 在首页 Feed 的主检索路径（Elasticsearch）不可用时，自动回落到 MySQL 查询，MUST NOT 直接向客户端返回失败。

触发条件与排除条件见下方场景。降级路径的 MySQL 查询 MUST 复用与 `cmd/es-sync`
**逐字一致**的收录口径：盲盒 `status=active AND on_sale=true AND lifecycle='selling'`
且供应商 `status=active`（**不筛 `is_featured`**，自 change `add-batch-settlement`
起已降为排序字段）；供应商 `status=active AND is_featured=true`。两侧口径不一致
会使降级返回不同的商品集合，破坏本能力「对客户端完全透明」的核心要求。

#### Scenario: ES 不可用触发降级

- **WHEN** ES 查询返回连接错误或超时
- **THEN** 系统 MUST 转而查询 MySQL 并返回结果
- **AND** 响应 MUST NOT 为 500

#### Scenario: ES 返回空结果不降级

- **WHEN** ES 查询成功但命中 0 条记录
- **THEN** 系统 MUST 返回 `blind_boxes` 与 `suppliers` 均为空数组的响应
- **AND** MUST NOT 因「结果为空」触发 MySQL 降级

#### Scenario: MySQL 亦不可用

- **WHEN** ES 与 MySQL 降级查询均失败
- **THEN** 系统 MUST 返回 500 与 `search_unavailable` 错误码

### Requirement: 降级响应与主路径结构同构

降级响应 MUST 与 ES 路径的响应**结构完全一致**，包括 `blind_boxes` 数组元素的全部字段、`suppliers` 数组元素的全部字段，以及 `fetched_at`。

客户端 MUST NOT 需要读取任何额外字段或响应头才能正确渲染降级结果。

`fetched_at` MUST 为本次降级时刻的时间戳。

#### Scenario: 客户端无感知

- **WHEN** 同一客户端分别收到 ES 路径与降级路径的响应
- **THEN** 两个响应的字段集合 MUST 完全一致
- **AND** 客户端 MUST NOT 需要任何条件分支来渲染降级结果

#### Scenario: 空数组而非 null

- **WHEN** 降级查询未命中任何记录
- **THEN** `blind_boxes` 与 `suppliers` MUST 为空数组，MUST NOT 为 `null`

### Requirement: 降级路径的活动价处理

盲盒 Feed 条目含 `promo_price` 字段。主路径该值来自索引预计算投影，降级路径 MUST 按下述规则处理：

- 能够确定活动价时 MUST 填充真实值。
- 无法确定时 MUST 置为 `null`（字段省略或显式为 `null` 均可）。

系统 MUST NOT 在降级路径返回未经校验的价格值 —— 展示原价是可接受的降级体验，展示错误价格是资损风险。

#### Scenario: 活动价可确定为 null

- **WHEN** 降级路径无法确定某盲盒的活动价
- **THEN** 该条目的 `promo_price` MUST 为 `null` 或字段缺省
- **AND** 该条目的 `price` MUST 仍为真实原价

#### Scenario: 不返回错误价格

- **WHEN** 降级路径产出盲盒条目
- **THEN** 任何价格字段的值 MUST 来源于数据库真实字段，MUST NOT 为估算或默认填充值

### Requirement: 降级结果的缓存回填约束

降级结果 MUST NOT 按主路径的正常 TTL 回填 Feed 缓存。

实现 MUST 满足以下二者之一：完全不回填缓存；或回填但使用显著更短的 TTL（不超过 30 秒）。

该约束的目的是防止 ES 恢复后，用户仍长时间看到降级期间产生的陈旧数据。

#### Scenario: 降级不污染正常缓存

- **WHEN** 一次请求走了降级路径并设置了 Feed 缓存
- **THEN** 该缓存项的 TTL MUST 不超过 30 秒
- **AND** ES 恢复后的请求 MUST 在 TTL 到期后取到主路径数据

#### Scenario: 主路径恢复后回填正常

- **WHEN** ES 从故障中恢复，且降级期间写入的短 TTL 缓存已过期
- **THEN** 系统 MUST 走 ES 路径并将结果按正常 TTL 回填缓存

### Requirement: 降级可观测性

系统 MUST 让降级行为可被观测，MUST NOT 静默降级。

- 降级发生时 MUST 记录 WARN 级别日志，内容包含触发原因、失败的索引名与耗时。
- 响应 MUST 携带标识数据来源的响应头，取值区分主路径与降级路径。

该响应头仅用于排障与灰度观察，MUST NOT 成为客户端逻辑的依赖。

#### Scenario: 降级被记录

- **WHEN** 请求因 ES 故障走降级路径
- **THEN** 系统 MUST 输出 WARN 日志
- **AND** 日志 MUST 包含触发原因与失败索引名

#### Scenario: 来源标识

- **WHEN** 客户端收到 Feed 响应
- **THEN** 响应 MUST 携带标明数据来源的响应头
- **AND** 客户端 MUST NOT 依赖该响应头决定渲染逻辑

### Requirement: 主路径超时约束

主检索路径 MUST 配置短超时（建议量级 800 毫秒），以保证故障时能及时转入降级，MUST NOT 让用户请求长时间阻塞在等待 ES 响应上。

降级路径的 MySQL 查询 MUST NOT 引入新的长等待。

#### Scenario: ES 超时后及时降级

- **WHEN** ES 响应时间超过配置的超时阈值
- **THEN** 系统 MUST 在规定时间内判定为不可用并转入降级
- **AND** 用户请求的端到端耗时应保持在可接受范围

### Requirement: 对既有约束的修订

本能力修订 `add-trading-card-blindbox-mvp` 中「Elasticsearch 不可用时不查 MySQL 兜底」的约束。

修订后：ES 不可用时 SHALL 降级到 MySQL 查询，MUST NOT 直接返回失败。

该既有 change 归档时 MUST 同步修订其相关表述，以避免两份规格对同一接口给出互斥约束。

#### Scenario: 约束以下降级为准

- **WHEN** 审视任一描述 `GET /user/home/feed` 故障行为的规格
- **THEN** 该规格 MUST 表述为「ES 不可用时降级到 MySQL」
- **AND** MUST NOT 存在与之互斥的「不查 MySQL 兜底」表述
