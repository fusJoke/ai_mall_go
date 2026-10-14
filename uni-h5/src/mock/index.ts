/**
 * Mock 注册表与开关。
 *
 * **边界（已修订）**：只覆盖登录页及其配套 4 条接口。
 * 商品（feed / blindbox）、抽卡（draw）、订单（orders）、当前用户信息（profile）
 * **一律走真实后端**，保证主链路是端到端真跑的，而不是演示性质的假数据流。
 *
 * 关于 `GET /user/profile` 从 mock 中移除：该接口已由后端实现
 * （`internal/handler/user/profile.go`）。留在 mock 里会返回**登录时刻的快照**，
 * 而该接口存在的唯一目的就是拿到下单后的**最新余额** —— 二者直接矛盾，
 * 保留 mock 等于把接口要修的缺陷原样盖回去。详见
 * specs/uni-h5-client/spec.md「本地 Mock 层开关与边界」。
 *
 * 全项目 mock 注册 URL 清单可 grep 本文件常量 `MOCK_ROUTES` 核对。
 */

import { USE_MOCK } from '@/config'
import { mockLoginMobile } from './login'
import { mockCreateSlider, mockVerifySlider } from './slider'
import { mockSendSms } from './sms'

type MockHandler = (data?: Record<string, unknown>) => unknown

/** 键格式：`METHOD /path`（路径不含 BASE_URL 前缀）。 */
const registry: Record<string, MockHandler> = {
  'POST /user/sms/send': mockSendSms,
  'GET /common/captcha/slider': mockCreateSlider,
  'POST /common/captcha/slider/verify': mockVerifySlider,
  'POST /user/login/mobile': mockLoginMobile,
}

/** mock 覆盖的 URL 清单，供自检使用。 */
export const MOCK_ROUTES = Object.keys(registry)

/** 命中则返回处理函数，否则返回 undefined（走真实网络）。 */
export function matchMock(method: string, url: string): MockHandler | undefined {
  if (!USE_MOCK) return undefined
  return registry[`${method.toUpperCase()} ${url}`]
}
