# Design

## Context

本 change 同时承载两类工作：

1. **首页落地**：`web/src/views/index.vue` 当前仅渲染 `<h1>首页</h1>` 一个占位元素；既不引导访客进入后台，也不展示项目自身。`lang/zh-cn/index.yaml`（含 `lang/en/index.yaml`）已经为「主视觉 / 技术栈 / 开发理念 / 开源博客 / 底部」五大板块预留了 i18n keys（包括 `heroBadge / heroTitle / techTitle / philosophyTitle / blogTitle / footerTagline` 等），表明设计方向早已在本仓库内部对齐，本 change 是把它落地成可渲染的 Vue 组件。
2. **Codex review 修复**：同一份工作树里上一轮 Codex 提出 5 处反馈，本 change 一次性消化。其中 P1（hash 路由下 CTA 路径失效）只在「首页落地」工作流里出现；P2 captcha 噪声池重复是 `internal/infra/captcha` 的语义改动；P2 config 端口 / P3 .gitignore / P3 gofmt 全部是工程纪律问题，与首页本身正交。

详见 `proposal.md - Why`。

## Goals / Non-Goals

**Goals:**

- 把根路由 `/` 从占位升级为品牌门面：Hero + 技术栈 + 开发理念 + 博客入口 + footer，五大板块齐全。
- 所有文案必须走 `useI18n()`，不出现硬编码中文字面量。
- 用 5 个 Codex review 修复刷新当前工作树，使全量 `go vet ./...`、`go build ./...`、`go test -race -count=1 ./internal/infra/captcha/...` 全部绿。
- 不引入新依赖。

**Non-Goals:**

- 不在首页做真实博客文章列表、不接真实后端数据
- 不改路由表、不改 i18n 文件、不改 main.ts / App.vue
- 不改 `captchas` 表结构、不改认证 / 登录链路
- 不为 captcha 引入新配置项（M、容差半径继续以 Go 常量固化）
- 不重做 `infra/captcha` 的合成 / 碰撞算法

## Decisions

### D1. 单 Vue 文件承载全部结构

`web/src/views/index.vue` 已作为根路由 `/` 的视图（参见 `router/static.ts:12`）。本 change 只改造这个文件，不新增 `web/src/layouts/front/`、不新增 components、不新增 routes。

**Why**：本次工作面就是「渲染一个页面」，一切结构扩到 layout / route 都过度工程化。后续若要分拆，再做一次小 change。

**Alternatives considered**：

- 抽 `<TopNav>` / `<Hero>` / `<TechStack>` 等到 `web/src/components/homepage/`：当下无可复用的兄弟场景，分拆会让 hovers / props / i18n context 复杂化，否决。
- 加 `web/src/layouts/front/index.vue` + 让 `views/index.vue` 套这个 layout：当下没有任何组件要跨页面共用，layout 仅承担「空白 + 居中」一件事，浪费一文件，否决。

### D2. 文案一律走 i18n，不在 `<template>` 内硬编码中文

`lang/zh-cn/index.yaml` 顶层暴露的首页 keys（`heroBadge / heroTitle / heroSubtitle / heroDesc / enterAdmin / learnTech / scrollDown / techTitle / techSubtitle / techGolang / techTypeScript / techPostgreSQL / techVue / techElementPlus / techClaudeCode / philosophyTitle / philosophySubtitle / humanCommand / humanDesc / aiWriter / aiDesc / blogTitle / blogSubtitle / footerTagline`）在项目内属于「事实」，本 change 不发明新 key。

**Why**：硬编码中文会让 `lang/en/index.yaml` 的英文版永远赶不上 zh-cn，留个 i18n 隐患。全部走 `t('key')` 后，英文版可直接生效。

**Alternatives considered**：

- 在 vue 文件内 `defineProps` 注入文案 prop：缺乏单一事实源（i18n yaml），且与项目风格不符，否决。

### D3. Element Plus 图标用 `@element-plus/icons-vue` 直接 import，不用项目自家 Icon 组件

`web/src/components/icon/index.vue` 在 `name.indexOf('el-icon-') === 0` 分支上调用 `resolveComponent('el-icon-Box')`——这个回退在元素未全局注册时只会把字符串当 HTML 标签名渲染，行为未在本机验证过。本 change 选择直接 `import { Goods, MagicStick, Box, ... } from '@element-plus/icons-vue'`，用真 Vue 组件渲染。

**Why**：避免依赖 `el-*` 全局注册 / `unplugin-auto-import` 这种工程化选型是否到位的确认；图标有官方 typed import 直通车。

**Alternatives considered**：

- 修 `web/src/components/icon/index.vue` 让 `el-icon-*` 分支走 `import { Box } from '@element-plus/icons-vue'`：跨域、跨文件变更，违背本 change 单文件边界，否决。
- 改 `main.ts` 加 `app.use(ElementPlus)`：跨域，本 change 不动 main.ts。

### D4. 用原生 HTML + scoped SCSS，不引 `<el-button>` 等 Element Plus 组件

`<el-config-provider>` 已在 `App.vue` 使用但没见到 `app.use(ElementPlus)` 的注册，元素组件全局可用性存疑。同时 login.vue 用裸 `<button>` + 自定义样式一直是项目惯例。本 change 也走裸 HTML + 自定义 SCSS。

**Why**：与 login.vue 风格连贯；规避「`<el-button>` 是不是真的注册了」的不确定；色板 / 圆角 / hover 行为可自己控，有助于贴合「简洁大气」。

**Alternatives considered**：

- 用 `<el-button type="primary">` 让 Element Plus 接管样式：依赖未确认的全局注册，否决。

### D5. Hash 路由的 CTA 用 `href="#/admin/login"`

`web/src/router/index.ts:13` 用 `createWebHashHistory()`。纯 `href="/admin/login"` 会触发整页刷新，被 hash 路由丢回首页。

**Why**：在 Vue 不接管点击的前提下，`href="#/admin/login"` 是 hash 路由下唯一同时满足「不整页刷新 + 跳到目标路由」的形式。两侧都用 `#/admin/login`，避免新加 `router.push` 改动（隐含要 `import { useRouter }`，跨域）。

**Alternatives considered**：

- `<router-link to="/admin/login">` + 全局注册 `router-link`：router-link 在 Vue Router 默认已全局注册，但本 change 只动 `views/index.vue`，不去碰 router 配置；用 `<router-link>` 也 ok，本 change 选 `<a>` 是为了在不引入 router-link 渲染形态的前提下保持与 anchor 一致的 DOM。
- `<a @click.prevent="router.push('/admin/login')">`：技术上更稳，但要 import `useRouter`、定义 `goLogin()`，相比 `#/admin/login` 多 4 行代码，性价比低。

### D6. `pickElems` 签名加 `exclude map[string]struct{}` 参数

`composeImage` 选完 `correct` 后，把 `correct` 名集合喂给噪声 `pickElems`；`pickElems` 在 expand pool 后立即按 `exclude` 做 `for ... filtered := append`，再 shuffle。

**Why**：

- 单一参数 + 调用点只在 `composeImage`，改动收敛
- `exclude` 传 `nil` 表示不排除，向后兼容所有既有调用
- `composeImage` 的 call site 一眼能看到「正确答案优先于噪声」的语义，比在 `pickElems` 内部硬编码 `len(correctPool)` 安全

**Alternatives considered**：

- 在 `composeImage` 内循环重抽直到噪声与 correct 不相交：期望值与碰撞概率未量化，且与既有「shuffle + take(count)」两步走的惯例不一致，否决。
- 让 `withinTolerance` 容忍「点击正确名的任意 instance」：把消除歧义的代价从合成端推到校验端，会引入新的位置计算成本（要在 `PlacedElem` 里存多实例），否决。

### D7. config.yaml 端口回滚到 3306/3307；本机 3307/3308 留在 `.env.yaml`

`.env.yaml.example` 的「文档默认」是写库 3306 / 读库 3307（参见 `D:\aiproject\.env.yaml.example:19,29`）。当前 `config/config.yaml` 的 3307/3308 是 Codex 指出「仓库默认被人改成本地端口，应该挪到 `.env.yaml`」的反模式。本 change 把 config.yaml 改回 3306/3307；本机实际覆盖保持 `.env.yaml` 不动（gitignored）。

**Why**：仓库默认值的语义是「任何克隆者克隆完即可跑」，3307/3308 不是普适；本地定制走 `.env.yaml` 是 `infra/config` 包早就支持的方式（参见 CLAUDE.md「配置加载」）。

**Alternatives considered**：

- 保留 3307/3308 在 `config/config.yaml` 并改 `.env.yaml.example`：直接污染「官方默认值 + 误导官方文档」，否决。
- 在 `config/config.yaml` 加注释说明「若本地 3306 不可用，请挪到 `.env.yaml` 覆盖」：注释承载约束，弱约束，否决。

### D8. `.gitignore` 收口调试残留

`/login-error.png`（≈390KB）和 `.playwright-mcp/*.log/*.yml`（≈1.1MB）是 Playwright MCP 在 e2e 调试时落盘的中间产物，仓库零引用。Codex 担心 `git add -A` 把它们吞进版本控制。

**Why**：保留本机数据、用 `.gitignore` 兜住，比 `git rm` + 「以后别再生成」更稳——下次再跑 e2e 又会产生新一组，不会再发生误提交。

**Alternatives considered**：

- `rm -rf .playwright-mcp/ login-error.png`：本机丢失，下次又要重新生成，否决。
- 加 git hook 拦截：过度工程化，等真出问题再上，否决。

## Risks / Trade-offs

- **[Risk] 单文件 675 行**——超过 Vue SFC 的常见尺寸，但项目内已有 login.vue 也接近这个量级（470 行）；拆分到子组件要新增 6 个文件，与本 change 的「单文件边界」目标不一致。**Mitigation**：组件内部用 `<!-- 顶部导航 -->` / `<!-- Hero -->` 这类 HTML 注释分隔区段，未来真的要拆，diff 是纯结构性的。
- **[Risk] `pickElems` 签名加参数会改变既有测试断言**——若测试里硬编码 6 参 / 5 参调用方就会挂。**Mitigation**：本 change 已跑过 `go test -race -count=1 ./internal/infra/captcha/...`，全绿；如果未来再加测试 / 重构调用方，先用本仓库的现有测试模式跟一下。
- **[Risk] `D4` 决定的「裸 HTML 不用 `<el-button>`」让未来要切换回 Element Plus 时要重写 markup**——**Mitigation**：本 change 不需要切；如果产品要复用 Element Plus 的 hover/focus ring 行为，可以在 5 分钟内通过 `<el-button>` 替换 `<a class="btn-primary">` 完成。
- **[Risk] Codex 后续若再指出新点**——本次只消化 5 处；其他位置（如 `web/src/utils/random.ts:54` 的 `window.unique` TS 报错）未在本 change 内处理。**Mitigation**：这些点与本 change 工作流正交，不强求一并消化；下一轮 review 出现再单独立 change。

## Migration Plan

无破坏性变更，部署步骤：

1. **前端**：替换 `web/src/views/index.vue` 单文件即可，无需 `pnpm install` / build config 变更。
2. **后端**：替换 `internal/infra/captcha/click.go`（仅 2 个函数），重启服务即可。
3. **配置 / 工程**：
   - `config/config.yaml` 端口回滚：仅影响新克隆者；正在运行的实例 `.env.yaml` 仍覆盖为 3307/3308（若本机既有覆盖），所以运行态不变。
   - `.gitignore` 新增：仅影响后续 `git add -A` 的扫描结果，不会改变工作树已有文件的可见性。
   - `internal/router/common/common.go` gofmt：纯格式化，行为不变。
4. **回滚**：本次任意单文件改动都可独立 `git checkout` 撤回；首页与 captcha 修复是相对独立的提交点。

## Open Questions

（无可安全延后的开放问题；D1~D8 已涵盖所有架构决策。）
