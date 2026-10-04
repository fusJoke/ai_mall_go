// src\api\supplier\product.ts — B 端盲盒商品管理
//
// 后端实现见 internal/handler/supplier/product.go；全部需 SupplierAuth。
import request from '/@/utils/request'
import { useSupplierInfo } from '/@/stores/supplier/supplierInfo'

export type SupplierBoxLifecycle = 'selling' | 'sold_out' | 'settled'

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
    lifecycle: SupplierBoxLifecycle
    created_at: string
}

/** 单盒销售进度快照（remaining=剩余可抽，sold=已售出）。 */
export interface SupplierPoolStat {
    remaining: number
    sold: number
}

export interface SupplierBlindBoxListResponse {
    items: SupplierBlindBox[]
    total: number
    page: number
    page_size: number
    /** key 为盲盒 ID 字符串（JSON map key 序列化）；缺 key = 未统计。 */
    stats: Record<string, SupplierPoolStat>
}

export interface PoolItemPayload {
    card_id: number
    rarity: 'SSR' | 'SR' | 'R' | 'N'
    weight: number
    stock: number
}

/** 我的盲盒列表；lifecycle 空 = 全部（状态筛选 tab）。 */
export function supplierProductList(params: {
    lifecycle?: string
    page?: number
    page_size?: number
}) {
    const supplierInfo = useSupplierInfo()
    return request.request<SupplierBlindBoxListResponse>({
        url: '/supplier/products/list',
        method: 'GET',
        params,
        headers: { Authorization: `Bearer ${supplierInfo.token}` },
    })
}

/** 盲盒详情（含卡池条目，编辑页预填；sold_out/settled 只读可见）。 */
export function supplierProductDetail(id: number) {
    const supplierInfo = useSupplierInfo()
    return request.request<{ box: SupplierBlindBox; items: PoolItemPayload[] }>({
        url: '/supplier/products/detail',
        method: 'GET',
        params: { id },
        headers: { Authorization: `Bearer ${supplierInfo.token}` },
    })
}

/** 重写卡池条目（补库存/调概率）；409 = lifecycle_forbidden / pool_locked（已开卖）。 */
export function supplierProductUpdatePool(id: number, items: PoolItemPayload[]) {
    const supplierInfo = useSupplierInfo()
    return request.request({
        url: '/supplier/products/update-pool',
        method: 'POST',
        data: { id, items },
        headers: { Authorization: `Bearer ${supplierInfo.token}` },
        __opts: { showErrorMessage: false },
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

/** 上下架。售罄（sold_out）/已结算（settled）批次返回 409 product.lifecycle_forbidden。 */
export function supplierProductToggleOnSale(id: number, onSale: boolean) {
    const supplierInfo = useSupplierInfo()
    return request.request({
        url: '/supplier/products/toggle-onsale',
        method: 'POST',
        data: { id, on_sale: onSale },
        headers: { Authorization: `Bearer ${supplierInfo.token}` },
    })
}

/** 再来一批：复制历史盲盒卡池生成新批次（下架态，返回新盲盒 ID）。 */
export function supplierProductDuplicate(id: number) {
    const supplierInfo = useSupplierInfo()
    return request.request<{ id: number }>({
        url: '/supplier/products/duplicate',
        method: 'POST',
        data: { id },
        headers: { Authorization: `Bearer ${supplierInfo.token}` },
        __opts: { showErrorMessage: false },
    })
}
