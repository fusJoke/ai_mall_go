# Proposal: add-batch-settlement

## Why

现实业务场景：供应商每次上架一个商品 = 进一批货；**库存买完 → 系统做完账 → 结算打款**；同款新一批来到 → 重新走一遍上架流程（复制上架）。现状的周期结算（admin 手动按周期 generate）与该场景不匹配：供应商看不见账单、无法确认、结算粒度太粗。

**商品实例 = 批次 = 结算单位（1:1:1）**。

## What Changes

- `mall_blind_boxes` 加 `lifecycle` 列（selling/sold_out/settled），售罄由 draw 事务内自动触发
- 新表 `mall_settlement_bills`（1:1 商品，blind_box_id UK）+ `mall_settlement_bill_items`（一订单一行快照）
- 账单状态机（domain/state 新增）：generated → confirmed → paid；rejected 可重核（version+1）；failed 可重试
- draw 事务内售罄检测 → 自动生成账单（复用现有资金流，supplier.balance 不变）
- 供应商端：账单中心（列表/详情/确认/驳回）+「再来一批」复制上架（duplicate）
- Admin 端：账单管理（list/detail/regenerate/mark-paid）
- ES 策略：es-sync 只收录 `active ∧ on_sale ∧ selling`；feed 查询 DSL 加同条件 filter（双层防御）

**范围裁定**：供应商入驻申请流（apply/approve/reject）不在本 change，另立 `add-supplier-application`。

## Impact

- Affected specs：批次结算（新增）、商品生命周期（新增）、draw 事务（扩展步骤 7.7 售罄检测）
- Affected code：
  - `internal/model/mall/`（+settlement_bill.go、blind_box.go 加 lifecycle）
  - `internal/domain/state/`（+settlement_bill_state.go）
  - `internal/repository/mall/`（+settlement_bill_repository.go）
  - `internal/service/mall/`（+bill.go 中立账单服务，user/admin 共用）
  - `internal/service/user/draw.go`（事务尾部售罄检测）
  - `internal/service/supplier/`（+bill.go、product.go 加 Duplicate）
  - `internal/handler/`、`internal/router/`（B 端 5 条 + Admin 4 条新路由）
  - `cmd/migrate/migrations/000015_mall_batch_settlement.{up,down}.sql`
  - `cmd/es-sync`（收录条件）、`internal/service/user/home.go`（feed filter）
  - `web/src/`（api/supplier/bill.ts、views/supplier/bills/*、products 增强、admin 账单页）
- 兼容性：现有周期结算代码不动（并存）；draw_order 零改动；C 端零改动
