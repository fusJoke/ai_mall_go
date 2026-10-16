# Proposal

## Why

`.env.yaml` 的 `database.read.enabled: true` 让 `gorm.io/plugin/dbresolver` 把 SQL 按类型分流：

| 操作 | 目标 |
| --- | --- |
| `Create` / `Update` / `Delete` / `Exec` / 事务内一切语句 | 主库 `127.0.0.1:3307` |
| `Find` / `First` / `Take` / `Raw`（非事务） | 从库 `127.0.0.1:3308` |

MySQL 是**异步复制**：主库提交后，从库应用该 binlog 有一段毫秒到秒级的窗口。窗口期内从库
返回的是「上一个版本」的数据。这本身是读写分离的正常代价，问题在于**现有代码里有一类读
不是普通读，而是写操作的判定依据**：

```
HasChildren(id)  ──SELECT(从库)──►  false  ──►  DELETE(主库)   ← 判定错了，有子的父被删
GetPID(id)       ──SELECT(从库)──►  ErrNotFound ──► 跳过/放行
ExistsByIDsAndName(...)  ──SELECT(从库)──►  false  ──►  Create(主库)  ← 唯一性没拦住
```

判定读的结果要与「刚由**上一个请求**提交的写」相比对，而它却走了从库。这不是理论风险，
是在 `add-admin-rule-management` 的 9.6 端到端复验里**实测复现**的：

| 用例 | 从库读 ON（现状） | 从库读 OFF（对照） |
| --- | --- | --- |
| `POST /admin/rule/batch-delete {ids: [父(有子), 叶子, 不存在]}` | ❌ `{"deleted":2,"skipped":1}` —— **有子节点的父被误删** | ✅ `{"deleted":1,"skipped":2}` |
| 删掉子规则后立刻删父 | ❌ `400 rule.delete.has_children` —— **父删不掉** | ✅ `200` |
| 同批验证中的 D1–D4 四条实现缺陷 | ❌ 全部复现 | ❌ **全部原样复现** |

最后一行是对照实验的关键：`read.enabled=false` 重启后**只有**两条时序用例由 FAIL 转 PASS，
D1–D4 一条不少 —— 干净地把「复制延迟抖动」与「真实实现缺陷」分开了。也就是说这两条用例
既不是实现 bug，也不是测试写错，而是**缺一条架构级不变量**。

更难查的是它的表现形态：从库本身完全健康（`Slave_IO_Running` / `Slave_SQL_Running` = Yes，
`Seconds_Behind_Master=0`），秒级粒度的监控指标**看不见毫秒级窗口**；用 `docker exec mysql`
做探针也测不出（探针自身开销几十毫秒 > 窗口）。于是缺陷只在 CI 上偶发失败、在本地永远
复现不出来，且**两个方向都会错**（该删的没删、不该删的删了），排查成本极高。

## What Changes

本 change **只做提案与设计，不改代码**。核心是引入一条横切不变量，并把它落到可执行的位置：

1. **显式读写通道**：在 `internal/repository` 暴露 `WriteDB(c)` / `ReadDB(c)`，与现有
   `DB(c)`（保持 dbresolver 自动路由）并存。需要读主库的地方不再依赖「碰巧没被路由走」
   或「自己包一层事务」这种隐式手法。
2. **判定读一律走主库**：所有**结果用于决定后续写行为**的读（存在性、父子守卫、唯一性
   预检、环检测）改用 `WriteDB(c)`。这些读在本请求/本进程内都拿不到「延迟容忍」的收益 ——
   它们错一次就是一次错误写入。
3. **守卫判定下沉到 DB**：能表达成「条件写」的守卫改为单事务内的
   `UPDATE ... WHERE NOT EXISTS (...)` 或 `SELECT ... FOR UPDATE`，让 DB 串行化判定，
   彻底消除窗口。这与仓库里已经确立的 `IncrementLoginFailure` 模式一致
   （见 `internal/repository/user/user_repository.go:210` 的注释）。
4. **验证基建**：把「偶发抖动」变成「确定性失败」。用 `STOP REPLICA SQL_THREAD` 注入
   人为复制延迟，写出可复现的端到端用例；这类用例在修复前**必然失败**，修复后**必然通过**。
5. **守住收益边界**：明确枚举**允许读从库**的路径（列表页、详情页、首页 feed 降级等
   容忍 stale 的读），它们 MUST NOT 被顺手改成走主库 —— 否则读写分离白做。

推进顺序：先落 ①②（无行为变化，只是多了通道），再逐条把守卫读迁到 ③，
最后用 ④ 加固回归。每一步可独立回滚。

### 非目标

- **不做跨请求的全局强一致（session sticky）**：不引入「用户写后 N 秒内其所有读都走主库」
  这类会话级粘滞。它要求把状态挂到会话/网关上，成本远超收益；本 change 只要求
  **判定读**走主库，普通读继续享受从库。
- **不引入分布式事务 / 中间件**：不引入 ProxySQL、不引入 GTID 等待、不做
  `WAIT_FOR_EXECUTED_GTID_SET` 之类的读后同步等待。
- **不改 DDL、不改 API 契约、不改前端**：全部改动落在 Go 侧的取连接方式上，
  对外行为除「判定不再出错」外无变化。
- **不新增业务能力**：不重新定义删除/创建的业务规则，不调整错误码。
  `admin-rule-management` 的两条需求（`Delete a rule with children refused`、
  `Batch delete mixed selection`）文本**不需要修改** —— 它们描述的行为一直是对的，
  错的是实现。本 change 是让实现回到 spec。
- **不在本 change 内关闭读写分离**：`read.enabled=false` 只是验证用的对照手段，
  不是修复方案。

## Capabilities

### New Capabilities

- `db-read-write-consistency`：主从分流下「判定读与写」的一致性契约。这是一条横切不变量
  （跨全部 repository 与 service），不属于任何单一业务 capability，故独立成篇。

### Modified Capabilities

无。现有 capability 的需求描述的都是**对外可观测行为**，这些行为本就应该成立；
本 change 不改变任何一个对外契约，只改变实现取连接的方式。按 openspec 惯例，
「实现违反了未变更的 spec」由实现修复，不改写 spec。

## Impact

- **代码（本 change 不动，实施时改）**：
  - `internal/repository/db.go` —— 新增 `WriteDB(c)` / `ReadDB(c)`；
  - `internal/repository/admin/rule_repository.go` —— `HasChildren` / `GetPID` /
    `ExistsByIDsAndName`（判定读）迁到写库或改条件写；
  - `internal/repository/base.go` —— 通用 CRUD 的 `Delete` / `Update` 若带前置判定，
    同款处理；
  - `internal/service/admin/rule.go` —— `Delete` / `BatchDelete` 的判定-然后-写序列
    合并进事务；
  - `internal/infra/database/database.go` —— 已具备 dbresolver 配置，预计无需改动
    （`Clauses(dbresolver.Write)` / `Clauses(dbresolver.Read)` 是**子句级**开关，
    在 repository 层使用即可，不必动插件装配）。
- **数据**：无 DDL、无数据迁移、无回填。
- **前端**：无改动。
- **验证**：
  - 新增「复制延迟注入」集成用例（`STOP REPLICA SQL_THREAD` / `START REPLICA`），
    对每条判定读路径各一条：修复前 FAIL、修复后 PASS；
  - 修复前后各跑一遍 9.6 口径的端到端脚本（`batch-delete mixed` +
    「删子后立刻删父」），要求从 FAIL 转 PASS 且其余 35 项断言不变；
  - 性能侧：确认列表类接口的读仍落在从库（可用 `SHOW STATUS LIKE 'Com_select'`
    对主从两侧计数，或看 dbresolver 的 SQL 日志），避免「一刀切全走主库」的隐性退化。
- **既有缓解点（不重复处理）**：`user_repository.IncrementLoginFailure` 已用
  单事务内回读解决；本 change 把它作为**范式引用**，而非改造对象。
