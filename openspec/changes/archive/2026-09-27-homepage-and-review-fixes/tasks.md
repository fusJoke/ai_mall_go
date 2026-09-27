# Tasks

## 1. 前端首页落地

- [x] 1.1 在 `web/src/views/index.vue` 中用 Element Plus 图标 + 裸 HTML + scoped SCSS 实现 5 大板块布局，并验证 `pnpm typecheck` 不出现 `src/views/index.vue` 相关报错
- [x] 1.2 将首页所有可见文案改为 `useI18n()` 取 `lang/zh-cn/index.yaml` 现成 keys（含 hero/tech/philosophy/blog/footer 5 组共 24 个键），并验证切换语言不出现硬编码中文
- [x] 1.3 把两处引导登录的 CTA `href="/admin/login"` 改为 `href="#/admin/login"`，并验证 hash router 下不会触发整页刷新

## 2. 点选验证码噪声池去重

- [x] 2.1 改造 `internal/infra/captcha/click.go` 中 `pickElems` 函数签名增加 `exclude map[string]struct{}` 参数，并在 pool 展开后立即按 `exclude` 过滤，向后兼容（`nil` 表示不排除）
- [x] 2.2 改造 `composeImage`，把已选定的 `correct` 名集合喂给噪声 `pickElems`，并验证 `go test -race -count=1 ./internal/infra/captcha/...` 全绿

## 3. 工程纪律收口

- [x] 3.1 把 `config/config.yaml` 的写库端口 3307 改回 3306、读库端口 3308 改回 3307，并验证与 `.env.yaml.example` 文档化默认一致
- [x] 3.2 在 `.gitignore` 中新增 `.playwright-mcp/` 和 `/login-error.png` 两条规则，并验证后续 Playwright MCP 残留不再被 `git add -A` 误吞
- [x] 3.3 对 `internal/router/common/common.go` 跑 `gofmt -w`，并验证 `gofmt -l` 对该文件返回空

## 4. 全量验证

- [x] 4.1 运行 `go vet ./...` 并验证无任何报错
- [x] 4.2 运行 `go build ./...` 并验证所有包编译通过
- [x] 4.3 运行 `go test -race -count=1 ./internal/infra/captcha/...` 并验证全部测试通过（含 pickElems 新签名下的断言）
