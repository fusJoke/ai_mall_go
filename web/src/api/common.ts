import type { ClickRequest } from '/@/components/clickCaptcha/index'
import type { AxiosRequestConfig } from 'axios'
import request from '/@/utils/request'

/**
 * 获取点选验证码
 */
export function getClickCaptcha(apiBaseURL?: string) {
    return request.request({
        url: '/common/captcha/create',
        method: 'GET',
        ...(apiBaseURL ? { baseURL: apiBaseURL } : {}),
    })
}

/**
 * 校验点选验证码
 */
export function checkClickCaptcha(data: ClickRequest, apiBaseURL?: string) {
    return request.request({
        url: '/common/captcha/verify',
        method: 'POST',
        data,
        __opts: { showErrorMessage: false },
        ...(apiBaseURL ? { baseURL: apiBaseURL } : {}),
    })
}

/**
 * 缓存清理接口
 */
export function clearCache(type: string) {
    return request.request({
        url: '/admin/clear-cache',
        method: 'POST',
        data: { type },
        __opts: { showSuccessMessage: true },
    })
}

/**
 * 通用文件上传（POST /admin/ajax/upload）。
 *
 * - `fd` 内部应包含 `file` 字段（以及可选 `topic` 等业务字段，取决于后端）；
 *   `params` 会拼到 query string，例如 `?topic=admin`。
 * - 默认不弹业务 / HTTP 错误消息：上传失败由调用方决定如何展示
 *   （el-upload 的 onError 已经处理 toast，避免重复弹）。
 */
export function fileUpload(
    fd: FormData,
    params: anyObj = {},
    config: AxiosRequestConfig = {}
): Promise<ApiResponse<{ file: { url: string; stored_path?: string; size?: number; suffix?: string } }>> {
    return request.request({
        url: '/admin/ajax/upload',
        method: 'POST',
        data: fd,
        params,
        ...config,
        __opts: {
            showErrorMessage: false,
            showHttpErrorMessage: false,
            ...(config.__opts ?? {}),
        },
    }) as any
}