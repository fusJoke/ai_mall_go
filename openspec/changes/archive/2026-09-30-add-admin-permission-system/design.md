# Design

## Context

`internal/model/admin.go` 已落地 `Admin` 与 `AdminRule`（后者由 `add-admin-rule-model` 在 baseline 之后引入）。`internal/model.All()` 自注册表已在 `database.Init()` 启动期被消费过（虽然 schema 已切到 golang-migrate，但注册路径仍保留给运行时一致性检查使用）。`internal/repository/` 与 `internal/service/` 目前是空目录，需要从零搭建 `admin` 子目录。

`CLAUDE.md` 明文规定分层与公共时间戳字段；`add-config-model/design.md` 与 `add-admin-rule-model` 的实现惯例均沿用 `namer.TableName(...)` + `Register(...)` + `gorm.DeletedAt` 软删除 + `created_at/updated_at` 自动维护这套范式，本 change 不引入新的 ORM 概念。

业务诉求：5 个权限管理方法（`GetRules` / `Check` / `GetGroups` / `GetRuleIds` / `IsSuperAdmin`）+ `*` 通配的超管识别。详见 `proposal.md` Why 与 `specs/admin-permission/spec.md`。

## Goals / Non-Goals

**Goals:**

- 落地 2 个新模型 + 1 张关系表的迁移；
- 按 CLAUDE.md 分层完成 Repository 与 Permission Manager；
- Permission Manager 的 5 个公开方法覆盖所有 `specs/admin-permission` 中定义的需求；
- 不引入新的第三方依赖（继续用 GORM + 标准库）。

**Non-Goals:**

- 不实现 handler 层鉴权中间件 —— 后续独立 change；
- 不实现后台【角色管理】/【菜单管理】UI —— 后续独立 change；
- 不实现 Redis 缓存 / sync.Map 缓存 —— 性能优化后续独立 change；
- 不预填任何 seed 数据（如「超级管理组」） —— 后续 seed change；
- 不修改既有 capability（admin-login / admin-logout / click-captcha / homepage）。

## Decisions

### D1. 三个模型同文件 `internal/model/admin.go`

`Admin` / `AdminRule` / `AdminGroup` / `AdminGroupAccess` 都是 admin 域相关。CLAUDE.md 明文「Model 不设子目录，一个文件可包含多个相关模型」。

- **理由**：同域聚合、注释风格一致、注册一次性完成。
- **替代方案**：按聚合根拆成 `admin.go` / `admin_rule.go` / `admin_group.go` / `admin_group_access.go` —— 拒绝，理由：文件碎片化、重复引用 schema 包。

### D2. 公共时间戳字段（created_at / updated_at / deleted_at）按 CLAUDE.md 强制补齐

User SQL 中 `admin_group` / `admin_group_access` 无这三个字段。CLAUDE.md 明文「每张业务表都需包含」，与 `add-admin-rule-model` 的 `D1` 处理方式一致。

- **理由**：与项目惯例对齐；后台未来需要审计谁在何时改了哪条；软删除可避免误删系统关键组。
- **替代方案**：严格按 user SQL 不补 —— 拒绝，理由：与既有模型风格不一致、CLAUDE.md 已明文要求。
- **细节**：`admin_group_access` 是关系表，`updated_at` 仍按惯例补齐（即便关系不易变更）。若后续证明不需要，可在独立 change 里调整。

### D3. `admin_group_access` 用复合主键 `(uid, group_id)`

User SQL 只声明 `INDEX uid / INDEX group_id`，未声明主键。关系表复合 PK 是更严格的表达「这个 uid 在这个 group 里」。

- **理由**：天然唯一、表达更准确；不需要额外的 `id` 字段；删除「(uid, group_id)」关系 = 直接删一行 PK。
- **替代方案 1**：自增 `id` + `UNIQUE(uid, group_id)` —— 拒绝，理由：多余的 `id` 列、Repository 代码要查 id 才能删。
- **替代方案 2**：跟随 user SQL 不建 PK —— 拒绝，理由：缺主键的表在 GORM 中行为不一致，且无 PK 的关系表语义模糊。
- **实现**：在 Go 结构体上同时给 `Uid` 加 `primaryKey;autoIncrement:false` 与 `GroupID` 加 `primaryKey;autoIncrement:false`，或者将两者声明为组合主键 —— GORM 支持 `gorm:"primaryKey"` 同时打两个字段。优先方案：在 SQL 里显式 `PRIMARY KEY (uid, group_id)`，模型 tag 用两个 `primaryKey`。

### D4. `admin_group.rules` 在 Go 用 `string`，由 Manager 解析

User SQL 字段类型 `text`，内容是逗号分隔的 `admin_rule.id` 列表，特殊值 `*`。

- **理由**：与 SQL 类型 1:1；逗号分隔文本无序列；解析逻辑集中在 Manager 一处，便于处理「`*` 通配」「空格容忍」「非数字段跳过」三种边界。
- **替代方案 1**：拆出独立的 `admin_group_rule` 关系表 —— 拒绝，理由：user SQL 明确用 text，超管语义 '*' 与文本字段契合；新建关系表会破坏 user SQL 的设计意图。
- **替代方案 2**：Go 端用 `[]uint` 自动序列化 —— 拒绝，理由：JSON / GORM 自动序列化复杂、跨驱动一致性差。
- **解析契约**（在 Manager 内部函数 `parseRules(string) (wildcard bool, ids []uint)` 中）：
  - 整串 `"*"` → wildcard=true, ids=nil；
  - 空字符串 → wildcard=false, ids=nil；
  - 其他 → 按 `,` split，每段 `strings.TrimSpace`，跳过空段与非数字段，`strconv.ParseUint(s, 10, 64)` 收集。

### D5. Repository 按聚合根拆四份

- `internal/repository/admin/admin_repository.go`：单条查询 `GetByID(uid)`（用于 Manager 入口处的 admin 存在性 + status 校验）。
- `internal/repository/admin/rule_repository.go`：CRUD on `admin_rule`。
- `internal/repository/admin/group_repository.go`：CRUD on `admin_group` + 解析 `rules` 字段的辅助方法（如 `ParseRuleIDs(s string) ([]uint, error)`）。
- `internal/repository/admin/access_repository.go`：按 uid 查 group_id 列表；按 uid+group_id 删；按 group_id 反查 uid 列表。

- **理由**：每个 Repository 单一职责，CLAUDE.md 「Repository 只做 DB 搬运」约束下最自然；Manager 入口的 admin 状态校验是独立关注点，分文件便于 mock 与替换。
- **替代方案**：合并成单一 `PermissionRepository` —— 拒绝，理由：违反 CLAUDE.md 「Repository 只做 DB 搬运、不掺业务」且方法数过多难维护。
- **替代方案 2**：把 admin 校验逻辑塞进 AccessRepository —— 拒绝，理由：跨聚合根职责混乱；admin 状态变更（启用 / 禁用）不影响分组关系。

### D6. Permission Manager 放 `internal/service/admin/permission.go`

- **理由**：CLAUDE.md 「Service → 业务逻辑编排」，权限 Manager 显然是业务编排层（跨 Repository 聚合、status/deleted_at 过滤、`*` 通配语义）。
- **替代方案**：放 `internal/infra/permission/` —— 拒绝，理由：与 CLAUDE.md「internal/infra 是基础设施」定义不符；权限含业务规则，不属于「database/config/captcha/token/upload」一类。
- **类名与构造**：`Manager` 类型，构造用 `New(repos Repositories) *Manager`（依赖注入 Repository 集合，便于测试用 mock 替换）；也提供 `Default()` 包级函数从 `database.Get()` + 全局 Repository 实例构造。

### D7. 不在本 change 加缓存

Permission Manager 的 5 个方法每次都查 DB。

- **理由**：scope 控制 —— 落地数据底座、跑通业务逻辑；性能与一致性权衡留待后续 change。
- **替代方案**：加 sync.Map 缓存（按 uid 缓存规则列表） —— 拒绝，理由：缓存一致性（组规则变化时如何失效）需独立设计；后续单独 change 处理。

### D8. `*` 通配优先级：组内任一即超管（OR 语义）

- **理由**：管理员可以同时属于多个组；只要其中一个组的 `rules` 字段含 `*`，该管理员就是超管。
- **替代方案**：要求所有组都含 `*` 才是超管（AND 语义） —— 拒绝，理由：与「任一分组授予超管」的常见直觉相反；与 admin-user 系统（如 FastAdmin / ThinkAdmin）惯例不符。
- **短路优化**：解析阶段任一 group 的 `rules == "*"` → 立即标记 super=true，跳过该 admin 的其余 group 解析。

### D9. status=0 / 软删除过滤由 Manager 层负责（不放 Repository）

- **理由**：CLAUDE.md 「Repository 只做 DB 搬运，不掺业务」。`status` 是业务字段（启用/禁用），属于业务规则，由 Manager 决定过滤。
- **GORM 自动软删**：`gorm.DeletedAt` 字段在 GORM 查询时会自动加 `deleted_at IS NULL`，无需手动处理；Manager 只需确认模型字段类型正确。
- **显式 status=1**：Repository 方法默认不加 status 过滤；Manager 在调用 Repository 后**自己**用 GORM `Where("status = ?", 1)` 收紧。

### D10. 查询模式：批量化避免 N+1

Permission Manager 的内部查询全部使用批量查询（`IN` / `Where` 一次命中）：

- `AccessRepository.ListGroupIDsByUID(uid)`：单次查询拿全部 group_id。
- `GroupRepository.ListByIDs(ids []uint)`：单次 `WHERE id IN (?)` 拿全部组。
- `RuleRepository.ListByIDs(ids []uint)`：单次 `WHERE id IN (?)` 拿全部规则。

- **理由**：每个管理员的权限检查最多 3 次 DB 查询（不论组数 / 规则数）。
- **替代方案**：单条 Repository 调用循环 —— 拒绝，理由：典型反模式，10 个组就会 10 次查询。

### D11. 数据流（Permission.Check 路径）

```
Check(uid uint, ruleName string) bool:
  admin = AdminRepository.GetByID(uid)          // 顺便校验存在 + status=1
  if admin == nil or admin.Status == 0: return false
  groupIDs = AccessRepository.ListGroupIDsByUID(uid)
  if len(groupIDs) == 0: return false
  groups = GroupRepository.ListByIDs(groupIDs)   // 过滤 deleted_at (GORM 自动)
  for g in groups:
    if g.Status == 0: continue                   // Manager 层 status 过滤
    wildcard, ids = GroupRepository.ParseRuleIDs(g.Rules)
    if wildcard: return true                     // 超管短路
  ruleIDs = union of all ids
  if len(ruleIDs) == 0: return false
  return RuleRepository.ExistsByIDsAndName(ruleIDs, ruleName)
```

- **理由**：每步独立可测；Repository 接口单一职责；Manager 控制流程与业务规则。
- **替代方案**：把 Check 写成单条 SQL JOIN —— 拒绝，理由：跨表聚合难以调试、性能与可读性都不及 Go 编排；后期加缓存时反而是负担。

## Risks / Trade-offs

| 风险 | 缓解 |
|---|---|
| 超管用户 GetRules 返回全表 —— 大数据量下慢 | `weight` ASC + `id` ASC 索引；按业务 1000 条以内无性能问题；后续 change 可加分页 / 缓存 |
| `admin_group_access` 无自增 id，外部代码难以单条引用 | 复合 PK (uid, group_id) 是关系表的天然主键；不需要外部引用 id |
| `rules` 字段是文本，DB 层无法做 FK 约束到 admin_rule | 应用层校验（Manager.ParseRuleIDs 时若需要可加「ID 必须存在」校验；本期不强制） |
| Permission.Check 内部三跳 DB 调用，单次响应时间受网络影响 | dbresolver 已读副本配置就绪；后续 change 加缓存可显著降延迟 |
| `*` 通配优先级 OR 语义与用户心智模型可能不一致 | 在 design / code 注释里明示；spec 里的 Scenario 已覆盖；后续若发现 bug 改成 AND 语义影响面小（仅 Manager 单函数） |
| Manager 与 Repository 之间的接口契约（返回值类型 / nil vs empty slice）需要约定 | design D9 + D10 已明示；tasks 阶段在 Repository 接口文档里补全；测试覆盖 nil/empty 边界 |
| admin_rule 表由 add-admin-rule-model 引入，本 change 假设其已存在 | tasks.md 在执行期首先确认 `000002_admin_rule` 已迁移落地；否则阻塞 |

## Migration Plan

部署步骤：

1. 确认 `000002_admin_rule` 已通过 golang-migrate 落库（`migrate version` 显示 ≥ 2）。
2. 合并本 change 代码后启动 `cmd/serve`，启动期 `migrate.Up()` 自动应用 `000003_admin_group`。
3. 验证：`SHOW CREATE TABLE admin_group` / `SHOW CREATE TABLE admin_group_access` 与 000003 SQL 字节级一致；`DESC admin_group_access` 显示主键为 `(uid, group_id)`。
4. 跑通单元测试（Permission Manager 五个方法 + 边界 case）。

回滚策略：

- 数据库：`go run ./cmd/migrate down 1` 回滚 `000003_admin_group.down.sql`（`DROP TABLE IF EXISTS admin_group_access; DROP TABLE IF EXISTS admin_group;`）。
- 代码：`git revert` 本 change commit，移除 `internal/model/admin.go` 中追加的两个模型段、`internal/repository/admin/` 整个子目录、`internal/service/admin/permission.go`。

## Open Questions

- **Permission Manager 是否提供「取全部 uid 可访问的菜单树」接口**：本期不提供，留待【菜单管理】UI change 触发。如确需，spec 已能容纳（新增 Requirement + Scenario）。
- **`admin_group.rules` 字段长度上限**：user SQL 用 `text`（最大 65535 字节）。`strconv.ParseUint` 64 位上限；1000 个规则 id（`1,2,3,...`）约 4000 字节，远低于 text 上限。如确需更大空间可改 mediumtext，但本期不触发。