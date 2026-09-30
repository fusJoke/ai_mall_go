import type { ClickRequest } from '/@/components/clickCaptcha/index'
import request from '/@/utils/request'
import type {
    InitResponse,
    LoginRequest,
    LoginResponse,
    LogoutResponse,
} from '/@/stores/interface'

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