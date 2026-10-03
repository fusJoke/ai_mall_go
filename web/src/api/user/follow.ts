// src\api\user\follow.ts — C 端关注供应商
//
// 后端实现见 internal/handler/user/follow.go。
// 所有端点均需登录态：调用方在 header 显式传 Authorization（与 seckill.ts draw 同模式）。
import request from '/@/utils/request'
import { useUserInfo } from '/@/stores/user/userInfo'

export interface FollowingSupplier {
    id: number
    name: string
    logo: string
    bio: string
    created_at: string
    updated_at: string
    deleted_at: string | null
}

export interface FollowingListResponse {
    items: FollowingSupplier[]
    total: number
    page: number
    page_size: number
}

/** 关注 supplier（幂等 upsert；重复关注不报错）。 */
export function followSupplier(supplierId: number) {
    const userInfo = useUserInfo()
    return request.request({
        url: '/user/follow',
        method: 'POST',
        params: { supplier_id: supplierId },
        headers: { Authorization: `Bearer ${userInfo.token}` },
        __opts: { showErrorMessage: false },
    })
}

/** 取关 supplier（幂等）。 */
export function unfollowSupplier(supplierId: number) {
    const userInfo = useUserInfo()
    return request.request({
        url: '/user/follow',
        method: 'DELETE',
        params: { supplier_id: supplierId },
        headers: { Authorization: `Bearer ${userInfo.token}` },
        __opts: { showErrorMessage: false },
    })
}

/** 我的关注列表（分页；返回 supplier 白名单投影）。 */
export function followingList(params: { page?: number; page_size?: number }) {
    const userInfo = useUserInfo()
    return request.request<FollowingListResponse>({
        url: '/user/follow/following',
        method: 'GET',
        params,
        headers: { Authorization: `Bearer ${userInfo.token}` },
        __opts: { showErrorMessage: false },
    })
}