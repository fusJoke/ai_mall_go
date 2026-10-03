# admin-group Specification

## Purpose

角色分组（RBAC）管理页：维护 admin_group 记录及其规则授权集合（rules）。页面纯前端实现，数据层当前为示例数据（API 契约与服务端 `/admin/group/*` 未来实现对齐），权限树当前为示例规则树（id 与示例菜单对齐）。

## ADDED Requirements

### Requirement: 分组列表与检索

角色分组页 SHALL 提供分组列表（名称/描述/权限数/状态/更新时间列）与关键字过滤（匹配名称/描述）。数据 SHALL 经 `groupList` API 获取，当前为示例数据实现。

#### Scenario: 名称过滤

- **WHEN** 用户在搜索框输入 "审计"
- **THEN** 列表仅显示名称含 "审计" 的分组

### Requirement: 分组增删改与启停

页面 SHALL 支持：新建分组（名称必填）、编辑分组（含保存权限勾选结果）、启停切换（1↔0）、删除（二次确认）。超管分组（id=1）SHALL 被保护：不可删除、名称与状态不可修改。

#### Scenario: 新建分组

- **WHEN** 用户填写名称、勾选权限树部分节点并保存
- **THEN** 列表新增该分组，其 rules 为勾选叶子与半选父节点的 id 并集

#### Scenario: 删除保护

- **WHEN** 用户查看超级管理员分组的操作列
- **THEN** 删除按钮为禁用态

### Requirement: 权限树勾选与回显

分组编辑弹窗 SHALL 以 el-tree 展示规则树（目录/菜单/权限节点三级，节点类型可见），支持勾选授权。回显时 SHALL 只对叶子节点 setCheckedKeys（父节点勾选/半选态由级联推导，避免父节点误勾全部子孙）；保存时 SHALL 收集全选与半选节点的并集作为 rules。

#### Scenario: 部分勾选回显为半选

- **WHEN** 打开 rules 仅含某目录下部分子节点的分组编辑弹窗
- **THEN** 该目录呈半选态，已含的子节点为勾选态，未含的为未选态

#### Scenario: 全选子节点勾选父节点

- **WHEN** 用户勾选某目录下全部子节点并保存
- **THEN** 该目录 id 进入 rules，再次打开时目录呈全选态
