# Tasks

> 本 change 当前处于**提案态**：以下任务均未开工。落地前需先完成 0.x 的前置确认，
> 且三个阶段必须串行推进（阶段 N 的对账闸门通过后才开阶段 N+1）。

## 0. 前置确认（动 DDL 之前）

- [ ] 0.1 盘点全部资金语义字段与列，产出「字段 → 列类型 → 阶段归属」清单，
  逐项标注是否参与可累积运算。已知涉及：`mall_settlements`、`mall_settlement_items`、
  `mall_settlement_bills`、`mall_settlement_bill_items`、`mall_platform_ledger`、
  `mall_suppliers`、`users`。
  验证：清单覆盖 `grep -rn "float64" internal/model/` 的全部资金语义命中项，
  且对每个非资金语义项（如 `price` 快照以外的一般数值）给出不纳入的理由。

- [ ] 0.2 数据量级核查：校验 `mall_platform_ledger.amount` 的现有最大值换算为最小单位后
  是否落在 `BIGINT` 范围内（`decimal(20,2)` 的上限换算后会溢出）。
  验证：对真库执行 `SELECT MAX(ABS(amount)) FROM mall_platform_ledger;`，据结果决定该列用
  `BIGINT` 还是 `DECIMAL(24,0)`。

- [ ] 0.3 确认迁移工具链可用：`cmd/migrate` 的 up / down / version 子命令在本地可跑通。
  验证：`go run ./cmd/migrate version` 输出当前版本（当前应为 17）。

## 1. 阶段 1 —— 边界定点化（纯增量，读写路径不变）

- [ ] 1.1 用 `migrate create add_money_cents_shadow_columns` 生成 `000018_*` 成对文件，
  新增 `*_cents` 影子列（台账用 `BIGINT` 或 `DECIMAL(24,0)`，见 0.2）。
  验证：文件名符合 `db-migration` spec 的 6 位版本号与 up/down 成对要求；
  `go run ./cmd/migrate up` 后 `DESCRIBE` 可见新列。

- [ ] 1.2 在同一 migration 中回填：`UPDATE t SET x_cents = ROUND(x * 100)`。
  验证：迁移可重复执行不报错（或明确标注不可重入并在 down 中覆盖）。

- [ ] 1.3 编写对账 SQL 并纳入验收脚本。
  验证：`SELECT ... WHERE ABS(x_cents - ROUND(x * 100)) > 0` 返回 0 行；
  人为插入一行偏差数据后该查询必须能检出（负向对照）。

- [ ] 1.4 阶段 1 闸门：对账零差异 + 线上读写路径未变（无代码改动）。
  验证：`git diff` 中除 migrations 外无业务代码改动。

## 2. 阶段 2 —— 双写 + 切读

- [ ] 2.1 写入路径双写：结算生成、批次账单生成、台账累加、余额增减同时落旧列与新列。
  验证：`go test ./internal/service/... ./internal/repository/...` 全绿，
  且新增用例断言「同一操作后两列值满足 `new == ROUND(old * 100)`」。

- [ ] 2.2 台账累加改用定点绑参（`internal/repository/mall/settlement_repository.go:294`）。
  验证：新增用例 —— 连续累加同一笔金额 N 次后，结果等于该金额与 N 的精确乘积。

- [ ] 2.3 读取路径按表逐个切到新列，每次只切一张表，切后跑一轮对账。
  验证：每张表切换后对账零差异，且该表相关接口的响应与切换前逐字段相同。

- [ ] 2.4 差异告警：对账 SQL 以定时任务或启动自检形式执行，差异非零时输出 ERROR 日志。
  验证：注入偏差数据后日志出现明确 ERROR（负向对照）。

- [ ] 2.5 阶段 2 闸门：双写在生产环境稳定运行至少 2 个完整结算周期且零差异。

## 3. 阶段 3 —— 收敛

- [ ] 3.1 Go 侧替换为定点载体（`int64` 分；费率用整数标度）。
  验证：`internal/model/` 下资金字段不再出现 `float64`；
  换算集中在一处边界辅助函数，`grep -rn "\* 100\|/ 100" internal/` 无业务散落命中。

- [ ] 3.2 移除 `roundHalfUp` 的浮点实现（`internal/service/admin/settlement.go:428`），
  由定点取整取代；结算（`admin/settlement`）与批次账单（`mall/bill`）两条链路口径必须一致。
  验证：新增用例覆盖 half-up 边界值（如 0.005 进位方向）与两条链路结果一致性。

- [ ] 3.3 补契约测试：同一金额在改造前后序列化结果一致。
  验证：`money-precision` spec 的「序列化结果等价」场景有对应用例。

- [ ] 3.4 回归：同一批订单用新旧两条路径重算，结果逐分相同。
  验证：对账脚本在阶段 3 完成后仍零差异。

- [ ] 3.5 确认稳定后，用 `migrate create drop_money_legacy_columns` 删除旧列
  （单向，接受不可回滚）。
  验证：删除后对账 SQL 与接口响应均无异常；上一版本二进制无法直接回滚，需在发版说明中标注。

## 4. 收尾

- [ ] 4.1 `go build ./...` + `go vet ./...` + `go test ./internal/...` 全绿。
- [ ] 4.2 `openspec validate fix-money-float-to-fixed-point` 通过；勾选本文件所有任务。
- [ ] 4.3 更新 `openspec/specs/money-precision/spec.md`（归档本 change 时合并 delta）。
