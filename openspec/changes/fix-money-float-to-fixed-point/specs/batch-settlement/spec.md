# batch-settlement Specification (Delta)

## MODIFIED Requirements

### Requirement: 手续费率快照

系统 SHALL 在生成账单时把手续费率**快照落表**（`commission_rate`），费率口径为 `supplier.commission_rate`，为空时回落 `config.mall.default_commission_rate`。供应商后续调整费率 SHALL 不影响已生成的历史账单。

金额 SHALL 按**逐订单 round 到 2 位小数后求和**的口径计算，与既有周期结算（`admin.settlement`）保持一致，避免「整体 round」与「分笔 round」之间的分位误差。

该口径的**外层语义**（分笔取整、快照、与周期结算一致）保持不变；但承载与运算方式 SHALL 满足 `money-precision` capability 的定点约束 —— 逐订单的手续费与应打款额 MUST 在定点域内计算，账单汇总 MUST 等于明细行的精确和。以 `float64` 计算后再取整 MUST NOT 被视为满足本要求。

#### Scenario: 费率变更不影响历史账单

- **WHEN** 账单已按 10% 生成，之后供应商把 `commission_rate` 改为 15%
- **THEN** 既有账单的 `commission_rate` 与金额保持不变，新生成的账单才使用 15%

#### Scenario: 供应商未设费率时回落默认值

- **WHEN** 供应商 `commission_rate` 为 NULL，系统默认费率为 10%
- **THEN** 账单 `commission_rate` 落为 0.10

#### Scenario: 账单汇总与明细精确一致

- **WHEN** 某批次账单包含 N 笔订单明细
- **THEN** 账单的 `total_amount` / `commission_amount` / `payout_amount` MUST 分别等于对应明细列的精确和
- **AND** 任意两者相差 1 分及以上的情况 MUST 被测试捕获
