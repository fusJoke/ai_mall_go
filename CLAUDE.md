# CLAUDE.md

Guidance for Claude Code in this repository.

## 代码探查：优先用 CodeGraph，不要先 Read（强制）

本项目已建 CodeGraph 索引（仓库根有 `.codegraph/`）。**探查代码时按下面的顺序，不要一上来就 Read 文件或 grep：**

1. **首选 `codegraph_explore`**（MCP 工具，一次调用返回相关符号的逐行源码 + 符号间调用链 + 影响面）。适用：任何"这段逻辑在哪/怎么实现/谁调用了它/我要改的地方会波及什么"的问题，或**动手改代码之前**。
2. MCP 工具不可用时，退回 shell：`codegraph explore "<符号名或英文关键词>"`，输出完全等价。
3. **只有在以下情况才用 Read/Grep/Glob**：CodeGraph 查不到、你需要读配置文件 / SQL 迁移 / yaml / md 等非代码文件、或要看 `.codegraph/` 未覆盖的内容。此时应直接说明"CodeGraph 未覆盖，改用 Read"。

**关键：查询词用英文符号名或英文代码词，不要用中文自然语言。** CodeGraph 走词法匹配（FTS + 符号名），中文 prompt 基本零命中。反例与正例：

| ✗ 无效（中文 → 零命中） | ✓ 有效（英文符号名 → 命中） |
| --- | --- |
| 批次账单的确认逻辑是怎么实现的 | `BillService Confirm` |
| 抽卡事务在哪里扣减库存 | `DrawOrder deduct stock transaction` |
| 商品售罄后触发什么 | `GenerateForBox sold out lifecycle` |

不确定确切符号名时，先用 `codegraph query <英文关键词>` 找符号，再 `codegraph explore`。

**关于 `UserPromptSubmit` hook**：每次提问会自动注入 `<codegraph_context>`。它是**词法**检索——你的 prompt 若是中文，注入块大概率是空转（"No relevant code found"）。这不是出错，而是提示你：**要么用英文符号名提问，要么忽略该空块、自己主动调 `codegraph_explore`**。不要把空转的注入块当成"CodeGraph 不可用"。

## 项目状态

Go 电商 mall 项目，`module ai-go-mall`，Go 1.27.1。已落地：

- **业务主线** trading-card-blindbox-mvp（`openspec/changes/add-trading-card-blindbox-mvp/`）：`internal/model/mall/` 10 表按表拆文件 + 迁移 `cmd/migrate/migrations/`（`000006` 起）
- **三端** user / supplier / admin 的 Repository / Service / Handler / Router 按身份拆子目录；**前端** `web/src/{layouts,api,views,lang,stores}/{user,supplier}/` + 路由 `web/src/router/static/{userBase,supplierBase}.ts`
- **基础设施** `internal/infra/{config,database,cache,search}/` + `internal/middleware/`（组件说明见下节）

**入口**：`cmd/` 下每个子目录一个入口，不要把命令全塞进单一 `cmd/main.go`。已落地 `serve`、`migrate`、`seed`、`es-sync`、`stock-sync`。

## 常用命令

无 Makefile / task runner，统一用 Go 原生命令：构建 `go build ./...`；起服务 `go run ./cmd/serve`；迁移 `go run ./cmd/migrate`；测试 `go test ./...`（单个 `go test -run TestName ./path/to/pkg`）；格式化 `gofmt -w .`、`goimports -w .`；静态检查 `go vet ./...`；查标准库/依赖 API 用 `go doc <pkg>.<Symbol>`，不要直接 Read 包源码。

> ⚠️ 本机需 `export GOCACHE="C:/Users/Public/gocache" GOTMPDIR="C:/Users/Public/gotmp"`（纯 ASCII 路径），
> 否则 `go vet` / `go build` 可能被 SIGTERM 打断。另：**`gofmt -l` 在本仓不是有效门禁** ——
> 全仓约 183 个文件因 CRLF（`core.autocrlf=true`）被标记，判断自己改的代码用
> `tr -d '\r' < f.go > /tmp/x.go && gofmt -l /tmp/x.go`。其余环境坑见 `doc/开发工作流.md` §6。

## 开发工作流（S1–S5，强制）

完整版见 **`doc/开发工作流.md`**。核心是四工具各占开发流一段，且**每段的输出正好是下一段的输入**：

| 阶段 | 工具 | 产出 |
|---|---|---|
| **S1 定位** | codegraph（`explore` / `impact`） | 影响面 → 写进 `proposal.md` |
| **S2 规划** | openspec change（proposal / design / tasks + spec delta） | `tasks.md` → **直接就是 S3 的输入** |
| **S3 执行** | superpowers `/superpowers:execute-plan`（无插件时：按 tasks 逐项做 + 先写测试） | 代码 + 测试 |
| **S4 验证** | `go build`/`vet`/`test`、`vue-tsc --noEmit`、构建（+ 定向 ocr） | 通过 / 不通过 |
| **S5 收口** | `openspec archive` → `codegraph sync` → `git commit` | specs 追平 + 索引新鲜 + 已提交 |

不变量：**`openspec/specs/` 是当前真相，`changes/` 是待应用的补丁，`archive` 是唯一的应用动作。**

**三条硬纪律**（详见文档 §3）：

1. **完成即归档**，不要攒 —— 积压的代价是 `specs/` 变成过期真相（曾积压 7 个、最久 15 天，
   导致 spec 落后代码 5~19 天）。
2. **同一 capability，同一时刻只允许一个未归档 change 在改它。** 这是**唯一需要人来守**的规则：
   工具按 `### Requirement: <名>` 匹配、**不看内容**，两个 change 改同名需求时后归档者会
   **静默覆盖**前者（无警告、exit 0）。修复类改动**优先追加进已有未归档 change**，不新开。
3. **验证有 token 预算**：L0（编译 + 单测 + **负向对照**）是主力；**ocr 默认不用**，确需时
   `--path` 限定到 **≤5 个改动文件**、先 `--preview`、**不传 `--effort`**；**禁止 `ocr scan` 全仓**。

**归档前必做**：`openspec validate <change>` **+ 逐条比对 delta 文本与代码现状** ——
曾出现 change 标着 `✓ Complete`、其 delta 却写着与代码相反的语义（mock 默认开/关），
归档会把错误文本永久冻进 specs。自检脚本见文档 §S5。

**测试断言纪律**：`sqlmock` 下「守卫失效」**也会返回错误**（`call ... was not expected`），
故 `err != nil` 是**弱断言** —— 必须让守卫返回**哨兵错误**并用 `errors.Is` 精确断言。
**每写一条关键断言，就临时把实现改回旧写法跑一次（负向对照）；若测试仍通过，这条断言是假的。**

## 分层架构（固定流向，不可变）

调用关系永远是 `Handler → Service → Repository → Model`，禁止跨层或反向依赖：

- `handler/` —— 解析请求（JSON→struct）、参数校验、序列化响应。**只碰参数与返回值，不做业务判断。**
- `service/` —— 业务编排、事务控制、业务校验（用户名是否存在、余额是否足够）。**只处理业务。**
- `repository/` —— 数据访问，**只做 DB ↔ 内存搬运，不掺业务。**
- `model/` —— 只定义表结构与 `TableName()`。

辅助分层：`dto/` 传输对象；`middleware/` HTTP 中间件；`router/` 路由注册与自动发现；`kit/` 业务层工具封装；`infra/` 基础设施（`database`、`config`、`captcha`、`token`、`upload` 等；多驱动能力如 `token`、`upload` 放同名 `driver/` 子目录，文件名即驱动名）；`pkg/` 可被外部引用的公共库（如 `random`、`filesystem`）。

**目录组织规则**：

- Handler / Service / Repository 按身份设子目录：`admin/`（后台）、`user/`（会员）、`business/`（供应商）；脱离身份的通用服务放 `common/` 或不设子目录（验证码、配置、地区数据）
- Model **不设子目录**，按业务模块组织文件（如 `admin.go` 可含 `Admin`、`AdminGroup`、`AdminRule`）；不同身份可共用同一模型

## 基础设施

**配置** `internal/infra/config/` 基于 viper：`Init(rootDir)` 按文件名升序合并 `rootDir/config/*.yaml`（跳过 `*.example`），再合并 `rootDir/.env.yaml`（存在则覆盖同名键，优先级最高）。

- `config/` 放默认模板（已提交）；根目录 `.env.yaml`（gitignored）与 `.env.yaml.example`（已提交）
- 新增配置文件丢进 `config/` 即可，无需改 loader
- `database` 节点：`type` / `prefix` 读写库共享；`write` 为写库连接与连接池；`read.enabled=false` 时读写都走写库，`true` 时由 dbresolver 按操作类型自动路由

**数据库** `internal/infra/database/` 基于 GORM（`gorm.io/gorm` + `gorm.io/driver/mysql`）：读写分离走 `gorm.io/plugin/dbresolver`——事务强制写库，Find/First/Take/Raw 走读副本；连接信息来自 `config.Get().Database`，取用走 `database.Get()`。

**缓存 / 搜索** `internal/infra/cache/`（Cache 接口 + L1/L2 driver）、`internal/infra/search/`（Elasticsearch v8 client）。

**中间件** `internal/middleware/{auth,rate_limit}.go` 按 token type 区分身份。

**Web 框架** `cmd/serve/` 基于 gin：路由暂写在 `cmd/serve/main.go`（`GET /healthz`、`GET /api/v1/ping`），后续迁至 `internal/router/` 自动发现；端口取 `config.yaml` 的 `server.port`；graceful shutdown 用 `signal.NotifyContext` 监听 SIGINT/SIGTERM → `srv.Shutdown`（10s 超时）。

## 数据表必备公共字段

每张业务表都需包含（MySQL 8.0，与 `cmd/migrate/migrations/*.up.sql` 风格一致）：

```sql
`id`         bigint       NOT NULL AUTO_INCREMENT                   COMMENT 'ID',
`updated_at` datetime(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3)
                          ON UPDATE CURRENT_TIMESTAMP(3)            COMMENT '更新时间',
`created_at` datetime(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3)     COMMENT '创建时间',
`deleted_at` datetime(3)  DEFAULT NULL                               COMMENT '删除时间',
PRIMARY KEY (`id`),
KEY `idx_<表名>_deleted_at` (`deleted_at`)
```

- Go 侧对应 `ID int64` + `CreatedAt` / `UpdatedAt time.Time` + `DeletedAt gorm.DeletedAt`；DB 默认值仅兜底，写入以 GORM 的 `autoCreateTime` / `autoUpdateTime` 为准。
- 时间戳统一 `datetime(3)`。
- `deleted_at` 必须建索引。

