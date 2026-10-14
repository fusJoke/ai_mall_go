/**
 * 请求层。
 *
 * 设计要点（见 design.md D3）：
 * - 唯一使用 `uni.request`，保证跨端可用（axios 依赖 XMLHttpRequest，小程序端不可用）。
 * - 自动注入 `Authorization: Bearer`，调用点不再各自拼头部。
 * - 成功直接 resolve 裸 JSON；失败统一抛 `ApiError { http, code, message, retryAfter }`。
 * - 401 统一拦截：清本地会话并跳登录页（携带回跳地址）。
 * - 429 统一提取 `retryAfter`，供调用方禁用按钮。
 * - mock 命中时短路，不发网络请求。
 */

import { BASE_URL, LOGIN_PATH, REQUEST_TIMEOUT } from '@/config'
import { clearAuth, getToken } from '@/store/user'
import { matchMock } from '@/mock'
import type { ApiError, ApiErrorShape, HttpMethod } from '@/types'

export interface RequestOptions {
  /** 去除 BASE_URL 后的路径，如 `/user/blindbox/detail`。 */
  url: string
  method?: HttpMethod
  data?: Record<string, unknown>
  /** 是否注入 Authorization，默认 true。 */
  auth?: boolean
  /** 401 时是否静默处理（不自动跳登录页）。 */
  silentAuth?: boolean
  header?: Record<string, string>
}

/** 防止并发 401 触发多次跳转。 */
let redirecting = false

/** 取当前页面的完整站内路径，作为登录后的回跳地址。 */
export function currentFullPath(): string {
  try {
    const pages = getCurrentPages()
    const cur = pages[pages.length - 1] as unknown as {
      route?: string
      options?: Record<string, string>
    }
    if (!cur?.route) return ''
    const query = cur.options
      ? Object.entries(cur.options)
          .map(([k, v]) => `${encodeURIComponent(k)}=${encodeURIComponent(String(v))}`)
          .join('&')
      : ''
    return `/${cur.route}${query ? `?${query}` : ''}`
  } catch {
    return ''
  }
}

/** 跳转登录页并携带回跳地址。 */
function redirectToLogin(): void {
  if (redirecting) return
  const redirect = currentFullPath()
  if (redirect.startsWith(LOGIN_PATH)) return
  redirecting = true
  const suffix = redirect ? `?redirect=${encodeURIComponent(redirect)}` : ''
  uni.navigateTo({
    url: `${LOGIN_PATH}${suffix}`,
    complete: () => {
      setTimeout(() => {
        redirecting = false
      }, 800)
    },
  })
}

/** 把 HTTP 状态码与响应体归一为 `ApiError`。 */
function buildError(http: number, payload: unknown, retryAfterHeader?: unknown): ApiError {
  const shape = (payload || {}) as ApiErrorShape
  const err = new Error(shape.message || `请求失败(${http})`) as ApiError
  err.http = http
  err.code = shape.code || `http.${http}`
  if (http === 429) {
    const parsed = Number(retryAfterHeader)
    err.retryAfter = Number.isFinite(parsed) && parsed > 0 ? parsed : 3
  }
  return err
}

export function request<T = unknown>(options: RequestOptions): Promise<T> {
  const method = (options.method || 'GET').toUpperCase() as HttpMethod

  // mock 短路：命中则不发网络请求。
  const mocked = matchMock(method, options.url)
  if (mocked) {
    return Promise.resolve(mocked(options.data)).then((res) => res as T)
  }

  return new Promise<T>((resolve, reject) => {
    const header: Record<string, string> = {
      'Content-Type': 'application/json',
      ...(options.header || {}),
    }

    if (options.auth !== false) {
      const token = getToken()
      if (token) header.Authorization = `Bearer ${token}`
    }

    uni.request({
      url: `${BASE_URL}${options.url}`,
      method,
      data: options.data,
      header,
      timeout: REQUEST_TIMEOUT,
      success: (res) => {
        const status = res.statusCode
        if (status >= 200 && status < 300) {
          resolve(res.data as T)
          return
        }

        const err = buildError(status, res.data, (res.header || {})['Retry-After'])

        if (status === 401 && options.auth !== false && !options.silentAuth) {
          clearAuth()
          redirectToLogin()
        }
        reject(err)
      },
      fail: (err) => {
        const apiErr = new Error(
          (err && (err as { errMsg?: string }).errMsg) || '网络异常，请稍后重试'
        ) as ApiError
        apiErr.http = 0
        apiErr.code = 'network.error'
        reject(apiErr)
      },
    })
  })
}

/** 类型安全的 GET 快捷方式。 */
export function get<T>(url: string, data?: Record<string, unknown>, options?: Partial<RequestOptions>) {
  return request<T>({ url, method: 'GET', data, ...options })
}

/** 类型安全的 POST 快捷方式。 */
export function post<T>(url: string, data?: Record<string, unknown>, options?: Partial<RequestOptions>) {
  return request<T>({ url, method: 'POST', data, ...options })
}
