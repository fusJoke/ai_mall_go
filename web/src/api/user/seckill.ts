// src\api\user\seckill.ts — C 端秒杀活动（列表 / 详情 / 秒杀下单）
//
// 后端实现见 internal/handler/user/seckill.go。
// list / detail 为公开路由；draw 需要会员 token，显式挂 Authorization 头
// （与 blindbox.ts 的 draw 同模式：全局拦截器只带 admin token）。
import request from '/@/utils/request'
import { useUserInfo } from '/@/stores/user/userInfo'

/** 秒杀活动（列表投影）。remaining_stock = -1 表示后端暂不可知。 */
export interface SeckillActivity {
    id: number
    blind_box_id: number
    blind_box_name: string
    blind_box_cover: string
    supplier_id: number
    supplier_name: string
    original_price: number
    seckill_price: number
    total_stock: number
    remaining_stock: number
    per_user_limit: number
    start_at: string
    end_at: string
    status: string
}

export interface SeckillListResponse {
    items: SeckillActivity[]
    total: number
    page: number
    page_size: number
}

export interface SeckillDetail extends SeckillActivity {
    description: string
}

/** 当前生效的秒杀活动列表（公开路由）。 */
export function seckillList(params: { page?: number; page_size?: number }) {
    return request.request<SeckillListResponse>({
        url: '/user/seckill/list',
        method: 'GET',
        params,
    })
}

/** 秒杀活动详情（公开路由；含实时剩余名额）。 */
export function seckillDetail(id: number) {
    return request.request<SeckillDetail>({
        url: '/user/seckill/detail',
        method: 'GET',
        params: { id },
    })
}

export interface SeckillDrawResult {
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

/** 秒杀下单（登录态；后端另有每分钟 30 次限流，429 由调用方处理）。 */
export function seckillDraw(seckillId: number) {
    const userInfo = useUserInfo()
    return request.request<SeckillDrawResult>({
        url: `/user/seckill/${seckillId}/draw`,
        method: 'POST',
        headers: { Authorization: `Bearer ${userInfo.token}` },
        __opts: { showErrorMessage: false },
    })
}
