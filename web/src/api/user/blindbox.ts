// src\api\user\blindbox.ts — C 端盲盒浏览（列表 / 详情 / 抽卡）
//
// 后端实现见 internal/handler/user/blindbox.go 与 draw.go。
// 需要登录态的接口（draw）由调用方传入 token，显式挂 Authorization 头 ——
// 全局请求拦截器只携带 adminInfo.token（管理端），会员 token 独立管理。
import request from '/@/utils/request'
import { useUserInfo } from '/@/stores/user/userInfo'

export interface BlindBox {
    id: number
    supplier_id: number
    name: string
    cover: string
    price: number
    status: string
    on_sale: boolean
    is_featured: boolean
    description: string
    created_at: string
}

export interface BlindBoxListResponse {
    items: BlindBox[]
    total: number
    page: number
    page_size: number
}

/** 卡池条目（详情页概率公示用）。 */
export interface PoolItem {
    id: number
    card_id: number
    rarity: 'SSR' | 'SR' | 'R' | 'N'
    weight: number
    stock: number
}

export interface BlindBoxDetail {
    blind_box: BlindBox
    supplier: {
        id: number
        name: string
        logo: string
        bio: string
    }
    pool: { id: number }
    items: PoolItem[]
    pool_total_stock: number
    active_promotion?: {
        id: number
        original_price: number
        promo_price: number
        start_at: string
        end_at: string
    } | null
    effective_price: number
    cached_at: string
}

export function blindBoxList(params: { page?: number; page_size?: number }) {
    return request.request<BlindBoxListResponse>({
        url: '/user/blindbox/list',
        method: 'GET',
        params,
    })
}

export function blindBoxDetail(id: number) {
    return request.request<BlindBoxDetail>({
        url: '/user/blindbox/detail',
        method: 'GET',
        params: { id },
    })
}

export interface DrawResult {
    order_id: number
    order_no: string
    actual_price: number
    cards: {
        item_id: number
        card_id: number
        rarity: string
        snapshot_name: string
        snapshot_image: string
    }[]
}

/** 抽卡（登录态；后端另有每分钟 30 次限流，429 由调用方处理）。 */
export function draw(blindBoxId: number) {
    const userInfo = useUserInfo()
    return request.request<DrawResult>({
        url: '/user/blindbox/draw',
        method: 'POST',
        data: { blind_box_id: blindBoxId },
        headers: { Authorization: `Bearer ${userInfo.token}` },
        __opts: { showErrorMessage: false },
    })
}
