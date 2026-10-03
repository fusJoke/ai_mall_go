// web\src\api\adminLog.ts
// 管理员日志 API —— 示例数据实现（服务端 /admin/admin-log/* 路由尚未实现）。
//
// 约定：函数签名与真实 API 形态一致（返回 Promise<ApiResponse<T>>，code=1 成功），
// 数据源为 web/src/mock/adminLogs.ts 的确定性示例数据，内存内完成分页 / 搜索 /
// 删除（刷新页面后恢复）。服务端接入后仅需把函数体替换为 request.request 调用。

import { exampleAdminLogs, type AdminLog } from '/@/mock/adminLogs'

export type { AdminLog }

/** 列表 / 搜索参数。 */
export interface AdminLogListParams {
    page?: number
    page_size?: number
    /** 关键字：匹配 username / title / url / ip（模糊） */
    keyword?: string
}

/** 列表响应：items + total + 分页元信息（与 BaseHandler.List 形态对齐）。 */
export interface AdminLogListResponse {
    items: AdminLog[]
    total: number
    page: number
    page_size: number
}

/** 批量删除响应。 */
export interface AdminLogBatchDeleteResponse {
    deleted: number
}

/** 模拟网络延迟（ms） */
const MOCK_DELAY = 200

function delay<T>(data: T): Promise<{ code: number; data: T; msg: string }> {
    return new Promise((resolve) => {
        setTimeout(() => resolve({ code: 1, data, msg: '' }), MOCK_DELAY)
    })
}

/**
 * 日志列表（分页 + 关键字过滤）。
 */
export function adminLogList(params: AdminLogListParams = {}) {
    const page = params.page ?? 1
    const pageSize = params.page_size ?? 20
    const keyword = (params.keyword ?? '').trim().toLowerCase()

    let items = exampleAdminLogs
    if (keyword) {
        items = items.filter(
            (log) =>
                log.username.toLowerCase().includes(keyword) ||
                log.title.toLowerCase().includes(keyword) ||
                log.url.toLowerCase().includes(keyword) ||
                log.ip.includes(keyword)
        )
    }

    const total = items.length
    const start = (page - 1) * pageSize
    return delay<AdminLogListResponse>({
        items: items.slice(start, start + pageSize),
        total,
        page,
        page_size: pageSize,
    })
}

/**
 * 删除单条日志（内存删除）。
 */
export function adminLogDelete(id: number) {
    const idx = exampleAdminLogs.findIndex((log) => log.id === id)
    if (idx > -1) {
        exampleAdminLogs.splice(idx, 1)
    }
    return delay<null>(null)
}

/**
 * 批量删除日志（内存删除）。
 */
export function adminLogBatchDelete(ids: number[]) {
    let deleted = 0
    for (const id of ids) {
        const idx = exampleAdminLogs.findIndex((log) => log.id === id)
        if (idx > -1) {
            exampleAdminLogs.splice(idx, 1)
            deleted++
        }
    }
    return delay<AdminLogBatchDeleteResponse>({ deleted })
}
