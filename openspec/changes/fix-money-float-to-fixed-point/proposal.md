# Proposal

## Why

金额在 DB 侧是定点（`decimal(14,2)` / `decimal(12,2)` / `decimal(5,4)` / `decimal(20,2)`），
在 Go 侧却全部声明为 `float64`。同一条金额跨过语言边界时从「精确十进制」退化为
「IEEE-754 二进制浮点」——`0.1`、`0.2`、`0.07` 这类在十进制下有限的小数在二进制下是无限循环小数，
只能取最近可表示值。这本身不致命；致命的是钱要参与三类**可累积**的运算：

1. **结算计算**：`commission = price * rate`、`payout = price - commission`
   （`internal/service/admin/settlement.go:253`、`internal/service/mall/bill.go:173`）。
   费率 `decimal(5,4)` 天然是四位小数，与订单价格相乘后必然落在二进制不可表示点上。
2. **台账累加**：`internal/repository/mall/settlement_repository.go:294` 的
   `ON DUPLICATE KEY UPDATE amount = amount + VALUES(amount)` —— 绑参是 `float64`。
   累加行的误差**单调累积**，跑得越久偏得越远，且永远不会自己回来。
3. **余额增减**：供应商 `balance = balance + actual_price` / `balance - payout`、
   用户 `balance = balance - price`（抽卡扣款）。这两列直接决定**打款金额**。

现有实现已经写了一层 `roundHalfUp(v float64, decimals int) float64`
（`internal/service/admin/settlement.go:428`），但它是在**已经被浮点化之后**的入参上取整——
四舍五入只能消掉尾数噪声，消不掉「入参本身已经是近似值」这件事。`roundHalfUp` 自己还用
`math.Pow(10, n)` 做移位，引入第二重误差。

后果不是「理论上不精确」，而是对账时账单汇总与订单明细之和可以差 1 分以上，
且差异随订单量与时间增长，无法通过重算消除。

## What Changes

本 change **只做提案与设计，不改 DDL、不改代码**。实施按三阶段推进，每阶段可独立上线与回滚：

| 阶段 | 动作 | 风险 |
| --- | --- | --- |
| 1. 边界定点化 | 新增 `*_cents BIGINT` 影子列；用 `ROUND(x * 100)` 回填；加对账 SQL 断言两列一致 | 低（纯增量，读写路径均不变） |
| 2. 双写 + 切读 | 写入同时落两列；读取按表逐个切到新列；对账差异触发告警 | 中 |
| 3. 收敛 | 删旧列；Go 侧改用定点载体；接口层仍以「元」暴露 | 中 |

实施顺序（按误差危害排序，非按表顺序）：

1. `mall_platform_ledger.amount` —— 累加行，误差单调累积；
2. `mall_suppliers.balance` —— 直接决定打款金额；
3. 结算单三件套（`mall_settlements` / `mall_settlement_bills` 及其 items）；
4. `users.balance` —— 与 ② 同批切换（抽卡扣款链路 `draw.go` 的 `DecrementBalance`）。

### 非目标

- **不改接口层的金额单位与类型**：对外仍是「元」，前端零改动。定点化是**服务端内部**的承载方式，
  不应外溢成 API 破坏性变更。
- **不在本 change 内把 model 字段从 `float64` 直接改成 `decimal.Decimal`**：
  GORM 会按 Go 类型推导列类型与绑定参数，只改类型而不动列、不动运算，只会把
  「浮点误差」换成「缩放/舍入行为不一致」，是更难查的一类 bug（见 design D2）。
- **不动 `roundHalfUp` 的服务范围**：阶段 1、2 期间它仍是线上唯一舍入口径；只有阶段 3
  切到定点运算后才由定点取整取代。提前替换会同时改变线上金额，无法与迁移做因果分离。
- **不在此单内扩展业务口径**：不重新定义费率规则、不引入多币种、不改分账规则。

## Capabilities

### New Capabilities

- `money-precision`：金额的承载、换算、运算、迁移与接口表示的统一约束。
  这是一条横切不变量（跨 settlement / supplier / ledger / user 四条链路），
  不适合塞进任何单一业务 capability，故独立成篇。

### Modified Capabilities

- `batch-settlement`：手续费率快照的金额计算口径由「逐订单 round 后求和」升级为
  「定点运算后求和」——外层口径（分笔取整、快照语义）保持不变，改变的是承载与运算方式。

## Impact

- **代码**：`internal/model/mall/{settlement,settlement_bill,supplier}.go`、
  `internal/model/user.go`、`internal/service/{admin/settlement,mall/bill,user/draw}.go`、
  `internal/repository/mall/settlement_repository.go`。以上均为**阶段 2/3 的改动**，本 change 不动。
- **迁移**：`cmd/migrate/migrations/` 新增 `000018` 起的 up/down 成对文件，
  命名与成对要求遵循 `db-migration` spec；需准备可回滚的 down（丢影子列，幂等）。
- **数据**：影子列回填是纯增量，不回写旧列；对账 SQL 用于阶段性验收，
  不通过则不进入阶段 2。
- **前端**：无改动。
- **验证**：阶段 1 用对账 SQL（`ABS(x_cents - ROUND(x*100)) > 0` 必须返回 0 行）；
  阶段 3 用「同一批订单重算，新旧路径结果必须逐分相同」的回归用例。
