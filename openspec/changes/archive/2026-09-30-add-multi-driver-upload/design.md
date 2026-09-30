# Design

## Context

项目目前没有统一的上传基础设施（`internal/infra/upload/` 是空目录）。`internal/infra/token/` 已经走通「Driver 接口 + driver/ 子目录 + Init 单例 + Get 门面」的多驱动模式，可以直接复用其分层思路。配置侧 `internal/infra/config/config.go` 用 viper 扫描 `config/*.yaml`，新增 `upload.yaml` 与 `UploadConfig` 结构体即可接入，无需改 loader。`cmd/serve/main.go` 已经按 `config.Init → database.Init → token.Init → captchaInfra.GetManager()` 顺序启动，新增 `upload.Init()` 自然落在 `database.Init()` 之后即可（上传服务本变更不依赖 DB，但保持既有顺序便于后续业务接入）。

## Goals / Non-Goals

**Goals:**
- 提供可插拔的 `Driver` 接口，本期落地 `local` 本地磁盘驱动。
- 配置驱动的统一规则（大小 / 后缀 / 命名模板）。
- 启动期一次性 `Init()` 缓存单例，业务侧 `upload.Get()` 拿 `*Service` 直接用。
- 测试可注入 mock driver（`NewService(driver Driver)` 入口）。

**Non-Goals:**
- 不接入任何业务端点（admin avatar / product image / 素材库），留待后续 OpenSpec change。
- 不实现非 local 驱动（OSS / S3 / 七牛等）；后续驱动只需在 `newDriver` 追加 case 并新增 `driver/<name>.go`。
- 不做分布式锁 / 跨节点去重（依赖 SHA1 + 文件系统天然幂等即可）。
- 不接 CDN 域名替换（`Url` 暂只拼本地 public prefix）。

## Decisions

### D1. Driver 接口方法集严格匹配用户清单

```go
type Driver interface {
    Save(ctx context.Context, content io.Reader, storedPath string) error
    Delete(ctx context.Context, storedPath string) error
    Url(storedPath string) string
    Exists(storedPath string) bool
    FullPath(storedPath string) string
}
```

不入参 `size int64`，因为业务侧会在 Service 层先把大小限制住（避免给 driver 重复实现一个 LimitReader）。`Save` 入参用 `io.Reader` 而非 `[]byte`，避免大文件全量入内存。

**备选**：driver 自己读取 + 校验大小。否决：违反单一职责，driver 只负责存储；规则校验属于 service。

### D2. 模板替换走简单 `strings.Replace`，不用 Go `text/template`

用户给的语法 `{topic}` / `{year}{mon}{day}` / `{fileName}{fileSha1}` / `{.suffix}` 是简单占位符，`strings.ReplaceAll` 即可；Go template 的 `{{...}}` 语法会与用户预期不符。

实现上定义：
```go
var formatPlaceholders = []string{"{topic}", "{year}", "{mon}", "{day}", "{fileName}", "{fileSha1}", "{.suffix}"}
```
按顺序替换；重复出现的占位符会全部被替换，符合 spec "same placeholder MAY appear more than once"。

**备选**：用 `text/template`。否决：用户语法不是 `{{...}}`，用 template 还得自定义 delim，复杂度上升而收益为零。

### D3. `{fileSha1}` 取 SHA1 全文的前 16 个 hex 字符

16 hex = 64 bit 碰撞空间，已足够业务侧避免同一 topic 下命名重复，且保持路径短。完整 SHA1 放在审计日志（如未来要加），当前不变。

**备选**：完整 SHA1（40 hex）。否决：路径过长；前 16 已经防碰撞。
**备选**：MD5。否决：密码学上不推荐。

### D4. `Service` 用 `bytes.Reader` 回放而非重新打开磁盘文件

`Upload` 第 3 步先把字节读进 buffer（大小受 max_size 限制，单文件 max 几 MB 完全可接受）。第 7 步用 `bytes.NewReader(buf)` 重置游标传给 `driver.Save`。

**备选**：第 3 步用 `io.MultiReader` + `io.TeeReader` 同时算 SHA1 + 限流。否决：逻辑上更绕，且无法同时拿到完整字节用于后续 reader 重放。Buffer 化方案代码更短、行为更可预期，且 max 几 MB 完全合理。

### D5. `GetSuffix` / `IsImage` 是 `Service` 的方法（不是包级函数）

把这两个 helper 挂在 `*Service` 上，而非包级函数，理由是：
- 业务侧通过 `upload.Get().GetSuffix(name)` 调用风格与 `Upload()` 一致，无需额外 import 路径；
- 未来若 driver 需要自定义白名单（比如 CDN 加速特定格式），可以走 `Service` 配置字段扩展；
- 单元测试时通过 `NewService(mockDriver)` 即可构造。

**备选**：包级函数 `upload.GetSuffix(name)`。否决：与既有 `Service.Upload` 调用风格不一致。

### D6. local driver 的两个配置字段：`base_dir` + `url_prefix`

`config/upload.yaml` 下 local 驱动需要：
- `local.base_dir`：磁盘存储根目录（绝对路径，或相对项目根的相对路径）；`Save` 时 `filepath.Join(baseDir, storedPath)`，`FullPath` 同理。
- `local.url_prefix`：对外暴露的 HTTP 路径前缀（如 `/uploads`），`Url` 拼 `urlPrefix + "/" + storedPath`（清理双斜杠）。

`NewLocalDriver(baseDir, urlPrefix)` 构造签名，把这些参数放在 driver 内部而非 service，是因为 `Url`/`FullPath` 是 driver 自己的语义。

**备选**：把所有 driver 配置塞到 `UploadConfig`。否决：driver 特有字段会让 `UploadConfig` 膨胀；按 driver 名字走嵌套 yaml 子树（`upload.local.base_dir` / `upload.local.url_prefix`）解析更清晰。

### D7. `Init()` / `Get()` / `Reset()` 与 token 完全同形

```go
func Init() error {
    if svc != nil { return nil }
    cfg := config.Get().Upload
    d, err := newDriver(cfg.Driver)
    if err != nil { return err }
    svc = NewService(d)
    return nil
}
func Get() *Service { return svc }
func Reset() { svc = nil }
```

`Reset` 同样专供测试。`newDriver` 在同包内，仅 `Init` 调用，业务侧不直接接触。

### D8. 配置 applyDefaults：默认值放在代码而不是 yaml 注释里

与 token.Driver、Server.Mode 等既有字段一致：
```go
func (c *Config) applyDefaults() {
    if c.Upload.Driver == "" { c.Upload.Driver = "local" }
    if c.Upload.MaxSize == 0 { c.Upload.MaxSize = 10 }
    if c.Upload.MaxSizeUnit == "" { c.Upload.MaxSizeUnit = "MB" }
    if len(c.Upload.Suffixes) == 0 {
        c.Upload.Suffixes = []string{"jpg","jpeg","png","gif","webp","pdf"}
    }
    if c.Upload.Format == "" {
        c.Upload.Format = "/{topic}/{year}{mon}{day}/{fileName}{fileSha1}{.suffix}"
    }
    if c.Upload.Local.BaseDir == "" { c.Upload.Local.BaseDir = "storage/uploads" }
    if c.Upload.Local.URLPrefix == "" { c.Upload.Local.URLPrefix = "/uploads" }
}
```

这样 `config/upload.yaml` 即使缺失文件也能用（spec "Default config is applied when upload.yaml is missing"）。

**备选**：yaml 缺失时返回错误。否决：违反现有 `applyDefaults` 的兜底风格，且首期没用户配置会卡死启动。

### D9. local driver 不做「目录已存在」冗余检查

`os.MkdirAll` 已经幂等；不要在 driver 里额外 `Stat` 检查。`Delete` 用 `os.Remove`，对不存在的文件返回的错误用 `errors.Is(err, fs.ErrNotExist)` 判别并吞掉，符合 spec "Delete is idempotent"。

### D10. 文件名 sanitize

`{fileName}` 用 `filepath.Base(originalName)` 去掉路径前缀（防止 `../../etc/passwd` 注入），再去掉非法字符（如 `/\?%*:|"<>`），最终留 ASCII / 数字 / `_-.`；空字符串时退化为 `"file"`。这样无论用户传什么怪名字，存储路径都是受控的。

**备选**：直接用 `originalName`。否决：template 注入风险（`{fileName}` 自身就可能被业务层注入恶意路径）。

## Risks / Trade-offs

- **`Save` 全量 buffer 化大文件** → 限制已被 `max_size` 控制；当前默认 10 MB 完全合理；未来若超过 100 MB 可改为 `io.Copy(io.MultiWriter(buf, sha1Hasher))` 流式路径（不在本期范围）。
- **`{fileSha1}` 16 hex 仍有理论碰撞** → SHA1 完整 40 hex 在 16 hex 截断空间下仍是 2^64 概率，业务场景不会触及；真要绝对去重可换 UUID，但当前需求是去重不是全局唯一。
- **`newDriver` 不可被测试直接调用** → 测试通过 `Init` + 配置注入 或 `NewService(mockDriver)` 两种路径覆盖；前者覆盖 factory，后者覆盖 service 行为。
- **public URL prefix 与前端 router 静态资源路由耦合** → 当前项目静态资源走 `web/dist` 而非后端 `/uploads`；spec 不强制约定 nginx / caddy 怎么反向代理，部署侧负责把 `/uploads/*` 反代到 `local.base_dir`。
- **`Driver` 接口未支持 multipart 直接上传** → 当前 `Save` 入参是 `io.Reader`，业务侧（未来 endpoint）自行处理 multipart 解析后再调 `Upload`，spec 已明确分层。

## Migration Plan

无数据库迁移、无现有 API 变更。本变更纯增量：

1. 部署时不需要任何额外步骤；`config/upload.yaml` 可选，缺失时按 D8 走默认。
2. 回滚 = 删 `internal/infra/upload/` 与 `config/upload.yaml`、还原 `config.go`、还原 `cmd/serve/main.go` 中的一行调用，业务侧未接入即零影响。
3. 真正业务侧接入（admin avatar / product image 等）走后续 OpenSpec change。

## Open Questions

无。后续接 OSS / S3 等新驱动是独立 change。
