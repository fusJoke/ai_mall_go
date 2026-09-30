# Proposal

## Why

站点级基础配置（站点名称、备案号、SEO 关键词、客服联系方式等）目前没有持久化层 —— 前端拿不到这些数据，后台也没办法管理。后台未来的【系统配置】功能需要一张 `config` 表来读写这些键值。本次变更**只先落地数据模型**，repository / service / handler / admin UI 留待后续独立 change 推进，避免一次性塞太多职责。

## What Changes

- 新增 `internal/model/config.go`：定义 `Config` 结构体，映射 SQL `config` 表（含 `name` 唯一索引、`group`/`title`/`tip`/`type`/`value`/`content`/`rule`/`extend`/`input_extend`/`allow_del`/`weigh` 字段）。
- 通过 `init() { model.Register(Config{}) }` 把模型注册到 `internal/model` 的自注册表，由 `internal/infra/database.Init()` 在 AutoMigrate 阶段统一建表 —— **不新增**单独迁移文件。
- 不修改：现有任何 capability、路由、handler、service、repository、`tokens` / `admins` / `users` / `captchas` 表结构。

## Capabilities

### New Capabilities

- `site-config`：站点级配置数据模型 —— 持久化站点运行所需的键值型配置项；唯一键为 `name`；`group` 用于在前端按类目分组渲染，`type` 指示后台【系统配置】界面应该渲染的输入控件类型（text / textarea / number / select / image 等），`value` / `content` / `rule` / `extend` / `input_extend` / `allow_del` / `weigh` 为表单生成与校验所需元数据。

### Modified Capabilities

<!-- 无：本次只新增模型，不改任何既有 capability 的需求。 -->

## Impact

- **代码**：新增 1 个文件 `internal/model/config.go`（约 60-80 行）。`internal/model/admin.go` 等既有模型文件不变。
- **数据库**：AutoMigrate 会新建 `config` 表（含 `name` 唯一索引）。无破坏性变更 —— 全新表，不影响既有数据。
- **依赖**：无新增第三方依赖；GORM 已落地。
- **后续 change 留口**：repository / service / handler / admin UI / 前端【系统配置】管理页都不在本 change 范围。