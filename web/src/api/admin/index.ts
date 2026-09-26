import type { ClickRequest } from '/@/components/clickCaptcha/index'
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