# CLAUDE.md

Guidance for Claude Code in this repository.

## 项目状态

Go 电商 mall 项目，`module ai-go-mall`，Go 1.27.1。已落地：

- **业务主线** trading-card-blindbox-mvp（`openspec/changes/add-trading-card-blindbox-mvp/`）：`internal/model/mall/` 10 表按表拆文件 + 迁移 `cmd/migrate/migrations/`（`000006` 起）
- **三端** user / supplier / admin 的 Repository / Service / Handler / Router 按身份拆子目录；**前端** `web/src/{layouts,api,views,lang,stores}/{user,supplier}/` + 路由 `web/src/router/static/{userBase,supplierBase}.ts`
- **基础设施** `internal/infra/{config,database,cache,search}/` + `internal/middleware/`（组件说明见下节）

**入口**：`cmd/` 下每个子目录一个入口，不要把命令全塞进单一 `cmd/main.go`。已落地 `serve`、`migrate`、`seed`、`es-sync`、`stock-sync`。

## 常用命令

无 Makefile / task runner，统一用 Go 原生命令：构建 `go build ./...`；起服务 `go run ./cmd/serve`；迁移 `go run ./cmd/migrate`；测试 `go test ./...`（单个 `go test -run TestName ./path/to/pkg`）；格式化 `gofmt -w .`、`goimports -w .`；静态检查 `go vet ./...`；查标准库/依赖 API 用 `go doc <pkg>.<Symbol>`，不要直接 Read 包源码。

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

## 参考

不确定时对照：`doc/Golang 项目目录结构.md`（分层选型背景、internal 内层按模块分目录）、`doc/Golang 编码风格最佳实践.md`（编码速查表）。
