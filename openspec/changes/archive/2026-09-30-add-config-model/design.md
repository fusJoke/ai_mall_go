# Design

## Context

`internal/model/` 已落地 4 个模型：`Admin` / `User` / `Captcha` / `Token`，全部走统一的 `init() { model.Register(...) }` 自注册 + `database.Init()` 内的 AutoMigrate 自动建表路径（详见 `internal/model/model.go`）。`Admin` / `Token` 等模型具备完整业务字段 + GORM 时间戳 + `gorm.DeletedAt` 软删除，注释中明确「字段标签规约」与「TableName(namer) 前缀处理」两条约定，新增模型需沿用。

`openspec/changes/archive/2026-09-28-admin-logout-endpoint/` 已落地登出能力，本次变更不涉及。

业务侧诉求：站点级配置（站点名称、备案号、SEO 关键词、客服联系方式等）需要持久化层；本次变更仅落数据模型，repository / service / handler / admin UI 留待后续独立 change。

## Goals / Non-Goals

**Goals:**

- 新增 `Config` 结构体映射 `config` 表，13 个业务字段 + 3 个 GORM 时间戳字段（含 `DeletedAt` 软删除）。
- 沿用既有 `internal/model` 自注册约定，AutoMigrate 阶段自动建表，无需修改 `database.Init()`。
- 与既有 `Admin` 模型注释风格保持一致（comment 永远第一位、约束按常用度排序、TableName 走 namer）。

**Non-Goals:**

- 不写 `repository` / `service` / `handler` —— 后续独立 change 推进。
- 不写后台【系统配置】UI —— 留待后续 change。
- 不预填示例数据（`site_name` / `icp` 等） —— 后续 change 在 seed / migration 里处理。
- 不修改既有任何 capability 或模型文件。

## Decisions

### D1. 公共时间戳字段 `created_at` / `updated_at` / `deleted_at` 主动补上

用户提供的 SQL 没有这三个字段，但 `CLAUDE.md` 明文规定「每张业务表都需包含」`created_at` / `updated_at` / `deleted_at`，且 `admin.go` / `user.go` / `common.go` / `token.go` 均已遵循。已在交互中与用户确认按 CLAUDE.md 惯例补上。

- **理由**：与项目惯例对齐；后台【系统配置】未来需要审计谁在何时改了哪条配置；软删除可避免误删系统关键项。
- **替代方案**：严格按用户 SQL，不补公共字段 —— 拒绝。理由：会与既有模型风格不一致，且 CLAUDE.md 已明文要求。
- **对齐细节**：`Admin.CreatedAt` / `UpdatedAt` 都带 `not null;autoCreateTime` / `autoUpdateTime`；`DeletedAt` 带 `index`。`Config` 沿用。

### D2. `ID` 类型为 `uint`

SQL `id int UNSIGNED` 在 MySQL 是 32 位无符号（0 ~ 2^32-1），Go 对应 `uint` 或 `uint32`。选择 `uint`：

- **理由**：`uint` 在 64 位平台是 64 位、32 位平台是 32 位，与 Go `int` 行为对称；`GORM` 对 `uint` 的处理与 `int64` 等同；项目里目前没有其他 `UNSIGNED` 模型可比对，但 Go 习惯用法是 `uint` 而非 `uint32`。
- **替代方案 1**：`uint32` —— 拒绝。理由：与 Go 习惯不符；写入时还需考虑转换。
- **替代方案 2**：`int64`（与 `Admin.ID` 一致）—— 拒绝。理由：与 SQL `int UNSIGNED` 类型语义不一致，未来如有外键引用可能引发类型转换 bug。

### D3. `type` 字段命名为 `Type string`

Go 不允许 `type` 作为变量名，但允许作为结构体字段。`Token.Type string` 已在 `common.go:37` 落地同样模式。

- **理由**：与 SQL 列名 1:1 对应；GORM 自动按字段名生成列名 `type`；struct tag 注释里写明用途（输入控件类型）。
- **替代方案**：在 Go 里改成 `InputType string` —— 拒绝。理由：与 SQL / 后续 admin UI 的字段名不一致，徒增映射成本。

### D4. `value` / `content` 为 `*string`

SQL 是 `longtext NULL`，与 `Admin.Email` / `Admin.Mobile` / `Captcha.Code` / `Captcha.Info` 一致 —— 全部用 `*string` 表达 NULL 语义。

- **理由**：nil 表示「未设置」；空字符串 `""` 与 NULL 是不同语义（用户显式把值清空 vs 还没填）。
- **替代方案**：`sql.NullString` —— 拒绝。理由：项目其他模型不用，引入会破坏一致性。
- **替代方案 2**：`string`（不允许 NULL）—— 拒绝。理由：与 SQL 定义不一致；GORM 会忽略 NULL，写入空字符串会让 NULL 值丢失。

### D5. `allow_del` 为 `bool`

SQL 是 `tinyint UNSIGNED`，但语义明确为 0 / 1 二值。Go `bool` 在 GORM 默认映射为 `TINYINT(1)`，MySQL 里 `bool`/`boolean` 即 `TINYINT(1)` 的别名。

- **理由**：`bool` 比 `int8` + 注释更自描述。
- **替代方案**：`int8`（与 `Admin.Status` 一致）—— 拒绝。理由：`Status` 预留扩展多值，`allow_del` 是严格二值；`bool` 更精确。

### D6. `TableName(namer)` 走 `namer.TableName("config")`

与 `admin.go:74-76` / `common.go:57-59` / `user.go:29-31` 完全一致。

- **理由**：让 `database.Init()` 中配置的 `NamingStrategy.TablePrefix`（来自 `config.yaml` 的 `database.prefix`）真正落到表名上。
- **替代方案**：实现 `Tabler` 接口返回 `"config"` 字面量 —— 拒绝。理由：会被 `namer` 绕过，前缀失效。

### D7. `weigh` 字段保留为 `int`

SQL `int NOT NULL DEFAULT 0`，Go `int` 即可。无需 `*int`（非 NULL）。`int` 在 64 位平台是 64 位 —— 比 SQL `int` 大，迁移时不会丢精度。

### D8. 注册方式：`init() { Register(Config{}) }`

与 `admin.go:82-84` / `common.go:64-68` / `user.go:37-39` / `captcha.go:48-51` 一致。

- **理由**：自注册表 `internal/model.All()` 由 `database.Init()` 内的 AutoMigrate 一次性消费；新增模型零调用方改动。
- **替代方案**：在 `database.Init()` 里手写 `db.AutoMigrate(&model.Config{})` —— 拒绝。理由：破坏 `model.All()` 自注册统一路径，每次加模型都要改 `database`。

## Risks / Trade-offs

| 风险 | 缓解 |
|---|---|
| 用户给的 SQL 与最终 Go 结构体字段数不一致（少了 3 个时间戳） | 已在交互中确认按 CLAUDE.md 惯例补；`design.md` D1 节明示 |
| `uint` 主键在未来如果有外键引用其他表（`int64` 主键），类型不匹配 | 当前没有外键；若未来出现，调用方做转换；Go 在算术/比较时不允许 `uint` 与 `int64` 直接运算，需显式转换 |
| `bool` 映射 `tinyint UNSIGNED` 在跨驱动时可能表现不一致（PostgreSQL 等） | 当前项目只用 MySQL（`gorm.io/driver/mysql` 已落地），无风险 |
| `value` / `content` 是 `*string`，调用方需处理 nil | 与 `Admin.Email` / `Captcha.Code` 一致，调用方已有心智模型；新代码 review 时关注 |
| 表名 `config` 在 MySQL 是保留字（小范围），但作为表名使用合法 | 反引号包裹后无歧义；GORM 会处理 |
| AutoMigrate 对已存在表的列变更不会自动 drop column | 本表全新，无此风险 |

## Migration Plan

部署步骤：

1. `go build ./...` —— 编译通过即代表模型自注册语法正确。
2. 启动服务（开发环境）触发 `database.Init()`，AutoMigrate 会自动创建 `config` 表。
3. 无需手动 SQL migration 文件。

回滚策略：

- 数据库：`DROP TABLE IF EXISTS config;`（全新表，无数据损失）。
- 代码：直接 `git revert` 本次 commit，移除 `internal/model/config.go` 即可。

## Open Questions

无。所有可能影响 schema 或 task 拆分的决策已在交互中确认。