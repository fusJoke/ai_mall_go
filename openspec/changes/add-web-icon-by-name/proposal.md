# Proposal

## Why

`web/src/components/icon/index.vue` 已是一个全局图标组件（处理 `el-icon-*`、`local-*`、CSS class 三类），但**完全不支持 `@lucide/vue`**。当前使用 lucide 图标需要每个调用方写 `import { Smile } from "@lucide/vue"` 再把组件当模板用，例如 `web/src/views/admin/login.vue` 直接 import 了 `Eye / EyeOff / Mail / Sparkles` 四个图标——这种"图标来源扩散到每个调用方"的做法让 lucide 从一个图标库变成了一个 import 噪声源，每加一处图标就要改一个文件的 import 列表。

目标：让 lucide 图标（以及未来其他图标源）能像 Element Plus 图标一样走 `<Icon name="..." />`，按名字渲染，调用方零 import。

## What Changes

- `web/src/components/icon/index.vue` 增加第四个分支：传入 `name` 以 `lucide:` 前缀开头时，从 `@lucide/vue` 解析出对应组件（PascalCase 严格匹配，例 `lucide:Smile` / `lucide:EyeOff`），并把现有 `size` / `color` props 透传给 lucide 图标。
- `web/src/views/admin/login.vue` 把 `import { Eye, EyeOff, Mail, Sparkles } from '@lucide/vue'` 移除，模板里 4 个图标改用 `<Icon name="lucide:Sparkles" :size="16" />` 等，作为新能力的端到端示例与回归。
- 现有 Icon 组件的 `el-icon-*` / `local-*` / CSS class 三类保持原行为，不破坏现有调用。

## Capabilities

### New Capabilities

- `web-icon-by-name`: 全局 `<Icon>` 组件增加 lucide 图标按名字渲染的能力（`name` 以 `lucide:` 前缀开头时，从 `@lucide/vue` 解析 PascalCase 组件名并透传 size/color）。

### Modified Capabilities

无。

## Impact

- 代码：
  - 改：`web/src/components/icon/index.vue`（增加一个 if 分支，~10 行）
  - 改：`web/src/views/admin/login.vue`（移除 lucide import，模板里 4 处标签换写）
- 依赖：无新增；`@lucide/vue` 已在 `web/package.json`。
- 不破坏现有调用：`el-icon-*` / `local-*` / CSS class 三条分支不变；login.vue 行为一致（图标相同 + 尺寸相同）。