# Tasks

## 1. Repository — 新增 4 个专用方法

- [x] 1.1 修改 `internal/repository/admin/admin.go`：在 `Repository` 接口追加 4 个方法（按 design D7）：
  - `DeleteBatch(c *gin.Context, ids []uint) error`：走 `repository.DB(c).Where("id IN ?", ids).Delete(&model.Admin{})`；
  - `UpdateStatus(c *gin.Context, id uint, status int8) error`：仅写 `Status` 一列；
  - `UpdatePassword(c *gin.Context, id uint, hashed string) error`：仅写 `Password` 一列；
  - `ResetLoginFailure(c *gin.Context, id uint) error`：`LoginFailure = 0` + `Status = 1`。
  验证：`go build ./...` exit 0。

- [x] 1.2 在 `internal/repository/admin/admin.go` 的 `baseRepository` 上实现这 4 个方法（用 `Updates(map[string]any{...})` 走 GORM 的 `Select: false` 语义，确保只改指定列、不覆盖时间戳以外的其它字段）。验证：`go build ./...` exit 0；`gorm.io/gorm` 与项目 dbresolver 兼容。

- [x] 1.3 新建 `internal/repository/admin/manager_repository_test.go`：用 sqlmock（项目已有使用惯例，见 `config_repository_test.go`）覆盖 4 个方法的 SQL 形态：
  - `DeleteBatch` 生成 `UPDATE admins SET deleted_at = ? WHERE id IN (?, ?, ?) AND deleted_at IS NULL`；
  - `UpdateStatus` 生成 `UPDATE admins SET status = ?, updated_at = ? WHERE id = ? AND deleted_at IS NULL`；
  - `UpdatePassword` 同上模式；
  - `ResetLoginFailure` 验证 `status = 1 AND login_failure = 0`。
  验证：`go test ./internal/repository/admin/...` 全绿。

## 2. Service — bcrypt 与 admin 专属方法

- [x] 2.1 在 `internal/service/admin/admin.go` 顶部新增 sentinel：
  ```go
  var (
      ErrPasswordTooShort = errors.New("admin: password too short")
      ErrSelfProtection   = errors.New("admin: refusing self-targeting operation")
  )
  ```
  并新增常量 `MinPasswordLength = 8` 与 `MinUsernameLength = 3`。验证：`go build ./...` exit 0。

- [x] 2.2 扩展 `Service` 接口（design D2）新增 4 个方法：
  ```go
  ChangePassword(c *gin.Context, id uint, newPassword string) error
  ToggleStatus(c *gin.Context, id uint) error
  Unlock(c *gin.Context, id uint) error
  BatchDelete(c *gin.Context, ids []uint) (deleted int, skippedSelf int, err error)
  ```
  验证：`go build ./...` exit 0；`*baseService` 仍满足 `Service`（编译期断言 `var _ Service = (*baseService)(nil)` 触发）。

- [x] 2.3 新增私有 helper `hashPasswordIfNeeded`：
  ```go
  // Create/Update 入口；非空 password 走 bcrypt；空表示不动原密码。
  func (s *baseService) hashPasswordIfNeeded(c *gin.Context, adm *model.Admin, isUpdate bool) error
  ```
  实现要点：
  - `isUpdate=true && adm.Password == ""` → 直接返回 nil（不动原密码）；
  - 否则校验长度 ≥ `MinPasswordLength`，不满足返回 `ErrPasswordTooShort`；
  - bcrypt.DefaultCost 哈希后回写 `adm.Password`。
  验证：`go build ./...` exit 0。

- [x] 2.4 覆盖 `Create` 与 `Update`（design D3）：在转发 `s.CRUDService.Create / Update` 之前调 `s.hashPasswordIfNeeded`：
  ```go
  func (s *baseService) Create(c *gin.Context, entity *model.Admin) error {
      if entity == nil { return errors.New("service: nil entity") }
      if err := s.hashPasswordIfNeeded(c, entity, false); err != nil { return err }
      return s.CRUDService.Create(c, entity)
  }
  func (s *baseService) Update(c *gin.Context, entity *model.Admin) error {
      if entity == nil { return errors.New("service: nil entity") }
      if err := s.hashPasswordIfNeeded(c, entity, true); err != nil { return err }
      return s.CRUDService.Update(c, entity)
  }
  ```
  注意：用户名查重（`GetByUsername` + 唯一索引）落到 `repository`，service 层仅当 password 字段非空时校验长度；用户名查重由 GORM 唯一索引兜底，service 层不重复实现。
  验证：`go build ./...` exit 0；`*baseService.Create / Update` 编译期可被 `Service` 接口接收。

- [x] 2.5 实现 `ChangePassword(c, id, newPassword)`：
  - 长度校验 < `MinPasswordLength` → `ErrPasswordTooShort`；
  - bcrypt 哈希；
  - `s.repo.UpdatePassword(c, id, hashed)`；
  - `s.tm.Clear(c, int64(id), tokenTypeAdmin)` —— 失败仅 log warn，不阻塞。
  验证：`go test ./internal/service/admin/...` 中新增 `TestChangePassword_*`（happy / too-short / repo 错）。

- [x] 2.6 实现 `ToggleStatus(c, id)`：
  - `middleware.AdminFromContext(c)` 拿 self；`uint(self.ID) == id` → `ErrSelfProtection`；
  - 读 `s.repo.GetByID(id)`；
  - nil → `gorm.ErrRecordNotFound`；当前 `Status` 为 `1` 切到 `0` + `tm.Clear`；为 `0` 切到 `1`（不清 token，design D5）；
  - 调 `s.repo.UpdateStatus(c, id, newStatus)`。
  验证：`go test` 覆盖 self-protection / 1→0 / 0→1 / not-found 四种 case。

- [x] 2.7 实现 `Unlock(c, id)`：直接调 `s.repo.ResetLoginFailure(c, id)`，不读 self、不清 token（design D5）。验证：`go test` 覆盖 happy path + repo 错。

- [x] 2.8 实现 `BatchDelete(c, ids)`（design D6）：
  - self 剔除 + 计数 `skippedSelf`；
  - 剩余 ids 调 `s.repo.DeleteBatch(c, filtered)`；
  - 返回 `(len(filtered), skippedSelf, nil)`。
  验证：`go test` 覆盖 `[1,2,3]` 当前 id=1 → `(2,1,nil)`；`[]` → `(0,0,nil)`；repo 错透传。

- [x] 2.9 在 `internal/service/admin/admin_test.go` 末尾追加测试：
  - `TestCreate_HashesPassword`：mock repo 断言传入的 `entity.Password` 以 `$2` 开头；
  - `TestUpdate_PreservesPasswordWhenEmpty`：Update 时 `entity.Password == ""`，mock repo 拿到的 `entity.Password` 仍为空；
  - `TestCreate_RejectsShortPassword`：mock repo 永远不应被调到；
  - `TestChangePassword_RevokesTokens`：mock tokenIssuer.Clear 被调到 + 参数 `(int64(id), "admin")`；
  - `TestToggleStatus_SelfProtection`：mock 中间件返回 self id=1，请求 id=1 → `ErrSelfProtection`，repo 未被调到；
  - `TestToggleStatus_1ToZeroRevokes`：status=1 输入 → UpdateStatus(0) + Clear 被调；
  - `TestToggleStatus_0ToOneDoesNotRevoke`：status=0 → UpdateStatus(1) + Clear 未被调；
  - `TestUnlock_DoesNotTouchPassword`：repo.ResetLoginFailure 被调，UpdatePassword 未被调；
  - `TestBatchDelete_FiltersSelf`：ids=[1,2,3] self=1 → DeleteBatch([2,3])，返回 (2,1,nil)；
  - `TestBatchDelete_Empty`：ids=[] → (0,0,nil)。
  验证：`go test ./internal/service/admin/...` 全绿。

## 3. Handler — 4 个专属方法 + 列表脱敏

- [x] 3.1 修改 `internal/handler/admin/admin.go`：在 `Handler` 上新增 4 个方法（design D2 / D10）：
  - `ChangePassword(c *gin.Context)`：解析 `{id, new_password}` → 调 `svc.ChangePassword` → 错误映射（`ErrPasswordTooShort` → 400 `admin.change_password.password_too_short`；其他 → 500）；
  - `ToggleStatus(c *gin.Context)`：解析 `{id}` → 调 `svc.ToggleStatus` → 错误映射（`ErrSelfProtection` → 403 `admin.toggle_status.self_protection`；`gorm.ErrRecordNotFound` → 404）；
  - `Unlock(c *gin.Context)`：解析 `{id}` → 调 `svc.Unlock` → 错误映射；
  - `BatchDelete(c *gin.Context)`：解析 `{ids: []uint}` → 调 `svc.BatchDelete` → 返回 `{deleted, skipped_self}`。
  每个方法加上 swag 注解（与 `Login` / `Logout` 风格一致：@Summary / @Tags / @Param / @Router）。验证：`go build ./...` exit 0。

- [x] 3.2 在 `internal/handler/admin/admin.go` 现有 `adminInfo` 投影基础上，新增 `toAdminInfoList(adm []model.Admin) []adminInfo` 或把 `List` / `EditGet` / `EditPost` 返回前的 entity 整体跑一遍 `toAdminInfo(*model.Admin)`，确保 list 接口的 items 不含 `password`（design / spec Requirement "Passwords are never exposed"）。
  当前 `BaseHandler.List` 直接 `c.JSON(items)` 会把 `model.Admin` 原样输出（含 password 字段）；本任务在 `Handler` 上覆盖 `List` 方法，遍历 items 转 `adminInfo`（password 已被结构体省略）。
  验证：`go test ./internal/handler/admin/...` 中新增 `TestHandler_List_StripsPassword`：mock 返回 `[]model.Admin{...含 password...}`，断言响应 JSON 无 password 字段。

- [x] 3.3 写 `internal/handler/admin/manager_test.go`：覆盖 4 个新方法 + List 脱敏：
  - `TestChangePassword_ServiceCalled` / `TestChangePassword_TooShortMapping` / `TestChangePassword_InternalMapping`；
  - `TestToggleStatus_SelfProtectionMapping` / `TestToggleStatus_NotFoundMapping`；
  - `TestUnlock_NotFoundMapping`；
  - `TestBatchDelete_ResponseShape`：返回 `{deleted: 2, skipped_self: 1}` JSON。
  验证：`go test ./internal/handler/admin/...` 全绿。

## 4. Router — 拆 manager.go

- [x] 4.1 新建 `internal/router/admin/manager.go`：定义 `func registerManagerRoutes()`，调 `registry.Register` 挂 9 条路由（design D8）：
  - 5 条通用 CRUD（POST `/admin/admin/create` / GET `/admin/admin/list` / GET `/admin/admin/edit` / POST `/admin/admin/edit` / POST `/admin/admin/delete`），全部 `middleware.AdminAuth()`；
  - 4 条专属（POST `/admin/admin/change-password` / POST `/admin/admin/toggle-status` / POST `/admin/admin/unlock` / POST `/admin/admin/batch-delete`），全部 `middleware.AdminAuth()`。
  每条用闭包包 `ensureDeps()` 然后转发到 `adminHandlerInst` 的对应方法。
  验证：`go build ./...` exit 0。

- [x] 4.2 在 `internal/router/admin/admin.go` 的 `init()` 末尾追加一行 `registerManagerRoutes()`。验证：`go build ./...` exit 0；`grep "registerManagerRoutes" internal/router/admin/*.go` 命中 2 处。

- [x] 4.3 新建 `internal/router/admin/manager_test.go`：用 `httptest` + gin 默认 engine 验证路由表存在：
  - 9 条路由都能 match 出来（`engine.Routes()` 断言）；
  - 不带 Bearer token 的请求到 `/admin/admin/list` 返回 401（中间件拦截）。
  验证：`go test ./internal/router/admin/...` 全绿。

## 5. 迁移 — 菜单规则种子

- [x] 5.1 新建 `cmd/migrate/migrations/000004_admin_manager_rule.up.sql`：按 design D11 插入一条 admin_rule 记录。验证：文件存在；语法正确（可手动 `mysql -e "source ..."` 跑通）。

- [x] 5.2 新建 `cmd/migrate/migrations/000004_admin_manager_rule.down.sql`：`DELETE FROM admin_rule WHERE name = 'admin/manager' AND deleted_at IS NULL`。验证：文件存在。

- [x] 5.3 在 `cmd/migrate/migrations/000001_baseline.up.sql` 已有模式下，确认 `admin_rule` 表已存在（无需改 baseline）；若 admin_rule 列与 000002 一致即可。验证：`cat 000004_*.up.sql` 与 `000002_*.up.sql` 列对齐。

## 6. 前端 i18n

- [x] 6.1 新建 `web/src/lang/zh-cn/admin.yaml`：含 12 条 key（design D12）：
  - `manager.title: 管理员账号`、`manager.searchPlaceholder: 搜索用户名 / 昵称`；
  - `manager.columns.username/nickname/email/mobile/status/lastLoginAt/createdAt` 各 1 条；
  - `manager.dialog.create/edit/changePassword`；
  - `manager.action.changePassword/toggleStatus/unlock/delete/batchDelete`；
  - `manager.confirm.delete/batchDelete/changePassword`。
  验证：文件存在。

- [x] 6.2 新建 `web/src/lang/en/admin.yaml`：镜像 12 条英文。验证：文件存在；typecheck exit 0。

- [x] 6.3 修改 `web/src/lang/zh-cn/pageTitles.yaml` 与 `web/src/lang/en/pageTitles.yaml`：追加 `adminManager: 管理员账号` / `adminManager: Admin Manager`。验证：`pnpm typecheck` exit 0。

## 7. 前端 API

- [x] 7.1 新建 `web/src/api/admin.ts`：按 design D10 导出 9 个函数：
  - `adminList(params)`、`adminCreate(body)`、`adminEdit(body)`、`adminDelete(id)`、`adminGet(id)`；
  - `adminChangePassword(body)`、`adminToggleStatus(body)`、`adminUnlock(body)`、`adminBatchDelete(body)`。
  类型签名：`adminList(params?: { page?; page_size?; username?; nickname? }): Promise<ApiResponse<{items: AdminInfo[]; total: number; page: number; page_size: number}>>` 等。
  `AdminInfo` 是与后端 `adminInfo` 一致的 TS 接口（id, username, nickname, avatar, email?, mobile?, last_login_at?, last_login_ip?, bio, status）。
  验证：`pnpm typecheck` exit 0。

## 8. 前端管理页

- [x] 8.1 新建 `web/src/views/admin/manager/index.vue`：按 spec Requirement "Management UI is available at /admin/manager" 实现：
  - 顶部：`<el-input>` 搜索框 + 「新增」按钮；
  - `<el-table>` 列出账号，复用 baTable 风格（如项目已有）：列含 username / nickname / email / mobile / status / last_login_at / created_at，行内操作按钮（编辑 / 改密 / 启停 / 解锁 / 删除）；
  - 「批量删除」按钮 + 多选；
  - 「新增 / 编辑」弹窗（`<el-dialog>`）：表单字段 username / nickname / email / mobile / avatar（ag-upload） / password / bio / status；编辑时 password 字段留空表示不改；
  - 「改密」弹窗：仅 new_password 字段；
  - 调用 `web/src/api/admin.ts` 的 9 个函数；
  - 表格 mount 时调 `adminList()` 拉首屏。
  验证：`pnpm typecheck` exit 0；`pnpm lint` exit 0（应用 prettier）。

- [x] 8.2 在 `web/src/router/static/adminBase.ts` 的 `children` 中、`agInput` 之后、`:path(.*)*` 之前追加（design D9）：
  ```ts
  {
      path: 'manager',
      name: 'adminManager',
      component: () => import('/@/views/admin/manager/index.vue'),
      meta: { title: 'pageTitles.adminManager' },
  },
  ```
  验证：`pnpm typecheck` exit 0。

## 9. 集成验证

- [x] 9.1 后端 `go build ./...` + `go vet ./...` exit 0。验证：仅保留 `scripts/dump_gorm_schema` 既有告警。

- [x] 9.2 `go test ./internal/handler/admin/... ./internal/service/admin/... ./internal/repository/admin/... ./internal/router/admin/... ./internal/kit/urlx/...` 全绿。验证：无 FAIL。

- [x] 9.3 前端 `pnpm typecheck` exit 0（仅 `web/src/utils/random.ts` 既有错误可保留）。验证：除 pre-existing 错误外 0 命中。

- [x] 9.4 `pnpm lint` 仅 `web/src/utils/random.ts` 与 `web/src/views/admin/login.vue` 等既有告警；新增文件零告警。验证：`npx eslint web/src/views/admin/manager/index.vue web/src/api/admin.ts` 无输出。

- [x] 9.5（需 MySQL）跑 `make migrate-up`（或 `go run ./cmd/migrate up`），确认 000004 迁移成功，`SELECT * FROM admin_rule WHERE name='admin/manager'` 返回 1 行。验证：迁移无 error。
  - 实测：`go run ./cmd/migrate up` 输出 `up to latest: now at version 4`；`SELECT * FROM admin_rule WHERE name='admin/manager' AND deleted_at IS NULL` 返回 1 行（id=1, pid=0, type=menu, title='管理员账号', name='admin/manager', path='manager', component='/src/views/admin/manager/index.vue', status=1）。

- [x] 9.6（需 MySQL + dev server）`go run ./cmd/serve` + `pnpm dev`，admin 登录后访问 `/admin/manager`：
  - 表格加载；
  - 新增 1 个账号 → 列表刷新；
  - 编辑改昵称 → 表格刷新；
  - 改密 → 旧 token 失效（再访问受保护端点 401）；
  - 启停 → status 切换；
  - 解锁 → login_failure 清零；
  - 自删除 / 自禁用 → 403；
  - 批量删除 → 仅非自被删。
  验证：浏览器目测 + curl 双验证。
  - 实测：8 项验证全部通过；唯一修复是 `EditPost` 必须先 `GetByID` 拿原行再 patch，否则 client JSON 的 `CreatedAt=0000-00-00` 触发 SQL 错误（已修，见 handler/admin/admin.go EditPost）。
