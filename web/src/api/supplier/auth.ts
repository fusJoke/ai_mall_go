// src\api\supplier\auth.ts — B 端供应商登录 / 登出
//
// 后端实现见 internal/handler/supplier/auth.go。
import request from '/@/utils/request'

export interface SupplierLoginResponse {
    user: {
        id: number
        supplier_id: number
        username: string
        status: number
        last_login_at?: string
        last_login_ip?: string
    }
    token: string
}

export function supplierLogin(data: {
    username: string
    password: string
    captcha_key: string
    points: { x: number; y: number }[]
    remember?: boolean
}) {
    return request.request<SupplierLoginResponse>({
        url: '/supplier/login',
        method: 'POST',
        data,
        __opts: { showErrorMessage: false },
    })
}

/** 幂等登出。 */
export function supplierLogout(token: string) {
    return request.request({
        url: '/supplier/logout',
        method: 'POST',
        headers: token ? { Authorization: `Bearer ${token}` } : {},
        __opts: { showErrorMessage: false },
    })
}
