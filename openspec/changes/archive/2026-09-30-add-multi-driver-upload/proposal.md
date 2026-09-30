# Proposal

## Why

项目目前没有统一的「文件上传」基础设施：商城后续会涉及商品图、头像、素材库、运营 banner 等多种上传场景，且这些场景天然需要切不同的存储后端（先落本地磁盘，未来接对象存储 / OSS / S3）。如果继续在业务侧各写一套上传逻辑，配置散落、命名规范不一致、无法跨场景复用白名单与大小限制。本变更一次性把上传基础设施落齐：可插拔的 driver 抽象 + 配置驱动的统一规则，让业务侧只需 `upload.NewService(...)` 一行接入。

## What Changes

- 新增配置文件 `config/upload.yaml` + 在 `internal/infra/config/config.go` 注册 `UploadConfig` 结构体，含 4 个可配置项：
  - `driver`：当前选用的驱动名（首期 `"local"`）。
  - `max_size` / `max_size_unit`：单文件上限（默认 `10 MB`，单位可选 `B/KB/MB/GB`）。
  - `suffixes`：后缀白名单（默认 `["jpg", "jpeg", "png", "gif", "webp", "pdf"]`）。
  - `format`：存储路径模板，支持变量 `/{topic}/{year}{mon}{day}/{fileName}{fileSha1}{.suffix}`（默认 `/{topic}/{year}{mon}{day}/{fileName}{fileSha1}{.suffix}`）。
- 新增 `internal/infra/upload/driver/` 子包，先实现 `local` 本地磁盘驱动（文件名 `local.go`），对外暴露 `Driver` 接口方法：`Save / Delete / Url / Exists / FullPath`。
- 新增 `internal/infra/upload/upload.go`：
  - 公开 `Driver` 接口与 `Service` 类型；
  - 公开 `NewService(driver Driver) *Service` 构造器（便于测试注入 mock driver）；
  - 公开 `Init() error` 启动期入口（读取 `config.Get().Upload` → 调内部 `newDriver(name)` → 调 `NewService(...)` 缓存单例），与既有 `token.Init()` / `captchaInfra.GetManager()` 风格保持一致；
  - 内部 `newDriver(name string) (Driver, error)` 工厂，目前仅支持 `"local"`；后续加新驱动只需在此追加 case；
  - `Service.Upload(file *UploadedFile) (*UploadResult, error)`：依次校验大小 / 后缀白名单 / 是否为图片 → 按 `format` 模板生成存储路径 → 调 `driver.Save` 写盘；
  - `Service.GetSuffix(filename string) string` 与 `Service.IsImage(filename string) bool`：纯函数辅助方法，被 `Upload` 与业务侧复用；
  - 暴露 sentinel errors：`ErrFileTooLarge` / `ErrInvalidSuffix` / `ErrDriverFailure`，便于业务侧用 `errors.Is` 区分。
- 接入启动流程：在 `cmd/serve/main.go` 中 `database.Init()` 之后调用 `upload.Init()`；当前变更不动业务侧（admin / 前台上传端点留待后续 OpenSpec change 接入）。

## Capabilities

### New Capabilities

- `upload`：通用文件上传能力，涵盖驱动抽象、配置规则、文件名模板生成、SHA1 去重、大小 / 后缀校验。

### Modified Capabilities

无。

## Impact

- 新增目录：
  - `internal/infra/upload/`（service + 公开类型）
  - `internal/infra/upload/driver/`（驱动实现）
- 新增 / 修改文件：
  - `config/upload.yaml`（新增）
  - `config/upload.yaml.example`（新增，仓库提交方便复制）
  - `.env.yaml.example`（追加 `upload` 节点示例，可选）
  - `internal/infra/config/config.go`（追加 `UploadConfig` 结构 + `applyDefaults` 兜底）
  - `internal/infra/upload/upload.go`（新增）
  - `internal/infra/upload/upload_test.go`（新增）
  - `internal/infra/upload/driver/local.go`（新增）
  - `internal/infra/upload/driver/local_test.go`（新增）
  - `cmd/serve/main.go`（在 `database.Init()` 之后追加 `upload.Init()`）
- 依赖：零新增（用 stdlib `crypto/sha1` / `path/filepath` / `mime` / `strings` 即可）。
- 后端其它：本次不动 model / repository / service / handler 层，留给后续业务变更（admin avatar upload、product image upload 等）按场景接入 `upload.Get()`。
- 前端：本次不动。
