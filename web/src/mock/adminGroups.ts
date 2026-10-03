// web\src\mock\adminGroups.ts
// RBAC 角色分组示例数据 —— 服务端 admin_group 表未实现前的前端兜底（示例数据约定）。
//
// - AdminGroup 字段对齐 buildadmin 的 admin_group 表：id / name / description /
//   status / rules（规则 id 集合，树形选择结果）/ created_at / updated_at；
// - GroupRuleNode 是权限树节点（与 mock/menus.ts 的 id 空间保持一致，便于
//   后续与真实 admin_rule 表对接：menuType node 为叶子权限点）。

export interface AdminGroup {
    id: number
    name: string
    description: string
    status: 0 | 1
    /** 拥有的规则 id 集合（含目录 / 菜单 / 权限节点） */
    rules: number[]
    created_at: string
    updated_at: string
}

export interface GroupRuleNode {
    id: number
    pid: number
    title: string
    type: 'dir' | 'menu' | 'node'
    children?: GroupRuleNode[]
}

/**
 * 权限树示例数据：目录 / 菜单 / 权限节点三级形态。
 * id 与 mock/menus.ts 的菜单 id 空间对齐（102 权限管理 / 108 常规管理...），
 * 权限节点 id 用 {菜单id}01.. 形式派生，避免与菜单 id 冲突。
 */
export const exampleGroupRuleTree: GroupRuleNode[] = [
    { id: 101, pid: 0, title: '后台主页', type: 'menu' },
    {
        id: 102,
        pid: 0,
        title: '权限管理',
        type: 'dir',
        children: [
            {
                id: 103,
                pid: 102,
                title: '管理员账号',
                type: 'menu',
                children: [
                    { id: 10301, pid: 103, title: '新增管理员', type: 'node' },
                    { id: 10302, pid: 103, title: '编辑管理员', type: 'node' },
                    { id: 10303, pid: 103, title: '删除管理员', type: 'node' },
                ],
            },
            {
                id: 104,
                pid: 102,
                title: '菜单规则',
                type: 'menu',
                children: [
                    { id: 10401, pid: 104, title: '新增规则', type: 'node' },
                    { id: 10402, pid: 104, title: '编辑规则', type: 'node' },
                    { id: 10403, pid: 104, title: '删除规则', type: 'node' },
                ],
            },
            {
                id: 106,
                pid: 102,
                title: '角色分组',
                type: 'menu',
                children: [
                    { id: 10601, pid: 106, title: '保存角色权限', type: 'node' },
                    { id: 10602, pid: 106, title: '删除角色', type: 'node' },
                ],
            },
            {
                id: 107,
                pid: 102,
                title: '管理员日志',
                type: 'menu',
                children: [{ id: 10701, pid: 107, title: '删除日志', type: 'node' }],
            },
        ],
    },
    {
        id: 108,
        pid: 0,
        title: '常规管理',
        type: 'dir',
        children: [
            { id: 109, pid: 108, title: '多驱动上传演示', type: 'menu' },
            { id: 110, pid: 108, title: 'iframe 示例', type: 'menu' },
        ],
    },
]

/** 全部规则 id 集合（超级管理员角色用） */
function allRuleIds(tree: GroupRuleNode[]): number[] {
    return tree.flatMap((node) => [node.id, ...(node.children ? allRuleIds(node.children) : [])])
}

export const exampleAdminGroups: AdminGroup[] = [
    {
        id: 1,
        name: '超级管理员',
        description: '拥有全部权限（示例数据：与 admin.super 直通权限等价）',
        status: 1,
        rules: allRuleIds(exampleGroupRuleTree),
        created_at: '2026-08-01 10:00:00',
        updated_at: '2026-08-01 10:00:00',
    },
    {
        id: 2,
        name: '内容编辑',
        description: '可查看后台与常规管理，不可触达权限管理',
        status: 1,
        rules: [101, 108, 109, 110],
        created_at: '2026-08-12 14:30:00',
        updated_at: '2026-09-20 09:15:00',
    },
    {
        id: 3,
        name: '审计员',
        description: '仅查看管理员日志',
        status: 1,
        rules: [101, 102, 107, 10701],
        created_at: '2026-09-01 16:45:00',
        updated_at: '2026-09-28 11:20:00',
    },
    {
        id: 4,
        name: '临时角色（停用）',
        description: '历史遗留角色，已停用',
        status: 0,
        rules: [101],
        created_at: '2026-07-20 08:00:00',
        updated_at: '2026-09-10 18:00:00',
    },
]
