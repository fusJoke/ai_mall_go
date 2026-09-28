# Spec Delta

## Purpose

基于 `github.com/golang-migrate/migrate` 提供 schema 版本化管理 —— 每张表的结构变化对应一对 `*.up.sql` / `*.down.sql` 文件，schema_migrations 表记录当前已应用版本；API 服务启动时阻塞式 up 到最新，CLI 命令用于人工调档与回滚。

## ADDED Requirements

### Requirement: 启动时阻塞式自动 up

API 服务进程启动时，在 HTTP server `ListenAndServe` 之前 MUST 自动调用一次"应用所有尚未应用的 up migration"。若本地 migrations 目录里没有任何未应用文件，则视为无操作直接通过；若存在未应用文件，全部应用成功之后才允许继续后续启动步骤。

#### Scenario: 首次部署 schema 全空

- **WHEN** `schema_migrations` 表不存在或 `version` 为 NULL，本地 `cmd/migrate/migrations/` 下存在一组 up migration 文件
- **THEN** 所有未应用文件 MUST 被顺序应用到写库，最终 `schema_migrations.version` 等于目录里最大的版本号
- **AND** HTTP server MUST NOT 在 up 完成前开始监听端口

#### Scenario: 已在最新版本上重启

- **WHEN** `schema_migrations.version` 等于本地最大版本号且 `dirty=false`
- **THEN** 启动 MUST NOT 重新跑任何 migration
- **AND** HTTP server 正常继续启动流程

#### Scenario: 落后若干个版本的重启

- **WHEN** `schema_migrations.version` 小于本地最大版本号
- **THEN** 中间所有版本的 up MUST 被顺序应用
- **AND** 应用过程中任一文件失败 MUST 中止后续 up 并阻塞启动

#### Scenario: 启动时 local 版本集合与 DB 版本集合不一致

- **WHEN** 同一 migration 文件修改已被应用在 DB 后，本地工作区对该文件改动了 SQL 内容（重放场景）
- **THEN** 该文件 MUST NOT 在启动时自动重跑；只有"未应用"的版本会被应用
- **AND** 启动 MUST NOT 因"内容变动但版本号未升"而失败

### Requirement: CLI 子命令用于人工调档

进程 MUST 提供一个独立 CLI（`cmd/migrate`）支持以下子命令，用于运维不重启 API 服务即可调整 schema：

| 子命令 | 行为 |
| --- | --- |
| `up [N]` | 应用剩余 N 个 up（缺省 N 为 0 表示全部） |
| `down [N]` | 回滚最近 N 个 down（缺省 N 为 1） |
| `goto V` | 应用或回滚到指定版本 V |
| `version` | 打印当前 `version` 和 `dirty` |
| `force V` | 把 `version` 强制设为 V，`dirty` 强制设为 `false` —— 仅用于"已知在 V-1 已成功但 V 标记为 dirty"的恢复 |
| `create NAME` | 在 `cmd/migrate/migrations/` 下生成 `NNNNNN_name.up.sql` 和 `NNNNNN_name.down.sql` 占位文件，版本号取当前目录最大版本号 +1（保留 6 位前导零） |

#### Scenario: 人工 up 增量

- **WHEN** 运维执行 `migrate up 2`
- **THEN** 当前 `schema_migrations.version` 之上的最多 2 个 up MUST 被应用
- **AND** 输出 MUST 包含应用前后的版本号

#### Scenario: down 回滚一个版本

- **WHEN** 运维执行 `migrate down 1` 且 `schema_migrations.dirty=false` 且还有未应用的 down
- **THEN** 最近一个版本的对应 `.down.sql` MUST 被应用
- **AND** `schema_migrations.version` MUST 减 1

#### Scenario: version 子命令

- **WHEN** 运维执行 `migrate version`
- **THEN** CLI MUST 打印形如 `<version> <dirty状态>` 的当前状态
- **AND** 尚未执行过任何 migration 时 MUST 打印 `no migration`（不是 panic 也不是非零退出码）

#### Scenario: force 用于恢复 dirty 状态

- **WHEN** DB 里 `schema_migrations.dirty=true` 且运维确认上一个 migration 已手动清理
- **THEN** `migrate force V` MUST 把版本号强制设为 V 并清除 dirty 标志
- **AND** CLI MUST 输出 `forced version to V` 之类的明确提示，避免误用不被掩盖

### Requirement: migration 文件命名与目录

`cmd/migrate/migrations/` MUST 作为 schema 变更的单一事实来源。每对文件 MUST 满足：

- 文件名形如 `<version>_<name>.up.sql` 与 `<version>_<name>.down.sql`，`version` 是 6 位十进制数字（前导零补齐）。
- 同版本号的 `.up.sql` 和 `.down.sql` MUST 共存；缺失任一份 MUST 在 `migrate up` / `migrate down` 时报明确错误。
- 文件 MUST 是 SQL 文本（即 `*.up.sql` / `*.down.sql` 文件 source driver），不允许嵌入 Go 代码。
- 文件内容 MUST 可由 MySQL client 直接执行（不依赖项目专有变量替换）；如需要环境相关值，由 Go 代码在运行时通过 change-set 或独立 SQL 注入，本次不做。
- 新增 migration MUST 通过 `migrate create NAME` 命令生成（保证版本号递增、前导零格式正确），不允许手敲文件名。

#### Scenario: 文件命名校验

- **WHEN** 启动或 CLI up 时发现 `12345_some_change.up.sql` 与 `12345_some_change.down.sql` 但其中一份丢失
- **THEN** MUST 返回文件名级别的错误（"missing 12345_some_change.down.sql"），不允许吞错后继续

#### Scenario: create 生成新文件

- **WHEN** 运维执行 `migrate create add_user_email_index`
- **THEN** MUST 在 `cmd/migrate/migrations/` 下生成 `<N+1>_add_user_email_index.up.sql` 与 `<N+1>_add_user_email_index.down.sql`，文件内容为合法 SQL 注释占位（如 `-- +migrate Up` / `-- +migrate Down`），由开发者后续填充

### Requirement: schema_migrations 作为版本事实来源

当前已应用的 schema 版本 MUST 由 `schema_migrations` 表（golang-migrate 自维护）记录。系统 MUST NOT 维护并行版本表。

#### Scenario: 启动前初始化

- **WHEN** 首次 `migrate up` 跑在全新 DB 上
- **THEN** golang-migrate MUST 在目标库内自动创建 `schema_migrations` 表（`version` 与 `dirty` 两列）
- **AND** 应用完 baseline 后 `schema_migrations.version` MUST 等于 baseline 版本号

#### Scenario: DB 与本地不一致时的错误

- **WHEN** `schema_migrations.version` 大于本地最大版本号（即本地 migration 文件被"丢回"）
- **THEN** 启动 MUST 阻塞并返回明确错误（不允许强行 down，因为本地已无可应用的 down）
- **AND** 错误信息 MUST 包含当前 DB version 与本地最大 version 的对比

### Requirement: 启动阻塞与错误传播

API 服务的启动流程 MUST 把"应用 up migration"的结果解析为清晰的成败信号。

#### Scenario: migration 文件 SQL 语法错误

- **WHEN** 任意一个待应用的 `.up.sql` 在 MySQL 上执行失败
- **THEN** 该文件 MUST NOT 被错误标记为 `dirty=true` 之外的成功
- **AND** 启动 MUST 在 `ListenAndServe` 之前 fatal 退出（cmd/serve 进程退出码非零）

#### Scenario: dirty 状态阻止自动 up

- **WHEN** `schema_migrations.dirty=true` 且当前是 API 服务启动路径
- **THEN** 启动 MUST 阻塞；不允许尝试 apply 下一步

### Requirement: DSN 与 migrations 路径来源

DB 连接信息 MUST 复用现有 `config.Get().Database.Write`（含 `host` / `port` / `username` / `password` / `dbname`），不允许在 migrate 包里硬编码或重新声明。

migration 源目录 MUST 在编译时由 build flag 或默认值定位到 `cmd/migrate/migrations/`（项目内相对路径），运行时不允许通过环境变量切换路径，避免误指向生产环境以外的目录。

#### Scenario: 配置文件缺失 database 节点

- **WHEN** `config.Get().Database.Write.Host` 为空字符串
- **THEN** migrate 包 MUST 在构造 `*migrate.Migrate` 之前返回明确错误（不允许落到连接失败这种二义错误）

### Requirement: 不再依赖 GORM AutoMigrate

为防止"双轨制"导致 schema 状态被两边竞速修改，`internal/infra/database.Init()` MUST NOT 调度任何 `AutoMigrate(...)`。

#### Scenario: 启动应用一个前所未有的新 model

- **WHEN** 开发者新增了一个 model 并通过 `model.Register(...)` 注册，但当前 `schema_migrations.version` 仍未包含新表结构
- **THEN** 启动 MUST 因"本地 migration 目录缺新版本的 up 文件"而失败（开发者必须先 `migrate create new_table` 并填 SQL，再重启启动流程）
- **AND** 启动 MUST NOT 通过 GORM AutoMigrate 隐式把新表建出来
