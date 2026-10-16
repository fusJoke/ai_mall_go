# site-config Specification

## Purpose
Lets the system persist site-level configuration items (site name, ICP record number, contact info, etc.) keyed by a unique `name`. Future capabilities (admin backend settings UI, public site config reads) will read and write rows from this table; this delta only establishes the data shape and uniqueness contract.

## Requirements

### Requirement: `config` 表持久化站点配置项

系统 SHALL 在 `config` 表持久化站点配置项，核心字段如下：

| 字段 | 类型 | 可空 | 默认值 | 说明 |
|---|---|---|---|---|
| `id` | `INT UNSIGNED` | 否 | auto-increment | 主键 |
| `name` | `VARCHAR(30)` | 否 | `''` | 配置项唯一标识 |
| `group` | `VARCHAR(30)` | 否 | `''` | 后台 UI 分组 |
| `title` | `VARCHAR(50)` | 否 | `''` | 后台展示标题 |
| `tip` | `VARCHAR(100)` | 否 | `''` | 后台提示文案 |
| `type` | `VARCHAR(30)` | 否 | `''` | 输入组件类型（text / textarea / number / select ...） |

表名 MUST 为 `config`（若配置了表前缀，由 GORM `NamingStrategy` 施加）。

#### Scenario: Core columns are present on the created table
- **WHEN** 迁移建出 `config` 表
- **THEN** 它 SHALL 含 `id` / `name` / `group` / `title` / `tip` / `type` 六列
- **AND** 除 `id` 外各列 SHALL NOT NULL 且默认 `''`

### Requirement: `config` 表取值与扩展字段

系统 SHALL 在 `config` 表提供下列取值与扩展字段：

| 字段 | 类型 | 可空 | 默认值 | 说明 |
|---|---|---|---|---|
| `value` | `LONGTEXT` | 是 | NULL | 配置项实际取值 |
| `content` | `LONGTEXT` | 是 | NULL | 选项型（select / radio / checkbox）的 JSON 载荷 |
| `rule` | `VARCHAR(100)` | 否 | `''` | 校验规则 |
| `extend` | `VARCHAR(255)` | 否 | `''` | 扩展属性 |
| `input_extend` | `VARCHAR(255)` | 否 | `''` | 输入元素扩展属性 |
| `allow_del` | `TINYINT UNSIGNED` | 否 | `0` | `0` 不可删除 / `1` 可删除 |
| `weigh` | `INT` | 否 | `0` | 排序权重 |

#### Scenario: Option-based types carry their payload in content
- **WHEN** 一个配置项的 `type` 为 `select` / `radio` / `checkbox`
- **THEN** 它的候选项列表 SHALL 以 JSON 存入 `content`
- **AND** 具体取值 SHALL 存入 `value`

#### Scenario: AutoMigrate creates the config table

- **WHEN** the database initialization runs AutoMigrate against a fresh database
- **THEN** a `config` table exists with all 13 columns matching the schema above
- **AND** `name` has a UNIQUE index

#### Scenario: Config struct is registered for AutoMigrate

- **WHEN** the Go process imports `internal/model`
- **THEN** `model.All()` includes `Config`
- **AND** subsequent `database.Init()` AutoMigrate creates the `config` table without requiring caller code changes

### Requirement: `name` 是配置项唯一标识

The system SHALL treat `name` as the unique identifier for a config item; inserts with a duplicate `name` MUST be rejected by the database.

#### Scenario: 插入重复 name 被拒绝

- **WHEN** a record with `name = "site_name"` is inserted twice
- **THEN** the second insert fails with a unique-constraint violation
- **AND** the first row remains intact

### Requirement: 可空字段允许 NULL

The system SHALL allow `value` and `content` to be `NULL` in the database (semantically "not set yet"), distinct from the empty string.

#### Scenario: 显式插入 NULL 的 value / content

- **WHEN** a record is inserted with `value = NULL` and `content = NULL`
- **THEN** the row is persisted
- **AND** a subsequent `SELECT` returns `NULL` for those columns, not `''`
