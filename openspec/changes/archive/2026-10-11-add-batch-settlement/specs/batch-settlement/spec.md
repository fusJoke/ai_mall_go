# Spec Delta

## Purpose

「商品即批次」结算模型：供应商每上架一个商品 = 进一批货，库存买完即自动做账，结算单位从「周期」收窄为「商品实例」。本 delta 沉淀三条可归档的行为契约——**账单聚合口径**（含售罄那一单的时序保证）、**生命周期的可见性约束**（ES 收录与 C 端查询双层过滤）、**资金操作的原子性**（打款复合事务与状态机防线）。

这套机制解决三个既有问题：供应商看不见账单、无法确认金额、结算粒度太粗（周期结算要求 admin 手动按窗口 generate，与实际「一批货一次结算」的业务节奏不匹配）。商品、批次、结算单位三者是 1:1:1 关系，由 `mall_settlement_bills.blind_box_id` 的唯一索引强制。

## ADDED Requirements

### Requirement: 售罄自动生成批次账单

系统 SHALL 在抽卡事务内检测池内总库存归零，并把该商品的 `lifecycle` 由 `selling` 原子推进为 `sold_out`，随后为**该商品**生成一张批次账单。账单生成 SHALL 与抽卡事务同属一个数据库事务——任一步失败则整体回滚，用户不会为「没有账单的抽卡」付款。

账单 SHALL 聚合该商品下**全部 `status='drawn'` 的订单**（而非仅未结算订单的增量），并为每个订单写入一行明细快照（`amount` / `commission_amount` / `payout_amount`）。

**时序约束（关键，P0）**：售罄检测与账单生成 MUST 在订单状态落定为 `drawn` **之后**执行。若提前调用，清空库存的那一笔订单会漏出账单聚合结果，导致供应商少结算一单。

#### Scenario: 售罄那一单必须出现在账单明细

- **WHEN** 某商品池内总库存为 N，用户连续抽卡 N 次恰好将库存抽至 0
- **THEN** 抽卡事务尾部触发账单生成，且账单明细行数等于 N，**包含最后清空库存的那一笔订单**

#### Scenario: 账单与抽卡事务原子提交

- **WHEN** 售罄检测通过但账单落库失败（如唯一键冲突、DB 错误）
- **THEN** 整个抽卡事务回滚，订单不落库，用户余额不被扣减，商品 `lifecycle` 不推进

#### Scenario: 未售罄不触发

- **WHEN** 用户抽卡后池内总库存仍大于 0
- **THEN** 不生成账单，商品 `lifecycle` 保持 `selling`

#### Scenario: 重复触发幂等

- **WHEN** 同一商品因并发抢先后重放、或补偿任务重跑而再次触发售罄落定
- **THEN** `lifecycle` CAS 影响 0 行（幂等继续），`GenerateForBox` 检测到已有账单直接返回既有账单，不产生第二张账单

### Requirement: 账单状态机与资金操作原子性

系统 SHALL 通过状态机约束账单流转：`generated → confirmed → paid`；`generated → rejected → generated`（重核，`version` +1）；`confirmed → failed → confirmed`。`paid` 为终态。任何非法转换（含自迁移、跳跃、逆向）SHALL 被拒绝。

admin 打款 SHALL 在**单个复合事务**内完成三件事：账单 `confirmed → paid` 的 CAS、扣减供应商余额、商品 `sold_out → settled` 的生命周期落定。任一步影响 0 行（并发抢先、余额不足、脏数据）则整体回滚。

#### Scenario: 跳过供应商确认直接打款被拒

- **WHEN** 账单处于 `generated` 状态，admin 调用打款
- **THEN** 系统返回状态机非法转换错误，账单状态、供应商余额、商品 `lifecycle` 均不变

#### Scenario: 打款复合事务全成功

- **WHEN** 账单已 `confirmed`，供应商余额充足，商品 `lifecycle` 为 `sold_out`
- **THEN** 账单置 `paid`、供应商余额扣减 `payout_amount`、商品 `lifecycle` 置 `settled`，三者同时生效

#### Scenario: 余额不足时打款整体失败

- **WHEN** 账单已 `confirmed`，但供应商余额小于 `payout_amount`
- **THEN** 系统返回余额不足错误，且账单**不**变为 `paid`（CAS 随事务回滚）

#### Scenario: 驳回后重核可再次确认

- **WHEN** 供应商驳回账单（`generated → rejected`），admin 重核并更新金额
- **THEN** 账单回到 `generated`、`version` 加 1、金额被整体覆盖为新值，供应商可再次确认

### Requirement: 手续费率快照

系统 SHALL 在生成账单时把手续费率**快照落表**（`commission_rate`），费率口径为 `supplier.commission_rate`，为空时回落 `config.mall.default_commission_rate`。供应商后续调整费率 SHALL 不影响已生成的历史账单。

金额 SHALL 按**逐订单 round 到 2 位小数后求和**的口径计算，与既有周期结算（`admin.settlement`）保持一致，避免「整体 round」与「分笔 round」之间的分位误差。

#### Scenario: 费率变更不影响历史账单

- **WHEN** 账单已按 10% 生成，之后供应商把 `commission_rate` 改为 15%
- **THEN** 既有账单的 `commission_rate` 与金额保持不变，新生成的账单才使用 15%

#### Scenario: 供应商未设费率时回落默认值

- **WHEN** 供应商 `commission_rate` 为 NULL，系统默认费率为 10%
- **THEN** 账单 `commission_rate` 落为 0.10

### Requirement: 商品生命周期约束 C 端可见性

系统 SHALL 用 `lifecycle` 列（`selling` / `sold_out` / `settled`）表达商品的批次阶段，并据此约束 C 端可见性与供应商操作：

- ES 收录 SHALL 仅纳入 `status=active ∧ on_sale=true ∧ lifecycle='selling'` 的商品；`es-sync` 的 diff 模式 SHALL 对比现有文档 `_id` 集与 MySQL 在售全集并删除差集，防止售罄文档滞留。
- C 端 feed 查询 DSL SHALL 额外附加 `term filter lifecycle=selling`，作为读时第二层防线。
- 供应商 `ToggleOnSale` SHALL 拒绝操作 `lifecycle != selling` 的商品（返回 `ErrLifecycleForbidden`）。

两层过滤是有意冗余：ES 侧收敛索引体积，查询侧兜住索引延迟与数据漂移。

#### Scenario: 售罄商品从 C 端消失

- **WHEN** 某商品 `lifecycle` 推进为 `sold_out`
- **THEN** 它不再出现在 ES 首页 feed 与 C 端商品列表中

#### Scenario: 已结算商品禁止重新上架

- **WHEN** 供应商尝试对 `lifecycle = settled` 的商品执行上架操作
- **THEN** 系统返回生命周期禁止错误，`on_sale` 保持不变

#### Scenario: 索引延迟时查询侧仍能兜住

- **WHEN** 商品已售罄但 ES 文档尚未同步删除
- **THEN** C 端 feed 查询因 `lifecycle=selling` 过滤而不返回该商品

### Requirement: 批次账单的可观测接口

系统 SHALL 提供供应商端与 admin 端两组账单读取接口：

- 供应商端 SHALL 能列出**自己**的账单（按 `id` 倒序，可按 `status` 筛选）、查看账单详情（校验归属，越权返回未找到）、确认、驳回（驳回必填原因）。
- admin 端 SHALL 能列出全量账单（可按 `status` 与 `supplier_id` 筛选）、查看详情、重核、打款。

#### Scenario: 供应商不能看到他人账单

- **WHEN** 供应商 A 请求供应商 B 的账单详情
- **THEN** 系统返回未找到，不泄露该账单是否存在

#### Scenario: 驳回必须填写原因

- **WHEN** 供应商提交驳回但原因为空或仅空白字符
- **THEN** 系统拒绝该请求，账单保持 `generated`
