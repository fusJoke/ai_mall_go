# Proposal

## Why

当前项目对「资源完整 URL」的拼装没有统一约定。后端在响应里直接返回 `Url` 字段（如 `/uploads/avatar/20260930/x.jpg`），前端需要把它变成可点击的完整地址时，只能各自拼装 `window.location.origin` 或 `import.meta.env.VITE_AXIOS_BASE_URL`。这套逻辑散落在多个视图里，并且没有处理两种常见特殊形态：

1. **base64 资源**（`data:image/png;base64,...`）—— 直接 `window.location.origin + data:...` 会变成坏 URL；
2. **带协议的资源**（`https://example.com/x.jpg`）—— 再拼一次 origin 就成了 `https://api.example.com/https://example.com/x.jpg`。

更关键的是，项目目前没有 CDN 接入配置。一旦把对象存储或 CDN 域名引入，所有调用点都需要改；缺少一个配置驱动的「CDN 域名 + CDN 参数」开关，部署侧就做不了一键回切。

本次新增一个前后端对称的 `FullURL` 工具：后端在响应生成阶段就能直接吐出完整 URL（前端若 SSR 也可复用）；前端镜像函数在视图层调用同一逻辑。所有规则由 `cdn_url` / `cdn_url_params` 两个新配置项驱动。

## What Changes

- 在 `config/config.yaml` 的 `server` 块下新增 `cdn_url`（CDN 域名，不带末尾 `/`）与 `cdn_url_params`（拼到 `cdn_url` 末尾的字符串，如 `format/heif`）两个配置项；缺失则视为不使用 CDN。
- 新建 Go 包 `internal/kit/urlx`，提供 `FullURL(c *gin.Context, resource string) string`：
  - 当 `resource` 为空、或以 `data:`（base64）开头、或以 `http://` / `https://` 开头时，原样返回；
  - 否则按 `cdn_url` 配置决定域名来源：`cdn_url` 非空则使用 `cdn_url + cdn_url_params + resource`；否则使用 `c.Request` 推断的当前域名（scheme + host + 可选端口）+ `resource`。
- 在 `web/src/stores/config.ts` 的 `useConfig` 暴露 `cdnUrl: string` 字段，并在 `setSiteConfig` 同步路径里被后端响应填充。
- 在 `web/src/utils/common.ts` 新增 `fullURL(resource: string)` 工具函数：当 `useConfig().cdnUrl` 非空时返回 `cdnUrl + cdn_url_params + resource`；否则走 `getBaseUrlPort() + resource`。base64 / 带协议的资源同样原样返回。
- 配套单元测试：`internal/kit/urlx/urlx_test.go` 覆盖三种返回原样 + CDN 优先 + 缺省走当前域 5 个分支；前端由 `pnpm test` 覆盖（如果项目有测试 runner）。

## Capabilities

### New Capabilities

- `full-url-helper`: 前后端统一的资源完整 URL 拼装能力。覆盖 base64、带协议、CDN、当前域名四种分支。

### Modified Capabilities

无（现有 spec 都不涉及资源 URL 拼装）。

## Impact

- 配置文件：`config/config.yaml`（默认配置，无需改 `.env.yaml.example`）
- Go 后端：新增 `internal/kit/urlx/` 包（一个 `.go` + 一个 `_test.go`），被 handler / service 在响应 `avatar` / `image` / `cover` 等字段时使用（不在 spec 强制要求替换既有返回；落地时按业务需要接入）
- 前端 stores：`web/src/stores/config.ts` 增字段
- 前端 utils：`web/src/utils/common.ts` 增函数
- 现有响应字段语义不变：driver 的 `Url()` 仍返回相对路径，由调用侧（handler）调用 `kit/urlx.FullURL` 转完整地址
- 无外部依赖新增；后端继续依赖 gin + viper，前端继续依赖 pinia