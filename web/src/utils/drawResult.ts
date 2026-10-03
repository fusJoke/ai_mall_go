// src\utils\drawResult.ts — 抽卡结果 sessionStorage 传递（add-user-draw-result-page）。
//
// 普通抽卡 DrawResult 与秒杀 SeckillDrawResult 结构一致，共用同一个 key 与开卡结果页。
// 读取即清除：结果页刷新或二次进入时无数据，由结果页 replace 订单列表兜底，防丢参白屏。

export interface DrawCardSnapshot {
    item_id: number
    card_id: number
    rarity: string
    snapshot_name: string
    snapshot_image: string
}

export interface DrawResultPayload {
    order_id: number
    order_no: string
    actual_price: number
    /** 来源页路径（普通抽卡 /user/blindbox/:id，秒杀 /user/seckill/:id），「再抽一单」返回用。 */
    source_path?: string
    cards: DrawCardSnapshot[]
}

const DRAW_RESULT_KEY = 'user:draw-result'

export function saveDrawResult(result: DrawResultPayload) {
    sessionStorage.setItem(DRAW_RESULT_KEY, JSON.stringify(result))
}

/** 取出并清除结果；无数据或解析失败返回 null。 */
export function takeDrawResult(): DrawResultPayload | null {
    const raw = sessionStorage.getItem(DRAW_RESULT_KEY)
    if (!raw) return null
    sessionStorage.removeItem(DRAW_RESULT_KEY)
    try {
        return JSON.parse(raw) as DrawResultPayload
    } catch {
        return null
    }
}
