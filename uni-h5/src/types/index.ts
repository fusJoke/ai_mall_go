/**
 * 全项目类型单一来源。
 *
 * 字段与后端 json tag 严格对齐（来源：`internal/handler/user/*.go` 与
 * `internal/model/mall/*.go`）。新增字段前先核对后端 DTO，不要凭猜测扩展。
 */

/* ============ 通用 ============ */

/** 后端错误响应信封：`{code, message}`。 */
export interface ApiErrorShape {
  code?: string
  message?: string
}

/** 业务侧统一错误对象。 */
export interface ApiError extends Error {
  http: number
  code: string
  /** 429 时的建议重试秒数，供调用方禁用按钮。 */
  retryAfter?: number
}

export type HttpMethod = 'GET' | 'POST' | 'PUT' | 'DELETE'

/** 分页响应外层（`blindbox/list` 与 `orders/list` 同构）。 */
export interface Paginated<T> {
  items: T[]
  total: number
  page: number
  page_size: number
}

/* ============ 用户与鉴权 ============ */

/** 登录响应中的用户投影（不含 password / login_failure）。 */
export interface UserInfo {
  id: number
  username: string
  nickname: string
  avatar: string
  /** 后端 omitempty：可能缺字段。 */
  email?: string
  /** 后端 omitempty：可能缺字段。 */
  mobile?: string
  /** 注意：后端以 `%.2f` 格式化为**字符串**。 */
  balance: string
  last_login_at?: string
  last_login_ip?: string
  status: number
}

/** `POST /user/login`：需点选验证码，points 为 `{x,y}` 数组。 */
export interface LoginRequest {
  username: string
  password: string
  captcha_key: string
  points: Array<{ x: number; y: number }>
  remember?: boolean
}

/** `POST /user/login` 与 `POST /user/login/mobile` 的响应。 */
export interface LoginResponse {
  user: UserInfo
  token: string
}

/** `POST /user/login/mobile` 请求体（本期由 mock 承接）。 */
export interface MobileLoginRequest {
  mobile: string
  sms_code: string
  /**
   * 滑块验证码字段 —— **本期登录页不发送**（滑块已从登录页移除，见
   * `pages/login/index.vue` 头注释）。保留为可选，供后续重新接入滑块时使用：
   * 届时把 login 页的提交体补回这四个字段即可，类型无需改动。
   */
  captcha_key?: string
  captcha_x?: number
  captcha_w?: number
  captcha_h?: number
}

/** `POST /user/sms/send`。 */
export interface SmsSendResponse {
  code: string
  message: string
  expires_in?: number
}

/* ============ 滑块验证码 ============ */

/** `GET /common/captcha/slider`。 */
export interface SliderCreateResponse {
  key: string
  /** 带缺口的背景图（data URI）。 */
  image: string
  /** 拼图块图（data URI）。 */
  block: string
  w: number
  h: number
  /** 拼图块初始纵向偏移。 */
  y: number
  /**
   * 拼图块原始尺寸。
   *
   * 服务端契约必须包含这两项：客户端需要按显示缩放比还原拖拽位移，
   * 缺省时客户端退化为按 0.16 的比例估算（仅保证视觉不崩，不保证判定精度）。
   */
  block_w?: number
  block_h?: number
  expires_in: number
}

/** `POST /common/captcha/slider/verify`。 */
export interface SliderVerifyResponse {
  code: string
  message: string
}

/* ============ 首页 Feed ============ */

export interface BlindBoxFeedItem {
  id: number
  supplier_id: number
  supplier_name: string
  name: string
  cover: string
  /** 原价。 */
  price: number
  /** 活动价；无活动时后端 omitempty，字段缺失。 */
  promo_price?: number | null
  rarity_summary: string
  hot_score: number
  created_at: string
}

export interface SupplierFeedItem {
  id: number
  name: string
  logo: string
  blind_box_count: number
  featured_rank: number
}

export interface FeedResponse {
  blind_boxes: BlindBoxFeedItem[]
  suppliers: SupplierFeedItem[]
  fetched_at: string
}

/* ============ 盲盒与卡池 ============ */

export type RarityCode = 'SSR' | 'SR' | 'R' | 'N'

export interface MallBlindBox {
  id: number
  supplier_id: number
  name: string
  cover: string
  price: number
  status: string
  on_sale: boolean
  lifecycle: string
  is_featured: boolean
  description: string
  updated_at: string
  created_at: string
}

/** 详情页供应商为白名单投影，仅 4 个字段。 */
export interface SupplierPublicView {
  id: number
  name: string
  logo: string
  bio: string
}

export interface MallCardPool {
  id: number
  blind_box_id: number
  updated_at: string
  created_at: string
}

export interface MallCardPoolItem {
  id: number
  pool_id: number
  card_id: number
  rarity: RarityCode | string
  /** 权重，同池合计 10000。 */
  weight: number
  /** 剩余库存。 */
  stock: number
  updated_at: string
  created_at: string
}

export interface MallPromotion {
  id: number
  supplier_id: number
  blind_box_id: number
  original_price: number
  promo_price: number
  start_at: string
  end_at: string
  status: string
  updated_at: string
  created_at: string
}

export interface BlindBoxDetailResponse {
  blind_box: MallBlindBox
  supplier: SupplierPublicView
  pool: MallCardPool
  items: MallCardPoolItem[]
  /** 卡池总库存 = Σ items.stock。 */
  pool_total_stock: number
  /** 无活动时字段缺失。 */
  active_promotion?: MallPromotion | null
  /** 实际售价（活动价优先，否则原价）。 */
  effective_price: number
  cached_at: string
}

/* ============ 抽卡 ============ */

export interface DrawRequest {
  blind_box_id: number
}

export interface DrawCard {
  /** 卡池条目 ID。 */
  item_id: number
  card_id: number
  rarity: RarityCode | string
  snapshot_name: string
  snapshot_image: string
}

export interface DrawResult {
  order_id: number
  order_no: string
  actual_price: number
  cards: DrawCard[]
}

/* ============ 订单 ============ */

export type OrderStatus = 'pending' | 'paid' | 'drawn' | 'failed'
export type OrderSource = 'normal' | 'seckill'

export interface OrderSummary {
  id: number
  order_no: string
  user_id: number
  blind_box_id: number
  supplier_id: number
  /** 实付金额。 */
  price: number
  status: OrderStatus | string
  source: OrderSource | string
  updated_at: string
  created_at: string
}

export interface OrderItem {
  id: number
  order_id: number
  card_id: number
  rarity: RarityCode | string
  snapshot_name: string
  snapshot_image: string
  updated_at: string
  created_at: string
}

export interface OrderDetail {
  order: OrderSummary
  items: OrderItem[]
}
