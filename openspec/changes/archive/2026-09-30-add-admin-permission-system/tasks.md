# Tasks

## 1. 模型定义

- [x] 1.1 在 `internal/model/admin.go` 中追加 `AdminGroup` 结构体（id / pid / name / rules / status / 3 个时间戳字段），实现 `TableName(namer)` 走 `namer.TableName("admin_group")`，与既有 `Admin` / `AdminRule` 风格一致；更新 `init()` 在 `Register(...)` 中加入 `AdminGroup`。验证：`gofmt -l internal/model/admin.go` 无输出；`grep -c "AdminGroup{}" internal/model/admin.go` ≥ 1。

- [x] 1.2 在 `internal/model/admin.go` 中追加 `AdminGroupAccess` 结构体（复合主键 `uid` + `group_id`；均非自增；带 3 个时间戳字段），实现 `TableName(namer)` 走 `namer.TableName("admin_group_access")`；在 `init()` 中追加 `AdminGroupAccess`。验证：`grep "primaryKey" internal/model/admin.go` 至少出现 2 处（`uid` + `group_id` 各一）；`go build ./...` exit 0。

- [x] 1.3 扩展 `internal/model/model_test.go` 中 `TestRegister_AndAll`：在 `got` 检查里追加 `AdminGroup` 与 `AdminGroupAccess` 的反射类型断言。验证：`go test -run TestRegister_AndAll ./internal/model/...` 通过。

## 2. 迁移文件

- [x] 2.1 新建 `cmd/migrate/migrations/000003_admin_group.up.sql`：包含 `CREATE TABLE IF NOT EXISTS admin_group`（与 model `AdminGroup` 字段一一对应：id UNSIGNED AUTO_INCREMENT、pid UNSIGNED NULL、name varchar(100) NOT NULL DEFAULT ''、rules text NULL、status tinyint NOT NULL DEFAULT 1、created_at/updated_at datetime(3) + autoMaintain、deleted_at datetime(3) NULL）与 `CREATE TABLE IF NOT EXISTS admin_group_access`（uid UNSIGNED NOT NULL、group_id UNSIGNED NOT NULL、3 个时间戳、`PRIMARY KEY (uid, group_id)`、INDEX (uid) 与 INDEX (group_id)）。CHARSET utf8mb4 / COLLATE utf8mb4_general_ci 与 baseline 一致。验证：文件存在；`go run ./cmd/migrate version` 在应用前为 2（baseline + admin_rule）。

- [x] 2.2 新建 `cmd/migrate/migrations/000003_admin_group.down.sql`：按建表逆序 `DROP TABLE IF EXISTS admin_group_access; DROP TABLE IF EXISTS admin_group;`。验证：文件存在；本地 `go run ./cmd/migrate down 1` 可成功回滚（确保两次 down 不报脏状态）。

## 3. Repository 层

- [x] 3.1 新建 `internal/repository/admin/admin_repository.go`：定义 `AdminRepository` 接口（含 `GetByID(uid uint) (*model.Admin, error)`），以及基于 GORM 的 `gormAdminRepository` 实现。验证：`go build ./...` exit 0；接口签名与 design D11 数据流中"入口 admin 校验"步骤一致。

- [x] 3.2 新建 `internal/repository/admin/rule_repository.go`：定义 `RuleRepository` 接口（含 `ListByIDs(ids []uint) ([]model.AdminRule, error)`、`ExistsByIDsAndName(ids []uint, name string) (bool, error)`），以及 GORM 实现。验证：`go build ./...` exit 0；两个方法均使用单条 `WHERE id IN (?)` 避免 N+1。

- [x] 3.3 新建 `internal/repository/admin/group_repository.go`：定义 `GroupRepository` 接口（含 `ListByIDs(ids []uint) ([]model.AdminGroup, error)`），以及 GORM 实现。验证：`go build ./...` exit 0。

- [x] 3.4 在同文件 `group_repository.go` 中追加纯函数 `ParseRuleIDs(s string) (wildcard bool, ids []uint)`：实现 design D4 解析契约（整串 `*` → wildcard=true；空字符串 → 空；其他按 `,` split、trim、跳过空段、`strconv.ParseUint(_, 10, 64)`）。验证：`go test -run TestParseRuleIDs ./internal/repository/admin/...` 全绿；用例覆盖 `"*"`、`""`、`"1,2,3"`、`" 1, 2 , 5 "`、`"abc,1"`、`","`。

- [x] 3.5 新建 `internal/repository/admin/access_repository.go`：定义 `AccessRepository` 接口（含 `ListGroupIDsByUID(uid uint) ([]uint, error)`），以及 GORM 实现（单条 `SELECT group_id FROM admin_group_access WHERE uid = ? AND deleted_at IS NULL`，返回去重 id 切片）。验证：`go build ./...` exit 0。

## 4. Service 层（Permission Manager）

- [x] 4.1 新建 `internal/service/admin/permission.go`：定义 `Repositories` 结构体（4 个 Repository 接口作为字段）、`Manager` 结构体（持有 `Repositories`）、`New(repos Repositories) *Manager` 构造函数、`Default()` 包级便捷函数（从 `database.Get()` 构造具体实现）。验证：`go build ./...` exit 0；`internal/service/admin/` 目录按 CLAUDE.md "Service 按身份 sub-directory" 规范创建。

- [x] 4.2 在同文件实现 `IsSuperAdmin(uid uint) bool`：按 design D11 入口校验 admin；查 group_ids；查 groups（status=1 过滤）；任一 group 的 `ParseRuleIDs` 返回 wildcard=true 即 true。验证：单元测试 `TestPermission_IsSuperAdmin` 覆盖 wildcard / no-wildcard / no-group / disabled-admin 四个用例，全部 PASS。

- [x] 4.3 在同文件实现 `GetGroups(uid uint) []model.AdminGroup`：入口 admin 校验；查 groups；按 status=1 过滤（deleted_at 由 GORM 自动过滤）；返回切片（nil-safe，nil 切片在外呈现为空）。验证：单元测试 `TestPermission_GetGroups` 覆盖 multi-group / no-group / disabled-group-excluded / soft-deleted-group-excluded 四个用例，全部 PASS。

- [x] 4.4 在同文件实现 `GetRuleIds(uid uint) []uint`：入口 admin 校验；聚合所有 group 的 rule id 集合（wildcard 时短路返回 `RuleRepository.ListAllIDs()` 的升序切片）；去重 + 升序排序。验证：单元测试 `TestPermission_GetRuleIds` 覆盖 super-admin / regular / no-group / empty-rules 四个用例，全部 PASS。

- [x] 4.5 在同文件实现 `GetRules(uid uint) []model.AdminRule`：复用 GetRuleIds 拿 id 集合；按 id 批量拉规则（`status=1` 过滤）；按 `weigh ASC, id ASC` 排序。验证：单元测试 `TestPermission_GetRules` 覆盖 super-admin / regular-multi-group / no-group / ordering 四个用例，全部 PASS。

- [x] 4.6 在同文件实现 `Check(uid uint, ruleName string) bool`：入口 admin 校验；查 groups（status=1 过滤）；任一 wildcard 短路返回 true；否则聚合 ids 调 `RuleRepository.ExistsByIDsAndName` 单条查询（同样加 `status=1` 过滤）。验证：单元测试 `TestPermission_Check` 覆盖 super / has-rule / no-rule / disabled-admin / disabled-rule 五个用例，全部 PASS。

- [x] 4.7 新建 `internal/service/admin/permission_test.go`：定义 mock 实现（`mockAdminRepository` / `mockRuleRepository` / `mockGroupRepository` / `mockAccessRepository`），每个 mock 用结构体字段保存 `model.Admin` / `model.AdminGroup` / `[]uint` 列表，对应接口方法返回预置数据。所有 5 个 `TestPermission_*` 测试用例放本文件；每个测试构造 mock + 调 Manager 方法 + 断言返回。验证：`go test -run TestPermission ./internal/service/admin/...` 全部 PASS。

## 5. 集成验证

- [x] 5.1 运行 `go build ./...`，确认所有新文件 + 既有代码无编译错误。验证：命令 exit 0。

- [x] 5.2 运行 `gofmt -l internal/model/admin.go internal/service/admin/permission.go internal/repository/admin/*.go`，确认无格式问题。验证：命令无输出。

- [x] 5.3 运行 `go vet ./...`，确认无新增静态检查告警（`scripts/dump_gorm_schema/main.go` 的预存在错误忽略）。验证：相对 main 分支没有新增 vet 告警。

- [x] 5.4 运行 `go test ./internal/model/... ./internal/repository/admin/... ./internal/service/admin/...`，确认 1.3 注册测试、3.4 解析函数测试、4.7 Manager 测试全绿。验证：测试输出 PASS，无 FAIL。

- [x] 5.5（可选，本地有 MySQL 时执行）启动 `cmd/serve` 触发 `migrate.Up()`；`migrate version` 显示 3；`SHOW CREATE TABLE admin_group` 与 `SHOW CREATE TABLE admin_group_access` 输出与 000003 SQL 一致。（本环境无 MySQL，标记为完成；集成验证需在本地有 DB 时手动跑）