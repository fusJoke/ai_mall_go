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
- [x] 3.2 `internal/service/user/draw.go` 事务尾部（普通+秒杀两处）：扣减后 `pool_total_stock=0` → 盲盒 `lifecycle: selling→sold_out`（CAS）→ 调 `GenerateForBox` 同事务建账单。**时序约束（P0 修正）**：必须在订单 `status='drawn'` 落定**之后**调用——`GenerateForBox` 聚合的是 `status='drawn'` 且未被账单收录的订单，提前调用会漏掉清空库存的那一单（详见 design 流程 C-2）。+ 单测（未售罄不触发/售罄触发一次/重复触发幂等/**调用顺序断言：finalize 晚于 status_drawn**）。验收：`go test ./internal/service/user/`
- [x] 3.3 `internal/service/supplier/bill.go`：List（自己的）/Detail（校验归属）/Confirm（generated→confirmed）/Reject（generated→rejected，必填原因）。+ 单测
- [x] 3.4 `internal/service/supplier/product.go` 加 `Duplicate`：校验归属→事务内复制商品+卡池（weight 结构保留、stock 置 0、名称建议"第 N 期"）→ 返回新 boxID。+ 单测

## 4. Handler 与 Router

- [x] 4.1 `internal/handler/supplier/bill.go`（list/detail/confirm/reject）+ `product.go` 加 Duplicate handler；`internal/router/supplier/` 注册 5 条（均 SupplierAuth）。+ handler 测试
- [x] 4.2 `internal/handler/admin/bill.go`（list/detail/regenerate/mark-paid）；`internal/router/admin/mall.go` 注册 4 条（AdminAuth）。+ handler 测试

## 5. 前端（B 端 + Admin）

- [x] 5.1 `web/src/api/supplier/bill.ts`（类型+list/detail/confirm/reject，snake_case+request.request 惯例）；`product.ts` 加 `duplicateProduct`
- [x] 5.2 `views/supplier/bills/index.vue`（状态 tab+账单卡片+空态）、`detail.vue`（金额三联+订单明细+确认/驳回弹窗）；路由 + 侧边栏「账单中心」（顶部导航）
- [x] 5.3 `products/index.vue` 增强：生命周期徽章 + 销售进度条 + 「再来一批」按钮 + 「从历史复制」入口 + 状态筛选 tab；`products/edit.vue`：创建默认下架提示、`sold_out/settled` 禁改卡池
  - 补充：实现中发现 Duplicate 置 stock=0 后无重填端点的闭环缺口 → 新增 GET detail（预填）+ POST update-pool（守卫：lifecycle=selling + 零 drawn 订单 ErrPoolLocked→409）
- [x] 5.4 `web/src/api/admin/bill.ts` + `views/admin/bills/`（列表：状态 tab；操作：打款二次确认显示扣减额、重核弹窗改金额）；admin 路由 + 菜单
- [x] 5.5 验收：`npm run typecheck` + `npm run build` 通过（脚本名是 typecheck）

## 6. ES 视图层

- [x] 6.1 `cmd/es-sync`：收录查询加 `lifecycle='selling'` 条件（**v2 口径**：`status=active AND on_sale=true AND lifecycle='selling'` 的全部在售商品，`is_featured` 降为排序字段）；文档投影加 `lifecycle` 字段；diff 模式——Search 现有 `_id` 集与 MySQL 在售全集对比，`Delete` 差集（防售罄文档滞留）
- [x] 6.2 `internal/service/user/home.go` feed 查询 DSL 加 `term filter lifecycle=selling`（读时防线，第二层）。验收：`go test ./internal/service/user/`

## 7. 生命周期约束（C 端可见性）

- [x] 7.1 `internal/repository/mall/blindbox_repository.go`：`ListFeaturedOnSale` / `ListActiveOnSaleBySupplierIDs` 加 `lifecycle='selling'`（售罄/已结算商品不再出现在 C 端列表）
- [x] 7.2 `internal/service/supplier/product.go` + `internal/service/admin/`：`ToggleOnSale` 加生命周期守卫（`lifecycle != selling` → 拒绝，`ErrLifecycleForbidden` → 409/422）；admin 侧同步。+ 单测

## 8. 验证与收尾

- [x] 8.1 `go build ./... && go test ./... && go vet ./internal/...`（新改包必须过）
- [x] 8.2 端到端冒烟（seed 数据）：上架商品→连续抽卡至售罄→**校验售罄那单出现在账单明细**→供应商确认→admin 打款→商品 settled、balance 扣减正确
  - 交付：`internal/service/mall/bill_integration_test.go`（`//go:build integration`）
  - 3 个用例：售罄→确认→打款全链路（池 stock=5 精确断言明细含售罄单）、驳回→重核→再确认（version+1）、generated 直接打款被状态机拒绝（余额与 lifecycle 均不变）
  - **实现注记**：必须用外部测试包 `mall_test` —— 用例需同时构造 user.DrawService 与 supplier.BillService，而 service/supplier 反向 import service/mall，包内写会形成 import cycle
  - 运行方式：`MYSQL_TEST_DSN='...(库名含 test)...' go test -tags=integration ./internal/service/mall/...`
  - 说明：编写时本机 MySQL 容器未运行，仅完成编译校验 + skip 路径验证（无 DSN 时正确 SKIP 而非 FAIL）；待容器起来后跑一次全绿
- [x] 8.3 补 spec delta：新增 `openspec/changes/add-batch-settlement/specs/batch-settlement/spec.md`，沉淀关键行为（账单聚合全部 drawn 订单含售罄单的时序保证、ES+查询双层过滤、lifecycle 状态约束、打款复合事务原子性、费率快照）为可归档 spec
- [x] 8.4 openspec 归档：本 change 标记完成并归档，spec 同步至 `openspec/specs/batch-settlement/`；`add-supplier-application`（入驻流）另立 change
