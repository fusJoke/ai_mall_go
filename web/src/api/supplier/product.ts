// src\api\supplier\product.ts — B 端盲盒商品管理
//
// 后端实现见 internal/handler/supplier/product.go；全部需 SupplierAuth。
import request from '/@/utils/request'
import { useSupplierInfo } from '/@/stores/supplier/supplierInfo'

export interface SupplierBlindBox {
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

export interface SupplierBlindBoxListResponse {
    items: SupplierBlindBox[]
    total: number
    page: number
    page_size: number
}

export interface PoolItemPayload {
    card_id: number
    rarity: 'SSR' | 'SR' | 'R' | 'N'
    weight: number
    stock: number
}

export function supplierProductList(params: { page?: number; page_size?: number }) {
    const supplierInfo = useSupplierInfo()
    return request.request<SupplierBlindBoxListResponse>({
        url: '/supplier/products/list',
        method: 'GET',
        params,
        headers: { Authorization: `Bearer ${supplierInfo.token}` },
    })
}

/** 创建盲盒 + 卡池（后端单事务；weight 加和必须 = 10000，否则 422）。 */
export function supplierProductCreate(data: {
    name: string
    cover?: string
    price: number
    description?: string
    items: PoolItemPayload[]
}) {
    const supplierInfo = useSupplierInfo()
    return request.request<SupplierBlindBox>({
        url: '/supplier/products/create',
        method: 'POST',
        data,
        headers: { Authorization: `Bearer ${supplierInfo.token}` },
        __opts: { showErrorMessage: false },
    })
}

/** 编辑盲盒（仅名称/封面/价格/描述；空字段 = 不改）。 */
export function supplierProductEdit(data: {
    id: number
    name?: string
    cover?: string
    price?: number
    description?: string
}) {
    const supplierInfo = useSupplierInfo()
    return request.request({
        url: '/supplier/products/edit',
        method: 'POST',
        data,
        headers: { Authorization: `Bearer ${supplierInfo.token}` },
    })
}

/** 上下架。 */
export function supplierProductToggleOnSale(id: number, onSale: boolean) {
    const supplierInfo = useSupplierInfo()
    return request.request({
        url: '/supplier/products/toggle-onsale',
        method: 'POST',
        data: { id, on_sale: onSale },
        headers: { Authorization: `Bearer ${supplierInfo.token}` },
    })
}
