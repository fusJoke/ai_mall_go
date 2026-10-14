import { get, post } from '@/utils/request'
import type {
  BlindBoxDetailResponse,
  DrawResult,
  FeedResponse,
  MallBlindBox,
  Paginated,
} from '@/types'

/** 首页推荐流。后端强依赖 ES，ES 不可用时由服务端降级到 MySQL（见能力 `home-feed-fallback`）。 */
export function fetchFeed() {
  return get<FeedResponse>('/user/home/feed')
}

/** 盲盒分页列表（当前范围内未使用，保留以对齐接口清单）。 */
export function fetchBlindBoxList(params: { page?: number; page_size?: number } = {}) {
  return get<Paginated<MallBlindBox>>('/user/blindbox/list', params)
}

/** 盲盒详情：概率公示 + 当前活动 + 有效价。注意 id 走 query 而非 path。 */
export function fetchBlindBoxDetail(id: number) {
  return get<BlindBoxDetailResponse>('/user/blindbox/detail', { id })
}

/** 抽卡下单。需登录，限流 30 次/分钟。 */
export function drawBlindBox(blindBoxId: number) {
  return post<DrawResult>('/user/blindbox/draw', { blind_box_id: blindBoxId })
}
