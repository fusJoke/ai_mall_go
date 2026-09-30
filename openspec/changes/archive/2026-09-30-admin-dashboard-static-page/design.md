# Design

## Context

后台初始化（`admin-init-endpoint`）登录完成后，layout 会按 `weigh ASC, id ASC` 跳到第一个菜单作为落地页。当前 `admin_rule` 表里 weigh 最小的菜单通常是控制台（dashboard），但项目里还没有 `web/src/views/admin/dashboard.vue` —— 登录闭环缺少最后一环。本变更只补齐这个视图，不动后端、不动路由、不动 store。

`web/package.json` 已经依赖 `vue@3.5.33`、`element-plus@2.13.7`、`@element-plus/icons-vue@2.3.2`、`@lucide/vue@^1.21.0`，且 `useAdminInfo`、`useConfig` 已在 `admin-init-endpoint` 落地后可用。本变更不引入新依赖。

## Goals / Non-Goals

**Goals:**
- 给后台控制台提供一个简洁、可作为落地页的 Vue 视图。
- 演示 store 读路径：admin 昵称 + 站点信息。
- 单文件 SFC、易读、可在 `pnpm typecheck` 下零类型错误。

**Non-Goals:**
- 不画图表、不做时间序列、不做对比图（用户明确要求极简）。
- 不接入真实 KPI 数据源（订单 / 用户 / 营收数值均为硬编码常量）。
- 不注册路由、不修改 router、不动 admin-init 协议。
- 不引入国际化（i18n key 列表已存在但本视图不消费）。
- 不动 `admin-init-endpoint` 的任何契约。

## Decisions

### D1. 顶部用 Element Plus `<el-row>` / `<el-card>`，不引入自定义栅格

三个 KPI 卡片用 `<el-row :gutter="16"><el-col :span="8">` 拆分三等列，每个 `<el-col>` 内嵌一个 `<el-card shadow="never">`，内容是 `<Icon>` + `<div class="kpi-label">` + `<div class="kpi-value">`。这是 Element Plus 推荐的统计卡片写法，与项目内既有的 admin 视图风格保持一致（参考 `web/src/views/admin/login.vue` 已有的卡片用法）。

**备选**：手写 CSS Grid。否决原因：项目内其它视图走 Element Plus 栅格，避免样式分叉。

### D2. KPI 数值硬编码为模块顶部 `const`

```ts
const kpis: ReadonlyArray<{ key: string; label: string; value: number; icon: string }> = [
  { key: 'orders',  label: '今日订单',   value: 128,    icon: 'lucide:ShoppingBag' },
  { key: 'users',   label: '用户总数',   value: 3562,   icon: 'lucide:Users' },
  { key: 'revenue', label: '营收（元）', value: 86420,  icon: 'lucide:TrendingUp' },
]
```

`kpis.value` 通过 `Intl.NumberFormat('zh-CN').format(...)` 渲染三位分隔（如 `3,562`），`label` 直接展示。后端联调时只需把 `const kpis` 换成响应式 ref 即可，不需要重写模板。

**备选**：把数值写在模板里。否决：写死在模板里难以复用与替换；放到 `const` 里既符合 spec ADDED Requirement 2，又方便将来无痛切到响应式。

### D3. 昵称兜底：`nickname || username || "管理员"`

`<script setup>` 里：

```ts
const adminInfo = useAdminInfo()
const displayName = computed(() => adminInfo.nickname || adminInfo.username || '管理员')
```

兜底串「管理员」避免极端情况（`/admin/init` 还没跑完就访问该路由）出现空白。

### D4. 站点信息按字段存在性条件渲染

```vue
<div v-if="siteConfig.name" class="info-line">站点：{{ siteConfig.name }}</div>
<div v-if="siteConfig.version" class="info-line">版本：{{ siteConfig.version }}</div>
```

不显示「备案号」字段，因为 design 不强制要求；如果后续 `record_number` 也要展示，加一行 `v-if` 即可。

**备选**：拼接成一段长字符串。否决：分行更清晰，且方便日后单独换样式。

### D5. 样式：用 `<style scoped>` 写少量必要样式

- `.dashboard-page { padding: 24px; }`
- `.greeting { font-size: 22px; font-weight: 500; margin-bottom: 24px; color: var(--el-text-color-primary); }`
- `.kpi-value { font-size: 28px; font-weight: 600; color: var(--el-color-primary); margin-top: 8px; }`
- `.info-footer { margin-top: 32px; padding-top: 16px; border-top: 1px solid var(--el-border-color-lighter); color: var(--el-text-color-secondary); font-size: 13px; }`
- `.info-line + .info-line { margin-top: 4px; }`

刻意使用 Element Plus CSS 变量（`--el-text-color-primary` 等），不引入自定义颜色 token，保持与项目其它视图一致。

**备选**：用 Tailwind / UnoCSS。否决：项目目前用 sass + scoped style，未引入 Tailwind。

### D6. 不挂任何生命周期钩子

数据来源（store）已是响应式，无需 `onMounted` 拉取；图表不画，不存在 resize 监听。整个 SFC 不含 `onMounted` / `onUnmounted` / `watch`。

## Risks / Trade-offs

- **KPI 数值可能让用户误以为是真实数据** → 文件顶部注释明确「演示数据，非真实业务指标」，避免线上误用。
- **路由没注册时该视图无法访问** → 这是预期的：本变更只产出视图文件，路由注册由 `admin-init-endpoint` 的 `admin_rule` 数据驱动；只要 `admin_rule` 里 weigh 最小的菜单 `path` 指向 `/admin/dashboard`、`component` 指向 `web/src/views/admin/dashboard.vue`，登录后就能跳到。
- **未来接真实 KPI 时需要替换 `const kpis`** → D2 已经把替换成本控制在单点改动。
- **`<style scoped>` 与 admin layout 全局样式可能冲突** → 用 BEM 风格类名（`.kpi-value`、`.greeting`）+ scoped 隔离，碰撞面极小。

## Migration Plan

无数据库迁移、无配置变更、无后端部署步骤。前端 `pnpm build` 或 `pnpm dev` 即可看到效果。回滚 = 删除 `web/src/views/admin/dashboard.vue` 单文件，无副作用。

## Open Questions

无。后续真实 KPI 数据接入是独立变更（届时再开新 OpenSpec change）。
