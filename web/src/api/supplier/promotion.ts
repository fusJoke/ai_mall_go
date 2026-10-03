// src\api\supplier\promotion.ts — B 端限时特价活动管理
//
// 后端实现见 internal/handler/supplier/promotion.go；全部需 SupplierAuth。
import request from '/@/utils/request'
import { useSupplierInfo } from '/@/stores/supplier/supplierInfo'

export interface SupplierPromotion {
    id: number
    supplier_id: number
    blind_box_id: number
    original_price: number
    promo_price: number
    start_at: string
    end_at: string
    status: 'active' | 'disabled'
}

export interface SupplierPromotionListResponse {
    items: SupplierPromotion[]
    total: number
    page: number
    page_size: number
}

export function supplierPromotionList(params: { page?: number; page_size?: number }) {
    const supplierInfo = useSupplierInfo()
    return request.request<SupplierPromotionListResponse>({
        url: '/supplier/promotions/list',
        method: 'GET',
        params,
        headers: { Authorization: `Bearer ${supplierInfo.token}` },
    })
}

/** 创建活动（时间窗重叠 → 422 promotion.time_overlap）。 */
export function supplierPromotionCreate(data: {
    blind_box_id: number
    original_price: number
    promo_price: number
    start_at: string
    end_at: string
}) {
    const supplierInfo = useSupplierInfo()
    return request.request<SupplierPromotion>({
        url: '/supplier/promotions/create',
        method: 'POST',
        data,
        headers: { Authorization: `Bearer ${supplierInfo.token}` },
        __opts: { showErrorMessage: false },
    })
}

/** 编辑活动价格（时段与归属盲盒不可改）。 */
export function supplierPromotionEdit(data: { id: number; original_price?: number; promo_price?: number }) {
    const supplierInfo = useSupplierInfo()
    return request.request({
        url: '/supplier/promotions/edit',
        method: 'POST',
        data,
        headers: { Authorization: `Bearer ${supplierInfo.token}` },
    })
}

export function supplierPromotionToggle(id: number) {
    const supplierInfo = useSupplierInfo()
    return request.request({
        url: '/supplier/promotions/toggle',
        method: 'POST',
        data: { id },
        headers: { Authorization: `Bearer ${supplierInfo.token}` },
    })
}

export function supplierPromotionDelete(id: number) {
    const supplierInfo = useSupplierInfo()
    return request.request({
        url: '/supplier/promotions/delete',
        method: 'POST',
        data: { id },
        headers: { Authorization: `Bearer ${supplierInfo.token}` },
    })
}
