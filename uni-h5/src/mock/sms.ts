/**
 * `POST /user/sms/send` 的 mock 实现。
 *
 * 行为：校验手机号格式 → 返回成功并打印固定验证码。不真正发短信。
 */

import type { SmsSendResponse } from '@/types'
import { FIXED_SMS_CODE, MOBILE_RE } from './constants'
import { mockError } from './error'

export function mockSendSms(data?: Record<string, unknown>): SmsSendResponse {
  const mobile = String(data?.mobile ?? '')
  if (!MOBILE_RE.test(mobile)) {
    throw mockError(400, 'login.invalid_input', '手机号格式不正确')
  }
  // 开发期提示：真实通道未接入时，从控制台取码。
  console.log(`[mock] 短信验证码（固定）：%s → %s`, FIXED_SMS_CODE, mobile)
  return { code: 'sms.ok', message: 'sent', expires_in: 300 }
}
