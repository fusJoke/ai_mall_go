/**
 * 运行时配置。
 *
 * 所有环境相关魔法值集中在此，页面与组件不得直接读取 import.meta.env。
 */

/** 接口基地址：开发期走 vite 代理（/api → 本地后端），生产期由环境变量注入。 */
export const BASE_URL = (import.meta.env.VITE_API_BASE_URL as string) || '/api'

/**
 * Mock 总开关。
 *
 * 边界（已修订）：**只覆盖登录页及其配套 4 条接口**。
 * 商品、抽卡、订单、当前用户信息一律走真实后端，保证主链路是端到端真跑的。
 */
export const USE_MOCK = (import.meta.env.VITE_USE_MOCK as string) !== 'false'

/**
 * 开发期真实用户令牌。
 *
 * 背景：后端 `POST /user/login` 强制「点选验证码」消费型校验，其答案只存服务端，
 * 客户端无法在 mock 模式下静默换取真 token。因此改为由开发者一次性注入真实令牌，
 * 使需鉴权接口（draw / orders）能真正打到库里。
 *
 * 为空时：会话降级为 mock-only，需鉴权接口会返回 401，界面会给出明确提示。
 */
export const DEV_TOKEN = (import.meta.env.VITE_DEV_TOKEN as string) || ''

/** 本地存储键。 */
export const STORAGE_KEYS = {
  token: 'uh5:token',
  user: 'uh5:user',
  drawResult: 'uh5:draw-result',
} as const

/** 登录页路径。 */
export const LOGIN_PATH = '/pages/login/index'

/** 订单列表页路径（结果页无数据时的兜底去处）。 */
export const ORDERS_PATH = '/pages/orders/index'

/** 网络超时（毫秒）。 */
export const REQUEST_TIMEOUT = 15000

/* ===== 表单校验与节奏参数（页面与 mock 共用，避免各自硬编码） ===== */

/** 中国大陆手机号格式。 */
export const MOBILE_RE = /^1[3-9]\d{9}$/

/** 短信验证码倒计时秒数。 */
export const SMS_COUNTDOWN = 60

/** 开发期固定短信验证码（真实通道未接入时使用）。 */
export const FIXED_SMS_CODE = '000000'

/** TabBar 页面路径：跳转这些页面必须用 switchTab，否则会失败。 */
export const TAB_PATHS = ['/pages/home/index', '/pages/orders/index', '/pages/mine/index']
