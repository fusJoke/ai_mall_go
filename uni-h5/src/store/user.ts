import { reactive } from 'vue'

import { STORAGE_KEYS } from '@/config'
import { getItem, removeItem, setItem } from '@/utils/storage'
import type { UserInfo } from '@/types'

interface AuthState {
  token: string
  user: UserInfo | null
  /**
   * 会话是否降级为 mock-only（未配置真实令牌）。
   * 此时需鉴权接口会返回 401，界面据此给出明确提示，而不是静默失败。
   */
  degraded: boolean
}

const state = reactive<AuthState>({
  token: getItem<string>(STORAGE_KEYS.token, ''),
  user: getItem<UserInfo | null>(STORAGE_KEYS.user, null),
  degraded: false,
})

/** 只读视图；写入必须走下方 action。 */
export const authState: Readonly<AuthState> = state

export function getToken(): string {
  return state.token
}

export function isLoggedIn(): boolean {
  return !!state.token
}

export function setAuth(token: string, user: UserInfo | null): void {
  state.token = token
  state.user = user
  state.degraded = false
  setItem(STORAGE_KEYS.token, token)
  if (user) setItem(STORAGE_KEYS.user, user)
}

/** 局部更新用户信息（如「我的」页刷新余额）。无既有快照时以本次数据建档。 */
export function patchUser(partial: Partial<UserInfo>): void {
  if (state.user) {
    state.user = { ...state.user, ...partial }
  } else {
    // 仅有 token 而无用户快照的场景（如联调时只注入 `VITE_DEV_TOKEN`、未种用户缓存）：
    // 不能丢弃刚拉到的真实 profile，否则「我的」页会一直显示「未登录 / ¥0.00」。
    // id 缺失说明 partial 不足以构成有效档案，维持忽略。
    if (partial.id === undefined) return
    state.user = {
      id: partial.id,
      username: partial.username ?? '',
      nickname: partial.nickname ?? '',
      avatar: partial.avatar ?? '',
      balance: partial.balance ?? '0.00',
      status: partial.status ?? 1,
    }
  }
  setItem(STORAGE_KEYS.user, state.user)
}

export function setDegraded(value: boolean): void {
  state.degraded = value
}

export function clearAuth(): void {
  state.token = ''
  state.user = null
  state.degraded = false
  removeItem(STORAGE_KEYS.token)
  removeItem(STORAGE_KEYS.user)
}
