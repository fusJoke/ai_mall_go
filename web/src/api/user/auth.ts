// src\api\user\auth.ts — C 端会员登录 / 登出
//
// 后端实现见 internal/handler/user/auth.go。
// 响应拦截器自动解包 {code,message,data} 信封（本项目后端返回裸 JSON，
// 拦截器对非信封响应原样放行，见 utils/request.ts isApiResponse）。
import request from '/@/utils/request'

export interface UserLoginResponse {
    user: {
        id: number
        username: string
        nickname: string
        avatar: string
        email?: string
        mobile?: string
        balance: string
        last_login_at?: string
        last_login_ip?: string
        status: number
    }
    token: string
}

export function userLogin(data: {
    username: string
    password: string
    captcha_key: string
    points: { x: number; y: number }[]
    remember?: boolean
}) {
    return request.request<UserLoginResponse>({
        url: '/user/login',
        method: 'POST',
        data,
        __opts: { showErrorMessage: false },
    })
}

/** 幂等登出：缺失 / 无效 token 都返回 200。 */
export function userLogout(token: string) {
    return request.request({
        url: '/user/logout',
        method: 'POST',
        headers: token ? { Authorization: `Bearer ${token}` } : {},
        __opts: { showErrorMessage: false },
    })
}
