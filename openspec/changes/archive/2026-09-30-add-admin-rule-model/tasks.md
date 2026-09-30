# Tasks

## 1. 模型定义

- [x] 1.1 在 `internal/model/admin.go` 中追加 `AdminRuleType` / `AdminRuleOpenType` / `AdminRuleExtend` 三个自定义 string 类型，加 9 个常量（`RuleTypeDir/Menu/Node`、`RuleOpenTab/Link/Iframe`、`RuleExtendNone/AddRouteOnly/AddMenuOnly`）。验证：`gofmt -l internal/model/admin.go` 无输出；`grep -c "RuleType\|RuleOpen\|RuleExtend" internal/model/admin.go` ≥ 9。

- [x] 1.2 在 `internal/model/admin.go` 中追加 `AdminRule` 结构体（id / pid / type / title / name / path / icon / open_type / url / component / keepalive / extend / remark / weigh / status 共 15 个业务字段 + UpdatedAt / CreatedAt / DeletedAt 三个时间戳字段），每个字段带 `comment` / `size` / `default` / `not null` 等 gorm tag。验证：`grep -c "comment:" internal/model/admin.go` 增加至少 15 处；`go build ./...` exit 0。

- [x] 1.3 在同文件追加 `AdminRule.TableName(namer schema.Namer) string` 方法，走 `namer.TableName("admin_rule")`；改 `init()` 为 `Register(Admin{}, AdminRule{})` 一次性注册两个模型。验证：`go build ./...` exit 0；`grep "namer.TableName(\"admin_rule\")" internal/model/admin.go` 命中。

## 2. 迁移文件

- [x] 2.1 新建 `cmd/migrate/migrations/000002_admin_rule.up.sql`：建 `admin_rule` 表（17 列含 3 个时间戳；`id` / `pid` int UNSIGNED；`type` / `extend` varchar(N) NOT NULL DEFAULT；`open_type` varchar(16) DEFAULT NULL；`keepalive` tinyint(1) DEFAULT 0；`status` tinyint DEFAULT 1；`weigh` int DEFAULT 0；`created_at` / `updated_at` datetime(3) + autoMaintain；`deleted_at` datetime(3) NULL；PRIMARY KEY (id)；KEY idx_admin_rule_pid、KEY idx_admin_rule_deleted_at；ENGINE=InnoDB CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci）。验证：文件存在；`gofmt -l` 报错信息不涉及该文件；本地可执行 `go run ./cmd/migrate version` 显示 1（baseline）后启动 `cmd/serve` 自动升至 2。

- [x] 2.2 新建 `cmd/migrate/migrations/000002_admin_rule.down.sql`：单条 `DROP TABLE IF EXISTS admin_rule;`。验证：文件存在。

## 3. 集成验证

- [x] 3.1 `go build ./...` 通过。验证：命令 exit 0；产物 `internal/model/admin.go` 编译无 error。

- [x] 3.2 `gofmt -l internal/model/admin.go` 无输出。验证：命令无输出，文件已是 LF（项目惯例）。

- [x] 3.3 `go test ./internal/model/...` 既有 8 个测试全绿（TestRegister_AndAll / TestRegister_DedupByType / TestRegister_Variadic / TestRegister_NilIgnored / TestAll_ReturnsCopy / TestToken_Registered / TestToken_TableName_NoPrefix / TestToken_TableName_WithPrefix）。验证：`go test ./internal/model/...` 输出 PASS，无 FAIL。

- [x] 3.4 `go vet ./...` 仅有 `scripts/dump_gorm_schema/main.go:40` 预存在错误（与本 change 无关）。验证：相对 main 分支没有新增 vet 告警。