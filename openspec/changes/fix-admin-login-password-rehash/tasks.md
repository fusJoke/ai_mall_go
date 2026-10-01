# Tasks

## 1. Repository — 新增登录路径定向列更新

- [x] 1.1 在 `internal/repository/admin/admin.go` 的 `Repository` 接口追加 2 个方法（design D2 / D3）：
  - `UpdateLoginFailure(c *gin.Context, id uint, failure int, locked bool) error`
  - `UpdateLoginSuccess(c *gin.Context, id uint, ip string, at time.Time) error`
  验证：`go build ./...` exit 0（编译期断言会先报 mock 未实现，属预期）。

- [x] 1.2 在 `baseRepository` 上实现 2 个方法，沿用 `Updates(map[string]any{...})` 风格：
  - `UpdateLoginFailure`：map 含 `"login_failure": failure`；`locked=true` 时追加 `"status": int8(0)`；单条 `UPDATE admins WHERE id = ?`；
  - `UpdateLoginSuccess`：map 含 `"login_failure": 0, "last_login_at": at, "last_login_ip": ip`。
  验证：`go build ./...` exit 0；`*baseRepository` 仍满足 `Repository`。

- [x] 1.3 新增 sqlmock 用例（`internal/repository/admin/admin_repository_test.go`，沿用 `manager_repository_test.go:newMockContext` 模式）：
  - `TestRepository_UpdateLoginFailure`：断言 SET 列恰为 `login_failure`（locked=false）；
  - `TestRepository_UpdateLoginFailure_Locked`：断言 SET 列为 `login_failure, status`；
  - `TestRepository_UpdateLoginSuccess`：断言 SET 列恰为 `login_failure, last_login_at, last_login_ip`，且 **不含 `password`**。
  验证：`go test ./internal/repository/admin/...` 全绿。

## 2. Service — Login 切换到定向更新

- [x] 2.1 密码错误分支（`admin.go:197` 附近）：`s.Update(c, adm)` → `s.repo.UpdateLoginFailure(c, adm.ID, failure, locked)`；`failure` / `locked` 计算逻辑与阈值判定保持在 service 层；`tm.Clear` 吊销时机不变。
- [x] 2.2 登录成功分支（`admin.go:211` 附近）：`s.Update(c, adm)` → `s.repo.UpdateLoginSuccess(c, adm.ID, c.ClientIP(), now)`；`now` 与 token `ExpiresAt` 共用同一 `time.Time`。
- [x] 2.3 确认 `s.Update` 在 service/admin 包内无其他完整模型调用点（`grep -n "s.Update(" internal/service/admin`）。
  验证：`go build ./...` exit 0。

## 3. Tests — 迁移断言 + 回归锁死

- [x] 3.1 `admin_test.go` mock 扩展：`updateLoginFailureFunc` / `updateLoginSuccessFunc` 可编程字段与调用记录（calls / last args）。
- [x] 3.2 迁移既有 Login 用例断言：`TestLogin_WrongPassword` / `TestLogin_WrongPassword_LocksAtThreshold` / `TestLogin_WrongPassword_BelowThreshold_NoClear` / `TestLogin_Success_ResetsLoginFailure` 等由 `repo.updateCalls` 改为对应新方法断言（锁定用例断言 `locked=true`，未锁定 `locked=false`，成功用例断言三列值）。
- [x] 3.3 新增回归用例：`TestLogin_WrongPassword_NeverTouchesUpdate`（失败路径 `repo.updateCalls == 0`）与 `TestLogin_Success_NeverTouchesUpdate`（成功路径 `repo.updateCalls == 0`）—— 防止再次把完整模型喂给 Update。
  验证：`go test ./internal/service/admin/...` 全绿。

## 4. 验证与收尾

- [x] 4.1 `go build ./...` + `go vet ./...` + `go test ./internal/...` 全绿。
- [x] 4.2 GUI 回归（黑盒）：`go run ./cmd/seed` 重置 root → 登录成功 → 登出 → 同密码再登录**成功**（缺陷确证实验反转）；DB 哈希在两次登录间保持不变。
- [x] 4.3 `openspec validate fix-admin-login-password-rehash` 通过；勾选本文件所有任务。
