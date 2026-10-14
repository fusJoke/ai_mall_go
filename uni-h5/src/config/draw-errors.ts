/**
 * 抽卡错误码 → 文案与动作的映射。
 *
 * 后端把 8 类失败全部归一为 `{code, message}`，其中 `message` 恒为 "draw failed"，
 * 对用户无意义，因此**必须**由客户端映射（见 `uni-h5-client` spec）。
 * 文案直接内联，动作决定界面后续行为。
 */

import type { ApiError } from '@/types'

/** 错误发生后客户端要做的事。 */
export type DrawErrorAction =
  | 'toast' // 仅提示
  | 'login' // 跳登录页并携带回跳
  | 'balance' // 提示余额不足并引导至「我的」
  | 'home' // 提示后退回首页
  | 'refresh-stock' // 提示售罄并重新拉取详情
  | 'rate-limit' // 禁用按钮 retryAfter 秒
  | 'retry' // 通用失败，允许重试

export interface DrawErrorRule {
  http: number
  code: string
  action: DrawErrorAction
  /** 面向用户的提示文案。 */
  message: string
}

/** 8 条规则，顺序无关。 */
export const DRAW_ERROR_RULES: DrawErrorRule[] = [
  { http: 400, code: 'draw.invalid_input', action: 'toast', message: '请求参数异常，请重试' },
  { http: 401, code: 'draw.unauthorized', action: 'login', message: '登录已失效，请重新登录' },
  {
    http: 402,
    code: 'draw.insufficient_balance',
    action: 'balance',
    message: '余额不足，请先充值',
  },
  {
    http: 403,
    code: 'draw.user_not_available',
    action: 'home',
    message: '账号状态异常，请联系管理员',
  },
  {
    http: 404,
    code: 'draw.blindbox_not_available',
    action: 'home',
    message: '该商品已下架',
  },
  { http: 409, code: 'draw.sold_out', action: 'refresh-stock', message: '该盲盒已售罄，库存已刷新' },
  { http: 429, code: 'rate_limited', action: 'rate-limit', message: '操作过于频繁，请稍后再试' },
  { http: 500, code: 'draw.internal', action: 'retry', message: '开卡失败，请重试' },
]

/** 按 code 优先、http 兜底匹配规则；未命中返回通用重试规则。 */
export function resolveDrawError(err: ApiError): DrawErrorRule {
  const byCode = DRAW_ERROR_RULES.find((rule) => rule.code === err.code)
  if (byCode) return byCode
  const byHttp = DRAW_ERROR_RULES.find((rule) => rule.http === err.http)
  if (byHttp) return byHttp
  return { http: err.http || 0, code: err.code || 'unknown', action: 'retry', message: '开卡失败，请重试' }
}
