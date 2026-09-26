// src\utils\http\pendingRequests.ts
//
// 自动取消重复请求：
// - 根据 method + baseURL + url + params + data 生成唯一 key
// - 同一 key 的请求未结束时，新发起的同名请求会自动 abort 前一个
// - 每个 key 对应一个 AbortController，请求完成 / 失败 / 取消时从 map 移除
//
// 调用方仅需在 axios 请求拦截器中调用 addPending(config)，
// 响应 / 错误拦截器中调用 removePending(config) 即可。

import type { AxiosRequestConfig, InternalAxiosRequestConfig } from 'axios'

/** 内部扩展 config：在拦截器链中携带 dedup key */
export interface DedupableConfig extends AxiosRequestConfig {
    /** dedup key（addPending 时写入，removePending 时读取） */
    __dedupKey?: string
}

/**
 * pending 请求的全局状态：key → AbortController 映射，统一放在一个对象里便于管理。
 */
const pendingState = {
    map: new Map<string, AbortController>(),
}

/**
 * 生成请求唯一 key
 *
 * 把 method、baseURL、url、params、data 全部纳入，
 * 保证同接口不同参数（GET 查询条件 / POST body）不会被错误合并。
 */
function makeKey(config: AxiosRequestConfig): string {
    const method = (config.method ?? 'get').toUpperCase()
    const baseURL = config.baseURL ?? ''
    const url = config.url ?? ''
    const params = config.params ? JSON.stringify(config.params) : ''
    let data = ''
    if (config.data !== undefined && config.data !== null) {
        data = typeof config.data === 'string' ? config.data : JSON.stringify(config.data)
    }
    return [method, baseURL, url, params, data].join('&')
}

/**
 * 把请求加入 pending map：
 * - 已存在同 key 请求 → abort 前一个
 * - 给当前请求挂上 AbortController.signal，并在内部扩展字段记录 key
 */
export function addPending(config: DedupableConfig): InternalAxiosRequestConfig {
    const key = makeKey(config)
    config.__dedupKey = key

    // 取消上一个同 key 请求
    const prev = pendingState.map.get(key)
    if (prev) {
        prev.abort()
        pendingState.map.delete(key)
    }

    // 给当前请求挂 abort signal
    const controller = new AbortController()
    config.signal = controller.signal
    pendingState.map.set(key, controller)

    return config as InternalAxiosRequestConfig
}

/**
 * 把请求移出 pending map（请求完成 / 失败 / 取消时调用）
 */
export function removePending(config: DedupableConfig | InternalAxiosRequestConfig): void {
    const key = (config as DedupableConfig).__dedupKey
    if (key && pendingState.map.has(key)) {
        // 仅当 controller 仍是当前请求对应的那一个时才移除
        // 避免「已被新请求 abort 并替换 controller」的旧实例误删
        const controller = pendingState.map.get(key)
        if (!controller || controller.signal === (config as any).signal) {
            pendingState.map.delete(key)
        }
    }
}

/**
 * 测试 / 调试用：当前 pending 数量
 * @internal
 */
export function _pendingCountForTest(): number {
    return pendingState.map.size
}