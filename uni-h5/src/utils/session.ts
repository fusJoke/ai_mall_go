/**
 * 会话与真实令牌。
 *
 * 背景（design.md D4'）：后端 `POST /user/login` 强制「点选验证码」消费型校验，
 * 答案只存服务端、响应不返回，客户端**无法**在 mock 模式下静默换取真 token。
 * 因此改为：开发者通过 `.env.local` 注入一个真实令牌（`VITE_DEV_TOKEN`），
 * 使需鉴权接口真正打到库里（真扣库存、真出订单）。
 *
 * 未注入时：会话降级为 mock-only，界面明确提示，不静默失败。
 */

import { DEV_TOKEN, STORAGE_KEYS } from '@/config'
import { request } from '@/utils/request'
import { authState, setAuth, setDegraded } from '@/store/user'
import { getItem, setItem } from '@/utils/storage'
import type { UserInfo } from '@/types'

/** 用注入的真实令牌替换 mock 会话；返回是否替换成功。 */
export function applyDevToken(fallbackUser: UserInfo | null): boolean {
  if (!DEV_TOKEN) {
    setDegraded(true)
    return false
  }
  setAuth(DEV_TOKEN, fallbackUser)
  return true
}

/**
 * 探测当前令牌是否被后端接受。
 *
 * 用最轻的需鉴权接口探活，不产生副作用。`silentAuth` 保证 401 时不跳登录页，
 * 由调用方决定提示方式。
 */
export async function probeToken(): Promise<boolean> {
  if (!authState.token) return false
  try {
    await request({
      url: '/user/orders/list',
      method: 'GET',
      data: { page: 1, page_size: 1 },
      silentAuth: true,
    })
    return true
  } catch {
    return false
  }
}

/** 读取本地缓存的用户信息（供降级场景展示）。 */
export function cachedUser(): UserInfo | null {
  return getItem<UserInfo | null>(STORAGE_KEYS.user, null)
}

/** 更新缓存的用户信息。 */
export function cacheUser(user: UserInfo): void {
  setItem(STORAGE_KEYS.user, user)
}
