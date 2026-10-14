/**
 * `POST /user/login/mobile` 的 mock 实现。
 *
 * 返回结构与真实契约同构：`{ user, token }`。
 * 校验固定验证码 `000000`；错误码与 `user-mobile-login` spec 对齐。
 *
 * 校验顺序：手机号格式 → 短信验证码。
 * **不校验滑块**：滑块已从登录页移除（见 `pages/login/index.vue` 头注释），
 * 请求体里的 `captcha_*` 字段保留为预留可选参数。重新接入时在此恢复
 * 「`captcha_key` 缺失 → 401 `login.invalid_captcha`」这一前置分支即可。
 *
 * 注意：此处签发的 mock token **不参与鉴权**。mock 模式下需鉴权接口的真实身份
 * 由开发者注入的真实令牌承接（见 `@/utils/session`）。
 */

import type { LoginResponse, UserInfo } from '@/types'
import { FIXED_SMS_CODE, MOBILE_RE } from './constants'
import { mockError } from './error'

function buildMockUser(mobile: string): UserInfo {
  return {
    id: 0,
    username: mobile,
    nickname: `玩家${mobile.slice(-4)}`,
    avatar: '',
    mobile,
    balance: '1288.00',
    status: 1,
    last_login_at: new Date().toISOString(),
    last_login_ip: '127.0.0.1',
  }
}

export function mockLoginMobile(data?: Record<string, unknown>): LoginResponse {
  const mobile = String(data?.mobile ?? '')
  const smsCode = String(data?.sms_code ?? '')

  if (!MOBILE_RE.test(mobile)) {
    throw mockError(400, 'login.invalid_input', '手机号格式不正确')
  }
  if (smsCode !== FIXED_SMS_CODE) {
    throw mockError(401, 'login.invalid_sms_code', '验证码错误')
  }

  return { user: buildMockUser(mobile), token: `mock-token-${mobile}` }
}
