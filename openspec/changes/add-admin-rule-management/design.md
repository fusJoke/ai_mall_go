# Design

See `proposal.md` for motivation and `specs/admin-rule-management/spec.md` for the requirement set. This document only covers architectural decisions.

## Context

`AdminRule` 模型（`internal/model/admin.go:135`）与 `admin_rule` 表（`cmd/migrate/migrations/000002_admin_rule.up.sql`）均已落地。`InitService`（`internal/service/admin/init.go:109`）通过 `RuleRepository.ListActiveAsMenu` 把启用规则喂给 `/admin/init`。但菜单规则本身没有任何管理入口：路由层 `internal/router/admin/admin.go` 只挂了 login / logout / init / upload / ping 与 manager 9 条；handler 层 `internal/handler/admin/admin.go`（`Handler` 管理管理员实体）已经嵌入 `*handler.BaseHandler[model.Admin]`，是后端 CRUD 的范式样板。

约束：
- 树形结构（pid 自引用）约束 PID 不可成环；
- 父子级联删除——直接删父节点会让孤儿节点 pid 指向无效行，所以必须"有子节点拒绝删"；
- 与 `admin-management` 风格对称（同样的 5 通用 CRUD + N 专属方法 + 同样的 sync.Once 懒装配）；
- 不能让 PID 校验变成 N+1 查询（每次 edit 都要判环）。

## Goals / Non-Goals

**Goals**：
- 复用 `*handler.BaseHandler[model.AdminRule]` 走通用 CRUD，不重复 5 条路由；
- 复用 `*repository.CRUDRepository[model.AdminRule]` 在 `Repository` 上叠 5 个写方法，符合"同实体单 Repository"惯例；
- service 层做 PID 环校验 + 批量跳过的业务规则；
- handler 层做错误码映射（与 manager_test.go 风格一致）。

**Non-Goals**：
- 拖拽排序（前端留接口，不实现 weigh 重排算法）；
- 批量导入 / 导出；
- 在 service 层做 `IsSuperAdmin` 二次校验（按 buildadmin 惯例）；
- 物理删除（沿用既有软删约定）。

## Decisions

### D1：handler 走 `BaseHandler[model.AdminRule]` 嵌入，复用 5 条通用 CRUD

`*RuleHandler` 嵌入 `*handler.BaseHandler[model.AdminRule]`（参照 `Handler` 嵌入 `*handler.BaseHandler[model.Admin]` 的写法），自动获得 `Create` / `List` / `EditGet` / `EditPost` / `Delete` + `RegisterRoutes`。router 调一次 `ruleHandlerInst.RegisterRoutes(rg, "rule")` 即可挂 5 条路由。

为什么不用 `Repository` 直通：service 层的 PID 校验必须在写之前完成，handler 调 `CRUDService.Update` 不经过 service 层业务校验——所以必须叠加 service。

### D2：service 接口叠加 3 个业务专属方法：`ToggleStatus` / `BatchDelete` / `ListAll`

```go
type Service interface {
    service.CRUDService[model.AdminRule]
    ToggleStatus(c *gin.Context, id uint) error
    BatchDelete(c *gin.Context, ids []uint) (deleted int, skipped int, err error)
    ListAll(c *gin.Context) ([]model.AdminRule, error)
}
```

为什么：
- `ToggleStatus` 与 `admin-management.ToggleStatus` 行为对称（不含 token 联动——规则不绑会话）；
- `BatchDelete` 与 `admin-management.BatchDelete` 形状对称（返回 `(deleted, skipped, err)`），但语义不同——admin 是 self-protection skip，rule 是"有子跳过"；
- `ListAll` 是菜单树特有的需求：admin 用全量做权限检查，rule 用全量做父节点选择。

### D3：service 层做 PID 环校验与"有子拒绝删"

PID 环校验的实现：

```go
func (s *baseRuleService) validatePID(c *gin.Context, id, newPID uint) error {
    if newPID == 0 { return nil } // 顶级永远合法
    if newPID == id { return ErrPIDCycle }
    // BFS 上溯 newPID 的祖先链：如果命中 id，则 newPID 是 id 的子孙，循环
    cur := newPID
    visited := make(map[uint]struct{})
    for cur != 0 {
        if cur == id { return ErrPIDCycle }
        if _, ok := visited[cur]; ok { break } // 防御：现存数据已有环就早停
        visited[cur] = struct{}{}
        parent, err := s.repo.GetPID(c, cur)
        if err != nil { return err }
        cur = parent
    }
    return nil
}
```

Repository 加一个最小方法 `GetPID(c, id) (uint, error)`，只取一列、走主键索引；这是 PID 校验唯一的 SQL 成本。Update 路径调用一次，最坏 O(depth)，树深度通常 < 5。

为什么不直接 `SELECT id FROM admin_rule WHERE pid = ?`：那是子节点校验，不是环校验；环校验必须顺着 `pid` 链上溯。

"有子拒绝删"实现：`Delete` / `BatchDelete` 调用前先 `SELECT id FROM admin_rule WHERE pid = ? AND deleted_at IS NULL LIMIT 1`，命中即 `ErrHasChildren`；空切片走短路。

为什么不级联删：菜单规则删错成本高（影响所有 admin 的路由注册）；级联删留待后续 change 单独评估。

### D4：Repository 扩展为单接口，加 5 个写方法 + 1 个 GetPID 读方法

`internal/repository/admin/rule_repository.go` 已有 4 个只读方法（`ListByIDs` / `ExistsByIDsAndName` / `ListActiveIDs` / `ListActiveAsMenu`），本次叠加：

- `Create(c, *model.AdminRule) error` — 单行插入；
- `Update(c, *model.AdminRule) error` — 全量更新（GORM `Save` 语义，与既有 CRUDService 一致）；
- `Delete(c, id uint) error` — 软删单条；
- `DeleteBatch(c, ids []uint) error` — 软删批量（空切片短路）；
- `UpdateStatus(c, id uint, status int8) error` — 仅更新 status 一列；
- `GetPID(c, id uint) (uint, error)` — 读单行 pid 字段，用于 PID 环校验。

为什么 `GetPID` 不走 `GetByID`：GORM `Pluck` 单列比 `Find` 全行省 IO；本路径在 service 层频繁调用。

为什么用单接口而非读写拆分：`group_repository.go` / `access_repository.go` / `config_repository.go` 都是单接口单实体（仅含只读方法），扩展到含写方法后是同一惯例；InitService / Permission Manager / 新增的 RuleService 都依赖同一接口，避免多 Repository 注入。

### D5：路由层拆 `internal/router/admin/rule.go`

参考 `add-admin-management` 已落地的 `internal/router/admin/manager.go`，新建 `rule.go` 容纳 8 条路由（5 通用 CRUD + ToggleStatus + BatchDelete + ListAll），全部挂 `AdminAuth`。在 `admin.go` 的 `init()` 末尾追加 `registerRuleRoutes()`。

为什么不开 `:path(.*)*` 通配：菜单路径按 buildadmin 风格硬编码 `name='admin/rule'`、前端静态路由硬编码 `/admin/rule`，与 manager 路由对称。

### D6：菜单种子数据走 `000005_admin_rule_menu.{up,down}.sql`

```sql
INSERT INTO admin_rule (pid, type, title, name, path, component, status, created_at, updated_at)
VALUES (0, 'menu', '菜单规则', 'admin/rule', 'rule', '/src/views/admin/rule/index.vue', 1, NOW(3), NOW(3))
ON DUPLICATE KEY UPDATE title = VALUES(title);
```

pid 暂设为 0（顶级），与 `000004_admin_manager_rule.up.sql` 对齐。后续若要归到「系统设置」分组再调整。

### D7：service 层 sentinel 错误与 handler 错误码映射

新增 3 个 sentinel：

```go
var (
    ErrPIDCycle      = errors.New("rule: pid forms cycle")
    ErrHasChildren   = errors.New("rule: rule has children")
    ErrPIDNotFound   = errors.New("rule: pid not found")
)
```

handler 错误码映射（与 manager 一一对应）：

| service error | HTTP | code |
|---|---|---|
| `ErrPIDCycle`    | 400 | `rule.create.pid_cycle` / `rule.edit.pid_cycle` |
| `ErrHasChildren` | 400 | `rule.delete.has_children` |
| `ErrPIDNotFound` | 400 | `rule.create.pid_not_found` / `rule.edit.pid_not_found` |
| `gorm.ErrRecordNotFound` | 404 | `rule.edit.not_found` / `rule.delete.not_found` / `rule.toggle_status.not_found` |
| 其他 | 500 | `rule.*.internal` |

### D8：handler `ListAll` 直接调 service `ListAll`，不走通用 List 路径

`ListAll` 返回 `[]model.AdminRule` 而不是 `{items, total}`——菜单树只需要 array，前端 el-tree-select 才能直接消费。

为什么不复用 `CRUDService.List(opts)` 然后翻 total：service 总数查与前端需求无关，省一次 `SELECT COUNT(*)`；PageSize 上限也省（ListAll 一次拉完）。

### D9：前端 i18n key 命名沿用 `manager.*` 同级，加 `rule.*` 命名空间

新增 key 落在 `web/src/lang/zh-cn/admin.yaml` 与 `web/src/lang/en/admin.yaml`：

- `rule.title` / `rule.searchPlaceholder`；
- `rule.columns.id` / `pid` / `type` / `title` / `name` / `path` / `icon` / `weigh` / `status` / `updated_at`；
- `rule.dialog.create` / `dialog.edit`；
- `rule.action.edit` / `toggleStatus` / `delete` / `batchDelete`；
- `rule.confirm.delete` / `confirm.batchDelete`；
- `rule.error.pidCycle` / `hasChildren` / `pidNotFound` / `notFound`。

每文件约 18 条 key。

## Risks / Trade-offs

- **[PID 环校验的 worst case]**：树深度无上界（理论上 admin 可以建任意深度），最坏一次 Update 触发 O(depth) 次 SELECT。**Mitigation**：admin_rule 业务上深度一般 < 5（菜单分组）；若深度异常则 GetPID 走主键索引每次 O(1)。`visited` map 防"现存数据已有环"导致的死循环。
- **[删除前子节点检查 N+1]**：`BatchDelete` 一次最多 200 个 id，每个都先查一次子节点，最坏 200 次 SELECT。**Mitigation**：可以一次 `SELECT pid, COUNT(*) FROM admin_rule WHERE pid IN (?) AND deleted_at IS NULL GROUP BY pid`，单条 SQL 拿到所有有子的 id；为本期简洁起见，本设计先用逐 id 查（与小批量 UI 操作匹配），未来若出现 > 100 的批量删除再优化。
- **[Repository 单接口承载读写]**：当前 GroupRepository / ConfigRepository 等都是只读，加入写方法后"接口是否仍然纯净"取决于后续是否新增 side-effect 方法。**Mitigation**：当前 6 个方法职责清晰（CRUD + PID 读），无事务 / 无外部 IO；写入走 GORM 标准语义，与既有 CRUDRepository 行为一致。
- **[ListAll 不分页]**：启用规则数量超 500 时一次性返回会拖慢响应。**Mitigation**：admin_rule 在生产环境一般 < 200 条；如未来超 1k 再加 `?pid=` 过滤或限制 max 500。
- **[/admin/rule 静态子路由与超管菜单规则双轨]**：超管需要这条菜单，按既有约定（`add-admin-management` 已落地 `000004`）双轨注册菜单规则种子 + 静态前端子路由。**Mitigation**：与 admin-manager 路由完全对称，未来 sync 时统一处理。

## Migration Plan

1. **数据库迁移**：部署 `000005_admin_rule_menu` 迁移 → 菜单出现；
2. **后端**：先发后端（新增接口 + 路由注册），不依赖前端；前端可用 curl / Postman 验证；
3. **前端**：发布 `/admin/rule` 静态路由 + i18n；
4. **回滚**：迁移提供 down 文件；后端 / 前端走正常 git revert。

## Open Questions

- **删除时是否要支持 force=true 跳过子节点检查？** buildadmin 有"强制删除"按钮，会先把子节点 pid 改到祖父。本期 spec 不暴露 force；若运营侧后续要，加 `?force=true` query 参数并在 service 层把"先把所有直接子 pid 改成祖父 pid"原子化（用 `UPDATE admin_rule SET pid = ? WHERE pid IN (?)`）。
- **是否需要规则复制（clone）接口？** copy 一个权限点改 title / name 比从零建快。本期不做；后续若 PM 反馈多，加 `POST /admin/rule/clone`。
