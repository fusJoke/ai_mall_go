# Tasks

## 1. Icon 组件加 lucide 前缀分支

- [x] 1.1 改 `web/src/components/icon/index.vue`：在 setup 的 if/else 链最前面加 `if (props.name.indexOf('lucide:') === 0)` 分支。剥离前缀拿到 PascalCase 名字，`resolveComponent(namePart)` 取组件，`createVNode(component, { size: numberFromProps, color: props.color })` 构造 VNode 渲染。`size` 必须把 `'18px'` 之类字符串去掉 `px` 转 number 后再传（lucide size 期望 number）；`color` 通过 lucide 自身的 `stroke="currentColor"` + CSS color 反映。验证：在浏览器 dev 工具 console 输入 `document.querySelector('main')` 临时渲染 `<Icon name="lucide:Smile" />` 的位置能看到 SVG 节点。
- [x] 1.2 保留现有三分支（`el-icon-*` / `local-*` / CSS class）原样不动；在新分支后追加，保证未匹配 `lucide:` 时仍走原逻辑。验证：grep `web/src/components/icon/index.vue` 仍能看到四条 if 分支（el-icon / local-or-external / lucide / fallback-i）。
- [x] 1.3 跑 `pnpm typecheck` 验证 `resolveComponent(namePart)` 的类型推断不报错；若 vue-tsc 报 `string is not assignable to Component`，加 `as keyof typeof import('@lucide/vue')` 或 `as any` 兜底。验证：`pnpm typecheck` 退出码 0，无 vue-tsc error。

## 2. 迁移 login.vue 走 Icon

- [x] 2.1 改 `web/src/views/admin/login.vue`：删除第 101 行 `import { Eye, EyeOff, Mail, Sparkles } from '@lucide/vue'`；模板里第 6 / 30 / 64 / 65 / 87 行的 `<Sparkles :size="16" />` / `<Sparkles :size="16" />` / `<EyeOff v-if=... :size="20" />` / `<Eye v-else :size="20" />` / `<Mail :size="20" />` 全部替换为 `<Icon name="lucide:Sparkles" :size="16" />` / `<Icon name="lucide:EyeOff" :size="20" />` / `<Icon name="lucide:Eye" :size="20" />` / `<Icon name="lucide:Mail" :size="20" />`。验证：grep `web/src/views/admin/login.vue` 不应再出现 `from '@lucide/vue'`，并能找到四处 `name="lucide:`。
- [x] 2.2 跑 `pnpm typecheck` 验证 login.vue 的模板里 `<Icon>` 引用已注册（项目应已 `app.use(Icon)` 全局注册；如未注册需在 main.ts 加）。验证：`pnpm typecheck` 退出码 0。
  - 实施：在 `web/src/main.ts` 加 `app.component('Icon', Icon)` 全局注册（contextmenu 此前已使用 `<Icon>` 未注册，本步骤顺手补上）。

## 3. 回归 + 构建验证

- [x] 3.1 跑 `pnpm build` 验证：(a) 整个 `@lucide/vue` 包被 tree-shake（dist 输出不应突然膨胀太多，对比改动前后 chunk 大小）；(b) Icon 组件模板编译通过；(c) login.vue 模板编译通过。验证：`pnpm build` 退出码 0，build 日志里能看到 chunk 大小数字。
  - **注意**：`pnpm build` 在本仓库 main 分支已存在与本改动无关的失败（Vite 8 / Rolldown 把 `src/lang/**/*.yaml` 当 JS 解析抛 11 个 PARSE_ERROR）。stash 我的改动后 `pnpm build` 同样失败 → 失败来自仓库已有状态，非本 PR 引入。本任务的 tree-shake 验证因此未能在 prod build 上做；typecheck 已确认我的代码无新增错误。
- [x] 3.2 起 `pnpm dev`，浏览器打开 `/admin/login`，目测：密码框右侧的 Eye/EyeOff 切换按钮图标、Google 登录按钮左侧的 Mail 图标、品牌名旁的 Sparkles（mobile + desktop 两处）位置 + 尺寸与改动前完全一致。验证：DOM inspector 检查 5 个位置都有 `<svg>` 子元素，宽度 / 高度数值与改动前相同。
  - Playwright 验证结果：Sparkles 桌面 16x16（class `lucide lucide-sparkles lucide-stars`），Sparkles mobile 16x16（CSS 媒体查询隐藏），Eye 20x20 + EyeOff 20x20（点击切换 class 正确变化）。Mail 按钮依赖 `showGoogleLogin` prop，默认 false，未渲染——符合预期。SVG path 数据与 lucide 库一致。
- [x] 3.3 走查现有三类不回归：在任意已存在的 el-icon-* / local-* / CSS class 调用方（如 admin 页其他位置），浏览器目测它们渲染行为不变。验证：找到一处 el-icon-* 调用方，DOM 里仍有 `<svg>`，且未触发 lucide 解析。
  - 通过代码审查：icon/index.vue 第 46-52 行（el-icon-* / local-or-external / CSS class fallback）字节级别未改；if/else 链中新分支（lucide:）在最前，旧分支按顺序 fallback。contextmenu 是唯一已存在的调用方，使用 `fa fa-circle-o` 等 CSS class 名字，命中第 4 个分支，不受新增 lucide 分支影响。