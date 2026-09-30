# Tasks

## 1. Repository 层扩展

- [x] 1.1 在 `internal/repository/admin/rule_repository.go` 中追加 `ListActiveAsMenu() ([]model.AdminRule, error)` 方法：单条 `SELECT * FROM admin_rule WHERE status = 1 ORDER BY weigh ASC, id ASC`，由 GORM 自动过滤软删。验证：`go build ./...` exit 0；接口签名与 design D5 一致。

- [x] 1.2 新建 `internal/repository/admin/config_repository.go`：定义 `ConfigRepository` 接口（含 `ListByNames(names []string) ([]model.Config, error)`），以及 GORM 实现（`WHERE name IN ?` 单条查询，name 列表为空返回空切片不查 DB）。验证：`go build ./...` exit 0；单元测试 `TestConfigRepository_ListByNames` 覆盖"齐全 / 部分缺失 / 全部缺失 / 空 names"四种场景，使用 sqlmock 或 mock DB（mock 接口实现亦可，参考 `access_repository.go` 风格）。

## 2. 中间件：把 admin 实体挂到 context

- [x] 2.1 扩展 `internal/middleware/auth.go` 的 `AdminAuth`：在 token 校验通过后，调 `adminRepo.NewAdminRepository(db)`（已有包 `internal/repository/admin`）查 admin，路径 `database.Get()`；`c.Set(adminContextKey, admin)`（新增包级常量 `adminContextKey = "admin.current"`）；提供 `AdminFromContext(c *gin.Context) *model.Admin` 导出 helper。验证：`go build ./...` exit 0；既有 `auth_test.go` 用 fake `checkToken` 不受影响（admin 查不到的路径 fallback 为 nil，不影响 401 路径）。

- [x] 2.2 在 `internal/middleware/auth_test.go` 中追加 `TestAdminAuth_SetsAdminInContext`：fake `checkToken` 返回合法 token + 用 adminRepo mock 实现，断言 `c.Get(adminContextKey)` 拿到的 admin 字段与 mock 一致；token 无效路径断言 context 中无 admin。验证：`go test -run TestAdminAuth ./internal/middleware/...` 全绿。

## 3. Service 层：InitService

- [x] 3.1 新建 `internal/service/admin/init.go`：定义 `InitService` 接口（`Init(c *gin.Context, uid uint) (*InitResponse, error)`）、`InitResponse` 结构体（`Admin adminInfo` / `SiteConfig map[string]string` / `Menus []map[string]any`）、`initService` 实现。验证：`go build ./...` exit 0。

- [x] 3.2 在同文件实现 `routeFromRule(*model.AdminRule) map[string]any` 纯函数（design D3 表）：空 name/component 退化规则严格按 spec ADDED Requirement 6。验证：单元测试 `TestRouteFromRule` 覆盖"menu+完整字段 / 目录 dir / 权限节点 node / 空 component / 空 name / open_type 为 nil"六个用例，全部 PASS。

- [x] 3.3 在同文件实现 `Init(c, uid)`：admin 校验（status=1，超管标识来自 Permission Manager）；查 site_config（`ConfigRepository.ListByNames(["name", "record_number", "version"])` 转 map，缺失键填空串）；查 menus（超管走 `RuleRepository.ListActiveAsMenu`，普通走 `PermissionManager.GetRules`，两者结果都过 `routeFromRule` 转 map 数组）；组装 `InitResponse`。验证：单元测试 `TestInitService_Init` 用 4 个 mock（AdminRepository / RuleRepository / GroupRepository / AccessRepository / ConfigRepository）+ Permission Manager 实际构造，覆盖"超管 / 普通 / 禁用管理员 / config 缺失"四种场景，全部 PASS。

## 4. Handler 层

- [x] 4.1 新建 `internal/handler/admin/init.go`：定义 `InitHandler`（持有 `adminSvc.InitService` + `AdminRepository`）+ `Init(c *gin.Context)` 方法；通过 `middleware.AdminFromContext(c)` 拿当前 admin；调 `h.initSvc.Init(c, uid)`；按 spec D9 分类错误：401 走中间件、403 返 `admin.account_disabled`、500 返 `admin.init.internal` envelope。验证：`go build ./...` exit 0；handler 单测 `TestInitHandler_Init` 用 mock InitService + 假 context，覆盖"成功 / 403 / 500"三种响应分支。

## 5. 路由注册

- [x] 5.1 修改 `internal/router/admin/admin.go` 的 `init()`：注册 `GET /admin/init`，挂 `middleware.AdminAuth()`；在 `ensureDeps()` 内同步装配 `adminInitHandlerInst`（与 `adminHandlerInst` 同一 `sync.Once`）；构造时传入 Permission Manager（`adminService.PermissionDefault()`） + Config Repository（`adminRepo.NewConfigRepository(db)`） + Admin Repository（已有）。验证：`go build ./...` exit 0；手动验证：`curl -i http://localhost:PORT/admin/init` 缺 token 返回 401。

## 6. 前端类型 + API

- [x] 6.1 在 `web/src/stores/interface/index.ts` 中追加三个类型：`SiteConfig { name: string; record_number: string; version: string }` / `MenuRule { id: number; pid: number; type: 'dir' | 'menu' | 'node'; title: string; name: string; path: string; icon: string; open_type?: 'tab' | 'link' | 'iframe'; url: string; component: string; keepalive: boolean; extend: string; weigh: number }` / `InitResponse { admin: AdminInfo; site_config: SiteConfig; menus: MenuRule[] }`。验证：`tsc --noEmit` 无类型错误（若 web 包配了 tsc）。

- [x] 6.2 在 `web/src/api/admin/index.ts` 中追加 `init()` 函数：`request.request<InitResponse>({ url: '/admin/init', method: 'GET', __opts: { showErrorMessage: false } })`，文件顶部 JSDoc 注明"调用于登录后的后台初始化，失败由 layout 层处理"。验证：`pnpm typecheck` 或 `tsc --noEmit` 无错误。

## 7. 前端 Store 扩展

- [x] 7.1 修改 `web/src/stores/config.ts`：在 composition store 内增加 `siteConfig = reactive<SiteConfig>({ name: '', record_number: '', version: '' })` + `setSiteConfig(data: Partial<SiteConfig>) { Object.assign(siteConfig, data) }`；return 中暴露 `siteConfig, setSiteConfig`。验证：`pnpm typecheck` 无错误；手测：`useConfig().setSiteConfig({ name: 'X' })` 后 `useConfig().siteConfig.name === 'X'`。

## 8. 前端 Layout：init 流程

- [x] 8.1 修改 `web/src/layouts/admin/index.vue`：新增 `<script setup lang="ts">`：导入 `onMounted` / `useRouter` / `useAdminInfo` / `useConfig` / `useMenu` / `init`；在 `onMounted` 内执行 spec ADDED Requirement 5 描述的 6 步流程；`router.addRoute` 前先 `if (router.hasRoute(route.name)) router.removeRoute(route.name)` 去重；menu `type === 'node'` 的跳过 `addRoute`。验证：`pnpm dev` 登录后浏览器跳到第一个菜单的 URL，刷新页面后不出现重复菜单 / 空白；token 失效时跳回登录页。

## 9. 集成验证

- [x] 9.1 运行 `go build ./...` + `go test ./internal/middleware/... ./internal/handler/admin/... ./internal/service/admin/... ./internal/repository/admin/...`，确认 2.2 / 3.2 / 3.3 / 4.1 全部 PASS。验证：测试输出无 FAIL。

- [x] 9.2 运行 `gofmt -l internal/middleware/auth.go internal/handler/admin/init.go internal/service/admin/init.go internal/repository/admin/config_repository.go internal/repository/admin/rule_repository.go`，确认无输出。验证：命令无输出。

- [ ] 9.3（可选，本地有 MySQL 时执行）`curl -i -H "Authorization: Bearer $TOKEN" http://localhost:PORT/admin/init`，响应 200 且 body 含 `admin / site_config / menus` 三字段；`site_config` 仅含 `name / record_number / version` 三键；超管 token 时 `menus.length` 等于 `SELECT COUNT(*) FROM admin_rule WHERE status=1`。