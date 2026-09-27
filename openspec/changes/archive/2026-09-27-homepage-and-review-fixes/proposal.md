# Proposal

## Why

仓库的根路由 `/` 当前仅作为占位存在（`views/index.vue` 只显示「首页」二字），缺少可被外部访问者识别的「门面」——既不引导用户进入后台，也没能体现项目技术栈与开发理念。同时，上一轮 Codex review 对当前工作树提出了 5 处反馈：登录 CTA 链接在 hash router 下失效、点选验证码噪声池会与正确答案重复导致 ~9% 误判、DB 端口写入了仓库默认配置、调试残留未进 `.gitignore`、新增 router 文件未 gofmt。本次 change 把「首页落地」和「Codex review 修复」打包，以一份变更记录统一归档。

## What Changes

- **新 capability `homepage`**：根路由 `/` 渲染单屏纵向流首页——含顶部导航、Hero、技术栈（6 卡）、开发理念（2 卡）、开源博客入口（占位）、底部栏；所有内容走 `useI18n()` 读 `lang/zh-cn/index.yaml` 现成 keys，硬编码中文文案统一收口到 i18n。
- **改造 capability `click-captcha`**：`composeImage` 选完正确答案后，把正确名集合作为 `exclude` 喂给噪声 `pickElems`，杜绝视觉双胞胎陷阱图；候选 `pickElems` 签名加 `exclude map[string]struct{}` 参数。
- **修复 P1**：将首页两处 `href="/admin/login"` 改为 `href="#/admin/login"`，兼容 `createWebHashHistory()`。
- **修复 P2 (config)**：把 `config/config.yaml` 的写库/读库端口从 3307/3308 还原成文档化的 3306/3307；本机的 3307/3308 覆盖保留在 gitignored 的 `.env.yaml`。
- **修复 P3 (.gitignore)**：新增 `.playwright-mcp/` 与 `/login-error.png` 规则，覆盖 Playwright MCP 调试残留。
- **修复 P3 (gofmt)**：对 `internal/router/common/common.go` 跑 `gofmt -w`，整理 import 排序与 var 块列对齐。

## Capabilities

### New Capabilities

- `homepage`: 根路由渲染门面首页，串联品牌、Hero、技术栈、开发理念与底部信息；前端 mock 数据不依赖后端。

### Modified Capabilities

- `click-captcha`: 在「verify 按原始坐标 + 容差半径判定」这一既有契约不变的前提下，新增「噪声元素不得与任何正确答案同名」约束，从源头消除 ~9% 验证码存在视觉重复目标的概率。

## Impact

- **前端**：
  - 改造 `web/src/views/index.vue`（整体重写，675 行）
  - 不动 `web/src/router/`、`web/src/main.ts`、`web/src/App.vue`、`web/src/lang/`
  - 不引入新依赖；仅用现成的 `@element-plus/icons-vue` + `vue-i18n` + 系统 CSS 变量
- **后端**：
  - 改造 `internal/infra/captcha/click.go`（仅 `pickElems` 与 `composeImage` 两个函数，签名 + 8 行业务逻辑）
  - 不动 handler / service / repo / model
  - 不动 `config/` 下 yaml 的内容性配置（仅端口数值回滚）
- **配置 / 工程**：
  - `config/config.yaml`：DB 端口回滚至 3306/3307
  - `.env.yaml`：本机覆盖端口 3307/3308 不变（gitignored）
  - `.gitignore`：新增两条调试残留规则
  - `internal/router/common/common.go`：纯格式化，行为不变
- **数据**：无 schema 变更；`captchas` 表不变
- **测试**：`go test -race -count=1 ./internal/infra/captcha/...` 在 P2 修复后仍绿
- **依赖**：无新增 Go / npm 依赖
