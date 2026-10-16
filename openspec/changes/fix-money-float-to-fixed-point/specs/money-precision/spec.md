# money-precision Specification (Delta)

## ADDED Requirements

### Requirement: 金额的定点承载

服务端所有**具有资金语义**的字段 MUST 以定点形式承载，禁止以 IEEE-754 二进制浮点承载。
判定「具有资金语义」的标准是：该字段参与过至少一种可累积运算（累加、余额增减、跨行汇总），
或作为资金划转的依据。

- DB 列 MUST 为定点类型（`DECIMAL` 或等价的整数最小单位列）。
- Go 侧的载体 MUST 为整数最小单位（`int64` 分）或等价的精确十进制类型；
  MUST NOT 为 `float64`。
- 元与最小单位之间的换算 MUST 收口在唯一的边界辅助函数中，
  MUST NOT 在业务代码中散落 `* 100` / `/ 100`。

费率这类非整数比例 SHOULD 以整数标度承载（如百万分之一），
其乘法 MUST 为整数运算加显式进位，MUST NOT 经过浮点。

#### Scenario: 累加行不使用浮点载体

- **WHEN** 台账类累加表（如 `mall_platform_ledger`）以 `ON DUPLICATE KEY UPDATE amount = amount + delta` 累加
- **THEN** 绑参 MUST 为定点/整数形式，累加结果 MUST 与逐笔精确求和的结果逐分相同
- **AND** 连续累加 N 次同一笔金额后，结果 MUST 等于该金额与 N 的精确乘积，不随次数漂移

#### Scenario: 余额增减不使用浮点载体

- **WHEN** 供应商余额或用户余额发生增减
- **THEN** 增减量与余额本身均为定点载体
- **AND** 无论操作序列如何，余额 MUST 等于所有历史增减的精确代数和

### Requirement: 金额运算与舍入

金额运算 MUST 在定点域内完成；任何需要舍入的中间步骤 MUST 使用显式、可复现的舍入规则，
且该规则 MUST 在结算与批次账单两条链路上保持一致。

舍入 MUST NOT 被用作「浮点误差的补救措施」——若一个值在参与运算前已经是有损近似的，
对其再取整不构成精度保证。

#### Scenario: 逐笔取整后求和与汇总口径一致

- **WHEN** 对 N 笔订单分别计算手续费与应打款额后求和
- **THEN** 汇总值 MUST 等于逐笔定点计算结果的精确和
- **AND** 账单汇总 MUST 等于其明细行的精确和（不允许汇总与明细相差 1 分）

#### Scenario: 舍入规则可复现

- **WHEN** 同一组输入被重复计算任意次
- **THEN** 输出 MUST 逐分相同
- **AND** 舍入规则 MUST 为四舍五入（half away from zero）或已明确定义并被测试覆盖的等价规则

### Requirement: 金额列的迁移与回填

金额列从一种承载形式迁移到另一种时 MUST 满足：

- 迁移 MUST 为增量式（新增影子列 + 回填 + 双写 + 切读 + 删旧列），
  MUST NOT 以一次原地改写覆盖唯一一份真值。
- 回填 MUST 在 DB 侧以定点运算完成（如 `ROUND(x * 100)`），
  MUST NOT 借道 Go 侧浮点类型做中转。
- 每个阶段之间 MUST 有对账断言作为闸门；差异非零时 MUST NOT 推进下一阶段。
- 迁移 MUST 提供可执行的回滚脚本；删除旧列的阶段为单向操作，
  只在双写对账稳定运行若干个结算周期后执行。

#### Scenario: 回填后两列零差异

- **WHEN** 影子列回填完成
- **THEN** 对账查询（形如 `SELECT ... WHERE ABS(x_cents - ROUND(x * 100)) > 0`）MUST 返回 0 行
- **AND** 存在差异行时 MUST 阻止进入双写阶段

#### Scenario: 回滚可回到迁移前状态

- **WHEN** 对影子列迁移执行 down
- **THEN** 影子列被移除，旧列与其取值保持不变

### Requirement: 金额的接口表示

对内改为定点承载 MUST NOT 改变对外契约：

- 对外金额单位 MUST 仍为「元」，精度 MUST 不低于改造前（2 位小数）。
- 若以 JSON number 输出，MUST NOT 输出超过 2 位小数的金额。
- 改造前后，同一笔金额的序列化结果 MUST 一致；消费方 MUST NOT 需要任何适配改动。

#### Scenario: 前端无需改动

- **WHEN** 服务端完成定点化改造
- **THEN** 既有前端读取同一接口，金额显示、表单提交与校验行为均不发生变化

#### Scenario: 序列化结果等价

- **WHEN** 对同一笔金额分别在改造前与改造后序列化
- **THEN** 两次结果 MUST 相同
