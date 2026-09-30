import type { ClickRequest } from '/@/components/clickCaptcha/index'
import request from '/@/utils/request'
import type { InitResponse, LoginRequest, LoginResponse, LogoutResponse } from '/@/stores/interface'

/**
 * 获取点选验证码
 */
export function getClickCaptcha(apiBaseURL?: string) {
    return request.request({
        url: '/common/captcha/create',
        method: 'GET',
        ...(apiBaseURL ? { baseURL: apiBaseURL } : {}),
    })
}

/**
 * 校验点选验证码
 */
export function checkClickCaptcha(data: ClickRequest, apiBaseURL?: string) {
    return request.request({
        url: '/common/captcha/verify',
        method: 'POST',
        data,
        __opts: { showErrorMessage: false },
        ...(apiBaseURL ? { baseURL: apiBaseURL } : {}),
    })
}

/**
 * 缓存清理接口
 */
export function clearCache(type: string) {
    return request.request({
        url: '/admin/clear-cache',
        method: 'POST',
        data: { type },
        __opts: { showSuccessMessage: true },
    })
}

/**
 * 管理员登录
 *
 * 调用方拿到的 res.data 是后端 LoginResponse（响应拦截器已自动解包 code/message/data 信封）。
 *
 * 业务错误（code != 0）不弹全局 toast，由调用方根据 err.message 用 setError 自行显示。
 */
export function login(data: LoginRequest) {
    return request.request<LoginResponse>({
        url: '/admin/login',
        method: 'POST',
        data,
        __opts: { showErrorMessage: false },
    })
}

/**
 * 管理员登出
 *
 * 后端实现见 internal/handler/admin/admin.go Logout。语义幂等 —— 不论是否登录、
 * token 是否有效，调用都返回 200。失败也用 `showErrorMessage: false` 静默处理：
 * 调用方拿到 res 后仍执行 adminInfo.reset() 做本地清理，避免 token 残留。
 */
export function logout() {
    return request.request<LogoutResponse>({
        url: '/admin/logout',
        method: 'POST',
        __opts: { showErrorMessage: false },
    })
}

/**
 * 后台初始化 —— 登录后调一次，聚合当前管理员信息 / 站点配置 / 权限菜单。
 *
 * 失败处理由调用方（layouts/admin/index.vue 的 init 流程）决定 —— 这里用
 * `showErrorMessage: false` 静默处理，把错误抛给调用方走专门的跳转逻辑：
 *   - 401：清空 adminInfo 并跳 /admin/login
 *   - 403：跳 /admin/login（admin 被禁用）
 *   - 5xx：跳 /admin/login 并提示
 *
 * 后端实现见 internal/handler/admin/init.go Init；规格见
 * openspec/changes/admin-init-endpoint/specs/admin-init/spec.md。
 */
export function init() {
    return request.request<InitResponse>({
        url: '/admin/init',
        method: 'GET',
        __opts: { showErrorMessage: false },
    })
}

// ============================================================================
// 管理员管理页 API —— openspec/changes/add-admin-management/specs/admin-management/spec.md
// ============================================================================

/** 与后端 adminInfo 一致：不含 password 字段。 */
export interface AdminInfo {
    id: number
    username: string
    nickname: string
    avatar: string
    email?: string
    mobile?: string
    last_login_at?: string
    last_login_ip?: string
    bio: string
    status: 0 | 1
}

/** 列表响应：items + total + 当前分页元信息。 */
export interface AdminListResponse {
    items: AdminInfo[]
    total: number
    page: number
    page_size: number
}

/** 列表 / 搜索参数。page=1, page_size=20 为后端默认值；上限 page_size=200。 */
export interface AdminListParams {
    page?: number
    page_size?: number
    username?: string
    nickname?: string
}

/** 通用 create / edit body：password 在编辑时可省略（后端视为"不改"）。 */
export interface AdminBody {
    id?: number
    username?: string
    nickname?: string
    email?: string
    mobile?: string
    avatar?: string
    password?: string
    bio?: string
    status?: 0 | 1
}

/** 改密 body。 */
export interface AdminChangePasswordBody {
    id: number
    new_password: string
}

/** 启停 / 解锁 body。 */
export interface AdminToggleStatusBody {
    id: number
}

/** 批量删除 body。 */
export interface AdminBatchDeleteBody {
    ids: number[]
}

/** 批量删除响应。 */
export interface AdminBatchDeleteResponse {
    deleted: number
    skipped_self: number
}

/**
 * 列表 / 搜索。
 *
 * 后端实现见 internal/handler/admin/admin.go List。
 */
export function adminList(params: AdminListParams = {}) {
    return request.request<AdminListResponse>({
        url: '/admin/admin/list',
        method: 'GET',
        params,
    })
}

/**
 * 取单行（待编辑）。
 *
 * 后端实现见 internal/handler/admin/admin.go EditGet。
 */
export function adminGet(id: number) {
    return request.request<AdminInfo>({
        url: '/admin/admin/edit',
        method: 'GET',
        params: { id },
    })
}

/** 创建管理员。 */
export function adminCreate(body: AdminBody) {
    return request.request<AdminInfo>({
        url: '/admin/admin/create',
        method: 'POST',
        data: body,
    })
}

/** 编辑管理员。body 必须含 id；password 字段空表示不改。 */
export function adminEdit(body: AdminBody) {
    return request.request<AdminInfo>({
        url: '/admin/admin/edit',
        method: 'POST',
        data: body,
    })
}

/** 删除管理员。后端已做 self-protection（self → 403）。 */
export function adminDelete(id: number) {
    return request.request({
        url: '/admin/admin/delete',
        method: 'POST',
        params: { id },
    })
}

/** 重置密码。成功后被改密 admin 的 token 会被吊销。 */
export function adminChangePassword(body: AdminChangePasswordBody) {
    return request.request({
        url: '/admin/admin/change-password',
        method: 'POST',
        data: body,
    })
}

/** 切换状态。1→0 吊销 token，0→1 不吊销；self → 403。 */
export function adminToggleStatus(body: AdminToggleStatusBody) {
    return request.request({
        url: '/admin/admin/toggle-status',
        method: 'POST',
        data: body,
    })
}

/** 解锁：重置 LoginFailure=0 + Status=1，不动密码 / 不清 token。 */
export function adminUnlock(body: AdminToggleStatusBody) {
    return request.request({
        url: '/admin/admin/unlock',
        method: 'POST',
        data: body,
    })
}

/** 批量删除：self 自动剔除，响应里 skipped_self 反映剔除数。 */
export function adminBatchDelete(body: AdminBatchDeleteBody) {
    return request.request<AdminBatchDeleteResponse>({
        url: '/admin/admin/batch-delete',
        method: 'POST',
        data: body,
    })
}
