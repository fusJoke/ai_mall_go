/**
 * 登录与验证码接口。
 *
 * 本期由 mock 承接的是后 4 条（sms / slider 两条 / login-mobile）；
 * `POST /user/login` 是**真实存在**的接口，保留以备后续接入点选验证码登录。
 *
 * 注意：滑块的两条接口（`createSlider` / `verifySlider`）**本期已无调用方** ——
 * 滑块已从登录页移除，组件与接口保留备用。删除它们前请先确认不再重新接入。
 */

import { get, post } from '@/utils/request'
import type {
  LoginRequest,
  LoginResponse,
  MobileLoginRequest,
  SmsSendResponse,
  SliderCreateResponse,
  SliderVerifyResponse,
} from '@/types'

/** 发送短信验证码。 */
export function sendSms(mobile: string) {
  return post<SmsSendResponse>('/user/sms/send', { mobile }, { auth: false })
}

/** 创建滑块挑战（本期登录页未接入，保留备用）。 */
export function createSlider() {
  return get<SliderCreateResponse>('/common/captcha/slider', {}, { auth: false })
}

/** 滑块预检，不消费 key（本期登录页未接入，保留备用）。 */
export function verifySlider(payload: { key: string; x: number; w: number; h: number }) {
  return post<SliderVerifyResponse>(
    '/common/captcha/slider/verify',
    payload as unknown as Record<string, unknown>,
    { auth: false }
  )
}

/** 手机号 + 短信验证码 登录（本期 mock）。 */
export function loginMobile(payload: MobileLoginRequest) {
  return post<LoginResponse>(
    '/user/login/mobile',
    payload as unknown as Record<string, unknown>,
    { auth: false }
  )
}

/** 用户名 + 密码 + 点选验证码 登录（真实接口，供 PC 端与后续接入使用）。 */
export function loginByPassword(payload: LoginRequest) {
  return post<LoginResponse>(
    '/user/login',
    payload as unknown as Record<string, unknown>,
    { auth: false }
  )
}

/** 登出（幂等）。 */
export function logout() {
  return post<{ code: string; message: string }>('/user/logout', {}, { silentAuth: true })
}
