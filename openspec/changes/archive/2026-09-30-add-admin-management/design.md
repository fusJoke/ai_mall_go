# Design

See `proposal.md` for motivation and `specs/admin-management/spec.md` for the full requirement set. This document only covers architectural decisions.

## Context

`Admin` 模型与 `*handler.BaseHandler[model.Admin]` 已存在；后者通过嵌入 `service.CRUDService[model.Admin]` 自动具备 `Create` / `List` / `EditGet` / `EditPost` / `Delete` 五个方法和 `RegisterRoutes`。但路由层（`internal/router/admin/admin.go`）当前只挂了 login / logout / init / upload / ping，**通用 CRUD 与 admin 专属接口均未暴露**。`service.CRUDService[model.Admin]` 内部走 `service.BaseCRUDService[model.Admin]`，再下发到 `repository.CRUDRepository[model.Admin]` —— 三层接口都已就位。

约束：
- 不能让 hash 密码流出 service 层以外的任何位置；
- 任何「改密 / 禁用 / 解锁」操作都可能改变当前会话的有效性，需要联动 `token.Manager.Clear`；
- admin 不能删除 / 禁用自己 —— 需要从 `middleware.AdminFromContext(c)` 读 `*model.Admin` 判等。

## Goals / Non-Goals

- 复用 `BaseHandler.RegisterRoutes` 挂通用 CRUD，避免重复写路由；
- 业务专属方法（`ChangePassword` / `ToggleStatus` / `Unlock` / `BatchDelete`）作为 `adminSvc.Service` 的新方法，不污染通用 CRUD 接口；
- service 层负责所有 bcrypt 与 token 联动，handler 只做参数校验与响应组装；
- 前端走 `el-table` + 弹窗 + agUpload 头像，CRUD 走 `web/src/api/admin.ts`。

Non-Goals：
- admin 自助改密 —— 单独 change；
- 2FA / OAuth / 审计日志 —— 后续。

## Decisions

### D1：通用 CRUD 走 `BaseHandler.RegisterRoutes`，不再单独定义

`*Handler` 已经嵌入 `*handler.BaseHandler[model.Admin]`，router 只需要在已有 `/admin` group 内调 `adminHandlerInst.RegisterRoutes(rg, "admin")`。这样 5 条路由 `POST/GET /admin/admin/{create,list,edit,delete}` 自动注册，不需要在 `Handler` 上写新方法。

为什么不单独定义 5 个 wrapper：handler 层无业务语义，重复定义等于绕过 CLAUDE.md「handler 不掺业务」的约束。

### D2：4 个 admin 专属方法挂在 `Service` 接口上，不挂 `CRUDService[T]`

`*Service`（`internal/service/admin/admin.go:86`）当前嵌入 `service.CRUDService[model.Admin]` + Login + Logout。新增 `ChangePassword` / `ToggleStatus` / `Unlock` / `BatchDelete` 4 个方法，签名统一为 `(c *gin.Context, ...) (..., error)`。

为什么：这些方法都包含「读取当前 admin + 联动 token 库 + 写 admin 库」三步逻辑，是业务编排而非通用 CRUD。放 `CRUDService` 会逼所有用户态的 service 都实现这 4 个方法，违反通用接口的语义。

### D3：service 层在 Create / Update 路径上做 bcrypt

新增 `hashPasswordIfNeeded(c, *model.Admin, isUpdate)` helper：

- Create：传入的 `Password` 必须非空且长度合法 → bcrypt → 写入；
- Update：`Password` 非空 → bcrypt；空 → 不动原密码（前端做部分更新时省略字段）。
- helper 内部判断「非空」与「长度」两件事，错误统一返回 `ErrPasswordTooShort`（新增 sentinel）。

为什么不放 handler：bcrypt 是业务决策，handler 应只解析请求体；放 service 让 `Update` 的"部分更新"语义集中在 service 一处，避免出现 handler 改密 / service 改密的二义性。

### D4：4 个专属接口全挂 `AdminAuth` 中间件，自保护靠 service 从 ctx 读当前 admin

router 注册时统一挂 `middleware.AdminAuth()`（handler 已在 `Middleware()` 钩子里读 `*model.Admin` 写进 context）。service 在 `ChangePassword` / `ToggleStatus` / `BatchDelete` 入口处 `middleware.AdminFromContext(c)` 拿当前 admin：

- `ToggleStatus`：目标 `id == self.ID` → 返回 `ErrSelfProtection`；
- `BatchDelete`：从 `ids` 切片里移除 `self.ID`，并把移除数记入响应；
- `ChangePassword` / `Unlock` / `Delete`：不自保护（admin 可以改 / 解锁 / 删除其他 admin）。
- `Delete`：也自保护（不希望 admin 把自己误删）。

为什么不靠前端隐藏按钮：前端可被绕过；后端是 source of truth，前端的隐藏只是 UX 优化。

### D5：`Status` 切到 `0` 时联动吊销 token

`ToggleStatus` 与 `ChangePassword` 成功后调 `s.tm.Clear(c, targetID, "admin")`（已有 `tokenIssuer.Clear` 接口）：

- `ToggleStatus`：仅当 `1 → 0` 时吊销；`0 → 1` 不吊销；
- `ChangePassword`：无论当前 status 一律吊销，让用户重新登录。
- `Unlock`：不清 token（admin 解锁是为了让用户继续登录，不是逼他下线；这是产品决策，按 buildadmin / 标准后台行为对齐）。

吊销失败不阻塞主流程：`if err := s.tm.Clear(...); err != nil { /* log warn */ }`。

### D6：`BatchDelete` 走「剔除 self + 软删」两步，避免事务

```go
func (s *baseService) BatchDelete(c *gin.Context, ids []uint) (deleted int, skippedSelf int, err error) {
    self := middleware.AdminFromContext(c)
    filtered := make([]uint, 0, len(ids))
    for _, id := range ids {
        if self != nil && id == uint(self.ID) {
            skippedSelf++
            continue
        }
        filtered = append(filtered, id)
    }
    if len(filtered) == 0 {
        return 0, skippedSelf, nil
    }
    if err := s.DeleteBatch(c, filtered); err != nil { // 走 repository
        return 0, skippedSelf, err
    }
    return len(filtered), skippedSelf, nil
}
```

为什么不开事务：删除是幂等的（重复删一行只会改 `deleted_at` 时间戳，业务无副作用）；软删不需要回滚。如果未来要加「删除时连带吊销 token」再开事务。

### D7：repository 暴露 4 个新方法，但保持现有 `CRUDRepository` 接口不破

`Repository` 接口（`internal/repository/admin/admin.go:18`）已经在 `CRUDRepository[model.Admin]` 上叠了 `GetByUsername`。本次新增：

- `DeleteBatch(c, ids []uint) error` —— 走 `repository.DB(c).Where("id IN ?", ids).Delete(&model.Admin{})`，依赖 GORM 自动加 `deleted_at IS NULL` 过滤；
- `UpdateStatus(c, id uint, status int8) error` —— 只改 `Status` 一列；
- `UpdatePassword(c, id uint, hashed string) error` —— 只改 `Password` 一列；
- `ResetLoginFailure(c, id uint) error` —— `LoginFailure = 0` + `Status = 1`。

为什么不直接用 `Update(c, *model.Admin)`：那样要先把当前行 fetch 出来再合并字段，多一次 SQL；专用列 update 一次走完，节省 IO 与锁。

### D8：路由分层注册，新增文件 `internal/router/admin/manager.go`

为了避免 `admin.go` 一文件塞 5 + 4 + 5 = 14 个 `registry.Register` 调用，新拆 `internal/router/admin/manager.go`：

```go
package admin

func registerManagerRoutes() {
    registry.Register("/admin", POST, "/admin/create", adminHandlerInst.Create, middleware.AdminAuth())
    // ...5 条通用 CRUD
    registry.Register("/admin", POST, "/admin/change-password", adminHandlerInst.ChangePassword, middleware.AdminAuth())
    // ...4 条专属
}
```

并在 `admin.go` 的 `init()` 末尾调一次 `registerManagerRoutes()`。

为什么不直接展开到现有 init：当前 init 已经塞了 5 条路由 + 一坨 sync.Once 装配逻辑；新增 9 条再加 init 里会让文件超过 200 行。拆文件更利于 review。

### D9：前端路径 `/admin/manager`，与管理页语义对齐

菜单规则 `admin_rule` 新增一条 `path='manager'`、`name='admin/manager'`、`component='/src/views/admin/manager/index.vue'`、`title='管理员账号'`。前端静态子路由在 `adminBase.ts` 的 children 里加：

```ts
{
    path: 'manager',
    name: 'adminManager',
    component: () => import('/@/views/admin/manager/index.vue'),
    meta: { title: 'pageTitles.adminManager' },
},
```

放在 `:path(.*)*` catch-all 之前（与 `add-ag-upload-component` 已落地的 agInput 路由同模式）。

为什么不放在动态菜单加载链路：超管也需要这条菜单，把它走静态路由 + 后端 `admin_rule` 双轨可让前后端独立刷新。

### D10：前端 API 走 `web/src/api/admin.ts` 新文件，不沿用 common.ts

`add-ag-upload-component` 已经把跨实体通用 API（如 `fileUpload`、`getClickCaptcha`）放 `common.ts`；admin 业务专属 API 放 `web/src/api/admin.ts`，与 buildadmin / 项目前端惯例一致。

调用清单：

- `adminList(params)` → `GET /admin/admin/list`；
- `adminCreate(body)` → `POST /admin/admin/create`；
- `adminEdit(body)` → `POST /admin/admin/edit`（含 id）；
- `adminDelete(id)` → `POST /admin/admin/delete?id=`；
- `adminChangePassword(body)` → `POST /admin/admin/change-password`；
- `adminToggleStatus(body)` → `POST /admin/admin/toggle-status`；
- `adminUnlock(body)` → `POST /admin/admin/unlock`；
- `adminBatchDelete(body)` → `POST /admin/admin/batch-delete`；
- `adminGet(id)` → `GET /admin/admin/edit?id=`（复用 BaseHandler.EditGet）。

### D11：菜单规则种子数据走 migration

新增迁移文件 `cmd/migrate/migrations/<seq>_admin_manager_rule.{up,down}.sql`，插入一条 `admin_rule` 记录：

```sql
INSERT INTO admin_rule (pid, type, title, name, path, component, status, created_at, updated_at)
VALUES (0, 'menu', '管理员账号', 'admin/manager', 'manager', '/src/views/admin/manager/index.vue', 1, NOW(), NOW())
ON DUPLICATE KEY UPDATE title = VALUES(title);
```

pid 暂设为 0（顶级），后续若要归到「系统设置」分组再调整。

为什么不放进 database seed / init.go：项目已切到 golang-migrate，菜单规则的版本化与回滚统一走迁移文件。

### D12：i18n key 命名约定

新增 key 落在 `web/src/lang/zh-cn/admin.yaml` 与 `web/src/lang/en/admin.yaml`（目录已存在），命名按 buildadmin 风格：

- `manager.title`、`manager.searchPlaceholder`；
- `manager.columns.username` / `nickname` / `email` / `mobile` / `status` / `lastLoginAt` / `createdAt`；
- `manager.dialog.create` / `dialog.edit` / `dialog.changePassword`；
- `manager.action.changePassword` / `toggleStatus` / `unlock` / `delete` / `batchDelete`；
- `manager.confirm.delete` / `confirm.batchDelete` / `confirm.changePassword`；
- `manager.toast.createOk` / `editOk` / `changePasswordOk` 等。

中文 ~12 条，英文镜像。

## Risks / Trade-offs

- **bcrypt 性能**：cost=10（默认）。每次 Update 都跑一次 bcrypt 在批量更新场景会被放大；缓解：Update 时仅在 `Password` 非空时哈希（D3）。
- **self-protection 只防自删 / 自禁用，不防自改密**：admin 可以让别人改自己的密码，行为符合 buildadmin 习惯；如果未来要禁止，在 ChangePassword 加 self check 即可。
- **菜单规则种子数据重复插入**：用 `ON DUPLICATE KEY UPDATE` 兜底，重复跑迁移不会爆。
- **前端 i18n key 缺失**：所有用到的 key 必须落在 `admin.yaml` 而非散落其他文件；code review 时通过 `grep -r "t('manager\\."` 自查。

## Migration Plan

1. **数据库迁移**：部署 `admin_manager_rule` 迁移 → 菜单出现；
2. **后端**：先发后端（新增接口 + 路由注册），不依赖前端；前端可用 curl / Postman 验证；
3. **前端**：发布 `/admin/manager` 静态路由 + i18n；
4. **回滚**：迁移提供 down 文件；后端 / 前端走正常 git revert。

## Open Questions

- **是否需要「禁用 admin 同时吊销其 token」的配置开关？** 当前实现默认吊销；如果运维侧希望禁用后保留会话（例如临时禁用 5 分钟后启用），可加 `admin_disable_revoke_token: bool` 配置。本期默认行为是吊销，配置开关留待后续 change。
- **首次创建的超管（id=1）是否能被编辑 / 删除？** 当前所有 admin 都受 self-protection，但 `id=1` 是种子超管，没有特殊保护。如果未来要锁死超管，再加 `IsSuperAdmin` 标记。本期不处理。
