# Design

## Context

`web/src/components/icon/index.vue` 是一个用 `defineComponent` + `setup(props)` + return render fn 写的轻量组件，**渲染函数分支写在 setup 里**（不是 template），通过 `createVNode` 直接构造 VNode 而非 template 渲染。现有三分支：`el-icon-*` → 包 `<el-icon>` 渲染 Element Plus 图标；`local-*` 或外部 URL → 渲染 `Svg` 子组件；其他 → 渲染 `<i class="xxx icon">`。

`@lucide/vue` 已在 `web/package.json`，但当前只有 `web/src/views/admin/login.vue` 直接 import（4 个图标：Eye / EyeOff / Mail / Sparkles），模板里把组件当 SVG 元素直接用（`<Sparkles :size="16" />`），每个用图标的位置都要进 import 列表。

按用户决策：API 用前缀模式（`lucide:Smile`），名字严格 PascalCase，顺手迁移 login.vue。

## Goals / Non-Goals

**Goals:**

- 在 `web/src/components/icon/index.vue` setup 内加第四个 `if` 分支，处理 `lucide:` 前缀
- 调用方零 import，跨文件可复用同一组件名约定
- 现有 `el-icon-*` / `local-*` / CSS class 三分支零行为变化
- `web/src/views/admin/login.vue` 移除 lucide 的直接 import，4 处模板改用 `<Icon>`，作为新能力的端到端验证

**Non-Goals:**

- 不引入新的图标源（heroicons / tabler 等本期不做）
- 不替换现有 Icon 组件或重命名为其他东西
- 不为 lucide 图标做懒加载 / 按需 import；保留 `resolveComponent` 走运行时解析，依赖 Vite/Rollup 的 tree-shaking 把 `@lucide/vue` 整个包 tree-shake 到只保留引用到的图标
- 不增加 icon name 校验（如 type-safe enum），保持 string prop

## Decisions

### Decision 1: 用 `lucide:` 前缀而非直接名字

- **选择**：`<Icon name="lucide:Smile" />`
- **原因**：现有 `el-icon-*` / `local-*` 已经是前缀约定，延续这个惯例最不破坏；直接名字（如 `Smile`）会让"PascalCase 字符串到底是 CSS class 还是 lucide 组件"产生歧义，必须再加 fallback 顺序判断。前缀让分支判断 O(1)（`indexOf('lucide-') === 0`），且语义自描述。
- **考虑的替代**：直接名字 + 全局注册所有 lucide 图标 → 太重；独立 `<LucideIcon>` 组件 → 多一个组件心智，新旧混用会乱；当前选择最贴合既有惯例。

### Decision 2: 用 `resolveComponent` 而非静态 import

- **选择**：在 setup 内 `resolveComponent('Smile')`，把结果传给 `createVNode`
- **原因**：`@lucide/vue` 在 `web/src/main.ts`（或类似入口）大概率已经把全部图标注册为全局组件（lucide-vue-next 默认行为），`resolveComponent` 直接命中；如未注册则走 module-level `import * as lucide from '@lucide/vue'` + `lucide[name]` 兜底。两种方式都不需要每个调用方写 import。
- **考虑的替代**：让用户在 main.ts 写一段 `app.component('lucideSmile', lucide.Smile)` 全部预注册 → 调用方还得按命名规则取；放弃 resolveComponent → 与现有 `el-icon-*` 分支不一致。

### Decision 3: PascalCase 严格，不做大小写归一化

- **选择**：用户传什么就 resolveComponent 什么，错误名字不报错
- **原因**：lucide 库自身导出的就是 PascalCase（如 `EyeOff`），归一化层（kebab-case ↔ PascalCase）会引入额外的字符串转换函数与一连串测试，得不偿失；前端开发者复制图标名从 lucide.dev 粘贴即可。错误名字静默失败（resolveComponent 找不到时返回字符串原值，渲染出 `[object Object]` 或类似）比抛错安全。
- **考虑的替代**：自动 kebab-case → PascalCase → 增加 utility + 单测，且容易出现命名冲突（`eye-off` 转完仍是 `EyeOff`，但 `eye` 是 Element Plus 内置图标名）。

### Decision 4: size/color 直接透传，不做 prop 重命名

- **选择**：Icon 组件的 `size: string` 转数字后传给 lucide 的 `size` prop；`color` 通过 CSS `color: <color>` 让 lucide 的 `stroke="currentColor"` 生效
- **原因**：Icon 组件已经有 `size` / `color` 两个 prop，调用方习惯写 `<Icon :size="20" color="#xxx" />`；lucide 的 size 也接受 string（内部会 parse），color 通过 CSS color → currentColor 是 lucide 自身惯例。最小改动透传，无需引入新的 prop。
- **考虑的替代**：把 `size` 改成 number prop → 破坏现有调用方；增加 `stroke-width` / `fill` 等 lucide 专属 prop → 当前 4 个图标都用默认 strokeWidth=2，不必暴露。

## Risks / Trade-offs

- **`@lucide/vue` 包体积大** → 当前 `web/package.json` 引 `^1.21.0` 已经包含全量图标，依赖 Vite 的 tree-shaking。`resolveComponent` 在源码层是字符串查找，对 tree-shaker 透明（不会因 `import * as lucide` 而带全包）。验证方式：`pnpm build` 后看 chunk 是否仅含引用到的 4 个图标。
- **`resolveComponent` 找不到时静默失败** → 当前选择是"渲染空"，UI 上会出现一个空白小方块。后续如要"未命中时 console.warn"可加，但不在本期 scope。
- **login.vue 迁移风险** → `:size="16"` 在 lucide 上是 number，在 Icon 上转回 string `'16px'`，再传回 lucide 时需要 Icon 内部 `parseInt` 或 `replace('px', '')` 后给 lucide，否则 lucide 会拿到 `'16px'` 字符串并 fallback 到默认 size。
- **未走 setup-script 改原写法** → Icon 组件用了非常规写法（setup return render fn）。本次只在 setup 内新增分支，不改它的写法。

## Migration Plan

- 单步发布：组件改动 + login.vue 改动同一次提交。
- 回滚：直接 revert 提交即可，无数据库 / 状态变更。
- 验证：`pnpm dev` 起服务，浏览器打开 `/admin/login`，目测 4 个图标与改动前视觉一致（同样的图标 + 同样尺寸）；DOM 检查 `<svg>` 在密码框右侧、登录按钮左边的位置正确。

## Open Questions

无（三个 scope 问题已在 propose 前与用户对齐）。