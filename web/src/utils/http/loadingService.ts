// src\utils\http\loadingService.ts
//
// 引用计数式的全局 Loading 服务（基于 Element Plus 的 ElLoading）：
// - 第一个开启 loading 的请求创建 ElLoading.service 实例
// - 后续开启的请求只增加计数，不重复创建
// - 计数归零时关闭实例，避免多个并发请求来回闪烁 loading
//
// 之所以用引用计数：避免「请求 A 结束 → 关闭 → 请求 C 还在飞 → 没有 loading」的窗口期。
//
// 与 src\utils\loading.ts 的区别：
// - 本文件只服务于 HTTP 请求场景，使用 Element Plus 组件
// - 那个文件是首屏加载动画，使用自定义 SCSS，无业务关联
// 二者不要混用。

import { ElLoading, type LoadingInstance } from 'element-plus'

/**
 * loading 内部状态：实例 + 引用计数，统一放在一个对象里便于管理。
 */
const loadingState = {
    instance: null as LoadingInstance | null,
    count: 0,
}

/**
 * 开启 loading（请求级）
 * @param text loading 文案，默认「加载中...」
 */
export function startLoading(text: string = '加载中...'): void {
    if (loadingState.count === 0) {
        loadingState.instance = ElLoading.service({
            lock: true,
            text,
            background: 'rgba(255, 255, 255, 0.7)',
        })
    }
    loadingState.count++
}

/**
 * 关闭 loading
 *
 * 仅在引用计数归零时真正关闭实例，保证并发场景下 loading 持续可见。
 */
export function stopLoading(): void {
    if (loadingState.count > 0) {
        loadingState.count--
        if (loadingState.count === 0 && loadingState.instance) {
            loadingState.instance.close()
            loadingState.instance = null
        }
    }
}

/**
 * 测试 / 调试用：重置内部状态
 * @internal
 */
export function _resetLoadingForTest(): void {
    if (loadingState.instance) {
        loadingState.instance.close()
        loadingState.instance = null
    }
    loadingState.count = 0
}