// web\src\api\group.ts
// RBAC 角色分组 API —— 示例数据实现（服务端 /admin/group/* 路由尚未实现）。
//
// 约定：函数签名与真实 API 形态一致（返回 Promise<ApiResponse<T>>，code=1 成功），
// 数据源为 web/src/mock/adminGroups.ts 的示例数据，内存内完成增删改查
// （刷新页面后恢复）。服务端接入后仅需把函数体替换为 request.request 调用。

import { exampleAdminGroups, exampleGroupRuleTree, type AdminGroup, type GroupRuleNode } from '/@/mock/adminGroups'

/** 分组 body（创建不传 id，编辑必传 id）。 */
export interface GroupBody {
    id?: number
    name?: string
    description?: string
    status?: 0 | 1
    rules?: number[]
}

/** 权限树响应：树形节点数组。 */
export type GroupRuleTreeResponse = GroupRuleNode[]

/** 模拟网络延迟（ms） */
const MOCK_DELAY = 200

function delay<T>(data: T): Promise<{ code: number; data: T; msg: string }> {
    return new Promise((resolve) => {
        setTimeout(() => resolve({ code: 1, data, msg: '' }), MOCK_DELAY)
    })
}

function now(): string {
    const d = new Date()
    const pad = (n: number) => String(n).padStart(2, '0')
    return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`
}

function nextId(): number {
    return exampleAdminGroups.reduce((max, g) => Math.max(max, g.id), 0) + 1
}

/**
 * 角色分组列表（无分页，分组数量级小）。
 */
export function groupList() {
    return delay<AdminGroup[]>(exampleAdminGroups.map((g) => ({ ...g, rules: [...g.rules] })))
}

/**
 * 取单个分组（待编辑）。
 */
export function groupGet(id: number) {
    const group = exampleAdminGroups.find((g) => g.id === id)
    return delay<AdminGroup>(group ? { ...group, rules: [...group.rules] } : ({} as AdminGroup))
}

/**
 * 创建角色分组。
 */
export function groupCreate(body: GroupBody) {
    const group: AdminGroup = {
        id: nextId(),
        name: body.name ?? '',
        description: body.description ?? '',
        status: body.status ?? 1,
        rules: body.rules ?? [],
        created_at: now(),
        updated_at: now(),
    }
    exampleAdminGroups.push(group)
    return delay<AdminGroup>({ ...group })
}

/**
 * 编辑角色分组（含保存权限树勾选结果）。
 */
export function groupEdit(body: GroupBody) {
    const idx = exampleAdminGroups.findIndex((g) => g.id === body.id)
    if (idx === -1) {
        return Promise.reject(new Error('group not found'))
    }
    const group = exampleAdminGroups[idx]
    if (body.name !== undefined) group.name = body.name
    if (body.description !== undefined) group.description = body.description
    if (body.status !== undefined) group.status = body.status
    if (body.rules !== undefined) group.rules = [...body.rules]
    group.updated_at = now()
    return delay<AdminGroup>({ ...group, rules: [...group.rules] })
}

/**
 * 切换启用状态（1↔0）。
 */
export function groupToggleStatus(id: number) {
    const group = exampleAdminGroups.find((g) => g.id === id)
    if (!group) {
        return Promise.reject(new Error('group not found'))
    }
    group.status = group.status === 1 ? 0 : 1
    group.updated_at = now()
    return delay<AdminGroup>({ ...group, rules: [...group.rules] })
}

/**
 * 删除角色分组。
 */
export function groupDelete(id: number) {
    const idx = exampleAdminGroups.findIndex((g) => g.id === id)
    if (idx > -1) {
        exampleAdminGroups.splice(idx, 1)
    }
    return delay<null>(null)
}

/**
 * 权限树（规则树形数据，供分组编辑弹窗的 el-tree 勾选）。
 */
export function groupRuleTree() {
    return delay<GroupRuleTreeResponse>(exampleGroupRuleTree)
}
