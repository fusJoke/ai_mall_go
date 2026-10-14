/** mock 层专用开关与常量转发。 */

export { FIXED_SMS_CODE, MOBILE_RE, SMS_COUNTDOWN } from '@/config'

/**
 * 滑块是否严格校验真实缺口位置。
 *
 * 默认 false —— 与 `slider-captcha` spec 的「预检接口无条件通过」一致。
 * 置为 true 可在本地验证「失败复位 / 连续 3 次失败重建」分支。
 */
export const MOCK_SLIDER_STRICT = false
