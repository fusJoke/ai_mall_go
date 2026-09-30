# Design

## Context

`internal/model/admin.go` 已落地 `Admin` 模型与 `model.Register(...)` 自注册约定（详见 `internal/model/model.go`），`Admin` / `Token` 等业务表具备完整的 `created_at` / `updated_at` / `deleted_at` GORM 时间戳 + `gorm.DeletedAt` 软删除。`cmd/migrate/migrations/` 在 baseline 之上使用 6 位前导零的版本号命名（`000001_baseline.{up,down}.sql`）。

CLAUDE.md 明文规定：
- 「每张业务表都需包含 `id` / `created_at` / `updated_at` / `deleted_at`」；
- 「Model 不设子目录，一个文件可包含多个相关模型」；
- 命名风格（导出大驼峰、bool 用 `is` / `has` / `can` 前缀、私有自复名等）。

`add-config-model/design.md` 已建立"user SQL 与项目惯例冲突时按项目惯例走" + "TableName(namer) 走 namer.TableName(...)" 两条约定，本 change 沿用。

业务诉求：后台【菜单 / 权限规则】需要持久化层，详情见 `proposal.md` Why 与 `specs/admin-rule/spec.md`。

## Goals / Non-Goals

**Goals:**

- 落地 `admin_rule` 表的 GORM 模型（`internal/model/admin.go` 追加）+ 一对 migration 文件；
- 类型安全：用 Go 自定义 string 类型表达 3 个枚举字段（`type` / `open_type` / `extend`），编译期拒绝错值；
- 时间戳与软删除按 CLAUDE.md 强制补齐（user SQL 中 `create_time` / `update_time` 与项目惯例不一致故舍弃）；
- 表名走 `namer.TableName("admin_rule")` 让 `database.prefix` 配置生效。

**Non-Goals:**

- 不实现 repository / service / handler —— 后续独立 change（在 `add-admin-permission-system` 中已规划 Permission Manager）；
- 不实现后台【菜单管理】UI —— 后续独立 change；
- 不预填 seed 数据（菜单树初始化） —— 后续 seed change；
- 不修改既有任何 capability 或模型文件结构（仅在 `admin.go` 末尾追加）。

## Decisions

### D1. 公共时间戳字段（created_at / updated_at / deleted_at）按项目惯例补齐

User SQL 中 `create_time` / `update_time` 是 `bigint UNSIGNED`（Unix 秒）类型，缺失 `deleted_at`。CLAUDE.md 明文「每张业务表都需包含」，与 `add-config-model/design.md` D1 的处理一致。

- **理由**：与既有模型（Admin / Token / Config / Captcha）完全对齐；后台未来需要审计谁在何时改的哪条菜单规则；软删除可避免误删关键规则。
- **替代方案**：严格按 user SQL 不补 —— 拒绝，理由：与项目惯例不一致、CLAUDE.md 已明文要求。
- **细节**：`AdminRule.CreatedAt` / `UpdatedAt` 都带 `not null;autoCreateTime` / `autoUpdateTime`；`DeletedAt` 带 `index`（与 Admin / Config 风格一致）。

### D2. `type` / `open_type` / `extend` 用自定义 string 类型 + 常量

User SQL 中三处用 `enum('...','...','...')`。Go 端选择自定义 string 类型（`type AdminRuleType string`）+ 常量（`RuleTypeDir AdminRuleType = "dir"` 等）。

- **理由**：
  1. 编译期拒绝拼写错值（写 `Type: Type.RuleTypeDri` 编译失败）；
  2. 与 SQL 实际存储 1:1（底层 string，DB 存 VARCHAR/VARCHAR）；
  3. 给 IDE / godoc 提供语义（跳转到常量定义可看注释）。
- **替代方案 1**：用 `string` + 包级普通常量（与 `Config.Type` 风格一致） —— 拒绝，理由：枚举语义比 Config.Type 强，3 个有效值明确，自定义类型收益更高。
- **替代方案 2**：在 DB 端用 `enum(...)` —— 拒绝，理由：跨驱动一致性差、扩展新值需要 DDL 变更、应用层 + DB 层双重约束冗余（与 `add-config-model/design.md` D2 节相同理由）。
- **DB 端**：migration 用 `varchar(16)` / `varchar(32)`，让应用层承担枚举约束。

### D3. `open_type` 用指针类型 `*AdminRuleOpenType`

User SQL 中 `open_type` 是 `DEFAULT NULL` 可空。

- **理由**：与 `Admin.Email` / `Admin.Mobile` / `Captcha.Code` / `Captcha.Info` 一致的 `*T` 表达 NULL 语义；nil 表示「不适用」（如纯权限节点 type=node 不需要该项），区别于「显式设为某个值」。
- **替代方案**：`sql.NullString` —— 拒绝，理由：项目其他模型不用，引入会破坏一致性。

### D4. `keepalive` 用 `bool`

User SQL 中 `keepalive` 是 `tinyint UNSIGNED NOT NULL DEFAULT 0`，但语义是严格二值（开 / 关）。

- **理由**：与 `Config.AllowDel` 一致 —— `bool` 比 `int8` + 注释更自描述；GORM 默认把 `bool` 映射为 `tinyint(1)`。
- **替代方案**：`int8`（与 `Admin.Status` 一致） —— 拒绝，理由：`Status` 预留扩展多值（0=禁用 / 1=启用，未来可能加 2=待审核），`keepalive` 严格二值，`bool` 更精确。

### D5. `pid` 与 `id` 同为 `uint`

User SQL 中 `pid` 和 `id` 都是 `int UNSIGNED`。

- **理由**：`uint` 在 64 位平台是 64 位、32 位平台是 32 位；与 `Config.ID uint` 一致；与 SQL `int UNSIGNED` 语义对齐，便于构造规则树时的算术比较。
- **替代方案**：`int64` —— 拒绝，理由：与 SQL 类型不一致，外键引用时类型转换易出错。

### D6. `id` 用 `uint`（与 `Config.ID` 一致）

- **理由**：同 D5，SQL `int UNSIGNED` → Go `uint`。
- **替代方案**：`int64`（与 `Admin.ID` 一致） —— 拒绝，理由：与 SQL `int UNSIGNED` 不一致，潜在类型转换 bug。

### D7. `weigh` / `status` 字段类型选择

- `weigh int`：SQL 是 `int NOT NULL DEFAULT 0`，非 NULL，Go `int` 即可。
- `status int8`：SQL 是 `tinyint NOT NULL DEFAULT 1`，Go `int8` 与 `Admin.Status` 一致，预留扩展多值空间。

### D8. 表名走 `namer.TableName("admin_rule")`

- **理由**：与 `admin.go` 中 `Admin.TableName` 走 `namer.TableName("admins")` 完全一致 —— 让 `database.Init()` 中配置的 `NamingStrategy.TablePrefix`（来自 `config.yaml` 的 `database.prefix`）真正落到表名上。
- **替代方案**：实现 `Tabler` 接口返回 `"admin_rule"` 字面量 —— 拒绝，理由：会被 `namer` 绕过，前缀失效。

### D9. 字段顺序：业务字段在前、UpdatedAt → CreatedAt → DeletedAt 收尾

- **理由**：与 `Admin` / `Token` / `Config` 完全一致；便于阅读（业务逻辑先、时间戳在后）。
- **细节**：`AdminRule.ID` / `Pid` / `Type` / `Title` / `Name` / `Path` / `Icon` / `OpenType` / `Url` / `Component` / `Keepalive` / `Extend` / `Remark` / `Weigh` / `Status` 共 15 个业务字段，之后 `UpdatedAt` → `CreatedAt` → `DeletedAt` 收尾。

### D10. 迁移文件命名：000002_admin_rule

- **理由**：`cmd/migrate/migrations/` 已有 `000001_baseline`；`add-admin-rule-model` 是 baseline 之后的第一个独立 migration，故用 `000002`。
- **替代方案**：合入 baseline —— 拒绝，理由：baseline 是 `replace-automigrate-with-golang-migrate` 落地时的"现状快照"，追加新表应当走独立 migration 文件。

### D11. 注册方式：init() { Register(Admin{}, AdminRule{}) }

- **理由**：与 `model.go` 注释「也支持一次性注册多个：`Register(User{}, Product{}, Order{})`」一致；自注册表 `model.All()` 由 `database.Init()` 消费。
- **替代方案**：在 `database.Init()` 里手写 `db.AutoMigrate(&model.AdminRule{})` —— 拒绝，理由：项目已切到 golang-migrate，`db.AutoMigrate` 不再被调用；保留 `model.Register` 自注册路径只为运行时一致性检查（如启动期日志打印已注册模型列表）。

## Risks / Trade-offs

| 风险 | 缓解 |
|---|---|
| user SQL 与最终 Go 结构体字段数不一致（多了 3 个时间戳；缺原 sql 保字段等） | D1 已明示 user SQL 改写原则；spec 已覆盖所有字段；migration SQL 与 struct 字段一一对齐 |
| `uint` 主键在跨表外键引用时类型不匹配（Admin 是 int64） | 当前没有跨表外键；若未来出现，调用方做转换；Go 不允许 `uint` 与 `int64` 直接算术/比较 |
| 自定义 string 类型 `AdminRuleType` 在 GORM AutoMigrate 时可能不被识别为 string | GORM 用反射取 `Kind()`，对自定义 string 类型会落到 `reflect.String`；且本期切到 golang-migrate 后 AutoMigrate 不再被调用，影响为零 |
| enum 类型在 DB 端用 varchar，缺少 DB 层约束 | 应用层编译期 + 启动期校验已足够；DB 层若需强约束可在后续 change 加 `CHECK` 约束（本期不引入，避免与 utf8mb4 / utf8mb4_general_ci 兼容性冲突） |
| `pid` 索引名为 `idx_admin_rule_pid`（GORM 自动生成），与 baseline 索引命名风格（`idx_<table>_<col>`）一致 | migration SQL 显式声明同名 |
| 表名 `admin_rule` 在 MySQL 是合法标识符；database.prefix 为空时表名即 `admin_rule` | 反引号包裹后无歧义；GORM 处理 |

## Migration Plan

部署步骤：

1. 合并本 change 代码 + 后续合并 `cmd/migrate/migrations/000002_admin_rule.{up,down}.sql`。
2. 启动 `cmd/serve`，启动期 `migrate.Up()` 自动应用 `000002_admin_rule`。
3. 验证：`SHOW CREATE TABLE admin_rule` 输出与 000002 SQL 字节级一致；`SHOW INDEX FROM admin_rule` 显示 `idx_admin_rule_pid` 与 `idx_admin_rule_deleted_at` 两个索引。
4. 无需预填数据；后续 seed change 处理菜单初始化。

回滚策略：

- 数据库：`go run ./cmd/migrate down 1` 回滚 `000002_admin_rule.down.sql`（`DROP TABLE IF EXISTS admin_rule;`）。
- 代码：`git revert` 本 commit，移除 `internal/model/admin.go` 中追加的 `AdminRule` / 3 个枚举类型 / 9 个常量段。

## Open Questions

- **是否需要给 `type` / `extend` 加 DB 层 `CHECK` 约束**：本期不引入；utf8mb4 下 MySQL 8.0 才稳定支持 CHECK，且 GORM tag 不直接生成 CHECK。若后续出现脏数据，可独立 change 加 `ALTER TABLE admin_rule ADD CONSTRAINT ... CHECK (type IN ('dir','menu','node'))`。
- **`keepalive` 是否要预留更多状态**：本期按严格二值；若未来加 `keepalive=2=记忆上次` 之类的语义，可单独 change 把类型改成 `int8`。