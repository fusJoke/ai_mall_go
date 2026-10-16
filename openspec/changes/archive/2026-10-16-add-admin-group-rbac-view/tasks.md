# Tasks

## 1. 数据与 API 层

- [x] 1.1 新增 `web/src/mock/adminGroups.ts`：4 个示例角色分组（含全部规则的超管/受限的内容编辑与审计员/停用角色）+ 三级权限树示例数据（id 与示例菜单对齐、节点 id 派生）。
- [x] 1.2 新增 `web/src/api/group.ts`：groupList / groupGet / groupCreate / groupEdit / groupToggleStatus / groupDelete / groupRuleTree，Promise<ApiResponse<T>> 形态 + 深拷贝返回。

## 2. 页面与 i18n

- [x] 2.1 新增 `web/src/views/admin/group/index.vue`：`defineOptions({ name: 'adminGroup' })`；搜索过滤 + 表格（名称/描述/权限数/状态/更新时间）+ 行内操作（编辑/启停/删除）。
- [x] 2.2 新建/编辑弹窗：名称/描述/启停 + 权限树 el-tree（show-checkbox、default-expand-all、节点类型 tag）；回显只对叶子 setChecked（父节点级联推导），提交收集全选+半选并集；超管分组（id=1）名称/状态禁用、删除禁用。
- [x] 2.3 `web/src/lang/{zh-cn,en}/admin.yaml` 追加 `group.*` i18n key。

## 3. 菜单接入与验证

- [x] 3.1 `web/src/mock/menus.ts`「权限管理」目录下追加「角色分组」叶子（`views/admin/group/index.vue`）。
- [x] 3.2 `pnpm typecheck` 通过（仅既存 random.ts 遗留报错）；ESLint 零告警。
- [x] 3.3 GUI 黑盒：新建分组勾选权限树保存、编辑回显勾选准确（部分勾选呈半选态）、启停切换、删除可用；超管保护生效。
- [x] 3.4 `openspec validate add-admin-group-rbac-view` 通过；勾选本文件所有任务。
