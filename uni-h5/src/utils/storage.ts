/**
 * `uni.setStorageSync` / `getStorageSync` 的薄封装。
 *
 * 统一 JSON 序列化，避免各调用点各自处理字符串与对象的差异。
 */

export function setItem<T>(key: string, value: T): void {
  try {
    uni.setStorageSync(key, JSON.stringify(value))
  } catch (err) {
    console.error('[storage] set failed:', key, err)
  }
}

export function getItem<T>(key: string, fallback: T): T {
  try {
    const raw = uni.getStorageSync(key)
    if (raw === '' || raw === null || raw === undefined) return fallback
    return JSON.parse(String(raw)) as T
  } catch (err) {
    console.warn('[storage] get failed, fallback used:', key, err)
    return fallback
  }
}

export function removeItem(key: string): void {
  try {
    uni.removeStorageSync(key)
  } catch (err) {
    console.warn('[storage] remove failed:', key, err)
  }
}
