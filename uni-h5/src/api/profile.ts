import { get } from '@/utils/request'
import type { UserInfo } from '@/types'

/**
 * 当前用户信息（含最新余额）。
 *
 * 该接口属于本期新增契约（服务端尚未实现，暂由 mock 承接）。
 * 「我的」页的余额必须走这里，而不是读登录时的快照。
 */
export function fetchProfile() {
  return get<UserInfo>('/user/profile')
}
