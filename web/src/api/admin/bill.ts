// src\api\admin\bill.ts — admin 批次账单管理接口（add-batch-settlement 任务 5.4）
//
// 后端实现见 internal/handler/admin/bill.go；全部走 AdminAuth
// （请求拦截器已自动携带 adminInfo.token，故此处不手动带 Authorization）。
//
// 状态机：generated → confirmed → paid / generated → rejected →
// generated（admin 重核 version+1）/ confirmed → failed → confirmed。
// 与「周期结算」（api/admin/mall.ts 的 settlement 段）是两套东西：
// 批次账单 = 1 商品 1 账单，售罄时由抽卡事务自动生成。
import request from '/@/utils/request'

/** 批次账单五态（对齐 mall.MallSettlementBillStatus）。 */
export type AdminBillStatus = 'generated' | 'confirmed' | 'rejected' | 'failed' | 'paid'

/** 批次结算账单主表（对齐 mall.MallSettlementBill json tag）。 */
export interface AdminSettlementBill {
    id: number
    blind_box_id: number
    supplier_id: number
    total_amount: number
    commission_rate: number
    commission_amount: number
    payout_amount: number
    status: AdminBillStatus
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
export interface AdminSettlementBillItem {
    id: number
    bill_id: number
    draw_order_id: number
    amount: number
    commission_amount: number
    payout_amount: number
    updated_at: string
    created_at: string
}

export interface AdminBillListResponse {
    items: AdminSettlementBill[]
    total: number
    page: number
    page_size: number
}

export interface AdminBillDetailResponse {
    bill: AdminSettlementBill
    items: AdminSettlementBillItem[]
}

export interface AdminBillListParams {
    status?: string
    supplier_id?: number
    page?: number
    page_size?: number
}

/** 批次账单列表（status / supplier_id 空 = 不筛）。 */
export function adminBillList(params: AdminBillListParams = {}) {
    const clean: Record<string, unknown> = { ...params }
    // 空字符串 / undefined 一律剔除，避免后端把 "" 当非法 status（400）。
    if (!clean.status) delete clean.status
    if (!clean.supplier_id) delete clean.supplier_id
    return request.request<AdminBillListResponse>({
        url: '/admin/bill/list',
        method: 'GET',
        params: clean,
    })
}

/** 账单详情（账单 + 全部订单明细）。 */
export function adminBillDetail(id: number) {
    return request.request<AdminBillDetailResponse>({
        url: '/admin/bill/detail',
        method: 'GET',
        params: { id },
    })
}

/** 重核金额（rejected → generated，version+1）；金额四件套整体快照更新。 */
export function adminBillRegenerate(body: {
    id: number
    total_amount: number
    commission_rate: number
    commission_amount: number
    payout_amount: number
    remark?: string
}) {
    return request.request({
        url: '/admin/bill/regenerate',
        method: 'POST',
        data: body,
        __opts: { showErrorMessage: false },
    })
}

/** 打款（confirmed → paid；事务内扣 supplier.balance + 商品置 settled）。 */
export function adminBillMarkPaid(id: number) {
    return request.request({
        url: '/admin/bill/mark-paid',
        method: 'POST',
        data: { id },
        __opts: { showErrorMessage: false },
    })
}
