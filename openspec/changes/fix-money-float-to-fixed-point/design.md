# Design

## Context

DDL 侧金额早已是定点：`mall_settlements` / `mall_settlement_bills` 的
`total_amount` / `commission_amount` / `payout_amount` 是 `decimal(14,2)`，
`mall_settlement_items` / `mall_settlement_bill_items` 是 `decimal(10,2)`，
`mall_platform_ledger.amount` 是 `decimal(20,2)`，`mall_suppliers.balance` 与 `users.balance`
是 `decimal(12,2)`，费率 `commission_rate` 是 `decimal(5,4)`。

Go 侧这些字段全部是 `float64`（`internal/model/mall/settlement.go:51,54,57,60` /
`settlement_bill.go:60,63,66,69` / `supplier.go:42,45,48` / `internal/model/user.go:45`）。

也就是说：**列的精度足够，丢精度发生在 Go 内存里与绑参上**。
MySQL 收到一个 `float64` 绑参时得到的是二进制双精度值，再按目标列 scale 收敛；
而 Go 侧在此之前已经用它做过加减乘。

本设计要回答的是「怎么换、按什么顺序换、怎么保证换的过程中不出错」，而不是「要不要换」。

## Goals / Non-Goals

- **Goals**：金额在服务端全链路以定点承载；迁移过程可回滚、可灰度、可对账；
  对外接口契约不变。
- **Non-Goals**：不改 API 单位与类型；不重新定义业务口径（费率规则、分账规则）；
  不在本 change 内改任何代码或 DDL；不在迁移途中同时替换 `roundHalfUp`。

## Decisions

### D1. 定点载体选 `int64`（分），而非 `shopspring/decimal`

| 方案 | 做法 | 取舍 |
| --- | --- | --- |
| A. `int64` 分（采纳） | DB 侧 `BIGINT`，Go 侧 `int64`；换算集中在边界层（`toCents` / `toYuan`） | 加减乘全为整数运算，天然精确；`int64` 上限 9.22e18 分 = 9.2e16 元，远超业务量级；无需引入依赖；绑参是整数，驱动与 MySQL 无歧义 |
| B. `shopspring/decimal` | Go 侧用任意精度十进制 | 语义最贴合「十进制钱」，但引入新依赖；`decimal.Decimal` 走 `driver.Valuer` 时按**字符串**绑定，与现有 `decimal(14,2)` 列的交互需要逐个字段验证；除法与舍入仍需显式指定 scale，复杂度并未消失 |
| C. 保持 `float64`，只加更多 `roundHalfUp` | 加大取整频次 | 治标不治本：入参已是近似值，取整只是把噪声搬到更小的量级；且每加一次取整就多一处口径分叉 |

采纳 A。**费率是唯一需要非整数精度的量**，用「百万分之一」为单位的整数承载
（`decimal(5,4)` 的 4 位小数 → 1e4 足够，取 1e6 留裕度），
乘法则为 `amount_cents * rate_ppm / 1e6`，除法用整数除法加显式 half-up 进位，
不经过任何浮点。

### D2. 为什么不能只改 Go 类型

GORM 的绑定参数类型由 Go 字段类型推导。把 `float64` 换成 `decimal.Decimal`
**而不动列定义、不动运算表达式**，会得到：

- 绑参从「双精度浮点」变成「字符串/定点」，MySQL 侧的隐式转换路径随之改变；
- 读取路径上 `decimal(14,2)` → `decimal.Decimal` 的 scale 可能被保留为列 scale，
  而既有代码假设它是普通数值，任何 `==` 比较或 `== 0` 判断都可能行为改变；
- `ON DUPLICATE KEY UPDATE amount = amount + VALUES(amount)` 的累加语义，
  会从「列 + 浮点参」变成「列 + 定点参」，舍入点位置改变。

结论：**类型、列、运算三者必须一起换**。本 change 因此把「新增影子列 → 双写 → 切读 → 删旧列」
作为唯一路径，让每一步的语义变化都是可观测、可回滚的。

### D3. 迁移用「影子列 + 回填 + 对账」，而不是原地 ALTER

原地把 `decimal(14,2)` 改成 `BIGINT` 需要一次数据重写，且一旦算错无法对照——
旧值已被覆盖，没有第二份真值。影子列方案下：

- 阶段 1：`ADD COLUMN x_cents BIGINT NOT NULL DEFAULT 0` + `UPDATE t SET x_cents = ROUND(x * 100)`，
  旧列保持不动，对账 SQL 可随时比对两列；
- 阶段 2：写入同时落两列，读取逐步切换；两列都在，差异可量化；
- 阶段 3：确认对账零差异后再删旧列。

`ROUND(x * 100)` 在 MySQL 侧执行，`x` 是 `decimal` 列，运算全程为十进制精确运算，
不存在「用有损类型转换去证明无损」的循环论证。

### D4. 台账（counter）优先于结算单

`mall_platform_ledger` 是唯一的**累加行**（`ON DUPLICATE KEY UPDATE amount = amount + delta`）。
结算单与账单是「一次写入、一次读取」的快照，误差有界（≤ 1 分）；
台账的误差随每次累加**单调增长**，且被用作对账基准，一旦偏移会污染所有下游比对。
供应商余额次之：它直接决定实际打款金额，误差即资金风险。
因此顺序为：`mall_platform_ledger` → `mall_suppliers.balance`（与 `users.balance` 同批，因抽卡链路同时触碰两者）→ 结算单三件套。

### D5. 接口层契约保持「元」

对外（JSON 响应、前端、第三方）继续暴露以**元**为单位的数值，且精度不低于现状（2 位小数）。

- 若输出 JSON number：2 位小数范围内的十进制值在 `float64` 上可精确表示，
  序列化往返安全；但**仅限 2 位小数**，不得输出 3 位以上（那些值必然带浮点表示误差）。
- 若输出十进制字符串：无精度上限，但要求前端与所有消费方接受字符串形态。

本 change 只锁定契约（单位 = 元、精度 ≥ 现状、前端零改动），具体形态在实施阶段按前端兼容性定稿，
并在 tasks 中要求补一组契约测试（同一金额在改造前后序列化结果必须一致）。

### D6. 阶段 3 之前不动 `roundHalfUp`

阶段 1、2 期间线上金额的计算路径完全不变，只有这样「引入了定点列之后线上金额有没有变」
才是一个**可归因**的问题。若同时替换舍入实现，一旦出现金额差异，
无法区分是迁移引起的还是舍入语义改变引起的。`roundHalfUp` 的替换与删除放在阶段 3，
与旧列删除同批。

## Risks / Trade-offs

- **双写期间两列可能不一致**：阶段 2 的对账 SQL 必须作为上线闸门，差异非零即回滚到阶段 1。
- **影子列使表变宽**：`*_cents BIGINT` 每列 8 字节，金额列总量有限，可接受。
- **`ROUND(x * 100)` 对极大值的溢出**：`decimal(20,2)` 的最大值换算成分会超出 `BIGINT`。
  实施时对 `mall_platform_ledger` 需先校验现有数据量级，必要时改 `DECIMAL(24,0)` 而非 `BIGINT`。
- **回滚**：阶段 1/2 的 down 是 `DROP COLUMN`（幂等，丢弃影子列即可回到原状）；
  阶段 3 删除旧列是**单向**的，必须在阶段 2 的对账稳定运行若干个结算周期后再执行。
