// src\api\admin\mall.ts — admin 对供应商 / 盲盒的管理接口（任务 6.11）。
//
// 后端实现见 internal/handler/admin/mall.go；全部走 AdminAuth
// （请求拦截器已自动携带 adminInfo.token）。
import request from '/@/utils/request'

export interface MallSupplier {
    id: number
    name: string
    logo: string
    bio: string
    contact_phone: string
    status: 'active' | 'disabled'
    is_featured: boolean
    balance: number
    total_sales: number
    commission_rate: number | null
    created_at: string
}

export interface MallSupplierListResponse {
    items: MallSupplier[]
    total: number
    page: number
    page_size: number
}

export interface MallSupplierListParams {
    page?: number
    page_size?: number
    status?: string
    is_featured?: boolean | string
    name?: string
}

export function mallSupplierList(params: MallSupplierListParams = {}) {
    const clean: Record<string, unknown> = { ...params }
    if (clean.is_featured === undefined || clean.is_featured === '') {
        delete clean.is_featured
    }
    return request.request<MallSupplierListResponse>({
        url: '/admin/supplier/list',
        method: 'GET',
        params: clean,
    })
}

export function mallSupplierToggleStatus(id: number) {
    return request.request({
        url: '/admin/supplier/toggle-status',
        method: 'POST',
        data: { id },
    })
}

export function mallSupplierToggleFeatured(id: number) {
    return request.request({
        url: '/admin/supplier/toggle-featured',
        method: 'POST',
        data: { id },
    })
}

export interface MallBlindBox {
    id: number
    supplier_id: number
    name: string
    cover: string
    price: number
    status: 'active' | 'disabled'
    on_sale: boolean
    is_featured: boolean
    created_at: string
}

export interface MallBlindBoxListResponse {
    items: MallBlindBox[]
    total: number
    page: number
    page_size: number
}

export function mallBlindBoxList(params: { page?: number; page_size?: number } = {}) {
    return request.request<MallBlindBoxListResponse>({
        url: '/admin/blindbox/list',
        method: 'GET',
        params,
    })
}

export function mallBlindBoxToggleStatus(id: number) {
    return request.request({
        url: '/admin/blindbox/toggle-status',
        method: 'POST',
        data: { id },
    })
}

export function mallBlindBoxToggleOnSale(id: number) {
    return request.request({
        url: '/admin/blindbox/toggle-onsale',
        method: 'POST',
        data: { id },
    })
}

export function mallBlindBoxToggleFeatured(id: number) {
    return request.request({
        url: '/admin/blindbox/toggle-featured',
        method: 'POST',
        data: { id },
    })
}

export function mallPromotionList(params: { page?: number; page_size?: number } = {}) {
    return request.request<MallPromotionListResponse>({
        url: '/admin/promotion/list',
        method: 'GET',
        params,
    })
}

/** admin 只读活动列表响应。 */
export interface MallPromotionListResponse {
    items: MallPromotion[]
    total: number
    page: number
    page_size: number
}

/** 单条活动（admin 只读视图）。 */
export interface MallPromotion {
    id: number
    supplier_id: number
    blind_box_id: number
    original_price: number
    promo_price: number
    start_at: string
    end_at: string
    status: string
}

// =============================================================================
// 结算管理（spec 8.9-8.11：list / detail / preview / generate / mark-paid）
// =============================================================================

/** 单条结算单（admin 视角）。 */
export interface MallSettlement {
    id: number
    supplier_id: number
    period_start: string
    period_end: string
    total_amount: number
    commission_rate: number
    commission_amount: number
    payout_amount: number
    status: 'pending' | 'processing' | 'paid' | 'failed'
    paid_at: string | null
    paid_by: number | null
    created_at: string
}

/** 结算明细（一结算单内的一条 order）。 */
export interface MallSettlementItem {
    id: number
    settlement_id: number
    draw_order_id: number
    amount: number
    commission_amount: number
    payout_amount: number
    created_at: string
}

/** 结算单列表响应。 */
export interface MallSettlementListResponse {
    items: MallSettlement[]
    total: number
    page: number
    page_size: number
}

/** 结算单详情响应：settlement + items。 */
export interface MallSettlementDetailResponse {
    settlement: MallSettlement
    items: MallSettlementItem[]
}

/** 结算预览响应（不写库）。 */
export interface MallSettlementPreview {
    supplier_id: number
    supplier_name: string
    period_start: string
    period_end: string
    commission_rate: number
    order_count: number
    total_amount: number
    commission_amount: number
    payout_amount: number
}

export function mallSettlementList(params: { page?: number; page_size?: number } = {}) {
    return request.request<MallSettlementListResponse>({
        url: '/admin/settlement/list',
        method: 'GET',
        params,
    })
}

export function mallSettlementDetail(id: number) {
    return request.request<MallSettlementDetailResponse>({
        url: '/admin/settlement/detail',
        method: 'GET',
        params: { id },
    })
}

export function mallSettlementPreview(body: { supplier_id: number; period_start: string; period_end: string }) {
    return request.request<MallSettlementPreview>({
        url: '/admin/settlement/preview',
        method: 'POST',
        data: body,
    })
}

export function mallSettlementGenerate(body: { supplier_id: number; period_start: string; period_end: string }) {
    return request.request<{ code: string; message: string; settlement: MallSettlement }>({
        url: '/admin/settlement/generate',
        method: 'POST',
        data: body,
    })
}

export function mallSettlementMarkPaid(id: number) {
    return request.request({
        url: '/admin/settlement/mark-paid',
        method: 'POST',
        data: { id },
    })
}
