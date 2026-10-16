# Design

## 背景约束

1. **dbresolver 的路由是「按语句类型」的，不是「按语义」的。** 它只知道这条语句是 SELECT
   还是 UPDATE，不知道这个 SELECT 的结果会不会被用来决定下一条 UPDATE。因此「判定读走主库」
   这件事**不可能由插件自动完成**，必须在调用点显式声明。
2. **事务内的一切语句都走主库。** 这是 dbresolver 的既有行为（`Sources` 留空时事务强制
   source）。所以「把先读后写包进同一个事务」是一条现成的、零新增基础设施的解法。
3. **从库在本地就是真从库。** `mysql-slave` 容器跑的是真实的 `CHANGE MASTER TO` +
   `START SLAVE`，因此可以用 `STOP REPLICA SQL_THREAD` 精确制造延迟，不需要引入
   延迟代理或 mock。
4. **「判定读」的集合很小。** 全仓扫描后，结果用于门控写行为的读集中在：存在性校验、
   父子/祖先守卫、唯一性预检、环检测、条件更新前的状态读。数量在个位数量级，
   逐条改造是可行的；若数量级在数百，本方案不成立。

---

## D1：为什么不做「全局 sticky / 所有读走主库」

**决策：不做。** 判定读走主库，普通读继续走从库。

理由：
- 一刀切走主库等于关闭读写分离，主库扛下全部读流量 —— 本 change 的收益（正确性）
  以牺牲既有容量规划为代价，且这个代价是**永久的**，不是过渡期的。
- sticky 方案（写后 N 秒内该会话读走主库）需要把「最近写过」的状态挂到会话上。
  本服务的请求作用域 DB 由 `database.DBMiddleware` 每请求从全局实例派生一次
  （`c.Set(CtxKey, base.WithContext(...))`），**没有跨请求的会话载体** —— 要实现 sticky
  就得引入 Redis 标记 + 网关/中间件判定，成本与本 change 完全不成比例。
- 收益侧站不住：真正需要「读到刚写的值」的读，是那些**要与刚提交的写做比对**的读。
  列表页晚 50ms 看到一条新记录，用户无感；判定读晚 50ms，就是一次错误写入。

**权衡**：需要人工维护一份「判定读清单」。清单腐化（新增判定读时忘了走主库）是主要风险，
用 D4 的验证基建兜底 —— 只要有对应的延迟注入用例，漏改会在 CI 上确定性失败，而不是偶发。

---

## D2：为什么「条件写」优于「先读判定、后写」

**决策：能下沉到 DB 的守卫一律下沉为条件写；不能下沉的才用「读主库 + 后写」。**

`HasChildren` 的两种写法对比：

```go
// 写法 A：先读判定、后写（现状，即使读走主库也仍有窗口）
ok, err := repo.HasChildren(c, id)      // SELECT
if err != nil { return err }
if ok { return ErrHasChildren }
repo.Delete(c, id)                      // UPDATE
// 两个语句之间：另一个请求插入子节点 → 父被删，子变孤儿

// 写法 B：条件写（DB 串行化判定，无窗口）
tx.Model(&AdminRule{}).
   Where("id = ?", id).
   Where("NOT EXISTS (SELECT 1 FROM admin_rule c WHERE c.pid = admin_rule.id AND c.deleted_at IS NULL)").
   Update("deleted_at", now)
// RowsAffected == 0 → 要么行不存在，要么有子 → 由调用方按存在性再区分
```

写法 B 的额外收益：`RowsAffected == 0` 把「被并发请求抢先」这类情况自然地变成
一次可观测的失败，而不是静默的错误写入。这与仓库既有的
`IncrementLoginFailure`（`user_repository.go:210`）与余额扣减
（`DecrementBalance`，影响 0 行 → `ErrInsufficientBalance`）是同一范式，
**本 change 不引入新范式，只是把它推广到守卫类判定**。

**代价**：写法 B 的 SQL 更难读，且 `RowsAffected == 0` 的两个原因（不存在 / 有子）
需要在应用层再区分一次 —— 这一次区分读是**无害的**（它只影响报哪个错误码，
不影响写不写），可以走从库。

**边界**：环检测（`ValidatePID`：新 pid 是否为自身或自身后代）无法表达成单条条件写，
它需要遍历祖先链。这类守卫保留在应用层，但**必须走主库**。

---

## D3：为什么加显式通道而不是给 `DB()` 加隐式 sticky 状态

**决策：新增 `WriteDB(c)` / `ReadDB(c)`，`DB(c)` 语义保持不变。**

```go
// internal/repository/db.go
func DB(c *gin.Context) *gorm.DB      { return database.FromContext(c) }        // 自动路由（不变）
func WriteDB(c *gin.Context) *gorm.DB { return DB(c).Clauses(dbresolver.Write) } // 强制主库
func ReadDB(c *gin.Context) *gorm.DB  { return DB(c).Clauses(dbresolver.Read) }  // 强制从库
```

关键性质：两者都是**子句级**的、`*gorm.DB` 的派生值，不改 `Statement` 之外的任何状态。

理由：
- **不加隐式状态**：若让 `DB()` 自己记住「本请求写过没有」，就必须在 gin.Context 上挂标记，
  而这个标记会在事务、`c.Copy()`（`RunInTx` 里用过）、中间件里出现语义歧义 ——
  「事务里写了、copy 出去的 ctx 知不知道？」这类问题会让一致性变成靠约定维持的隐式契约。
- **不改 `DB()` 默认值**：全仓 12 处 `NewBaseRepository` 与大量直接 `repository.DB(c)` 调用
  都依赖现状；改默认值等于隐式改变全部读的路由，会同时引入正确性变化与容量变化，
  无法与新代码做因果分离。显式新增通道则保证「没改的地方行为一模一样」。
- **`ReadDB` 存在的意义**：它让「这条读我确认容忍 stale」成为**写在代码里的声明**，
  而不需要读者去推断「哦这里没包事务所以走从库」。D5 的清单靠它落地。

**已知边界**：`Clauses(dbresolver.Write)` 与既有的事务路径叠加时，
事务句柄本身已锁定到主库连接，再叠 `Clauses` 不会出错（dbresolver 在事务态下直接
返回 source），但会让代码读起来多一层。约定：**已在事务内就不再叠 `Clauses`**。

---

## D4：验证为什么用「注入延迟」而不是「等它抖动」

**决策：用 `STOP REPLICA SQL_THREAD` 制造确定性延迟，把偶发失败变成必然失败。**

现状的问题不是「测不出来」，而是「测出来不可信」—— 9.6 里这两条用例的失败**看起来**
像环境噪声，需要跑一次 `read.enabled=false` 的对照实验才能确认它是真问题。
这个确认过程本身不可复现，下次仍然要重来。

做法：

```
STOP REPLICA SQL_THREAD;        -- 从库停止应用 binlog，延迟无上界
  执行判定读相关的端到端用例       -- 修复前：必然失败；修复后：必然通过
START REPLICA SQL_THREAD;       -- 恢复
```

配合一个前置断言（先确认从库确实在复制、再停线程），避免「从库本来就是断的」让用例
假通过。这套 harness 只在 `//go:build integration` 下启用，对单元测试零影响。

**为什么不测 `Seconds_Behind_Master`**：它是**秒级**粒度，毫秒窗口恒为 0，
用它做断言永远通过 —— 这正是这个缺陷长期没被发现的原因之一。

---

## D5：收益边界的守门方式

**决策：在 spec 里显式枚举「允许读从库」的路径类别，并要求用 `ReadDB` 标注。**

允许读从库的类别（收敛为「读的结果不门控任何写」）：

- 列表 / 分页查询；
- 详情展示（用户看到的商品、订单、规则详情）；
- 首页 feed 与各类降级路径；
- 统计 / 看板类聚合（口径本身容忍分钟级延迟）。

必须读主库的类别：

- 存在性校验（建/删的前置条件）；
- 父子、祖先守卫；
- 唯一性预检（`ExistsByIDsAndName` 这类）；
- 条件更新前的状态读（余额、库存、状态机当前态）。

**可验证性**：性能回归时对比主从两侧的 `Com_select` 增量。若某类列表接口修复后
主库 `Com_select` 明显上升，说明 `ReadDB` 标注漏了或某处误用了 `WriteDB`。

**接受的残留风险**：清单靠人工维护。本 change 不为它引入静态检查（Go 侧的
「这个 read 的结果是否被用于 if」无法用 lint 表达）。以 D4 的用例集为事实上的护栏：
新加一条判定读而没加用例，就没有护栏；这是本方案明确接受的缺口，写在 design 里而不是
假装不存在。
