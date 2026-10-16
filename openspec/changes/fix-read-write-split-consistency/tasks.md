# Tasks

> 本 change 当前处于**提案态**：以下任务均未开工。落地顺序为 0 → 1 → 2 → 3 → 4 → 5，
> 其中 1、2 是纯增量（不改任何既有行为），3 才开始改变运行路径。
> 3 的每一条子任务必须**先有 2 的用例、再看它由 FAIL 转 PASS**，不允许「先改代码、
> 后补用例」—— 那会把「修复」与「碰巧不抖了」混在一起，无法区分。

## 0. 前置确认（动代码之前）

- [ ] 0.1 全仓盘点**判定读**清单：产出「文件:方法 → 判定类型 → 处置方式」表格，
  逐条标注是迁到主库（`WriteDB`）还是改成条件写。已知命中：
  `rule_repository.HasChildren`、`rule_repository.GetPID`、
  `rule_repository.ExistsByIDsAndName`、`service/admin/rule.ValidatePID`（环检测）。
  验证：清单需覆盖 `grep -rn "HasChildren\|ExistsBy\|GetPID\|ValidatePID\|FOR UPDATE" --include=*.go internal/`
  的全部非测试命中项；对每个未纳入的方法给出「其读不门控写」的理由。

- [ ] 0.2 盘点**容忍 stale 的读**清单，收敛为 proposal 里枚举的四类（列表 / 详情 /
  feed 与降级 / 聚合），并逐条确认其读结果不参与任何 if 分支去决定写。
  验证：对每个列表类 handler 反查其 service → repository 调用链，确认链上无写操作。

- [ ] 0.3 确认本地主从复制可用且可控：`mysql-slave` 容器可执行 `STOP REPLICA SQL_THREAD`
  与 `START REPLICA SQL_THREAD`（MySQL 8.0.22+ 语法；旧语法 `STOP SLAVE` 亦需可用）。
  验证：手工停线程后 `SHOW REPLICA STATUS` 的 `Replica_SQL_Running` 为 `No`，
  起线程后恢复 `Yes`；且 `SHOW REPLICAS` 在停之前显示两个线程均为 `Yes`。

- [ ] 0.4 确认 dbresolver 子句级开关在当前版本可用：`Clauses(dbresolver.Write)` /
  `Clauses(dbresolver.Read)` 能改变单条语句的路由。
  验证：写一个临时集成用例，在主从两侧分别查 `@@port`（或 `SHOW VARIABLES LIKE 'server_id'`），
  确认默认走 3308、叠 `Write` 子句后走 3307。

## 1. 基础设施：显式读写通道（纯增量，不改行为）

- [ ] 1.1 在 `internal/repository/db.go` 新增 `WriteDB(c)` / `ReadDB(c)`，
  `DB(c)` 语义保持不变。
  验证：三者的集成用例各一条，断言落到的主库/从库端口符合预期。

- [ ] 1.2 为三者补齐文档注释，写明「何时该用哪个」的判据
  （判定读 → `WriteDB`；容忍 stale → `ReadDB`；其余 → `DB`）与
  「已在事务内不再叠 `Clauses`」的约定。
  验证：`go vet ./internal/repository/...` 干净；注释覆盖 design D3 的全部结论。

- [ ] 1.3 确认 `WriteDB` 与既有 `RunInTx` 路径叠加时不冲突
  （事务态下 dbresolver 直接返回 source）。
  验证：新增一条「事务内调 `WriteDB`」的集成用例，断言 SQL 仍落在主库且事务边界正常。

## 2. 验证基建：可注入的复制延迟（纯增量，不改行为）

- [ ] 2.1 实现 `//go:build integration` 下的 harness：
  `assertReplicaHealthy(t)` → `stopReplicaSQLThread(t)` → `defer startReplicaSQLThread(t)`。
  要求先确认复制健康（两个线程均 Yes），并在 `defer` 中无条件恢复。
  验证：先手工 `STOP` 从库线程，再跑 harness —— 必须在 `assertReplicaHealthy` 处失败，
  证明「本来就断的从库」不会让用例假通过。

- [ ] 2.2 为 harness 补一条自检用例：停线程后，一条通过默认通道的 `SELECT` 读不到
  刚在主库提交的行。
  验证：该用例在停线程下稳定复现「读不到」，起线程后稳定读到；连跑 5 次结果一致
  （排除偶发）。

- [ ] 2.3 写「缺陷复现用例」：`batch-delete [父(有子), 叶子, 不存在]` 期望
  `{deleted:1, skipped:2}`；「删子后立刻删父」期望 `200`。
  验证：**这两条在停线程 + 修复前的实现下必须 FAIL** —— 这是它们作为护栏的前提，
  若不 FAIL 说明用例没打到点上，需先修用例。

## 3. 改造判定读（开始改变运行路径，逐条独立可回滚）

- [ ] 3.1 `rule_repository.HasChildren`：删除判定改为单事务内的条件写
  （`UPDATE ... WHERE NOT EXISTS (子行)`），以 `RowsAffected` 区分「有子」与「不存在」。
  验证：2.3 的两条用例由 FAIL 转 PASS；`TestRepository_HasChildren_NotFound` 与
  集成用例 `TestIntegration_HasChildren`（有子/无子/不存在三态）保持 PASS。

- [ ] 3.2 `rule_repository.GetPID`：迁移到 `WriteDB`（存在性判定读，走主库即可，
  无需条件写）。
  验证：2.3 的用例保持 PASS；`TestIntegration_GetPID_NotFound` 保持 PASS。

- [ ] 3.3 `service/admin/rule.ValidatePID` 的环检测：祖先链遍历迁移到 `WriteDB`。
  验证：环检测的既有单测全部保持 PASS；新增一条「停从库线程下环检测仍生效」的用例。

- [ ] 3.4 `rule_repository.ExistsByIDsAndName`：评估是否可改为条件写；不可则迁 `WriteDB`。
  验证：既有唯一性用例保持 PASS；结论（改条件写 / 只迁主库）写入 design D2 的边界说明。

- [ ] 3.5 通用 CRUD（`internal/repository/base.go`）的 `Delete` / `Update`：确认其内部
  无前置判定读；若有，同款处理。`GetByID` 属通用读，**保持默认通道**。
  验证：`grep` 确认 base.go 无判定读；如确认无改动，在 tasks 里标注「确认无需改动」而非留空。

- [ ] 3.6 复核 `user_repository.IncrementLoginFailure`：确认其事务内回读路径仍然正确，
  本 change 不改它，只把它登记为范式引用。
  验证：既有并发错密码用例保持 PASS；注释中补一句指向本 capability。

## 4. 回归与收益验证

- [ ] 4.1 重跑 9.6 口径的端到端脚本（37 项断言）：要求 `batch-delete mixed` 与
  「删子后立刻删父」由 FAIL 转 PASS，且其余 35 项断言一条不变。
  验证：脚本输出 37 通过 / 0 失败；与复验 v3 的 37/37 逐项对比，确认无回归。

- [ ] 4.2 在**不注入延迟**的干净环境下同样跑一遍 4.1，确认修复没有引入新的失败。
  验证：两种环境（正常 / 停线程）结果一致。

- [ ] 4.3 收益边界验证：确认列表类接口的读仍落在从库，未因本次改动被顺手改成主库。
  验证：对比修复前后主从两侧 `Com_select` 增量，列表接口调用 N 次后主库增量不变
  或仅由判定读引起（可逐条对应到 0.1 的清单）。

- [ ] 4.4 并发压力下的确定性复验：对 `batch-delete` 与删除链路做并发压测，
  确认条件写路径在竞争下不出现「该删的没删 / 不该删的删了」。
  验证：并发 N 轮后，主子关系无孤儿（`SELECT ... WHERE pid NOT IN (SELECT id ...)` 返回 0 行）。

- [ ] 4.5 全量门禁：`gofmt`（LF 归一后）0 违规、`go build ./...`、
  `go vet ./internal/...`（含 `-tags=integration`）、`go test -count=1`、
  `go test -tags=integration`（真连 MySQL）、`openspec validate --all`。
  验证：全部通过，且 `openspec validate --all` 的 passed 数为 28（新增 1 个 spec）。

## 5. 归档

- [ ] 5.1 归档前自检：确认本 change 的 delta 只使用 `## ADDED Requirements`
  （首个引入该 capability），且无其他未归档 change 同时改 `db-read-write-consistency`。
  验证：逐个数 `## ` 章节；`ls openspec/changes/*/specs/db-read-write-consistency` 只命中本 change。

- [ ] 5.2 `openspec archive fix-read-write-split-consistency --yes`，确认
  `openspec/specs/db-read-write-consistency/spec.md` 生成且需求数为 5。
  验证：`openspec validate --all` 仍 0 failed；`openspec list` 不再出现本 change。

- [ ] 5.3 把「判定读清单」的最终形态写入 `openspec/specs/db-read-write-consistency/`
  或最近的 daily log —— 让下一个新增判定读的人知道该走哪条通道。
  验证：清单可直接对应到代码里的 `WriteDB` 调用点。
