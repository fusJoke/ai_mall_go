# Proposal

## Why

后台初始化完成后（`admin-init-endpoint`），登录态会自动跳到权重最小（weigh ASC）的第一个菜单作为落地页。当前 `admin_rule` 表里通常第一个菜单是「控制台 / Dashboard」，但项目里还没有 `web/src/views/admin/dashboard.vue` —— 登录后只会看到一个空白页或 404，体验断层。本变更提供一个极简的静态 Dashboard 落地页，先把路由指向的视图补齐，让登录闭环可演示；所有展示数据硬编码，不引入新的后端契约或 store。

## What Changes

- 新建 `web/src/views/admin/dashboard.vue`：一个 `<script setup lang="ts">` 的 Vue SFC，作为后台控制台的落地视图。
- 风格极简：
  - 顶部一行欢迎语 + 当前管理员昵称（从 `useAdminInfo` 读取，**仅这一个字段是动态的**，其余展示数据全部硬编码）。
  - 中部放 3 个 KPI 概览卡片（数值硬编码，例如「今日订单 / 用户总数 / 营收」），不画图表、不做时间序列、不做对比图。
  - 下方一段简短的「系统信息」说明（站点名 / 版本号，取自 `useConfig().siteConfig`；这是唯二从已有 store 读取的字段）。
- 不修改 router、store、API；不新增 echarts 图表组件、不引入 Element Plus 之外的依赖。
- 不引入后端变更、不新增 DB 表或迁移。

## Capabilities

### New Capabilities

- `admin-dashboard`：后台控制台视图的展示契约（页面布局、字段来源、硬编码数据集合）。

### Modified Capabilities

无。

## Impact

- 前端代码：
  - 新增文件 `web/src/views/admin/dashboard.vue`（单文件 SFC，约 100 行）。
- 既有依赖（已在 `package.json`）：
  - `vue`、`element-plus`、`@element-plus/icons-vue` —— 用于卡片布局。
  - 无新增依赖；本变更**不引入 echarts 图表**（用户明确要求极简、不要太多图表和统计）。
- 后端、数据库、配置：均无影响。
- 路由：未变更。`/admin/dashboard` 路由若不存在，由后续 `admin-init-endpoint` 实际落地的菜单规则（`admin_rule` 表中 weigh 最小的那条）指向本视图；本变更不负责注册路由。
