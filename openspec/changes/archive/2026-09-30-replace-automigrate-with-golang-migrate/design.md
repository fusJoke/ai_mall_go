# Design

## Context

参见 `proposal.md - Why`：当前 `internal/infra/database/database.go:Init()` 通过 GORM `AutoMigrate(model.All()...)` 自动建表，缺版本化、不可回滚、不可审计。本次要把 schema 管理切到 `github.com/golang-migrate/migrate/v4`，并把"启动自动 up"和"CLI 手动调档"两条路径并入。

约束来自现有结构：
- `cmd/migrate/` + `cmd/migrate/migrations/` 已存在但内容为空，本次正好落地。
- 配置在 `internal/infra/config/`，DAG 顺序：`config.Init → database.Init → captcha.Init → HTTP server`（cmd/serve/main.go 维护）。
- `cmd/serve/main.go` 已用 `signal.NotifyContext` 做优雅退出；插入 migration 步骤时必须保留这一退出链路，不能把 fatal log 与 graceful shutdown 串接错位。
- model 注册表 `internal/model.All()` 仍保留（`model.Admin{}` 等文件用 init() 注册），本次只是去掉"被谁消费"。是否同时删 model 注册表本身属未来重构范畴，不在本设计内展开。
- `database.Init` 用 `config.Get().Database.Write` 打开 *gorm.DB，附带 dbresolver 读副本与连接池；连接池配置在 `applyPool` 内。

## Goals / Non-Goals

**Goals**

- 通过 `internal/infra/migrate` 包封装 `golang-migrate/migrate/v4` 的构造与调用细节，对外暴露 `Up / Down / Steps / Goto / Version / Force / Create`。
- cmd/serve 启动流程在 `database.Init()` 之后、HTTP `ListenAndServe` 之前同步阻塞 `migrate.Up()`。
- cmd/migrate 作为独立 CLI（与 cmd/serve 解耦），提供给运维在不重启 API 的情况下做版本调整。
- baseline migration 文件覆盖当前 GORM AutoMigrate 期望的"首批"表，结构与现有 GORM 生成结果一致。
- 完整移除 GORM AutoMigrate 调用点（不引入并行轨道）。

**Non-Goals**

- 不重写 GORM 已有的连接池 / dbresolver / 中间件逻辑。
- 不切到 Go 编程式 migration（用户已确认用纯 SQL 文件）。
- 不为 `golang-migrate` 增加 migrations 表重命名、driver 扩展等高级特性；保持 upstream 默认行为。
- 不动既有 capability（`admin-login` / `admin-logout` / `click-captcha` / `homepage`）的需求。
- 不提供数据库 schema 校验工具（`migrate validate` 之类）—— 本次只用到 `migrate up` / `version` / `force`，校验留给 review 与 lint。

## Decisions

### D1. 把 *migrate.Migrate 构造集中到 internal/infra/migrate 包

**理由**：migrate 库的 API 表面（`migrate.NewWithInstance` / `migrate.New` / `Driver` 错误值集合）较脏；用一层薄包装隔离脏错误（`ErrNoChange` / `ErrDirty` / `ErrLocked` 等 sentinel），调用方读起来语义明确（"没有更新" vs "脏状态阻塞" vs "锁未释放"）。同时把"从 config 拿 DSN + 拼 `mysql://` URL"这一步放进来，cmd/serve 和 cmd/migrate 都只调 `migrate.New(...)`。

**替代方案**：直接在 cmd/serve 和 cmd/migrate 各写一份 Driver 构造代码。否决：两处都得判断 sentinel，注定走样。

### D2. DSN 用 `mysql://` URL 形式而不是打开 `*sql.DB` 二次借用

**理由**：golang-migrate 既支持"`*sql.DB` + `mysql.WithInstance`"也支持"`mysql://` URL + `mysql.Open`"。前者要自己用 `sql.Open(...)` + ping 后再传给 WithInstance，逻辑分散；后者一行就完成"打开数据库 + 注册 mysql driver + 传 url"。我们的场景已经走 GORM 拿到 *gorm.DB，再 paste mysql go-sql-driver/mysql 也有点重复，但 golang-migrate 不能直接用 `*gorm.DB`（GORM 的连接池语义与 migrate 期望的 database/sql.DB 不同），必须独立 `sql.Open`。为简化构造，URL 形式更直观。

**替代方案**：`mysql.WithInstance` 复用 GORM 持有的 `*sql.DB`。否决：migrate 期望调用方绝对控制 `driver.WithInstance(db, &instance)`，把 `*gorm.DB` 内部的 `*sql.DB` 取出来需要 `db.DB()`，但 GORM 已经持有连接池的所有权，把同一个 `*sql.DB` 同时挂给 migrate 与 dbresolver 风险高、debug 难，并且会让 migrate 与 dbresolver 的生命周期耦合。

**结论**：migrate 包内部独立 `sql.Open("mysql", dsn)`，传 `mysql.Open(dsn)` 给 golang-migrate。写库配置直接读 `config.Get().Database.Write`；前缀、charset、parseTime、loc 复用 `database.buildMySQLDSN` 的拼装方式，但要去掉 `timestamptz` / GORM 特化项。

### D3. CLI 用标准库 `flag` + 手动子命令路由，不引入 cobra

**理由**：cmd/migrate 只有 ~6 个子命令，每个子命令独立一行 flag 解析足够。引入 cobra 会带来 `go.mod` 体积 + 一个 `commands/` 子目录，不值得。用户已说"想最小依赖也可以用 flag + os.Args[1:] 直接路由"，进一步确认走这条路。

**结构**：
```
cmd/migrate/main.go
  - 读 config.Init(rootDir) → database 模式不打开，仅取 DSN
  - routes["up"] / routes["down"] / routes["goto"] ...
  - 每个路由一个新文件：cmd/migrate/up.go / down.go / version.go / ...
```

但考虑项目惯例是"少文件、文件即类型"，首选：
```
cmd/migrate/main.go        // 入口 + 子命令 dispatch + Usage
cmd/migrate/migrate.go     // 真正调 migrate 库的函数包装（Up / Down / Steps / ...）
```

**替代方案**：cobra。否决：增加依赖、复杂度与本 change 收益不匹配。

### D4. migrations 目录用编译期路径嵌入，不暴露给环境变量

**理由**：spec 已规定"DNS 与路径来源"的语义：DSN 来自 config，但 migrations 路径不在运行期被改写，避免误指向其他目录。go-migrate 的 file source 支持 `embed.FS` 与本地路径两种；用 `embed.FS` 把整个 `cmd/migrate/migrations/` 嵌入到二进制里是最稳的做法，运行时无需在 host 上挂目录。

**实施要点**：
- 在 `internal/infra/migrate/source.go` 用 `//go:embed migrations/**/*` 注：embed 的根目录需要是 Go 文件所在目录，所以这里有两种选择：
  - (a) 把 migrations 复制/移动到 `internal/infra/migrate/migrations/`，embed source 改用该路径，cmd/migrate 调用一份非 embed 的本地 path 版本（dev 模式好用）；
  - (b) 让 `cmd/migrate/` 与 `internal/infra/migrate/` 都接受同一个 iofs.FS 抽象，只在 cmd/serve 里走 embed；cmd/migrate 走 os path（它本来就是 CLI，明确指向开发机的文件系统）。

**选择 (a) 在内部嵌一份 prod 视图**：
- `cmd/migrate/migrations/` 保留作为 developer 写 SQL 的位置；
- `internal/infra/migrate/migrations_embed.go` 通过文件名前缀（`//go:embed all:cmd_migrate_migrations`？不行，embed 不能跨越 package 边界）...

实际上 `//go:embed` 不能 embed 另一个 package 的目录，所以选 (b)：让 cmd/serve 直接用 `os.DirFS("cmd/migrate/migrations")` 打开本地路径；生产部署则通过 `go build -o serve -ldflags "-X main.migrationsDir=/etc/mall/migrations"` 之类的 ldflags 在编译期固定路径，避免运行期 env。**用户已确认"运行期不允许环境变量切换路径"**，而 `-X` 是 build 时固定、运行期不变，符合这一约定。

但生产环境必须把 migrations 复制到 `/etc/mall/migrations` 之类的固定路径，与 cmd/serve binary 路径并列。运维层面要求在新版 deploy 脚本里把 `cmd/migrate/migrations/` 同步过去。本次设计文档里给出 deploy 步骤但不强制实现 deploy 自动化（属于运维范畴）。

**替代方案**：embed.FS。否决：embed 路径会被绑死在 Go package 同级目录，与"developer 在 cmd/migrate/migrations/ 下写 SQL"的现状冲突；强行让 develop 在 `internal/infra/migrate/migrations` 下写 SQL 又偏离了 cmd/ 写 SQL 的天然位置。

### D5. baseline migration 文件 ID 必须是 000001 且显式落表

**理由**：现有 GORM AutoMigrate 在不同时间生成了 `admins` / `captchas` / `tokens` / `configs` 等表（见 `internal/model/*.go`）。baseline 把每张表都写一次，使得"既有的本地数据库（已经被 GORM 建过表）"与"全新部署"在同一 baseline 版本号之后的状态一致。

**注意**：CLAUDE.md 里"MySQL 表必备公共字段"提到 PostgreSQL `timestamptz` 等，但当前 `database.Init()` 用的是 MySQL（`gorm.io/driver/mysql`），所以 baseline 实际写 MySQL DDL，使用 `DATETIME(3) DEFAULT CURRENT_TIMESTAMP(3)` + 软删除 `deleted_at DATETIME(3) NULL`，与现有 GORM 行为一致。

**待办（不在本次 tasks 但应在 deploy runbook 里）**：现存被 GORM 建过表的环境，需要在 baseline 应用前后由运维手工做一次 `schema` 比对，确保存量字段与 baseline SQL 字节级一致。任何 field type / charset / collation 不一致都视为迁移动作的失败信号，本次不写自动化对齐脚本（避免在本次 change 中再开一个跨多环境的工具）。

### D6. cmd/serve 启动流程插入点

当前位置（main.go 的大致流程）：
```
config.Init(...)
database.Init()
captcha.Init() // 如有
httpServer := newRouter(...)
httpServer.ListenAndServe() // 在 goroutine
<-ctx.Done()
httpServer.Shutdown(10s)
```

**插入点**：`database.Init()` 之后立刻 `mig.Up()`，原因：
- 读库 dbresolver 在 dbresolver 回调里依赖写库 schema 存在；如果 up 在 dbresolver 之后跑，dblink 失效（实际行为取决于 dbresolver 实现，但安全起见永远把 schema 到位放在 dbresolver 之前）。
- HTTP 服务的注册在 `newRouter`；注册路由是 Go 静态表，没必要把 up 推到更靠后位置。
- `captcha.Init` 是否依赖 DB 取决于 driver，但即便有，也是"读 captcha 表 / token 表"，先 up 让 baseline 表存在才安全。

**错误传播**：`mig.Up()` 失败时直接 `log.Fatalf(...)` 或返回 `error`（取决于 main.go 当前风格）。当前 main.go 风格：用 `if err != nil { log.Fatalf(...) }`。沿用。

### D7. 移除 `database.Init()` 里的 `AutoMigrate` 调用

**删什么**：
- `internal/infra/database/database.go` 中的 `if err := opened.AutoMigrate(model.All()...); err != nil` 调用。
- `database.Init` 不再 import `internal/model`（如果全文件都不再用到）。

**保留什么**：
- `model.All()` / `model.Register` / `model.Reset` 仍保留（测试 / 业务代码可能用到）；本次不删除 model 包自身，避免跨 change 的连带。

**理由**：spec 的 "Requirement: 不再依赖 GORM AutoMigrate" 已经约束该行为；本设计只是把删除点落到行。

### D8. baseline SQL 的内容组织

按"一张表一个文件"展开会让迁移号退化到 ~6 个零散的 000002~000007；按"首批表合并到一个 000001_baseline"会让一次性的 baseline 集中且 review 友好。**选择后者**，理由：
- baseline 是"snapshot of current state"，不是"演进过程"；后续真正的演进用 000002+ 一次一文件。
- 单一文件更便于 diff 校对（这是第一个 baseline，最后一次）。

后续演进（000002_add_user_email_index 等）遵循社区习惯：一次一主题、一文件前向迁移。

## Risks / Trade-offs

**[R1] 既有部署不能直接 `go run ./cmd/serve`**
- 既有环境在 GORM AutoMigrate 下已经建过 `admins` 等表；切到 baseline 后 `migrate up` 会因为目标表已存在而失败（DDL 重复）。
- **Mitigation**：在 deploy runbook 里写明"切换 migration 体系"的一次性步骤——一个手工脚本，把当前 schema dump 转成 baseline 对齐，逐表做 schema 比对；本次不在 cmd 里提供自动化。

**[R2] baseline 写错会导致启动长期阻塞**
- 一旦 baseline 里有错 DDL，应用一次 dirty=true 之后启动永远失败；除非用 `migrate force` 强制修复。
- **Mitigation**：在 design / tasks 里把 `force` 子命令作为强制恢复手段明确写出来；review 重点放在 baseline 的 DDL 是否与 GORM 现行产物一致。

**[R3] 双数据库：写库 + 读副本**
- 现有 `database.Init()` 在 `AutoMigrate` 之后再 `Use(dbresolver)` 注册读副本。baseline 也会通过 `migrate up` 落到写库；但读副本如果是物理独立的实例，schema 不会同步。
- **Mitigation**：本次主要关注 schema_migrations 表 / 业务表存在于写库这件事；读副本的存在与否由基础设施（gtid / orchestrator 等）保证。本次在 design 里指出但不展开实现。

**[R4] go-migrate 与 GORM 都依赖 go-sql-driver/mysql；版本不一致会冲突**
- **Mitigation**：保留 go.sum 不破坏；用 `go get github.com/go-sql-driver/mysql` 强制 lock 与 GORM 同一版本。

**[R5] baseline 与 GORM 现状可能存在细微差异**
- 比如 GORM 生成的默认 `CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci`（MySQL 8）与 MySQL 5.7 默认 `utf8mb4_general_ci`。
- **Mitigation**：本次 baseline 显式声明 `CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci`（更老 MySQL 兼容），既符合"百货店 / mall / 电商老项目"的口味，也与 GO ORM 当前 deployment 环境（MySQL 8 也支持）语义一致。如果项目当前就是 MySQL 8，统一改 `utf8mb4_0900_ai_ci` 也可以——本次以"general_ci"为 baseline 默认值。

## Migration Plan

1. **新建 baseline**：本次 change 把 SQL 写到 `cmd/migrate/migrations/000001_baseline.up.sql` 与 `.down.sql`，结构与 GORM 现行结果对齐。
2. **既有部署一次性切换**（写在 deploy runbook，不在本 change）：
   - 停 API 服务（down 状态）。
   - 备份当前 schema：`mysqldump --no-data > schema_pre_baseline.sql`。
   - 跑一次 baseline：`migrate -path cmd/migrate/migrations/ force 0 && migrate -path cmd/migrate/migrations/ up 1`。
   - 对比 baseline 之后的 schema 与 dump，若有差异则手动修表（character set / collation / 索引名等）。
   - 用版本化迁移体系部署。
3. **新部署**：直接 `go run ./cmd/serve`，启动自动跑 up。

## Open Questions

无。所有影响 spec / approach / tasks 的决策已在前面给出。
