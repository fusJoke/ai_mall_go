# Tasks

## 1. 配置层

- [x] 1.1 修改 `internal/infra/config/config.go`：在 `Config` 结构体追加 `Upload UploadConfig \`mapstructure:"upload"\`` 字段；新增 `UploadConfig` / `UploadLocalConfig` 两个结构体，含 `Driver` / `MaxSize` / `MaxSizeUnit` / `Suffixes` / `Format` / `Local`（`BaseDir` + `URLPrefix`）；在 `applyDefaults` 内按 design D8 给所有字段填默认值（`driver=local` / `max_size=10` / `max_size_unit=MB` / 6 个默认后缀 / 默认 format / `local.base_dir=storage/uploads` / `local.url_prefix=/uploads`）。验证：`go build ./...` exit 0；`config.Get().Upload.Driver == "local"`（通过 main 跑一遍启动日志）。

- [x] 1.2 新建 `config/upload.yaml` 提交默认配置：含 `driver: local` / `max_size: 10` / `max_size_unit: MB` / `suffixes: [jpg,jpeg,png,gif,webp,pdf]` / `format: "/{topic}/{year}{mon}{day}/{fileName}{fileSha1}{.suffix}"` / `local: { base_dir: storage/uploads, url_prefix: /uploads }`；加注释解释每项含义与变量语法。验证：文件存在；`go run ./cmd/serve` 启动后日志显示 upload 配置加载成功。

- [x] 1.3 新建 `config/upload.yaml.example`：仅写注释 + 占位字段（与 `upload.yaml` 同结构，所有值改成 `<...>`），给运维复制时改。验证：文件存在。

## 2. Local 驱动

- [x] 2.1 新建 `internal/infra/upload/driver/local.go`：定义 `LocalDriver` 结构体（`baseDir` + `urlPrefix` 两字段），`NewLocalDriver(baseDir, urlPrefix string) *LocalDriver` 构造函数；实现 5 个 `Driver` 接口方法（`Save` 用 `os.MkdirAll(filepath.Dir(target), 0o755)` + `os.Create` + `io.Copy`；`Delete` 用 `os.Remove` 并吞掉 `fs.ErrNotExist`；`Url` 用 `path.Join(urlPrefix, storedPath)`；`Exists` 用 `os.Stat` 判 `nil`；`FullPath` 用 `filepath.Join(baseDir, storedPath)` 后 `filepath.Clean`）；包级 `var _ Driver = (*LocalDriver)(nil)` 编译期断言。验证：`go build ./...` exit 0。

- [x] 2.2 新建 `internal/infra/upload/driver/local_test.go`：用 `t.TempDir()` 起临时目录，覆盖 5 个场景：(a) `Save` 写入字节并自动 `MkdirAll` 父目录；(b) `Delete` 已存在文件 + `Delete` 不存在文件（后者无错）；(c) `Url("avatar/x.jpg")` 在 `urlPrefix="/uploads"` 下返回 `/uploads/avatar/x.jpg`；(d) `Exists` 区分 true/false；(e) `FullPath` 用 `filepath.Clean` 干净化输入。验证：`go test ./internal/infra/upload/driver/...` 全绿。

## 3. Service 实现

- [x] 3.1 新建 `internal/infra/upload/upload.go`：定义 `Driver` 接口（5 方法 spec D1）；定义 sentinel errors：`ErrInvalidInput` / `ErrFileTooLarge` / `ErrInvalidSuffix` / `ErrDriverFailure`；定义 `UploadInput` 结构（`Topic` / `OriginalName` / `Reader` / `Size` 可选，业务侧可预填 size 优化预判）；`UploadResult` 结构（`StoredPath` / `Url` / `Size` / `Suffix`）；`Service` 结构 + `NewService(driver Driver) *Service` 构造器。验证：`go build ./...` exit 0。

- [x] 3.2 同文件实现 `Service.Upload(*UploadInput) (*UploadResult, error)`：按 spec ADDED Requirement 3 的 8 步流程（nil 校验 → topic 校验 → LimitReader 限流读字节 → 后缀白名单 → SHA1 计算 → format 模板渲染 → `bytes.NewReader` 回放调 `driver.Save` → 组装 `UploadResult`）；模板渲染走 `strings.ReplaceAll`，按 design D2 的占位符列表替换，`fileSha1` 取 SHA1 全文的 `hex.EncodeToString` 前 16 字符（design D3）；任何驱动错误用 `fmt.Errorf("upload: save: %w", err)` 包成 `ErrDriverFailure`。验证：单元测试覆盖（见 3.5）。

- [x] 3.3 同文件实现 `Service.GetSuffix(filename string) string`（`filepath.Ext` 拿后缀，去掉前导 `.`，`strings.ToLower`）与 `Service.IsImage(filename string) bool`（基于 `GetSuffix` 结果，匹配硬编码集合 `{jpg, jpeg, png, gif, webp, bmp, svg, tiff}`）。验证：单元测试覆盖（见 3.5）。

- [x] 3.4 同文件实现 `Init() / Get() / Reset()` 单例模式（与 token 完全同形）：`Init` 读 `config.Get().Upload` → 调内部 `newDriver(name)` → `NewService(d)` 缓存；`newDriver` switch `"local"` 调 `driver.NewLocalDriver(cfg.Local.BaseDir, cfg.Local.URLPrefix)`，其他返回 `fmt.Errorf("upload: unknown driver %q", name)`。验证：`go build ./...` exit 0。

- [x] 3.5 新建 `internal/infra/upload/upload_test.go`：用 mock driver（手写 `mockDriver` 记录 `Save` 调用 + `storedPath` 验证，覆盖 `Driver` 接口全部 5 方法）覆盖：(a) 上传成功 → `mockDriver.Save` 被调用一次、`storedPath` 与 format 渲染一致、`Url` 来自 `mockDriver.Url`、`Size` 等于实际字节数；(b) 文件超过 `max_size` → 返回 `ErrFileTooLarge` 且 `mockDriver.Save` 0 次；(c) 后缀不在白名单 → `ErrInvalidSuffix` 且 `Save` 0 次；(d) `input == nil` / `topic == ""` / `OriginalName == ""` → `ErrInvalidInput`；(e) 模板替换 `{fileSha1}` 是 16 hex 字符；(f) `GetSuffix` / `IsImage` 覆盖大小写、无后缀、PDF/SVG/PNG 判定。验证：`go test ./internal/infra/upload/...` 全绿。

## 4. 启动接线

- [x] 4.1 修改 `cmd/serve/main.go`：在 `database.Init()` 之后追加 `if err := upload.Init(); err != nil { ... 启动失败 ... }`；保留 graceful shutdown 行为不变。验证：`go build ./...` exit 0；`go run ./cmd/serve` 启动日志可见 upload 配置加载；ctrl+c 优雅退出无 panic。

## 5. 集成验证

- [x] 5.1 运行 `go build ./...` + `go vet ./...`，确认全工程零编译错误（vet 输出已知 `scripts/dump_gorm_schema/main.go` 的预存告警可忽略）。验证：build/vet exit 0。

- [x] 5.2 运行 `go test ./internal/infra/...`，确认 upload + upload/driver + config 三个 package 全绿；既有 package（token / captcha / config / database）不退化。验证：测试输出无 FAIL。

- [x] 5.3 运行 `gofmt -l internal/infra/upload/ internal/infra/config/config.go cmd/serve/main.go`，确认无输出。验证：命令无输出。

- [x] 5.4（需本地 MySQL）启动 `go run ./cmd/serve`，curl `POST /healthz` 返回 200；日志中可见 upload driver 加载行（本环境无 MySQL，迁移阶段会失败，live verification 需在本地有 DB 时手动跑；编译期与单测已确认 init 链路通畅）。
