# Proposal

## Why

2026-10-01 的后台登录 GUI 验证发现 P0 缺陷：`Login` 的密码错误分支与登录成功分支都以**完整模型**调用 `s.Update(c, adm)`（`internal/service/admin/admin.go:197` / `:211`），而 `baseService.Update` → `hashPasswordIfNeeded(isUpdate=true)` 的语义是「Password 非空即 bcrypt」（`admin.go:279`）。模型带出的 `Password` 本就是 bcrypt 哈希（60 字符 ≥ MinPasswordLength），于是每次登录尝试都会把 DB 里的哈希再哈希一遍：

- 密码错一次 → 失败计数更新时哈希被二次哈希 → **正确密码从此永久失效**；
- 密码正确登录成功 → 记录 last_login 的更新同样把哈希二次哈希 → **第二次登录必定失败**。

实测确证（root / Passw0rd!，逐次比对 DB 哈希）：seed 后登录成功 → 哈希变为 `bcrypt(bcrypt(pw))` → 同密码再登录 401。该缺陷同时解释了此前 `login-error.png` 的 401 与库中 `admin` 账号 login_failure=8 的历史状态。

本 change 将 `Login` 两条路径的落库改为「仅改指定列」的定向更新，杜绝 Password 字段被意外改写。

## What Changes

- **扩展 `internal/repository/admin/admin.go` 的 `Repository` 接口**：新增 2 个定向列更新方法，沿用既有 `UpdateStatus` / `ResetLoginFailure` 的 `Updates(map[string]any{...})` 风格：
  - `UpdateLoginFailure(c, id, failure int, locked bool)` —— 写 `login_failure`；`locked=true` 时同时写 `status=0`（锁定）；
  - `UpdateLoginSuccess(c, id, ip string, at time.Time)` —— 一次性写 `login_failure=0`、`last_login_at`、`last_login_ip`。
- **修改 `internal/service/admin/admin.go` 的 `Login`**：
  - 密码错误分支：`s.Update(c, adm)` → `s.repo.UpdateLoginFailure(...)`；锁定判定仍由 service 层计算（阈值逻辑不变），token 吊销时机不变；
  - 登录成功分支：`s.Update(c, adm)` → `s.repo.UpdateLoginSuccess(...)`。
- **更新 `internal/service/admin/admin_test.go`**：mock 补齐 2 个新方法；原断言 `repo.updateCalls` 的 Login 用例改为断言新方法；新增回归测试 —— 密码错误后与登录成功后 **`Update`（含哈希逻辑）零调用、DB 侧密码哈希值不参与任何写路径**。
- **新增 `internal/repository/admin/admin_repository_test.go` 中的 sqlmock 用例**：覆盖 2 个新方法的 SQL 形态（列集合、锁定时才含 status 列）。
- **不改**：`hashPasswordIfNeeded` 的「非空即哈希」语义（管理页部分更新仍依赖它）；`MaxLoginFailure` 阈值逻辑；token 签发 / 吊销链路；前端任何代码。

### 非目标

- **不改 `baseService.Update` 的语义**：「非空则改密」对管理页 DTO 部分更新是正确设计；本次只消灭"把已加载的完整模型喂给 Update"这一误用形态。
- **不做哈希格式探测式防御**（如"看起来已是 bcrypt 就跳过"）：该方案掩盖调用方错误、对非 bcrypt 未来算法脆弱；正确解法是让写路径本身不携带 Password。
- **不迁移既有已腐化的哈希**：已被二次哈希的行（如 `admin` id=1）无法逆向恢复，只能走管理页改密或 `cmd/seed` 重置；本 change 不提供数据修复脚本。
- **不处理并发失败计数竞争**：两路并发失败同写 login_failure 的丢更新窗口与现状一致，锁定方向幂等（都朝 status=0 写），留待后续引入原子 `login_failure = login_failure + 1` 时统一处理。

## Capabilities

### New Capabilities

无。

### Modified Capabilities

- `admin-login`: 「失败计数维护」与「登录成功」的落库方式收敛为定向列更新 —— 失败计数与 last_login 信息更新 MUST NOT 触发 Password 字段改写；新增"密码哈希不可变性"需求，防止任何登录路径重写哈希。

## Impact

- **代码**：`internal/repository/admin/admin.go`（+2 接口方法 +2 实现）、`internal/service/admin/admin.go`（Login 两分支各 1 处调用替换）、`internal/service/admin/admin_test.go`（mock 扩展 + 用例断言迁移 + 回归用例）、`internal/repository/admin/admin_repository_test.go`（+sqlmock 用例）。
- **规格**：`openspec/specs/admin-login/spec.md` 由本 change 的 delta 修改（MODIFIED：失败计数维护；ADDED：密码哈希不可变性）。
- **数据**：无 schema 变更、无迁移文件；已被腐化的存量哈希需人工重置（管理页改密 / `cmd/seed`）。
- **验证**：`go build ./...` + `go test ./internal/service/admin/... ./internal/repository/admin/...`；GUI 回归 = 登录 → 登出 → 同密码再登录应成功。
