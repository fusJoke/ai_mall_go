// src\api\user\order.ts — C 端订单查询（列表 / 详情）
//
// 后端实现见 internal/handler/user/order.go；均需登录态。
import request from '/@/utils/request'
import { useUserInfo } from '/@/stores/user/userInfo'

export interface DrawOrder {
    id: number
    order_no: string
    user_id: number
    blind_box_id: number
    supplier_id: number
    price: number
    status: 'pending' | 'paid' | 'drawn' | 'failed'
    source: 'normal' | 'seckill'
    created_at: string
}

export interface DrawOrderItem {
    id: number
    order_id: number
    card_id: number
    rarity: string
    snapshot_name: string
    snapshot_image: string
    created_at: string
}

export interface OrderListResponse {
    items: DrawOrder[]
    total: number
    page: number
    page_size: number
}

export function orderList(params: { page?: number; page_size?: number }) {
    const userInfo = useUserInfo()
    return request.request<OrderListResponse>({
        url: '/user/orders/list',
        method: 'GET',
        params,
        headers: { Authorization: `Bearer ${userInfo.token}` },
    })
}

export interface OrderDetailResponse {
    order: DrawOrder
    items: DrawOrderItem[]
}

export function orderDetail(id: number) {
    const userInfo = useUserInfo()
    return request.request<OrderDetailResponse>({
        url: '/user/orders/detail',
        method: 'GET',
        params: { id },
        headers: { Authorization: `Bearer ${userInfo.token}` },
        __opts: { showErrorMessage: false },
    })
}
