# admin-dashboard Specification

## Purpose
Provides the static landing view at `/admin/dashboard` that admin users see right after login, so the admin shell has a real destination before any data-driven dashboard work lands. The view is intentionally minimal — a greeting, a handful of hardcoded KPI tiles, and a short system-info footer — and reads only the admin nickname plus the site name/version already loaded by the admin init flow.

## Requirements

### Requirement: Static dashboard landing view

The system SHALL render a static dashboard view at `web/src/views/admin/dashboard.vue` whose layout consists of, in order from top to bottom: (1) a greeting line, (2) a row of three KPI tiles, and (3) a short system-info footer.

The greeting line SHALL display the text 「欢迎回来，{nickname}」 where `{nickname}` is the currently logged-in admin's nickname read from `useAdminInfo().nickname`. When `nickname` is empty, the line SHALL fall back to the username.

The three KPI tiles SHALL each show: an icon (lucide), a label, and a numeric value. The numeric values SHALL be hardcoded constants in the component (not fetched, not derived from the store, not animated). The three tiles' default labels and values SHALL be:

| label | value |
| --- | --- |
| 今日订单 | 128 |
| 用户总数 | 3,562 |
| 营收（元） | 86,420 |

The system-info footer SHALL display the site name and version read from `useConfig().siteConfig` (`name` and `version`). If either field is empty, the corresponding line SHALL be omitted (no placeholder text).

The view SHALL NOT contain any chart, graph, table, timeline, or comparison visualization. The view SHALL NOT trigger any HTTP request on mount or render.

#### Scenario: Admin lands on dashboard with nickname and site config populated
- **WHEN** an authenticated admin opens `/admin/dashboard` with `useAdminInfo().nickname = "Alice"` and `useConfig().siteConfig = { name: "AI Mall", version: "v1.0.0", record_number: "京ICP-1" }`
- **THEN** the greeting line reads 「欢迎回来，Alice」
- **AND** the three KPI tiles render with the hardcoded values 128 / 3,562 / 86,420
- **AND** the footer shows two lines: site name "AI Mall" and version "v1.0.0"
- **AND** no network request is issued by this view

#### Scenario: Admin lands on dashboard with empty nickname
- **WHEN** an authenticated admin opens `/admin/dashboard` with `useAdminInfo().nickname = ""` and `useAdminInfo().username = "root"`
- **THEN** the greeting line reads 「欢迎回来，root」 (falls back to username)

#### Scenario: Admin lands on dashboard with missing site config fields
- **WHEN** an authenticated admin opens `/admin/dashboard` with `useConfig().siteConfig = { name: "", version: "v1.0.0", record_number: "" }`
- **THEN** the footer renders only the version line "v1.0.0" and skips the empty site-name line

#### Scenario: Component contains no chart code
- **WHEN** the dashboard source is inspected
- **THEN** it does not import `echarts`, `@element-plus/charts`, or any chart/component module
- **AND** it does not import `vue-echarts` or call `chart.setOption`
- **AND** the rendered DOM contains no `<canvas>` or `<svg>` chart element

### Requirement: Composition API and TypeScript

The dashboard view SHALL be written as a Vue 3 Single File Component using `<script setup lang="ts">`. All KPI values, labels, and icon names SHALL be declared as typed constants in the script block.

#### Scenario: Script block uses setup + ts
- **WHEN** the dashboard source is opened
- **THEN** the `<script>` tag reads `<script setup lang="ts">`
- **AND** the KPI labels and values are declared as `const` bindings with explicit string/number types

### Requirement: No new dependencies and no API/store mutations

The dashboard view SHALL NOT add any dependency to `web/package.json`, SHALL NOT mutate any Pinia store, and SHALL NOT register any new API endpoint. It MAY read from existing stores (`useAdminInfo`, `useConfig`) but SHALL NOT trigger writes.

#### Scenario: Source review for side effects
- **WHEN** the dashboard source is reviewed
- **THEN** `package.json` is unchanged by this change
- **AND** no call to a Pinia `$patch`, `setXxx`, or action that mutates state is present
- **AND** no `request(...)` / `axios(...)` / `fetch(...)` call is present in the component
