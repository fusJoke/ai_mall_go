# Spec Delta

## Purpose

Lets the system persist site-level configuration items (site name, ICP record number, contact info, etc.) keyed by a unique `name`. Future capabilities (admin backend settings UI, public site config reads) will read and write rows from this table; this delta only establishes the data shape and uniqueness contract.

## ADDED Requirements

### Requirement: `config` 表持久化站点配置项

The system SHALL persist site-level configuration items in a `config` table with the following columns:

| Column | SQL type | Nullable | Default | Notes |
|---|---|---|---|---|
| `id` | `INT UNSIGNED` | NOT NULL | auto-increment | primary key |
| `name` | `VARCHAR(30)` | NOT NULL | `''` | unique identifier for a config item; MUST have a UNIQUE index |
| `group` | `VARCHAR(30)` | NOT NULL | `''` | grouping for admin UI rendering |
| `title` | `VARCHAR(50)` | NOT NULL | `''` | display title in admin UI |
| `tip` | `VARCHAR(100)` | NOT NULL | `''` | helper text in admin UI |
| `type` | `VARCHAR(30)` | NOT NULL | `''` | input component type (text / textarea / number / select / ...) |
| `value` | `LONGTEXT` | NULL | NULL | the actual config value |
| `content` | `LONGTEXT` | NULL | NULL | JSON payload for option-based types (select / radio / checkbox) |
| `rule` | `VARCHAR(100)` | NOT NULL | `''` | validation rule |
| `extend` | `VARCHAR(255)` | NOT NULL | `''` | extension attributes |
| `input_extend` | `VARCHAR(255)` | NOT NULL | `''` | input-element extension attributes |
| `allow_del` | `TINYINT UNSIGNED` | NOT NULL | `0` | `0` = cannot be deleted, `1` = can be deleted |
| `weigh` | `INT` | NOT NULL | `0` | sort weight |

The `name` column MUST have a UNIQUE index.

The table MUST be named `config` (database table-prefix, if configured, is applied by the GORM `NamingStrategy`).

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