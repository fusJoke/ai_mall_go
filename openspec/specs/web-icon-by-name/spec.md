# Web Icon By Name Specification

## Purpose

让全局 `<Icon>` 组件支持通过名字渲染 `@lucide/vue` 图标，调用方无需在每个文件里写 `import { Smile } from "@lucide/vue"`。

## Requirements

### Requirement: lucide 前缀按名字渲染 lucide 图标

`<Icon>` 组件 SHALL 在 `name` prop 以 `lucide:` 前缀开头时，从 `@lucide/vue` 解析出对应的图标组件并渲染；前缀后的名字必须与 lucide 库导出的 PascalCase 名字精确一致（如 `Smile` / `EyeOff` / `CirclePlus`）。

#### Scenario: 渲染 lucide Smile
- **WHEN** 调用方写 `<Icon name="lucide:Smile" />`
- **THEN** 渲染出 `@lucide/vue` 中 `Smile` 的 SVG 图标
- **AND** 调用方所在文件不需要 `import` lucide 任何东西

#### Scenario: 渲染 lucide 多词图标
- **WHEN** 调用方写 `<Icon name="lucide:EyeOff" />`
- **THEN** 渲染出 `@lucide/vue` 中 `EyeOff` 的 SVG 图标

### Requirement: 名字大小写严格 PascalCase

`lucide:` 前缀后的名字 MUST 与 `@lucide/vue` 的导出名完全一致（PascalCase）；不接收 kebab-case（如 `eye-off`）、不接收小写（如 `smile`）、不自动归一化。名字在 lucide 库中找不到对应导出时 MUST 渲染失败（不抛错，但 UI 位置不显示图标，等同于 `resolveComponent` 未命中）。

#### Scenario: 小写名字未命中
- **WHEN** 调用方写 `<Icon name="lucide:smile" />`
- **THEN** 图标位置不显示任何图标
- **AND** MUST NOT 抛出运行时异常

#### Scenario: 不存在的 lucide 图标名未命中
- **WHEN** 调用方写 `<Icon name="lucide:NotARealIcon" />`
- **THEN** 图标位置不显示任何图标
- **AND** MUST NOT 抛出运行时异常

### Requirement: size / color 透传至 lucide 图标

`<Icon>` 组件 SHALL 把 `size` prop（默认 `18px`）透传给 lucide 图标的 `size` 属性，把 `color` prop（默认 `#000000`）反映到 lucide 图标的 `currentColor`（lucide 的 `stroke` 默认继承 `currentColor`）。调用方按现有 `<Icon>` 写法的 `:size="20"` 等不必改动含义。

#### Scenario: 自定义尺寸
- **WHEN** 调用方写 `<Icon name="lucide:Sparkles" :size="20" />`
- **THEN** 渲染出的 lucide 图标 SVG 宽高均为 20（像素）

#### Scenario: 自定义颜色
- **WHEN** 调用方写 `<Icon name="lucide:Eye" color="#4f46e5" />`
- **THEN** 渲染出的 lucide 图标 stroke 颜色表现为 `#4f46e5`（通过 CSS color → currentColor）

### Requirement: 现有三类渲染分支不变

`<Icon>` 组件 SHALL 保持现有三条分支行为不变：`el-icon-*` 走 Element Plus 图标、`local-*` 或外部 URL 走 `Svg` 子组件、其他未带前缀的名字走 CSS class（`<i class="..." />`）。

#### Scenario: el-icon 前缀走 Element Plus
- **WHEN** 调用方写 `<Icon name="el-icon-Bell" />`
- **THEN** 渲染 Element Plus 的 Bell 图标（与新增能力前一致）

#### Scenario: local 前缀走 Svg 子组件
- **WHEN** 调用方写 `<Icon name="local-logo" />`
- **THEN** 走 `Svg` 子组件渲染（与新增能力前一致）

#### Scenario: 普通名字走 CSS class
- **WHEN** 调用方写 `<Icon name="smile" />`
- **THEN** 渲染 `<i class="smile icon" />`（与新增能力前一致）
- **AND** 不会触发 lucide 解析