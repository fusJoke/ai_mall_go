// src\api\user\home.ts — C 端首页 Feed（ES 推荐流，公开接口）
//
// 后端实现见 internal/handler/user/home.go；ES 不可用 → 500 search_unavailable。
import request from '/@/utils/request'

export interface BlindBoxFeedItem {
    id: number
    supplier_id: number
    supplier_name: string
    name: string
    cover: string
    price: number
    promo_price?: number | null
    rarity_summary: string
    hot_score: number
    created_at: string
}

export interface SupplierFeedItem {
    id: number
    name: string
    logo: string
    blind_box_count: number
    featured_rank: number
}

export interface FeedResponse {
    blind_boxes: BlindBoxFeedItem[]
    suppliers: SupplierFeedItem[]
    fetched_at: string
}

export function homeFeed() {
    return request.request<FeedResponse>({
        url: '/user/home/feed',
        method: 'GET',
        __opts: { showErrorMessage: false },
    })
}
