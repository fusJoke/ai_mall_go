# Proposal

## Why

RBAC 的数据底座已在服务端就绪（`admin-permission` capability：admin_group / admin_group_access / admin_rule 三张表 + 权限查询接口），但**角色分组本身没有任何管理入口**：新建分组、调整分组权限（勾选规则树）、启停分组都只能改库。参考 buildadmin（`views/admin/group` + `api/group`）先落前端管理页，数据层以示例数据兜底（服务端 `/admin/group/*` 路由未实现），待后端落地后替换 API 实现即可。

## What Changes

- **新增 `web/src/mock/adminGroups.ts`**：4 个示例角色分组（超级管理员/内容编辑/审计员/停用角色）+ 权限树示例数据（目录/菜单/权限节点三级，id 空间与 `mock/menus.ts` 的菜单 id 对齐，节点 id 以 `{菜单id}01..` 派生避免冲突）。
- **新增 `web/src/api/group.ts`**：`groupList` / `groupGet` / `groupCreate` / `groupEdit` / `groupToggleStatus` / `groupDelete` / `groupRuleTree` 七个函数，签名与真实 API 形态一致（Promise<ApiResponse<T>>），当前为内存示例数据实现。
- **新增 `web/src/views/admin/group/index.vue`**：角色分组管理页 —— 搜索过滤、表格（名称/描述/权限数/状态/更新时间）、新建/编辑弹窗（名称/描述/启停 + **RBAC 权限树 el-tree 勾选**）、启停切换、删除（超管分组保护：id=1 不可删、名称与状态不可改）。
- **i18n**：`web/src/lang/{zh-cn,en}/admin.yaml` 追加 `group.*` key。
- **菜单接入**：`web/src/mock/menus.ts` 在「权限管理」目录下追加「角色分组」叶子。

### 非目标

- **不实现服务端**：`/admin/group/*` CRUD 路由、分组与管理员绑定关系的管理不在本次范围。
- **不做管理员-分组绑定管理**：admin_group_access 的分配界面留待后续 change。
- **权限树数据不接真实 admin_rule**：当前用示例树（id 与示例菜单对齐）；服务端接入后由 `groupRuleTree` 换真实数据。

## Capabilities

### New Capabilities

- `admin-group`: 角色分组管理页 —— 分组 CRUD + 启停 + 权限树勾选（当前为前端示例数据实现，API 契约与服务端对齐）。

### Modified Capabilities

无。`admin-permission`（服务端数据底座）的需求不变；`admin-layout` 的示例菜单数据新增一个叶子（属 add-admin-layout-shell 的示例数据扩充）。

## Impact

- **代码**：新增 3 个前端文件 + 2 个 i18n 文件追加 + 1 处示例菜单扩充；无后端改动。
- **规格**：新增 `admin-group` capability spec。
- **验证**：`pnpm typecheck`（仅既存 random.ts 遗留报错）；ESLint 零告警；GUI 黑盒——新建分组勾选权限树保存、编辑回显勾选、启停、删除、超管保护均可用。
