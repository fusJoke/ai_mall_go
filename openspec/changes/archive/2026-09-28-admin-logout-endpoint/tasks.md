# Tasks

## 1. Backend: service 层扩展

- [x] 1.1 扩展 `internal/service/admin/admin.go` 的 `tokenIssuer` 接口，加 `Delete(ctx context.Context, rawToken string) error` 方法。`var _ tokenIssuer = (*token.Manager)(nil)` 编译期断言 `*token.Manager` 仍满足接口（`Manager.Delete` 已在 `internal/infra/token/token.go:131-137`）。验证：`go build ./...` 退出码 0。
- [x] 1.2 在 `internal/service/admin/admin.go` 的 `Service` 接口（`admin.go:71-85`）加 `Logout(ctx context.Context, rawToken string) error`；在 `*baseService` 实现：直接 `return s.tm.Delete(ctx, rawToken)`。验证：`go build ./...` 退出码 0。

## 2. Backend: handler 层新增 Logout

- [x] 2.1 在 `internal/handler/admin/admin.go` 的 `Login` 之后新增 `Logout(c *gin.Context)`。先 `authz := c.GetHeader("Authorization")`；`rawToken := strings.TrimSpace(strings.TrimPrefix(authz, "Bearer "))`；若 `rawToken == ""` 直接 `c.JSON(200, gin.H{"code": "logout.ok", "message": "no active session"})` 返回。否则 `err := h.svc.Logout(c.Request.Context(), rawToken)`，错误时 `c.JSON(500, gin.H{"code": "logout.internal", "message": err.Error()})`，成功 `c.JSON(200, gin.H{"code": "logout.ok", "message": "ok"})`。验证：缺失 `strings` import 时补上；`go build ./...` 退出码 0。
- [x] 2.2 在 `Logout` 方法上方加 swagger 注释（紧贴 Login 的注释风格）：`@Summary` 管理员登出 / `@Description` 软删除当前调用方持有的 admin token；幂等 —— 缺失 / 格式错误 / 已过期 / 不存在都返回 200 / `@Tags admin` / `@Accept json` / `@Produce json` / `@Success 200 {object} map[string]string "logout.ok"` / `@Failure 500 {object} map[string]string "logout.internal"` / `@Router /admin/logout [post]`。验证：grep `internal/handler/admin/admin.go` 看到完整注释块。

## 3. Backend: 路由注册

- [x] 3.1 在 `internal/router/admin/admin.go` 的 `init()` 内紧跟现有 `/admin/login` 注册之后追加：`registry.Register("/admin", http.MethodPost, "/logout", func(c *gin.Context) { ensureDeps(); adminHandlerInst.Logout(c) })`。验证：`go build ./...` 退出码 0。

## 4. Backend: 单元测试

- [x] 4.1 扩展 `internal/service/admin/admin_test.go` 的 `mockIssuer`，新增 `Delete` 方法实现 + 记录字段（`deleteCalled bool`、`deleteToken string`、`deleteErr error`）。同时新增 `var _ tokenIssuer = (*mockIssuer)(nil)` 编译期断言（即使既有测试没加，加了能兜底未来）。验证：`go build ./...` 退出码 0。
- [x] 4.2 新增 `TestLogout_Success`：mockIssuer.Delete 返回 nil；service.Logout(ctx, "abc.def") 返回 nil；断言 `mock.deleteCalled == true && mock.deleteToken == "abc.def"`。验证：`go test -run TestLogout_Success ./internal/service/admin/...` 通过。
- [x] 4.3 新增 `TestLogout_DeletePropagatesError`：mockIssuer.Delete 返回哨兵 `errMock`；service.Logout 返回的错误 `errors.Is(err, errMock)` 为 true。验证：`go test -run TestLogout_DeletePropagatesError ./internal/service/admin/...` 通过。
- [x] 4.4 新增 `TestLogout_NoRepoAccess`：mock repo 计数器断言 logout 流程不触达 admin repo（只走 tm）。验证：`go test -run TestLogout_NoRepoAccess ./internal/service/admin/...` 通过。

## 5. Backend: 验证

- [x] 5.1 跑 `go test ./internal/service/admin/...`，确认新增 3 个 logout 测试 + 既有 login 测试全绿。验证：测试输出 PASS，无 FAIL。
- [x] 5.2 跑 `go test ./...`，确认全仓干净（包括 service、handler、router、token infra 各包）。验证：测试输出 PASS，无 FAIL。
- [x] 5.3 跑 `swag init -g cmd/serve/main.go`（项目根），重新生成 `docs/docs.go`。验证：`docs/swagger.json` 内出现 `/admin/logout` 路径条目。
- [x] 5.4 跑 `go run ./cmd/serve` 启动，gin-debug 日志确认 `[GIN-debug] POST /admin/logout` 已注册；swagger UI 由 `docs/swagger.json` 自动渲染，无需手动验证。
- [x] 5.5 curl 联调：四次响应均 200 `logout.ok`（无 header → "no active session"；bogus bearer / malformed `Token` 前缀 / 空 bearer → "ok"，均由 Manager.Delete 幂等 no-op 保证）。

## 6. Frontend: 类型 + API

- [x] 6.1 在 `web/src/stores/interface/index.ts` 新增 `export interface LogoutResponse { code: string; message: string }`（信封形状与 login 响应一致）。验证：grep `web/src/stores/interface/index.ts` 看到该接口。
- [x] 6.2 在 `web/src/api/admin/index.ts` 新增 `logout()`：调用 `request.request<LogoutResponse>({ url: '/admin/logout', method: 'POST', __opts: { showErrorMessage: false } })`，并在文件顶部 import `LogoutResponse`。验证：grep `web/src/api/admin/index.ts` 看到 `logout` 函数与 `LogoutResponse` 引用。

## 7. Frontend: store action

- [x] 7.1 在 `web/src/stores/adminInfo.ts` 的 actions 段新增 `reset()` action：先 `removeToken()`，再 `this.$patch({ id: 0, username: '', nickname: '', avatar: '', last_login_at: '', last_login_ip: '', super: false })`。不在注释里写 `$reset()` 字样避免误导。验证：grep `web/src/stores/adminInfo.ts` 看到 `reset` action。

## 8. Frontend: 临时注销 bar

- [x] 8.1 改 `web/src/views/admin/login.vue`：`<script setup>` 顶部新增 `import { logout } from '/@/api/admin'` 与 `const adminInfo = useAdminInfo()`。template 内 `.page` 顶部加 `v-if="adminInfo.token"` 的 `<div class="logged-in-bar">已登录为 {{ adminInfo.username }} <button type="button" class="link-btn" @click="doLogout">注销</button></div>`，并在注释里写明 `TODO(admin-header): replace this banner`。script 加 `async function doLogout() { try { await logout() } catch { /* 静默 */ } adminInfo.reset() }`。验证：grep `login.vue` 看到 `adminInfo.token` 与 `doLogout`。

## 9. Frontend: 验证

- [x] 9.1 跑 `pnpm typecheck`，确认新接口 / store action / login.vue 模板编译干净。验证：本变更触及的 4 个文件（`web/src/api/admin/index.ts`、`web/src/stores/adminInfo.ts`、`web/src/stores/interface/index.ts`、`web/src/views/admin/login.vue`）零 vue-tsc error；仓内另有 4 个 pre-existing `window.unique` 报错位于 `web/src/utils/random.ts`（`f29c936` 提交引入，与本 PR 无关）。
- [x] 9.2 起 `pnpm dev`，浏览器手工验证（Playwright 联调）：
  - 未登录访问 `/admin/login` → 看到原表单（无注销 bar）。✅
  - 注入 fake adminInfo 后访问 → 看到 "已登录为 alice [注销]"。✅
  - 点击 [注销] → DevTools Network 看到 `POST http://localhost:8080/admin/logout` 已发（后端未起 → ECONNREFUSED，验证 try/catch 路径生效）；bar 消失；`localStorage.adminInfo` 清零为 `{id:0,username:'',token:'',super:false,...}`。✅
  - 重新注入 adminInfo → bar 重新出现 with "已登录为 bob"，证明 store 状态正确 reset，可重新登录。✅
  验证：四条目测全部通过。
