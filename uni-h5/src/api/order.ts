import { get } from '@/utils/request'
import type { OrderDetail, OrderSummary, Paginated } from '@/types'

/** 每页条数上限与后端一致（后端 >200 会截断）。 */
export const PAGE_SIZE = 20

/** 我的订单分页列表。user_id 一律由 token 身份决定，不由请求参数指定。 */
export function fetchOrders(page = 1, pageSize = PAGE_SIZE) {
  return get<Paginated<OrderSummary>>('/user/orders/list', {
    page,
    page_size: Math.min(pageSize, 200),
  })
}

/** 订单详情。注意 id 走 query 而非 path。 */
export function fetchOrderDetail(id: number) {
  return get<OrderDetail>('/user/orders/detail', { id })
}
