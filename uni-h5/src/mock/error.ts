import type { ApiError } from '@/types'

/**
 * 构造与真实错误**同构**的 ApiError。
 *
 * mock 抛出的错误必须与 request 层归一化后的错误形状完全一致，
 * 否则业务层的 catch 分支在切到真接口后会失效。
 */
export function mockError(http: number, code: string, message: string): ApiError {
  const err = new Error(message) as ApiError
  err.http = http
  err.code = code
  return err
}
