import request from '/@/utils/request'

/**
 * 菜单规则 API —— 见 openspec/changes/add-admin-rule-management/specs/admin-rule-management/spec.md
 *
 * 8 个函数对应后端 8 条路由（5 通用 CRUD + 3 专属），类型签名与
 * internal/model/admin.go 的 AdminRule 字段对齐（含 JSON 蛇形 tag）。
 */

// ============================================================================
// 类型定义
// ============================================================================

/** 规则类型枚举（与 model.AdminRuleType 对齐）。 */
export type AdminRuleType = 'dir' | 'menu' | 'node'

/** 菜单打开方式枚举（与 model.AdminRuleOpenType 对齐；可为 null）。 */
export type AdminRuleOpenType = 'tab' | 'link' | 'iframe' | null

/** 扩展属性枚举（与 model.AdminRuleExtend 对齐）。 */
export type AdminRuleExtend = 'none' | 'add_route_only' | 'add_menu_only'

/** 单条菜单规则。与后端 internal/model/admin.go:AdminRule 对齐，字段顺序保持一致。 */
export interface AdminRule {
    id: number
    pid: number
    type: AdminRuleType
    title: string
    name: string
    path: string
    icon: string
    open_type: AdminRuleOpenType
    url: string
    component: string
    keepalive: boolean
    extend: AdminRuleExtend
    remark: string
    weigh: number
    status: 0 | 1
    created_at: string
    updated_at: string
}

/** 列表响应：items + total + 分页元信息（与 BaseHandler.List 一致）。 */
export interface RuleListResponse {
    items: AdminRule[]
    total: number
    page: number
    page_size: number
}

/** 列表 / 搜索参数。page=1, page_size=20 为后端默认值；上限 page_size=200。 */
export interface RuleListParams {
    page?: number
    page_size?: number
}

/**
 * 创建 / 编辑 body。id 在编辑时必填；创建时不传或传 0。
 * 创建时建议填齐 type / title / name / pid 四项，pid=0 表示顶级。
 */
export interface RuleBody {
    id?: number
    pid?: number
    type?: AdminRuleType
    title?: string
    name?: string
    path?: string
    icon?: string
    open_type?: AdminRuleOpenType
    url?: string
    component?: string
    keepalive?: boolean
    extend?: AdminRuleExtend
    remark?: string
    weigh?: number
    status?: 0 | 1
}

/** 启停 body。 */
export interface RuleToggleStatusBody {
    id: number
}

/** 批量删除 body。 */
export interface RuleBatchDeleteBody {
    ids: number[]
}

/** 批量删除响应：deleted 实删数，skipped 跳过数（含「有子」「不存在」）。 */
export interface RuleBatchDeleteResponse {
    deleted: number
    skipped: number
}

// ============================================================================
// API 函数
// ============================================================================

/**
 * 列表 / 分页。
 *
 * 后端实现见 internal/handler/admin/rule.go（继承 BaseHandler.List）。
 */
export function ruleList(params: RuleListParams = {}) {
    return request.request<RuleListResponse>({
        url: '/admin/rule/list',
        method: 'GET',
        params,
    })
}

/**
 * 取单行（待编辑）。
 *
 * 后端实现见 internal/handler/admin/rule.go（继承 BaseHandler.EditGet）。
 */
export function ruleGet(id: number) {
    return request.request<AdminRule>({
        url: '/admin/rule/edit',
        method: 'GET',
        params: { id },
    })
}

/**
 * 创建菜单规则。
 *
 * 后端 service.Create 已做 PID 校验：
 *   - 42201 pid_cycle        → 不能将规则设为自身或子孙的子节点
 *   - 42202 pid_not_found    → 上级规则不存在或已软删
 *   - 42203 has_children     → 删除路径
 * 错误码后端走 rule.create.{pid_cycle,pid_not_found}。
 */
export function ruleCreate(body: RuleBody) {
    return request.request<AdminRule>({
        url: '/admin/rule/create',
        method: 'POST',
        data: body,
    })
}

/**
 * 编辑菜单规则。body 必须含 id。
 *
 * 错误映射同 ruleCreate：rule.edit.{pid_cycle,pid_not_found}。
 */
export function ruleEdit(body: RuleBody) {
    return request.request<AdminRule>({
        url: '/admin/rule/edit',
        method: 'POST',
        data: body,
    })
}

/**
 * 删除菜单规则。有子节点时拒绝（422 rule.delete.has_children）。
 *
 * 后端实现见 internal/handler/admin/rule.go（继承 BaseHandler.Delete）。
 */
export function ruleDelete(id: number) {
    return request.request({
        url: '/admin/rule/delete',
        method: 'POST',
        params: { id },
    })
}

/**
 * 切换启用状态。1↔0；不联动 token（规则不绑会话）。
 *
 * 错误映射：rule.toggle_status.not_found → 404。
 */
export function ruleToggleStatus(body: RuleToggleStatusBody) {
    return request.request({
        url: '/admin/rule/toggle-status',
        method: 'POST',
        data: body,
    })
}

/**
 * 批量删除。返回 deleted / skipped 两类计数（带跳过原因的有子 / 不存在）。
 */
export function ruleBatchDelete(body: RuleBatchDeleteBody) {
    return request.request<RuleBatchDeleteResponse>({
        url: '/admin/rule/batch-delete',
        method: 'POST',
        data: body,
    })
}

/**
 * 列出全部启用规则（无分页）。用于管理页「上级规则」下拉框。
 *
 * 后端实现见 internal/handler/admin/rule.go ListAll；按 weigh ASC, id ASC 排序。
 */
export function ruleListAll() {
    return request.request<AdminRule[]>({
        url: '/admin/rule/all',
        method: 'GET',
    })
}
