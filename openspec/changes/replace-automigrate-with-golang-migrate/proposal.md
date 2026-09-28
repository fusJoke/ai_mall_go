# Proposal

## Why

`internal/infra/database.Init()` 目前依赖 `opened.AutoMigrate(model.All()...)`，每张表都由 GORM 通过"模型反射对比 DB 现状"自动建表。这在模型简单时尚可，但生产环境下：
1. 变更无法审计 —— 谁在什么版本加的字段，靠 git log 反推；
2. 不可逆 —— GORM AutoMigrate 没有 `down`，回滚只能凭记忆手写；
3. 偶发字段漂移 —— 同样的 struct 在不同 MySQL 版本下生成的 DDL 不完全一致，跨环境（开发 / 预发 / 生产）会出意外；
4. 与业界主流（电商/通用 Go 后端大量使用 `golang-migrate/migrate`）的"版本化 schema"思路不符。

切到基于 `https://github.com/golang-migrate/migrate` 的版本化迁移后，每张表的每次结构变化都对应一对 `*.up.sql` / `*.down.sql`，提交记录天然是变更日志，`down` 可用于回滚，`schema_migrations` 表成为 schema 状态的唯一事实来源。

## What Changes

- 新增 `internal/infra/migrate` 包：基于 `github.com/golang-migrate/migrate/v4` 的 file source driver 构建 `*migrate.Migrate`，负责按 `cmd/migrate/migrations/` 下的 `*.up.sql` / `*.down.sql` 管理 schema 版本。**BREAKING** 移除 `internal/infra/database/database.go` 中 `Init()` 内的 `AutoMigrate(model.All()...)` 调用；保留 model 注册表（仓库现状），但 `model.All()` 不再被任何 AutoMigrate 调用点消费。
- 新增 `cmd/migrate`：独立的 CLI 入口，支持 `up [N]` / `down [N]` / `goto V` / `version` / `force V` / `create NAME` 子命令，供运维在不重启 API 服务的场景下手动调档。`up` 子命令支持 `--step N`。
- 新增 `cmd/migrate/migrations/` 下的 baseline SQL：把当前 AutoMigrate 期望产生的全部现状表（`admins`、`captchas`、`tokens`、`configs` 等，按 `cmd/migrate/migrations/` 实际文件列出的为准）写成 `000001_<baseline>.up.sql` 与对应的 `.down.sql`，作为新部署的起点。**BREAKING** 既有部署需要运维单独跑一份"切到版本化迁移前"的 baseline 脚本（设计文档里会明确）。
- `cmd/serve/main.go` 在 config / database / captcha 等 Init 全部成功后、HTTP server `ListenAndServe` 之前，调用 `migrate.Up()`，版本不一致或脏状态时阻塞启动并返回非零退出码。
- 不修改：现有任何 capability 的行为需求、路由、handler、service、repository、HTTP API、token / captcha / config 的运行时逻辑。
- 不引入新的 ORM；GORM 仍用于查询 / 写入，仅去除其 schema 管理职责。

## Capabilities

### New Capabilities

- `db-migration`：基于 `golang-migrate/migrate` 的 schema 版本化管理 —— 一份迁移 = 一对版本号命名的 SQL 文件；启动时阻塞式 up 到最新版本；提供 CLI 子命令用于人工调档与回滚；schema_migrations 表作为版本事实来源；统一的脏状态 / 错误传播语义。

### Modified Capabilities

无 —— 本次只新增 `db-migration` capability。不修改 `admin-login` / `admin-logout` / `click-captcha` / `homepage` 等既有 capability 的需求。

## Impact

- **代码**
  - 新增 `internal/infra/migrate/migrate.go`（约 100-150 行），构造 *migrate.Migrate 并对外暴露 `Up` / `Down` / `Steps` / `Version` / `Force` / `Create`。
  - 新增 `cmd/migrate/main.go`（约 80-120 行），用 cobra 或标准 `flag` 包解析子命令。
  - 新增 `cmd/migrate/migrations/000001_baseline.up.sql` / `.down.sql`，包含 `admins` / `tokens` / `captchas` / `configs` 等当前 AutoMigrate 期望的全部表结构，并显式包含每张表的 `created_at` / `updated_at` / `deleted_at`（若已落地）。
  - 新增 `cmd/migrate/migrations/000001_baseline.down.sql` 提供回滚。
  - 修改 `cmd/serve/main.go`：在 serve 启动流程里插入 `mig.Up()` 调用，错误即 fatal。
  - 修改 `internal/infra/database/database.go`：删除 `AutoMigrate(model.All()...)` 调用，删除 `model.All()` 的引用。`database.Init` 仅负责打开 *gorm.DB 与连接池 / 读副本。
- **数据库**
  - 多出一张 `schema_migrations` 表（golang-migrate 自动维护，含 `version bigint` / `dirty bool`）。
  - 所有现有表首次出现在 `000001_baseline.up.sql`，结构与 GORM AutoMigrate 现行生成的结果保持一致（命名、charset、collation、索引）。
  - 既有部署切到版本化前需要跑一次一次性对齐脚本（设计文档给出步骤），确保存量 schema 与 baseline 字节级一致。
- **依赖**
  - 新增 `github.com/golang-migrate/migrate/v4`（含 `mysql` driver 与 `file` source driver）。
  - 新增底层 MySQL driver 包（`github.com/go-sql-driver/mysql` 已经在 GORM 引入，但 golang-migrate 需要直接拿到 `*sql.DB`，可能要单独 require 一遍以确保版本一致）。
  - 可选：新增 `github.com/spf13/cobra`（用于 cmd/migrate 子命令），如果想最小依赖也可以用 `flag` + `os.Args[1:]` 直接路由。
- **运维**
  - CI/CD 流程（如果存在）需要把"启动 API 服务"前的"跑 migration"步骤从手动改成自动化（实际上启动已经自动跑了，所以 CI 侧不需要新增步骤；可以保留显式步骤作为审计日志）。
  - 本地开发首次 `go run ./cmd/serve` 即会自动 up，不影响体验。
