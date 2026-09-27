import { createVNode, render } from 'vue'
import ClickCaptchaConstructor from './index.vue'
import { shortUuid } from '/@/utils/random'

interface ClickCaptchaOptions {
    // 验证码弹窗的自定义class
    class?: string
    // 验证失败的提示信息
    error?: string
    // 验证成功的提示信息
    success?: string
    // 验证码 API 的基础 URL，默认为当前服务端 URL（VITE_AXIOS_BASE_URL）
    apiBaseURL?: string
}

/**
 * 弹出点击验证码。
 *
 * 流程：
 *   - 弹窗 → GET /common/captcha/create → 用户按 elements 顺序在图片上点击 →
 *     POST /common/captcha/verify（预检，deleteOnSuccess=false）→ 成功后回调
 *     callback(captchaKey, points)，由调用方把 captchaKey + points 传给 /admin/login。
 *
 * @param uuid 开发者自定义的标识，仅用于前端引用关系，不上送后端
 * @param callback 验证成功回调；第一参是后端分配的 captchaKey（透传给登录接口），第二参是用户在图片原始 350×200 坐标系下的点击点（供登录接口二次校验）
 * @param options
 */
const clickCaptcha = (
    uuid: string,
    callback?: (captchaKey: string, points: { x: number; y: number }[]) => void,
    options: ClickCaptchaOptions = {}
) => {
    const container = document.createElement('div')
    const vnode = createVNode(ClickCaptchaConstructor, {
        uuid,
        callback,
        ...options,
        key: shortUuid(),
        onDestroy: () => {
            render(null, container)
        },
    })
    render(vnode, container)
    document.body.appendChild(container.firstElementChild!)
}

/**
 * 组件的 props 类型定义。
 *
 * callback 形参：
 *   1) captchaKey — backend 分配的 captchaKey
 *   2) points — 用户在图片原始坐标系的点击点序列（图片坐标系已归一化到 naturalWidth/Height）
 * captchaInfo 不再整体透传 —— 坐标分两份：points 单独透传用于登录二次校验。
 */
export interface Props extends ClickCaptchaOptions {
    uuid: string
    callback?: (captchaKey: string, points: { x: number; y: number }[]) => void
}

/**
 * 提交给后端做点选验证码校验的请求体（POST /common/captcha/verify）。
 */
export interface ClickRequest {
    key: string
    points: PointXY[]
    w: number
    h: number
}

export interface PointXY {
    x: number
    y: number
}

export default clickCaptcha
