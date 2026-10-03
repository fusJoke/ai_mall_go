# Design

## Context

服务端已有 admin_group 表与权限查询底座（`admin-permission` capability），但分组 CRUD 路由未实现。buildadmin 的角色分组页（`views/admin/group` + `api/group`）是成熟参考：编辑弹窗内嵌规则树勾选。前端先以示例数据落地，API 契约对齐。

## Goals / Non-Goals

- Goals：分组增删改查 + 启停 + 权限树勾选/回显交互完整；超管分组保护；示例数据确定性。
- Non-Goals：服务端实现；管理员-分组绑定管理；真实 admin_rule 树接入。

## Decisions

### D1. 权限树回显只对叶子 setCheckedKeys

分组 `rules` 存的是「全选 + 半选」父节点与叶子的并集（与 buildadmin 一致）。回显时若对含子节点的目录 id 直接 `setCheckedKeys`，el-tree 级联会误勾其全部子孙。方案：先收集树内全部父节点 id，把 `rules` 过滤为叶子集合后 `setCheckedKeys(leafIds, false)`，父节点勾选/半选态由级联自动推导。提交时反向收集 `getCheckedKeys() + getHalfCheckedKeys()` 并集存回。

### D2. 权限树 id 空间与示例菜单对齐、节点 id 派生

目录/菜单节点直接复用 `mock/menus.ts` 的 id（101/102/103/104/106/107/108/109/110），权限节点以 `{菜单id}01..` 派生（如 10301 = 管理员账号下的新增权限）——两套数据天然对齐，未来接真实 admin_rule 时 id 语义一致。

### D3. 超管分组保护在前端表达

id=1 的「超级管理员」分组：删除按钮禁用、名称与状态字段禁用（权限树仍可看）；对应服务端未来应由权限中间件兜底，前端保护仅是交互层约定（示例数据约定下足够）。

### D4. API mock 宿主与 adminLog 同模式

`api/group.ts` 内存 CRUD + 深拷贝返回（避免调用方改到 mock 源），`groupRuleTree` 返回示例树；服务端接入后仅换函数体。

## Risks / Trade-offs

- 提交收集半选节点使 rules 含父节点 id：与 buildadmin 存储形态一致，回显算法（D1）已配套。
- 状态开关无二次确认：低风险操作且表格即时反馈，与 buildadmin 一致。

## Migration Plan

纯前端新增；无 schema / 后端面。

## Open Questions

无。
