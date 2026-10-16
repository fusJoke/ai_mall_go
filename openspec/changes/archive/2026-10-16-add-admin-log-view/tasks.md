# Tasks

## 1. 数据与 API 层

- [x] 1.1 新增 `web/src/mock/adminLogs.ts`：66 条确定性示例日志（固定起点 2026-09-28、3 小时步进、操作/管理员/IP/UA 轮换表），字段对齐 admin_log 表。
- [x] 1.2 新增 `web/src/api/adminLog.ts`：`adminLogList`（分页 + keyword 模糊匹配 username/title/url/ip）/ `adminLogDelete` / `adminLogBatchDelete`，Promise<ApiResponse<T>> 形态 + 200ms 模拟延迟。

## 2. 页面与 i18n

- [x] 2.1 新增 `web/src/views/admin/log/index.vue`：`defineOptions({ name: 'adminLog' })`；搜索框 + 多选表格（id/username/title/url/ip/createtime，title 语义色 tag）+ 行内操作（详情/删除）+ 批量删除（二次确认）+ el-pagination（20/50/100/200）+ 详情弹窗（el-descriptions 全字段含 User-Agent）。
- [x] 2.2 `web/src/lang/{zh-cn,en}/admin.yaml` 追加 `log.*` i18n key（columns/action/dialog/confirm/searchPlaceholder）。

## 3. 菜单接入与验证

- [x] 3.1 `web/src/mock/menus.ts`「权限管理」目录下追加「管理员日志」叶子（`views/admin/log/index.vue`）。
- [x] 3.2 `pnpm typecheck` 通过（仅既存 random.ts 遗留报错）；ESLint 零告警。
- [x] 3.3 GUI 黑盒：列表加载、关键字搜索过滤、分页切换、详情弹窗、删除/批量删除可用；刷新后示例数据恢复。
- [x] 3.4 `openspec validate add-admin-log-view` 通过；勾选本文件所有任务。
