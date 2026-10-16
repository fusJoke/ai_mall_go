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
 *
 * 【安全默认】只有**显式** `VITE_USE_MOCK === 'true'` 才启用（OCR 审查 P2-3）。
 *
 * 原实现是 `!== 'false'`（默认开启），后果不是数据泄露 —— mock 登录返回的是
 * `mock-token-${mobile}` 这类假令牌，真后端一律 401 —— 而是让「忘记配变量」的代价
 * 从「构建/运行直接报错」变成「静默用假数据跑」：生产环境表现为「界面显示已登录 +
 * 抽卡/订单全 401 的半瘫」，且没人能第一时间看出根因。
 * 默认关死后，漏配变量只会在开发期暴露，不会带上线。
 */
export const USE_MOCK = (import.meta.env.VITE_USE_MOCK as string) === 'true'

/**
 * 开发期真实用户令牌。
 *
 * 背景：后端 `POST /user/login` 强制「点选验证码」消费型校验，其答案只存服务端，
 * 客户端无法在 mock 模式下静默换取真 token。因此改为由开发者一次性注入真实令牌，
 * 使需鉴权接口（draw / orders）能真正打到库里。
 *
 * 为空时：会话降级为 mock-only，需鉴权接口会返回 401，界面会给出明确提示。
 *
 * 【部署卫生】该值会被 Vite 在**构建期静态内联**进产物 —— 任何访客都能从 JS 里
 * 读出这串令牌并直接调用需鉴权接口。因此：
 *   1. 只写在 `.env.development.local`（dev 模式专用 + 已 gitignore），
 *      **不要**写进 `.env.local`（它在 production 构建里同样会被加载）；
 *   2. `assertProductionHygiene()` 在 PROD 构建里见到它就立刻抛错（OCR 审查 P2-4）。
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

/** TabBar 页面路径：跳转这些页面必须用 switchTab，否则会失败。 */
export const TAB_PATHS = ['/pages/home/index', '/pages/orders/index', '/pages/mine/index']

/* ===== 生产构建卫生 ===== */

/**
 * 生产构建硬断言：把「不该进产物的开发期配置」变成构建期/启动期可见的失败。
 *
 * 为什么需要它：`import.meta.env.VITE_*` 会被 Vite 在**构建期静态内联**到 JS 里，
 * 于是「误把开发配置带上线」这件事在代码里完全看不出来 —— 只有访问者 view-source
 * 才能发现。这里在入口显式断言，让它在 CI / 冒烟阶段就炸掉。
 *
 * 在 `main.ts` 顶层调用一次。`import.meta.env.PROD` 是编译期常量，
 * dev 构建下整个函数体会被摇掉，不会有运行期开销。
 */
export function assertProductionHygiene(): void {
  if (!import.meta.env.PROD) return

  if (USE_MOCK) {
    throw new Error(
      '[uni-h5] USE_MOCK 不得在生产构建中启用：生产包会走假数据（登录返回假令牌、' +
        '抽卡/订单全 401），表现为「界面已登录但功能半瘫」。请检查 VITE_USE_MOCK 是否被显式设为 true。'
    )
  }
  if (DEV_TOKEN) {
    throw new Error(
      '[uni-h5] VITE_DEV_TOKEN 不得出现在生产构建中：它是能直接读写真实用户数据的令牌，' +
        '且会被静态内联进 JS 供任何访客读取。请把令牌移到 .env.development.local。'
    )
  }
}
