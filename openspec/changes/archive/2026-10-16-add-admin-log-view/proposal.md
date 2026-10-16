# Proposal

## Why

后台管理员日志（谁在什么时间从哪个 IP 做了什么操作）目前完全没有查看入口：服务端 `admin_log` 表与 `/admin/admin-log/*` 路由均未实现，但运营侧已经需要日志检索来排查误操作与审计登录行为。参考 buildadmin（`views/admin/log` + `api/adminLog`）先落前端管理页，数据层以示例数据兜底，待服务端表落地后替换 API 实现即可，页面与交互无需返工。

## What Changes

- **新增 `web/src/mock/adminLogs.ts`**：管理员日志示例数据 —— 66 条确定性生成的记录（固定步进时间、轮换管理员/操作/IP/UA），字段对齐 buildadmin 的 `admin_log` 表（id/admin_id/username/title/url/ip/useragent/createtime）。
- **新增 `web/src/api/adminLog.ts`**：`adminLogList`（分页 + keyword 模糊匹配 username/title/url/ip）/ `adminLogDelete` / `adminLogBatchDelete` 三个函数，签名与真实 API 形态一致（Promise<ApiResponse<T>>，code=1 成功），当前为内存示例数据实现（刷新后恢复）。
- **新增 `web/src/views/admin/log/index.vue`**：列表页 —— 搜索框、多选表格（id/管理员/操作标题/请求路径/IP/操作时间）、操作标题语义色 tag、详情弹窗（el-descriptions 全字段含 User-Agent）、删除与批量删除（二次确认）、分页（20/50/100/200）。
- **i18n**：`web/src/lang/{zh-cn,en}/admin.yaml` 追加 `log.*` key。
- **菜单接入**：`web/src/mock/menus.ts` 在「权限管理」目录下追加「管理员日志」叶子（component 指向新页面）。

### 非目标

- **不实现服务端**：`admin_log` 表、请求日志落库中间件、`/admin/admin-log/*` 路由全部不在本次范围（后续 Go 侧 change 处理）。
- **不做日志导出**：不提供 CSV/Excel 导出。
- **不做时间范围筛选**：本期仅关键字搜索 + 分页。

## Capabilities

### New Capabilities

- `admin-log`: 管理员日志查看页 —— 列表/搜索/分页/详情/删除（当前为前端示例数据实现，API 契约与服务端对齐）。

### Modified Capabilities

无。`admin-layout` 的示例菜单数据新增一个叶子（属 add-admin-layout-shell 的示例数据扩充，不改变布局行为）。

## Impact

- **代码**：新增 3 个前端文件 + 2 个 i18n 文件追加 + 1 处示例菜单扩充；无后端改动、无路由注册改动（菜单路由由 init 流程动态注册）。
- **规格**：新增 `admin-log` capability spec。
- **验证**：`pnpm typecheck`（仅既存 random.ts 遗留报错）；ESLint 零告警；GUI 黑盒——登录后从「权限管理/管理员日志」进入，列表加载、搜索过滤、分页切换、详情弹窗、删除/批量删除均可用。
