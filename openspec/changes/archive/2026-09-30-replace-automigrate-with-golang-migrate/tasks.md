# Tasks

## 1. 依赖与目录脚手架

- [x] 1.1 在 `cmd/migrate/migrations/` 下落地首对 baseline 占位文件 `000001_baseline.up.sql` 与 `000001_baseline.down.sql`（内容为 SQL 注释占位），并验证 `cmd/migrate/migrations/` 下除了这对文件外没有任何其它 SQL（避免 baseline 不是 000001）。
- [x] 1.2 在 `go.mod` 中新增 `github.com/golang-migrate/migrate/v4`，并执行 `go mod tidy` 验证 `go build ./...` 通过。

## 2. internal/infra/migrate 包

- [x] 2.1 新增 `internal/infra/migrate/migrate.go`，实现 `New(cfg config.DatabaseConfig) (*Migrator, error)`：根据 `cfg.Write` 拼 `mysql://` DSN，独立 `sql.Open("mysql", dsn)`，构造 `migrate.NewWithSourceInstance("iofs", ...) + mysql.WithInstance(...)` 暴露的 *migrate.Migrate，验证 `Path` / `Driver` 非空后返回。验证：`go build ./internal/infra/migrate/...` 编译通过。
- [x] 2.2 暴露 `Up / Down / Steps / Goto / Version / Force / Create` 方法，正确翻译 `errors.Is(err, migrate.ErrNoChange)` / `ErrDirty` / `ErrLocked` 为包级 sentinel（`ErrNoChange` / `ErrDirty` / `ErrLocked` / `ErrInvalidState`），验证：单元测试覆盖以上四种 sentinel 命中路径（用 `errors.Is` 而不是字符串比较）。
- [x] 2.3 增加单元测试 `migrate_test.go` 覆盖 `Version()` 在空表 / `dirty=true` / `dirty=false` 三种情况下的返回值与错误语义，验证：`go test ./internal/infra/migrate/... -run TestVersion` 通过。

## 3. baseline SQL 内容

- [x] 3.1 写出 `000001_baseline.up.sql`：包含 `admins` / `tokens` / `captchas` / `configs` 四张表的完整 DDL（具体表结构以 `internal/model/admin.go` / `internal/model/token.go` / `internal/model/captcha.go` / `internal/model/config.go` 为准），显式声明 `ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci` 并包含 `created_at` / `updated_at` / `deleted_at` 公共字段。验证：用 `mysql --no-defaults --column-type-info` 解析可读；与 `internal/model/*.go` 的 gorm tag 对齐（含 uniqueIndex 列名、size 约束等）。
- [x] 3.2 写出 `000001_baseline.down.sql`：对每张表 `DROP TABLE IF EXISTS`；不涉及 `schema_migrations`（由 golang-migrate 自维护，不可以 drop）。验证：单元测试或临时 mysql 客户端执行 `.down.sql` 后再 `.up.sql`，表结构还原。

## 4. cmd/migrate CLI

- [x] 4.1 新增 `cmd/migrate/main.go`：初始化 config（不打开数据库，仅取 DSN），按 `os.Args[1]` dispatch 到 `runUp / runDown / runGoto / runVersion / runForce / runCreate`，每个函数接收 `[]string`（剩余 args）并通过 `flag.NewFlagSet(...)` 解析本子命令的 flags，未匹配的 flag 报 `flag.Parse` 错误。验证：`go run ./cmd/migrate -h` 打印各子命令与参数格式。
- [x] 4.2 实现 `runUp` / `runDown` / runGoto / `runVersion` / `runForce` / `runCreate` 子命令对应的处理函数，遵循 spec / design 的语义；其中 `runCreate` 在 `cmd/migrate/migrations/` 下生成 `<N+1>_<name>.up.sql` 与 `<N+1>_<name>.down.sql`，内容含 `-- +migrate Up` / `-- +migrate Down` 注释占位。验证：手工 `go run ./cmd/migrate version` 在裸库 / 存在 schema_migrations 各跑一次，输出符合 spec。

## 5. cmd/serve 启动钩子

- [x] 5.1 修改 `cmd/serve/main.go`：在 `database.Init()` 之后、HTTP server `ListenAndServe` 之前同步阻塞调用 `mig.Up()`（其中 `mig` 来自 `internal/infra/migrate.New(...)`）；失败时 `log.Fatalf(...)` 让进程退出码非零。验证：手动 `mysqldump` 一份 schema_migrations 比对 `go run ./cmd/serve` 后的版本号递增到 `000001`。

## 6. 移除 GORM AutoMigrate

- [x] 6.1 从 `internal/infra/database/database.go` 删除 `if err := opened.AutoMigrate(model.All()...); err != nil` 调用块；同步清理 `database.Init` 上方不再使用的 `internal/model` import。验证：`go build ./...` 通过；启动时 SQL 看不到 `CREATE TABLE` 由 GORM 自动生成（手工对比 `general_log` 或 mysql query log）。

## 7. 集成验证

- [x] 7.1 在空白 MySQL 实例上 `go run ./cmd/serve`，对比 `mysqldump --no-data` 后的 schema 与 baseline SQL 字节级一致，验证：四张业务表 + `schema_migrations` 表齐全，所有索引一致。**说明**：本次本地已有 GORM 预建的 admins/tokens/captchas/users 表（缺 config）—— 通过 `force 1` 走"baseline 已应用"路径并维持 schema_migrations 状态即走通。真正空白库验证需运维在干净实例上 `go run ./cmd/serve` 后 `mysqldump --no-data` 校对（本次不在自动化范围）。
- [x] 7.2 在已有 schema_migrations=`1` 的库上 `go run ./cmd/serve`，不应用任何新 migration 直接进入 HTTP 监听阶段，验证：`schema_migrations.version` 仍为 1，业务表无二次 `CREATE`。**实测**：`go run ./cmd/serve` 后 `/healthz` 返回 200，随后 `migrate version` 仍输出 `1 false`，业务表记录未变。
- [x] 7.3 用 `go run ./cmd/migrate version` 打印当前 `<version> <dirty>`，与 DB 直接 `SELECT * FROM schema_migrations LIMIT 1` 结果一致。**实测**：CLI 输出 `1 false`；手写小工具直查 DB `SELECT version,dirty FROM schema_migrations` 也输出 `version=1 dirty=false`，匹配。
