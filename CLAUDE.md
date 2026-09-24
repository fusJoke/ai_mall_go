# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## 项目状态

Go Web 项目（mall/电商方向），`module ai-go-mall`，`go 1.27.1`。已落地：

- 配置：`internal/infra/config/` 基于 viper 的 YAML 加载（详见「配置加载」）
- 数据库：`internal/infra/database/` 基于 GORM + dbresolver 的写库 + 读副本（详见「数据库」）
- Web 框架：`cmd/serve/` 使用 gin，注册 `/healthz` 与 `/api/v1/ping` 测试路由（详见「Web 框架」）

## 回答偏好

- 使用中文回复
- 遇到有多种实现方案时，列出选项让我选择，而不是直接选一种
- 当我询问某方案是否合理时，请先根据社区惯例判断是否合理，合理直接实现，不合理取消实现并解释原因（社区惯例指对应技术栈的社区，如 golang 开源社区，Gin 开源社区，开源高星仓库，官方文档，权威 blog 等）

## 常用命令

仓库尚无 Makefile / task runner，统一使用 Go 原生命令：

- 构建：`go build ./...`
- 运行（API 服务）：`go run ./cmd/serve`
- 数据库迁移（待 `cmd/migrate` 创建后）：`go run ./cmd/migrate`
- 测试：`go test ./...`
- 跑单个测试：`go test -run TestName ./path/to/pkg`
- 格式化：`gofmt -w .` / `goimports -w .`
- 静态检查：`go vet ./...`

## 分层架构（固定流向，不可变）

调用关系永远是 `Handler → Service → Repository → Model`，禁止跨层或反向依赖：

- `internal/handler/` —— 解析请求（JSON→struct）、参数格式合法性校验、序列化响应。**只处理参数与返回值，不做业务判断。**
- `internal/service/` —— 业务逻辑编排、事务控制、业务合法性校验（如用户名是否存在、余额是否足够）。**只处理业务。**
- `internal/repository/` —— 数据访问层，**只做 DB → 内存 / 内存 → DB 的搬运，不掺业务。**
- `internal/model/` —— 只定义表结构与 `TableName()`。

辅助分层：

- `internal/dto/` —— 数据传输对象
- `internal/middleware/` —— HTTP 中间件
- `internal/router/` —— 路由注册与自动发现
- `internal/kit/` —— 业务层通用工具封装
- `internal/infra/` —— 基础设施（`database`、`config`、`captcha`、`token`、`upload` 等），其中 `token`、`upload` 这类多驱动能力的实现放在同名 `driver/` 子目录下，文件名即驱动名（如 `redis.go`、`database.go`、`local.go`）。`config/` 与 `database/` 已落地，其余待后续按需补充。
- `pkg/` —— 可被外部项目引用的公共库（如 `random`、`filesystem`）

**目录结构规划**：

随着项目功能扩张，各层按以下规则组织子目录：

| 层级         | 子目录规则                                                             |
| ---------- | ----------------------------------------------------------------- |
| Handler    | 按用户身份设立子目录：`admin/`（后台用户/后台端）、`user/`（前台会员/会员端）、`business/`（供应商）等 |
| Service    | 按用户身份设立子目录，与 Handler 对应                                           |
| Repository | 按用户身份设立子目录，与 Handler 对应                                           |
| Model      | **不设子目录**，按业务模块组织文件（如 `admin.go`、`user.go`），一个文件可包含多个相关模型         |

**设计理由**：

1. **Handler/Service/Repository 按用户身份分目录**：不同身份的用户（后台管理员、前台会员、供应商）有各自独立的业务逻辑和 API 接口，分目录隔离更清晰
2. **脱离用户身份的通用服务**：可放入 `common/` 子目录或不设子目录（如验证码、配置、地区数据等跨身份服务）
3. **Model 不按身份分目录**：
   - 一个模型文件可包含多个相关模型（如 `admin.go` 内同时定义 `Admin`、`AdminGroup`、`AdminRule`）
   - 不同身份用户可能共用同一模型（如验证码、配置、省份数据等）

## 入口设计

- `cmd/` 下每个子目录一个入口（多入口设计），不要把所有命令塞到 `cmd/main.go` 一个包里
- 已落地的入口：`cmd/serve`（启动 API 服务，含 `/healthz` 与 `/api/v1/ping` 测试路由）、`cmd/migrate`（DB 迁移，迁移文件放 `cmd/migrate/migrations/`）

## 配置加载

`internal/infra/config/` 基于 `github.com/spf13/viper`：

- `Init(rootDir)` 扫描 `rootDir/config/*.yaml`（按文件名升序合并，跳过 `*.example`），再合并 `rootDir/.env.yaml`（缺失则跳过；存在则覆盖同名键，优先级最高）
- 配置文件全部使用 YAML。`config/` 目录内放默认模板（已提交），`.env.yaml` 放在仓库根目录（gitignored），`.env.yaml.example` 同样在根目录（已提交，方便复制）
- 增加新配置文件只需丢进 `config/`，无需改 loader 代码

数据库配置结构（`config.yaml` 中 `database` 节点）：

- `type` / `prefix`：写库与读库共享
- `write`：写库连接与连接池
- `read.enabled=false` 时所有读写都走写库；`enabled=true` 时通过 dbresolver 自动按操作类型路由

## 数据库

`internal/infra/database/` 基于 GORM：

- `gorm.io/gorm` 核心 + `gorm.io/driver/mysql`（其他驱动按需加）
- 读写分离使用 `gorm.io/plugin/dbresolver`，自动按操作路由（事务强制写库；Find/First/Take/Raw 走读副本）
- `Init()` 不需要传参，内部从 `config.Get().Database` 读所有连接信息
- 调用方通过 `database.Get()` 拿到 `*gorm.DB` 后正常用 GORM 即可

## Web 框架

`cmd/serve/` 使用 `github.com/gin-gonic/gin`：

- `newRouter(serverName)` 内注册当前路由，含：
  - `GET /healthz` —— 健康检查，返回 `ok`
  - `GET /api/v1/ping` —— 测试路由，返回 `{message, server, time}`
- 路由注册逻辑当前写在 `cmd/serve/main.go` 里。后续 `internal/router/` 落地后会被替换为路由自动发现
- 监听端口取自 `cfg.Server.Port`（来自 `config.yaml` 中 `server.port`）
- graceful shutdown：`signal.NotifyContext` 监听 SIGINT/SIGTERM → `srv.Shutdown`（10s 超时）

## 命名规范（强制）

| 场景                 | 风格             | 示例                       |
| ------------------ | -------------- | ------------------------ |
| 文件夹 / 包名           | 全小写、单数、无下划线无驼峰 | `api`、`service`、`model`  |
| 文件名                | 全小写、单词间下划线分隔   | `user_service.go`        |
| 私有变量 / 函数 / 方法     | 小驼峰            | `userID`、`getUserByID`   |
| 导出变量 / 函数 / 结构体    | 大驼峰            | `UserID`、`GetUserByID`   |
| 常量                 | 全大写 + 下划线      | `STATUS_OK`              |
| PostgreSQL 表名 / 字段 | 小写 + 下划线（蛇形）   | `user_name`、`created_at` |

补充要点：

- 布尔值用 `is` / `has` / `can` 前缀
- 禁止单字母变量（循环 `i`/`j`/`k` 除外）、禁止拼音、禁止无意义缩写
- 结构体 JSON tag 一律用蛇形命名
- 包名描述的是「这个包是什么」而非「这个包里装了什么」，用单数

## MysqlSQL 表必备公共字段

每张业务表都需包含：

```sql
id          bigint PRIMARY KEY GENERATED ALWAYS AS IDENTITY,
created_at  timestamptz NOT NULL DEFAULT now(),
updated_at  timestamptz NOT NULL DEFAULT now(),
deleted_at  timestamptz NULL
```

## 进一步参考

`doc/Golang 项目目录结构.md` 详细描述了选型背景（对比了 DDD 风格、`app/` 顶层、本方案三种取舍）以及 `internal` 内层按业务模块分目录的方式；`doc/Golang 编码风格最佳实践.md` 有完整的速查表。生成或修改代码前如有不确定，对照这两份文档。