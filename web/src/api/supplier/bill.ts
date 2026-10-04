// src\api\supplier\bill.ts — B 端供应商批次账单（add-batch-settlement 任务 5.1）
//
// 后端实现见 internal/handler/supplier/bill.go；全部需 SupplierAuth。
// 账单状态机：generated → confirmed → paid / generated → rejected /
// confirmed → failed → confirmed。金额字段均为元（decimal 快照）。
import request from '/@/utils/request'
import { useSupplierInfo } from '/@/stores/supplier/supplierInfo'

export type SupplierBillStatus = 'generated' | 'confirmed' | 'rejected' | 'failed' | 'paid'

/** 批次结算账单主表（对齐 mall.MallSettlementBill json tag）。 */
export interface SupplierSettlementBill {
    id: number
    blind_box_id: number
    supplier_id: number
    total_amount: number
    commission_rate: number
    commission_amount: number
    payout_amount: number
    status: SupplierBillStatus
    reject_reason: string
    version: number
    confirmed_at: string | null
    paid_at: string | null
    paid_by: number | null
    remark: string
    updated_at: string
    created_at: string
}

/** 账单明细（一订单一行，金额快照；对齐 mall.MallSettlementBillItem）。 */
export interface SupplierSettlementBillItem {
    id: number
    bill_id: number
    draw_order_id: number
    amount: number
    commission_amount: number
    payout_amount: number
    updated_at: string
    created_at: string
}

export interface SupplierBillListResponse {
    items: SupplierSettlementBill[]
    total: number
    page: number
    page_size: number
}

export interface SupplierBillDetailResponse {
    bill: SupplierSettlementBill
    items: SupplierSettlementBillItem[]
}

/** 我的批次账单列表；status 空 = 全部。 */
export function supplierBillList(params: { status?: string; page?: number; page_size?: number }) {
    const supplierInfo = useSupplierInfo()
    return request.request<SupplierBillListResponse>({
        url: '/supplier/bills/list',
        method: 'GET',
        params,
        headers: { Authorization: `Bearer ${supplierInfo.token}` },
    })
}

/** 账单详情（账单 + 全部订单明细；非本供应商账单 → 403）。 */
export function supplierBillDetail(id: number) {
    const supplierInfo = useSupplierInfo()
    return request.request<SupplierBillDetailResponse>({
        url: '/supplier/bills/detail',
        method: 'GET',
        params: { id },
        headers: { Authorization: `Bearer ${supplierInfo.token}` },
    })
}

/** 确认账单（generated → confirmed；409 = 状态机拒绝或并发抢先）。 */
export function supplierBillConfirm(id: number) {
    const supplierInfo = useSupplierInfo()
    return request.request({
        url: '/supplier/bills/confirm',
        method: 'POST',
        data: { id },
        headers: { Authorization: `Bearer ${supplierInfo.token}` },
        __opts: { showErrorMessage: false },
    })
}

/** 驳回账单（generated → rejected；reason trim 后必填，否则 422）。 */
export function supplierBillReject(id: number, reason: string) {
    const supplierInfo = useSupplierInfo()
    return request.request({
        url: '/supplier/bills/reject',
        method: 'POST',
        data: { id, reason },
        headers: { Authorization: `Bearer ${supplierInfo.token}` },
        __opts: { showErrorMessage: false },
    })
}
