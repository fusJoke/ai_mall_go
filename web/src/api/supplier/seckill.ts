// src\api\supplier\seckill.ts — B 端供应商秒杀活动管理
//
// 后端实现见 internal/handler/supplier/seckill.go；全部需 SupplierAuth。
// 时间窗字段用 RFC3339（el-date-picker value-format 产出）。
import request from '/@/utils/request'
import { useSupplierInfo } from '/@/stores/supplier/supplierInfo'

export interface SupplierSeckill {
    id: number
    supplier_id: number
    blind_box_id: number
    seckill_price: number
    total_stock: number
    per_user_limit: number
    start_at: string
    end_at: string
    status: 'active' | 'disabled'
    redis_initialized: boolean
}

export interface SupplierSeckillListResponse {
    items: SupplierSeckill[]
    total: number
    page: number
    page_size: number
}

export function supplierSeckillList(params: { page?: number; page_size?: number }) {
    const supplierInfo = useSupplierInfo()
    return request.request<SupplierSeckillListResponse>({
        url: '/supplier/seckill/list',
        method: 'GET',
        params,
        headers: { Authorization: `Bearer ${supplierInfo.token}` },
    })
}

/** 创建秒杀活动（时间窗非法 / 秒杀价 ≥ 原价 / total_stock 超卡池库存 → 422）。 */
export function supplierSeckillCreate(data: {
    blind_box_id: number
    seckill_price: number
    total_stock: number
    per_user_limit: number
    start_at: string
    end_at: string
}) {
    const supplierInfo = useSupplierInfo()
    return request.request<SupplierSeckill>({
        url: '/supplier/seckill/create',
        method: 'POST',
        data,
        headers: { Authorization: `Bearer ${supplierInfo.token}` },
        __opts: { showErrorMessage: false },
    })
}

/** 编辑秒杀活动（仅价 / 限购；时间窗与盲盒不可改）。 */
export function supplierSeckillEdit(data: { id: number; seckill_price?: number; per_user_limit?: number }) {
    const supplierInfo = useSupplierInfo()
    return request.request({
        url: '/supplier/seckill/edit',
        method: 'POST',
        data,
        headers: { Authorization: `Bearer ${supplierInfo.token}` },
        __opts: { showErrorMessage: false },
    })
}

export function supplierSeckillToggle(id: number) {
    const supplierInfo = useSupplierInfo()
    return request.request({
        url: '/supplier/seckill/toggle',
        method: 'POST',
        data: { id },
        headers: { Authorization: `Bearer ${supplierInfo.token}` },
    })
}
