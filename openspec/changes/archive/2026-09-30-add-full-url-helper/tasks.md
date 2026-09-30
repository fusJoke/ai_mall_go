# Tasks

## 1. 配置层

- [x] 1.1 修改 `internal/infra/config/config.go`：在 `ServerConfig` 追加 `CDNURL string \`mapstructure:"cdn_url"\`` 与 `CDNURLParams string \`mapstructure:"cdn_url_params"\`` 两个字段。`applyDefaults` 不给字段填默认（缺失即空串，spec 视为关闭 CDN）。验证：`go build ./...` exit 0。

- [x] 1.2 修改 `config/config.yaml`：在 `server` 块下追加 `cdn_url: ""` 与 `cdn_url_params: ""` 两行，并在每行上方加注释说明「末尾不带 `/`」「如 `format/heif`」。验证：文件存在；`go run ./cmd/serve` 启动后 `config.Get().Server.CDNURL == ""`。

## 2. 后端 `kit/urlx` 包

- [x] 2.1 新建 `internal/kit/urlx/urlx.go`：定义 `FullURL(c *gin.Context, resource string) string`（按 design D5/D6 的 4 步决策）：(1) 空串 → 空串 → 返回 `""`；(2) `strings.HasPrefix(resource, "data:")` → 原样返回；(4) `strings.HasPrefix(resource, "http://") || "https://"` → 原样返回；(5) 读 `config.Get().Server.CDNURL`：非空 → `cfg.CDNURL + cfg.CDNURLParams + resource`；空 → 推断 `scheme := schemeFromRequest(c)` (`c.Request.TLS != nil` ? `"https"` : `c.Request.Header.Get("X-Forwarded-Proto")` 当非空时用其值，否则 `"http"`)、`host := c.Request.Host`，拼 `scheme + "://" + host + resource`。包级 import 包含 `gin`、`strings`、`ai-go-mall/internal/infra/config`。验证：`go build ./...` exit 0。

- [x] 2.2 新建 `internal/kit/urlx/urlx_test.go`：用 `httptest.NewRecorder() + gin.CreateTestContext()` 构造 ctx，覆盖 5 个场景：(a) `resource == ""` → 返回 `""`；(b) `resource == "data:image/png;base64,..."` → 原样返回；(c) `resource == "https://example.com/x.jpg"` → 原样返回；(d) 未配 CDN + HTTP 请求到 `api.example.com:8080` → 返回 `http://api.example.com:8080/uploads/x.jpg`；(e) 已配 `cdn_url=https://cdn.example.com` + `cdn_url_params=format/heif` → 返回 `https://cdn.example.com/format/heif/uploads/x.jpg`，且不出现 `api.example.com`。注入配置走 `config.SetForTest(&config.Config{Server: config.ServerConfig{CDNURL: "..."}})` + `t.Cleanup` 还原（与 upload 测试同模式）。验证：`go test ./internal/kit/urlx/...` 全绿。

## 3. 前端 stores

- [x] 3.1 修改 `web/src/stores/config.ts`：在 `useConfig` 内新增 `cdnUrl: ''` 与 `cdnUrlParams: ''` 两个 ref（或 `reactive` 字段），并在 `setSiteConfig(data)` 里允许 `cdn_url` / `cdn_url_params` 进入；若后端 `siteConfig` 当前不返回这两个键，则保持空串即可（前端先就位等后端填）。验证：`pnpm typecheck`（或对应 ts 校验命令）exit 0。

## 4. 前端 utils

- [x] 4.1 修改 `web/src/utils/common.ts`：新增导出函数 `fullURL(resource: string): string`，分支与 backend 一致：(1) 空串 → `""`；(2) `resource.startsWith('data:')` → 原样；(3) `startsWith('http://') || 'https://'` → 原样；(4) 读 `useConfig().cdnUrl`：非空 → `cdnUrl + useConfig().cdnUrlParams + resource`；(5) 否则 `getBaseUrlPort() + resource`（`getBaseUrlPort` 已存在 `web/src/utils/request.ts`）。需 import `useConfig`（注意避免 pinia 循环：util 内部走 lazy require 或调用方传入 store 实例；项目已有先例 `useAdminInfo` 的 lazy 引用方式，照搬）。验证：本地 `pnpm dev` 跑通；至少手动打开浏览器控制台跑一遍 `fullURL('https://x.com')`、`fullURL('data:image/png;base64,xxx')`、`fullURL('/uploads/x.jpg')` 三种输入得到预期值。

## 5. 集成验证

- [x] 5.1 运行 `go build ./...` + `go vet ./...`，确认后端零编译错误。验证：build/vet exit 0。（vet 仅剩 scripts/dump_gorm_schema 预存告警，与本 change 无关）

- [x] 5.2 运行 `go test ./internal/kit/urlx/...`，确认新包测试全绿。验证：测试输出无 FAIL。

- [x] 5.3 运行 `gofmt -l internal/kit/urlx/ internal/infra/config/config.go config/config.yaml`（yaml 跳过），确认无输出。验证：命令无输出。

- [x] 5.4（需本地 MySQL）启动 `go run ./cmd/serve`，curl `GET /healthz` 返回 200；启动日志无新增错误。
  - 实测：`go run ./cmd/serve` → `ai-go-mall listening on :8080`；`curl http://127.0.0.1:8080/healthz` 返回 `ok` + HTTP 200；启动期无错误。