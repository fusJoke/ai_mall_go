/**
 * mock 层专用常量。
 *
 * 【边界】本文件只放「真实的短信验证码是 000000」这类**开发期假值**。
 * 它不是一个通用的常量出口 —— 真实业务参数（表单校验用的 `MOBILE_RE`、
 * 倒计时 `SMS_COUNTDOWN`）仍然从 `@/config` 取，只是在这里转发一下方便 mock 模块统一引用。
 *
 * 【为什么不放在 `@/config`】迁移历史（OCR 审查 P2-5）：`FIXED_SMS_CODE` 原先定义在
 * `src/config/index.ts` 并从那里 export，于是它对**全部业务代码**可见 ——
 * 一个「固定短信验证码」常量出现在通用配置里，很容易被非 mock 路径误用
 * （例如误当成真实校验的兜底值，变成生产后门）。现在 `config/` 不再导出它。
 *
 * 【谁可以引用它】只有两类调用点，且都必须显式受 `USE_MOCK` 约束：
 *   1. mock 实现内部（`src/mock/sms.ts`、`src/mock/login.ts`）；
 *   2. 登录页面向**开发者**展示的提示文案（`pages/login/index.vue`）——
 *      它只在 `USE_MOCK === true` 时才渲染，用来告诉开发者该填什么。
 * 第 2 类不是 mock 模块，所以上面的边界表述是「不得用于真实校验路径」而非
 * 「非 mock 文件一律不得 import」。
 */

export { MOBILE_RE, SMS_COUNTDOWN } from '@/config'

/**
 * 开发期固定短信验证码。
 *
 * 生效范围严格限定在：mock 的短信/登录接口（`src/mock/sms.ts`、`src/mock/login.ts`）
 * 与登录页在 `USE_MOCK=true` 下渲染的开发者提示。真实后端不认这个值。
 * 生产包不会包含 mock 路径（构建期门禁 + `assertProductionHygiene()` 双重保证），
 * 因此它不会成为线上可用的「后门验证码」。
 */
export const FIXED_SMS_CODE = '000000'

/**
 * 滑块是否严格校验真实缺口位置。
 *
 * 默认 false —— 与 `slider-captcha` spec 的「预检接口无条件通过」一致。
 * 置为 true 可在本地验证「失败复位 / 连续 3 次失败重建」分支。
 */
export const MOCK_SLIDER_STRICT = false
