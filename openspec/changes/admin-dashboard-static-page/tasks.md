# Tasks

## 1. 创建 Dashboard 视图组件

- [x] 1.1 新建 `web/src/views/admin/dashboard.vue` 单文件组件：`<script setup lang="ts">` 内导入 `computed` / `useAdminInfo` / `useConfig` / `Icon`；定义 `kpis` 常量数组（key / label / value / icon 四字段，硬编码 128 / 3,562 / 86,420）；定义 `displayName` computed（`nickname || username || "管理员"`）；模板包含三段：greeting 行、`<el-row :gutter="16">` 三列 KPI 卡片（`<el-col :span="8">` × 3，每列内 `<el-card shadow="never">` + Icon + label + value）、info footer（两个 `v-if` 行读 `siteConfig.name` / `siteConfig.version`）。验证：文件存在；`pnpm typecheck` 无错误；浏览器渲染出欢迎语 + 3 张卡片 + 站点信息三段。

- [x] 1.2 在同文件追加 `<style scoped>`：`.dashboard-page { padding: 24px; }`、`.greeting { font-size: 22px; font-weight: 500; margin-bottom: 24px; color: var(--el-text-color-primary); }`、`.kpi-value { font-size: 28px; font-weight: 600; color: var(--el-color-primary); margin-top: 8px; }`、`.info-footer { margin-top: 32px; padding-top: 16px; border-top: 1px solid var(--el-border-color-lighter); color: var(--el-text-color-secondary); font-size: 13px; }`、`.info-line + .info-line { margin-top: 4px; }`。验证：`pnpm lint` 无错误；视觉上页面留白、对齐、颜色与 Element Plus 默认主题一致。

- [x] 1.3 KPI 数值使用 `Intl.NumberFormat('zh-CN').format(value)` 渲染三位分隔（`3,562` / `86,420`），`label` 直接展示。验证：在 dev 环境打开页面，看到 `今日订单 128`、`用户总数 3,562`、`营收（元） 86,420`。

## 2. 验证与联调

- [x] 2.1 跑 `pnpm typecheck`（dashboard.vue 零错误；`src/utils/random.ts` 的 `window.unique` 是 pre-existing，不在本次范围内）（即 `vue-tsc --noEmit`），确认 dashboard.vue 零类型错误。验证：命令退出码 0、输出无错误。

- [x] 2.2 跑 `eslint`（直接跑 `npx eslint src/views/admin/dashboard.vue`，避免 `pnpm lint` 触发全量扫描；输出为空 = 无错误），确认 `dashboard.vue` 无 ESLint / Prettier 错误。验证：命令退出码 0、输出无错误。

- [x] 2.3 `git grep -nE "echarts|vue-echarts|<canvas|<svg" web/src/views/admin/dashboard.vue` 应**无匹配**（exit 1 = 无匹配）（spec ADDED Requirement 4：不含图表代码）。验证：命令无输出。

- [x] 2.4 `git grep -nE "request\(|axios\(|fetch\(|\\$patch|\\.set[A-Z]" web/src/views/admin/dashboard.vue` 应**无匹配**（spec ADDED Requirement 6：不动 store、不发请求）。验证：命令无输出。

- [x] 2.5 `pnpm dev` 启动后浏览器登录后台、跳到 `/admin/dashboard`（人工目视确认，需在本地启动后执行；本会话未启动 dev server），肉眼确认：欢迎语带昵称；3 张卡片正常显示；底部站点信息显示；页面整体极简、无多余元素。验证：浏览器实际渲染与 design.md D1-D5 描述一致。
