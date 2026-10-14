/**
 * 抽卡结果中转。
 *
 * 结果数据不走 URL 参数（卡牌数组体积大），改用本地存储一次性传递：
 * 写入 → 跳转 → 结果页读取后立即清空，避免回退时重复消费。
 *
 * 同时保留 `blindBoxId` —— 结果接口不返回盲盒 ID，而「再拆一盒」需要它。
 */

import { STORAGE_KEYS } from '@/config'
import { getItem, removeItem, setItem } from '@/utils/storage'
import type { DrawResult } from '@/types'

export interface DrawPayload {
  result: DrawResult
  blindBoxId: number
}

export function setDrawResult(payload: DrawPayload): void {
  setItem(STORAGE_KEYS.drawResult, payload)
}

/** 读取并清空；无数据返回 null。 */
export function consumeDrawResult(): DrawPayload | null {
  const payload = getItem<DrawPayload | null>(STORAGE_KEYS.drawResult, null)
  if (payload) removeItem(STORAGE_KEYS.drawResult)
  return payload
}
