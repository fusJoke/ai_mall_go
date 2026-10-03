# Tasks: add-batch-settlement

> 执行方式（superpowers 流程）：逐任务 TDD——先写失败测试再实现，每任务完成即 `git commit`；任务内标注验收命令。

## 1. 数据层与状态机

- [x] 1.1 迁移 `000015_mall_batch_settlement.{up,down}.sql`：`mall_blind_boxes` 加 `lifecycle varchar(16) NOT NULL DEFAULT 'selling'` + 索引 `(supplier_id, lifecycle)`；建 `mall_settlement_bills`（`blind_box_id` UK + `(supplier_id,status,created_at)` + `(status,created_at)`）；建 `mall_settlement_bill_items`（`draw_order_id` UK + `(bill_id)`）。验收：`migrate up` + down 可逆
- [x] 1.2 `internal/model/mall/settlement_bill.go`：`MallSettlementBill` / `MallSettlementBillItem` + `MallSettlementBillStatus` 枚举（generated/confirmed/rejected/failed/paid）；`blind_box.go` 加 `MallBlindBoxLifecycle` 常量（Selling/SoldOut/Settled）+ `lifecycle` 字段；`mall.go` 注册。验收：`go build ./...`
- [x] 1.3 `internal/domain/state/settlement_bill_state.go`：`BillStates` 集合 + 合法转换表（generated→confirmed/rejected；confirmed→paid/failed；rejected→generated；failed→confirmed；paid 终态）+ 单测（合法/自迁/跳跃/逆向/跨类型全拒绝）。验收：`go test ./internal/domain/state/`

## 2. Repository 层

- [x] 2.1 `internal/repository/mall/settlement_bill_repository.go`：`CreateWithItems`（账单+明细同事务）、`GetByID`（带 items）、`GetByBoxID`、`ListBySupplier`（分页+status 筛选）、`ListAdmin`（分页+status+supplier 筛选）、`UpdateStatus`（CAS：WHERE status=期望态，返回 RowsAffected）、`Regenerate`（金额更新+version+1+回 generated，CAS）+ sqlmock 单测。验收：`go test ./internal/repository/mall/`

## 3. Service 层（TDD：先测后码）

- [x] 3.1 `internal/service/mall/bill.go`（中立包，user/admin 共用）：`GenerateForBox(tx, boxID)`（聚合该商品全部 drawn 订单→按 commission_rate 快照算 commission/payout→建账单+明细，幂等：已有账单则跳过）、`MarkPaid`（confirmed→paid CAS + `supplier.balance -= payout` 同事务 + 商品置 settled）、`Regenerate`。+ 单测。验收：`go test ./internal/service/mall/`
- [x] 3.2 `internal/service/user/draw.go` 事务尾部（普通+秒杀两处，7.5/7.6 之后加 7.7）：扣减后 `pool_total_stock=0` → 盲盒 `lifecycle: selling→sold_out`（CAS）→ 调 `GenerateForBox` 同事务建账单。+ 单测（未售罄不触发/售罄触发一次/重复触发幂等）。验收：`go test ./internal/service/user/`
- [ ] 3.3 `internal/service/supplier/bill.go`：List（自己的）/Detail（校验归属）/Confirm（generated→confirmed）/Reject（generated→rejected，必填原因）。+ 单测
- [ ] 3.4 `internal/service/supplier/product.go` 加 `Duplicate`：校验归属→事务内复制商品+卡池（weight 结构保留、stock 置 0、名称建议"第 N 期"）→ 返回新 boxID。+ 单测

## 4. Handler 与 Router

- [ ] 4.1 `internal/handler/supplier/bill.go`（list/detail/confirm/reject）+ `product.go` 加 Duplicate handler；`internal/router/supplier/` 注册 5 条（均 SupplierAuth）。+ handler 测试
- [ ] 4.2 `internal/handler/admin/bill.go`（list/detail/regenerate/mark-paid）；`internal/router/admin/mall.go` 注册 4 条（AdminAuth）。+ handler 测试

## 5. 前端（B 端 + Admin）

- [ ] 5.1 `web/src/api/supplier/bill.ts`（类型+list/detail/confirm/reject，snake_case+request.request 惯例）；`product.ts` 加 `duplicateProduct`
- [ ] 5.2 `views/supplier/bills/index.vue`（状态 tab+账单卡片+空态）、`detail.vue`（金额三联+订单明细+确认/驳回弹窗）；路由 + 侧边栏「账单中心」
- [ ] 5.3 `products/index.vue` 增强：生命周期徽章 + 销售进度条 + 「再来一批」按钮 + 「从历史复制」入口 + 状态筛选 tab；`products/edit.vue`：创建默认下架提示、`sold_out/settled` 禁改卡池
- [ ] 5.4 `web/src/api/admin/bill.ts` + `views/admin/bills/`（列表：状态 tab；操作：打款二次确认显示扣减额、重核弹窗改金额）；admin 路由 + 菜单
- [ ] 5.5 验收：`npm run type-check` + `npm run build` 通过

## 6. ES 视图层

- [ ] 6.1 `cmd/es-sync`：收录查询加 `lifecycle='selling'` 条件；diff 模式——Search 现有 `_id` 集与 MySQL 在售全集对比，`Delete` 差集（防售罄文档滞留）
- [ ] 6.2 `internal/service/user/home.go` feed 查询 DSL 加 `filter: [active, on_sale, selling]`（读时防线）。验收：`go test ./internal/service/user/`

## 7. 验证与收尾

- [ ] 7.1 `go build ./... && go test ./... && go vet ./internal/...`（新改包必须过）
- [ ] 7.2 端到端冒烟（seed 数据）：上架商品→连续抽卡至售罄→账单自动生成→供应商确认→admin 打款→商品 settled、balance 扣减正确
- [ ] 7.3 openspec 归档：本 change 标记完成，`add-supplier-application`（入驻流）另立 change
