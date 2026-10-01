# Tasks

## 1. Repository — 扩展 RuleRepository 接口与 GORM 实现

- [x] 1.1 在 `internal/repository/admin/rule_repository.go` 的 `RuleRepository` 接口上追加 6 个方法（design D4）：
  - `Create(c *gin.Context, rule *model.AdminRule) error`
  - `Update(c *gin.Context, rule *model.AdminRule) error`
  - `Delete(c *gin.Context, id uint) error`
  - `DeleteBatch(c *gin.Context, ids []uint) error`
  - `UpdateStatus(c *gin.Context, id uint, status int8) error`
  - `GetPID(c *gin.Context, id uint) (uint, error)`
  验证：`go build ./...` exit 0；`InitService` / `Permission Manager` 仍调用既有 4 个只读方法，零改动。

- [x] 1.2 在 `gormRuleRepository` 上实现 6 个方法（沿用 `admin_repository.go` 既有的 `Updates(map[string]any{...})` / `Delete(&T{})` 风格）：
  - `Create`：`repository.DB(c).Create(rule)`；
  - `Update`：`repository.DB(c).Save(rule)`（与既有 CRUDService 一致）；
  - `Delete`：`repository.DB(c).Where("id = ?", id).Delete(&model.AdminRule{})`；
  - `DeleteBatch`：空切片短路返回 nil；非空走 `repository.DB(c).Where("id IN ?", ids).Delete(&model.AdminRule{})`；
  - `UpdateStatus`：`Updates(map[string]any{"status": status})`；
  - `GetPID`：`repository.DB(c).Model(&model.AdminRule{}).Where("id = ?", id).Pluck("pid", &pid)`。
  验证：`go build ./...` exit 0；`*gormRuleRepository` 仍满足 `RuleRepository` 接口。

- [x] 1.3 新建 `internal/repository/admin/rule_repository_test.go`：用 sqlmock 覆盖 6 个新方法的 SQL 形态与空切片短路语义，沿用 `manager_repository_test.go:newMockContext` 的辅助函数：
  - `TestRepository_Create` 验证 `INSERT INTO admin_rule ...`；
  - `TestRepository_Update` 验证 `UPDATE admin_rule SET ... WHERE id = ?`；
  - `TestRepository_Delete` 验证软删 UPDATE；
  - `TestRepository_DeleteBatch` 验证 IN 子句软删；
  - `TestRepository_DeleteBatch_EmptyShortCircuit` 验证空切片零 SQL；
  - `TestRepository_UpdateStatus` 验证仅更新 status 列；
  - `TestRepository_GetPID` 验证 SELECT pid FROM admin_rule WHERE id = ?。
  验证：`go test ./internal/repository/admin/...` 全绿；现有 4 个只读方法的 sqlmock 测试不破。

## 2. Service — RuleService 接口、PID 校验、批量跳过

- [x] 2.1 新建 `internal/service/admin/rule.go`：定义 `Service` 接口与 sentinel 错误（design D2 / D7）：
  ```go
  var (
      ErrPIDCycle    = errors.New("rule: pid forms cycle")
      ErrHasChildren = errors.New("rule: rule has children")
      ErrPIDNotFound = errors.New("rule: pid not found")
  )
  type Service interface {
      service.CRUDService[model.AdminRule]
      ToggleStatus(c *gin.Context, id uint) error
      BatchDelete(c *gin.Context, ids []uint) (deleted, skipped int, err error)
      ListAll(c *gin.Context) ([]model.AdminRule, error)
  }
  ```
  验证：`go build ./...` exit 0。

- [x] 2.2 在 `baseRuleService` 上实现 `validatePID(c, id, newPID)` 私有 helper（design D3）：
  - `newPID == 0` → return nil；
  - `newPID == id` → return ErrPIDCycle；
  - BFS 上溯：`cur := newPID`，循环取 `repo.GetPID(c, cur)` 直到 `cur == 0` 或命中 `id`；
  - `visited` map 防御现存数据已有环导致的死循环；
  - `GetPID` 返回 gorm.ErrRecordNotFound 时区分：若在第一跳 newPID 上是 not-found → return ErrPIDNotFound；上溯过程中 not-found → 视为"祖先链断在合法点"，return nil。
  验证：`go build ./...` exit 0。

- [x] 2.3 覆盖 `Create`（design D3）：在转发 `s.CRUDService.Create` 之前调 `s.validatePID(c, 0, entity.Pid)`；非零 pid 且 `GetPID` 不存在 → ErrPIDNotFound。验证：`go test ./internal/service/admin/...` 中新增 happy / pid-zero / pid-cycle / pid-not-found 四个 case。

- [x] 2.4 覆盖 `Update`：在转发 `s.CRUDService.Update` 之前调 `s.validatePID(c, entity.ID, entity.Pid)`；service.GetByID 拿不到行 → gorm.ErrRecordNotFound 透传。验证：`go test` 覆盖 happy / self-cycle / descendant-cycle / not-found 四个 case。

- [x] 2.5 实现 `Delete`：转发前调 `hasChildren(c, id)`（内部走 `repo.GetPID` 取该 id 的子节点？不对——应该走单独子节点查询）：
  - 调整为新增私有 `hasChildren(c, id)` helper：走 `repo.HasChildren(c, id)`（Repository 加 1 个新方法，service 私有调用，避免被外部接口暴露）；
  - 有子 → ErrHasChildren；无子 → 转发 CRUDService.Delete。
  验证：`go test` 覆盖 happy / has-children / not-found。

- [x] 2.6 实现 `BatchDelete`：遍历 ids，对每个 id 调 `hasChildren` + 不存在则 skipped++；其余调 `Delete(c, id)`；返回 `(deleted, skipped, nil)`。
  验证：`go test` 覆盖 [3 个混合] / [空切片] / [全跳过] / [repo 错] 四种 case。

- [x] 2.7 实现 `ToggleStatus`：转发前 service.GetByID 拿行；`Status == 1` 切到 0，反之切到 1；调 `repo.UpdateStatus`。注意：与 admin-management 不同，**不联动 token 吊销**（规则不绑会话，design D2）。
  验证：`go test` 覆盖 1→0 / 0→1 / not-found / repo 错。

- [x] 2.8 实现 `ListAll`：复用 `repo.ListActiveAsMenu`（已存在），返回全量启用规则按 weigh/id 排序。
  验证：`go test` 覆盖 happy / repo 错。

- [x] 2.9 调整：把 2.5 提到的 `HasChildren(c, id)` 加到 `RuleRepository` 接口与 gormRuleRepository 实现（`SELECT 1 FROM admin_rule WHERE pid = ? AND deleted_at IS NULL LIMIT 1`，命中即 true）。验证：`go build ./...` exit 0；同步在 `rule_repository_test.go` 补 `TestRepository_HasChildren` SQL 形态断言。

- [x] 2.10 编译期断言：`var _ Service = (*baseRuleService)(nil)` 触发，验证 baseRuleService 实现 Service 接口。验证：`go build ./...` exit 0；移除断言后 build 仍成功但接口契约无法静态保证——保留断言。

- [x] 2.11 新建 `internal/service/admin/rule_test.go`：mock RuleRepository 覆盖上述所有 service 方法 + validatePID + hasChildren 私有 helper：
  - `TestValidatePID_TopLevel` / `TestValidatePID_SelfCycle` / `TestValidatePID_DescendantCycle` / `TestValidatePID_NotFoundOnNewPID` / `TestValidatePID_ExistingDataCycle`；
  - `TestCreate_HappyPath` / `TestCreate_PIDZero` / `TestCreate_PIDCycle` / `TestCreate_PIDNotFound`；
  - `TestUpdate_PIDCycleDescendant` / `TestUpdate_NotFound`；
  - `TestDelete_HasChildren` / `TestDelete_HappyPath`；
  - `TestBatchDelete_MixedSkipReasons` / `TestBatchDelete_Empty`；
  - `TestToggleStatus_1To0` / `TestToggleStatus_0To1` / `TestToggleStatus_NotFound`；
  - `TestListAll_ReturnsActiveMenu`。
  验证：`go test ./internal/service/admin/...` 全绿；既有 `admin_test.go` / `manager_service_test.go` / `permission_test.go` 不破。
  备注：与既有 `permission_test.go` 的同名 mock 与 `manager_service_test.go` 的同名测试名冲突，本文件改名为 `mockRuleRepo` / `TestRuleToggleStatus_NotFound`；同时给既有 permission_test.go 的 mockRuleRepository 与 init_test.go 的 initMockRule 补齐 9 个新接口方法的 stub 返回零值实现，保证 RuleRepository 接口契约。

## 3. Handler — RuleHandler 与 3 个业务专属方法

- [x] 3.1 新建 `internal/handler/admin/rule.go`：定义 `RuleHandler` 结构（design D1）：
  ```go
  type RuleHandler struct {
      *handler.BaseHandler[model.AdminRule]
      svc ruleSvc.Service
  }
  func NewRuleHandler(svc ruleSvc.Service) *RuleHandler
  ```
  嵌入 `*handler.BaseHandler[model.AdminRule]` 复用 5 条通用 CRUD；额外持有 `svc` 以便调用 3 个业务专属方法。

- [x] 3.2 在 `RuleHandler` 上新增 3 个方法（design D2 / D7）：
  - `ToggleStatus(c *gin.Context)`：解析 `{id}` → 调 `svc.ToggleStatus` → 错误映射（`gorm.ErrRecordNotFound` → 404 `rule.toggle_status.not_found`；其他 → 500）；
  - `BatchDelete(c *gin.Context)`：解析 `{ids: []uint}` → 调 `svc.BatchDelete` → 返回 `{deleted, skipped}`；
  - `ListAll(c *gin.Context)`：调 `svc.ListAll` → 返回 `[]model.AdminRule` JSON 数组。
  每个方法加 swag 注解（与 manager 风格一致：`@Summary` / `@Tags` / `@Param` / `@Router`）。注意 `ToggleStatus` 在 update 路径上还需检查 `ErrPIDCycle` 等 service 错误的映射（Create / EditPost 走 BaseHandler 默认 500——可在后续 task 覆盖）。

- [x] 3.3 在 `RuleHandler` 上覆盖 `EditPost` 与 `Create`：调用 BaseHandler 默认实现之前插入 service 层 PID 校验（design D3）：
  - `Create`：调 `svc.Create` 前先 `svc.ValidatePID`（新增 Service 接口方法，或在 BaseHandler.Create 内部用 service.Update + GetByID 替代）；
  - 调整：把 `validatePID` 升级为 `Service` 接口的公开方法 `ValidatePID(c, id, newPID)`，handler 在 Create / EditPost 入口调；
  - 错误码映射：`ErrPIDCycle` → 400 `rule.create.pid_cycle` / `rule.edit.pid_cycle`；`ErrPIDNotFound` → 400 `rule.create.pid_not_found` / `rule.edit.pid_not_found`；`ErrHasChildren` → 400 `rule.delete.has_children`（Delete 路径）。
  验证：`go build ./...` exit 0。
  备注：service 层的 Create/Update 已经做 PID 校验（rule.go 内 Create/Update override 了 BaseCRUDService），handler 只需在错误返回时把 sentinel 错误映射到对应 HTTP code + 错误码串，不需要再调一次 svc.ValidatePID。

- [x] 3.4 新建 `internal/handler/admin/rule_test.go`：仿 `manager_test.go` 的 `stubSvc` 风格，覆盖：
  - `TestRuleHandler_ListAll_ReturnsArray`：mock svc 返 3 条 → 响应 JSON 是 array；
  - `TestRuleHandler_ToggleStatus_NotFoundMapping`；
  - `TestRuleHandler_ToggleStatus_InternalMapping`；
  - `TestRuleHandler_BatchDelete_ResponseShape`：mock svc 返 `(1, 2, nil)` → 响应 `{deleted:1, skipped:2}`；
  - `TestRuleHandler_Create_PIDCycleMapping`：stub svc.Create 返 ErrPIDCycle → 400 `rule.create.pid_cycle`；
  - `TestRuleHandler_Create_PIDNotFoundMapping`：stub svc.Create 返 ErrPIDNotFound → 400 `rule.create.pid_not_found`；
  - `TestRuleHandler_Delete_HasChildrenMapping`：stub svc.Delete 返 ErrHasChildren → 400 `rule.delete.has_children`；
  - `TestRuleHandler_EditPost_PIDCycleMapping`：stub svc.Update 返 ErrPIDCycle → 400 `rule.edit.pid_cycle`。
  验证：`go test ./internal/handler/admin/...` 全绿（12 个新 case + 既有不破）。

## 4. Router — 8 条路由注册

- [x] 4.1 新建 `internal/router/admin/rule.go`：定义 `func registerRuleRoutes()`，调 `registry.Register` 挂 8 条路由（design D5）：
  - 5 条通用 CRUD（`BaseHandler.RegisterRoutes`）：`POST /admin/rule/create`、`GET /admin/rule/list`、`GET /admin/rule/edit`、`POST /admin/rule/edit`、`POST /admin/rule/delete`；
  - 3 条专属：`POST /admin/rule/toggle-status`、`POST /admin/rule/batch-delete`、`GET /admin/rule/all`；
  - 全部挂 `middleware.AdminAuth()`。
  每条用闭包包 `ensureRuleDeps()` 然后转发到 `ruleHandlerInst` 的对应方法。验证：`go build ./...` exit 0。
  备注：handler 延后到首次请求时通过 `ensureRuleDeps()` 装配；`NewRuleRepository(db)` 需要 `*gorm.DB`，所以从 `database.Get()` 拿，而不能像 admin 仓库那样在包级 var 装配。

- [x] 4.2 在 `internal/router/admin/admin.go` 的 `init()` 末尾追加 `registerRuleRoutes()`。验证：`go build ./...` exit 0；`grep "registerRuleRoutes" internal/router/admin/*.go` 命中 2 处。

- [x] 4.3 新建 `internal/router/admin/rule_test.go`：仿 `manager_test.go:38` 的 `engine.Routes()` 断言覆盖 8 条路由全注册：
  ```go
  var expectedRuleRoutes = []struct{ method, path string }{
      {http.MethodPost, "/admin/rule/create"},
      {http.MethodGet,  "/admin/rule/list"},
      {http.MethodGet,  "/admin/rule/edit"},
      {http.MethodPost, "/admin/rule/edit"},
      {http.MethodPost, "/admin/rule/delete"},
      {http.MethodPost, "/admin/rule/toggle-status"},
      {http.MethodPost, "/admin/rule/batch-delete"},
      {http.MethodGet,  "/admin/rule/all"},
  }
  ```
  验证：`go test ./internal/router/admin/...` 全绿（manager + rule 都过）。
  备注：registry.Apply 会清空 mounts，所以测试在 Apply 前补一次 `registerRuleRoutes()` 让本测试独立于 manager 测试的运行顺序。

## 5. 迁移 — 菜单规则种子

- [x] 5.1 新建 `cmd/migrate/migrations/000005_admin_rule_menu.up.sql`：按 design D6 插入一条 admin_rule 记录：
  ```sql
  INSERT INTO admin_rule (pid, type, title, name, path, component, status, created_at, updated_at)
  VALUES (0, 'menu', '菜单规则', 'admin/rule', 'rule', '/src/views/admin/rule/index.vue', 1, NOW(3), NOW(3))
  ON DUPLICATE KEY UPDATE title = VALUES(title);
  ```
  验证：文件存在；语法正确（可手动 `mysql -e "source ..."` 跑通）。

- [x] 5.2 新建 `cmd/migrate/migrations/000005_admin_rule_menu.down.sql`：`DELETE FROM admin_rule WHERE name = 'admin/rule' AND deleted_at IS NULL`。验证：文件存在。

- [x] 5.3 确认 `cmd/migrate/migrations/000002_admin_rule.up.sql` 中 `admin_rule` 表 schema 与 AdminRule 模型字段对齐；000005 引用所有列名不超出 baseline。验证：`cat 000005_*.up.sql` 与 `000002_*.up.sql` 列对齐。
  备注：000005 引用的全部 15 列（pid / type / title / name / path / component / open_type / url / keepalive / extend / remark / weigh / status / updated_at / created_at）均在 000002_admin_rule.up.sql 中存在；引用未超 baseline。

## 6. 前端 i18n

- [x] 6.1 修改 `web/src/lang/zh-cn/admin.yaml`：在 `manager.*` key 后追加 18 条 `rule.*` key（design D9）：
  - `rule.title: 菜单规则`、`rule.searchPlaceholder: 搜索标题 / 名称`；
  - `rule.columns.id / pid / type / title / name / path / icon / weigh / status / updated_at`；
  - `rule.dialog.create: 新建菜单规则`、`rule.dialog.edit: 编辑菜单规则`；
  - `rule.action.edit / toggleStatus / delete / batchDelete`；
  - `rule.confirm.delete / confirm.batchDelete`；
  - `rule.error.pidCycle: 不能将规则设为自身或子孙的子节点` / `rule.error.hasChildren` / `rule.error.pidNotFound` / `rule.error.notFound`。
  验证：文件存在。

- [x] 6.2 修改 `web/src/lang/en/admin.yaml`：镜像 18 条英文。验证：文件存在；`pnpm typecheck` exit 0。

- [x] 6.3 修改 `web/src/lang/zh-cn/pageTitles.yaml` 与 `web/src/lang/en/pageTitles.yaml`：追加 `adminRule: 菜单规则` / `adminRule: Menu Rule`。验证：`pnpm typecheck` exit 0。
  备注：追加项与文档中列出的 key 集合一致；typeOptions / statusOptions 视管理页表单下拉框需要而补，不在原 design 列举的 18 条内但与 columns 对齐 —— 属合理扩展。

## 7. 前端 API

- [x] 7.1 新建 `web/src/api/rule.ts`：按 design 导出 8 个函数：
  - `ruleList(params)` → `GET /admin/rule/list`；
  - `ruleGet(id)` → `GET /admin/rule/edit?id=`（复用 BaseHandler.EditGet）；
  - `ruleCreate(body)` → `POST /admin/rule/create`；
  - `ruleEdit(body)` → `POST /admin/rule/edit`（含 id）；
  - `ruleDelete(id)` → `POST /admin/rule/delete?id=`；
  - `ruleToggleStatus(body)` → `POST /admin/rule/toggle-status`；
  - `ruleBatchDelete(body)` → `POST /admin/rule/batch-delete`；
  - `ruleListAll()` → `GET /admin/rule/all` 返回 `AdminRule[]`。
  类型签名：
  ```ts
  type AdminRule = { id: number; pid: number; type: 'dir' | 'menu' | 'node'; title: string; name: string; path: string; icon: string; open_type: 'tab' | 'link' | 'iframe' | null; url: string; component: string; keepalive: boolean; extend: 'none' | 'add_route_only' | 'add_menu_only'; remark: string; weigh: number; status: 0 | 1; created_at: string; updated_at: string };
  ```
  验证：`pnpm typecheck` exit 0。
  备注：8 个函数全部导出；类型定义（AdminRule / RuleBody / RuleListResponse 等）放在文件顶部，命名风格与 web/src/api/admin/index.ts 对齐（AdminInfo / AdminBody）。

## 8. 前端管理页

- [x] 8.1 新建 `web/src/views/admin/rule/index.vue`：按 spec Requirement "Management UI is available at /admin/rule" 实现：
  - 工具栏：搜索框 + 新建按钮 + 批量删除按钮；
  - el-table 列：id / pid / type / title / name / path / icon / weigh / status / updated_at + 行内操作（编辑 / 启停 / 删除）；
  - el-pagination 分页；
  - 新建 / 编辑 弹窗：title / name / pid / type / path / icon / weigh / status；pid 走 el-select 下拉框，选项从 `ruleListAll()` 拉（顶级 pid=0 也作为单独 option）；
  - 行内操作：handleToggleStatus / handleDelete / handleBatchDelete（带 ElMessageBox 二次确认）。
  验证：文件存在；组件模板 + script + style 三段齐全。
  - 顶部：`<el-input>` 搜索框 + 「新增」按钮；
  - `<el-table>` 列出规则，列含 id / pid / type / title / name / path / icon / weigh / status / updated_at，行内操作按钮（编辑 / 启停 / 删除）；
  - 「批量删除」按钮 + 多选；
  - 「新增 / 编辑」弹窗（`<el-dialog>`）：表单字段 pid（el-tree-select，调 `ruleListAll`）/ type / title / name / path / icon / open_type / url / component / keepalive / extend / remark / weigh / status；
  - 调 `web/src/api/rule.ts` 的 8 个函数；
  - 表格 mount 时调 `ruleList()` 拉首屏；el-tree-select mount 时调 `ruleListAll()` 拉全量。
  验证：`pnpm typecheck` exit 0；`pnpm lint` exit 0（应用 prettier）。

- [x] 8.2 在 `web/src/router/static/adminBase.ts` 的 `children` 中、`manager` 之后、`:path(.*)*` 之前追加（参考 manager 路由 D9）：
  ```ts
  {
      path: 'rule',
      name: 'adminRule',
      component: () => import('/@/views/admin/rule/index.vue'),
      meta: { title: 'pageTitles.adminRule' },
  },
  ```
  验证：`pnpm typecheck` exit 0。
  备注：实际文件已加 `noAuth: false` 与 manager 一致；title 用 `pageTitles.adminRule`（i18n key）。

## 9. 集成验证

- [x] 9.1 后端 `go build ./...` + `go vet ./...` exit 0。验证：仅保留 `scripts/dump_gorm_schema` 既有告警。

- [x] 9.2 `go test ./internal/handler/admin/... ./internal/service/admin/... ./internal/repository/admin/... ./internal/router/admin/...` 全绿。验证：无 FAIL。

- [x] 9.3 前端 `pnpm typecheck` exit 0（仅 `web/src/utils/random.ts` 既有错误可保留）。验证：除 pre-existing 错误外 0 命中。

- [x] 9.4 `pnpm lint` 仅既有告警；新增文件零告警。验证：`npx eslint web/src/views/admin/rule/index.vue web/src/api/rule.ts` 无输出（运行后 ESLint 无任何告警；自动 fix 修复了 2 个 prettier 行尾换行）。

- [ ] 9.5（需 MySQL）跑 `go run ./cmd/migrate up`，确认 000005 迁移成功，`SELECT * FROM admin_rule WHERE name='admin/rule'` 返回 1 行。验证：迁移无 error。

- [ ] 9.6（需 MySQL + dev server）`go run ./cmd/serve` + `pnpm dev`，admin 登录后访问 `/admin/rule`：
  - 表格加载；
  - 父节点下拉框加载全量启用规则；
  - 新增一条规则 → 列表刷新；
  - 编辑改 title → 表格刷新 + `updated_at` 更新；
  - PID 自循环 / 改成子孙 → 400 `rule.edit.pid_cycle`；
  - 启停 → status 切换；
  - 删有子规则 → 400 `rule.delete.has_children`；
  - 删叶规则 → 软删成功；
  - 批量删除混合选中 → 仅叶规则被删。
  验证：浏览器目测 + curl 双验证。
  备注：9.5 / 9.6 需要 MySQL + 启动 dev server，当前沙箱无 MySQL，留给用户本地或 CI 验证。
