// src\utils\request.ts
//
// 基于 axios 的统一请求入口。
//
// 设计要点：
// - 默认导出 axios 实例本身（`import request from '/@/utils/request'`），
//   调用方可直接 `request.get<T>(url, config)` / `request.post(url, data, config)`。
// - 不在内部包一层 .then 链或 async 函数：所有增强（token / loading / dedup / 提示）
//   都通过拦截器实现，避免污染 Promise 链。
// - 业务级选项通过 config.__opts 传递，由模块声明合并提供类型。
// - 后端统一响应结构 {code, message, data} 由响应拦截器自动解包：
//   code === 0 → res.data 改写为 Response.Data；code !== 0 → 弹业务错误并 reject。

import axios, {
    AxiosError,
    AxiosHeaders,
    type AxiosInstance,
    type AxiosRequestConfig,
    type AxiosResponse,
} from 'axios'
import { ElMessage } from 'element-plus'
import { useAdminInfo } from '/@/stores/adminInfo'
import { startLoading, stopLoading } from '/@/utils/http/loadingService'
import { addPending, removePending, type DedupableConfig } from '/@/utils/http/pendingRequests'

/**
 * 业务级请求选项
 */
export interface RequestOptions {
    /** 是否显示全屏 loading，默认 false */
    showLoading?: boolean
    /** loading 文案，默认「加载中...」 */
    loadingText?: string
    /** 是否在业务错误（code !== 0）时弹出提示，默认 true */
    showErrorMessage?: boolean
    /** 是否在 HTTP / 网络错误（4xx/5xx、断网）时弹出提示，默认 true */
    showHttpErrorMessage?: boolean
    /** 是否在业务成功（code === 0）时弹出成功提示，默认 false */
    showSuccessMessage?: boolean
    /** 是否启用重复请求自动取消（按 method+url+params+data 去重），默认 false */
    dedup?: boolean
}

/**
 * 模块声明合并：把业务选项挂到 axios 的 config 上，便于调用方直接通过 config 传入。
 */
declare module 'axios' {
    export interface AxiosRequestConfig {
        /** 业务级选项（仅前端拦截器使用，不会随请求发送） */
        __opts?: RequestOptions
    }
}

/**
 * 基础 URL（不带尾斜杠处理，调用方自行决定是否拼接）
 */
export function getBaseUrl(): string {
    return import.meta.env.VITE_AXIOS_BASE_URL || ''
}

/**
 * 基础 URL（带端口号形式）。
 *
 * 例如 baseURL = `http://api.example.com:8080/v1` → 返回 `http://api.example.com:8080`，
 * 自动剥除 path 部分，仅保留 scheme + host + port。
 * 当 baseURL 未指定端口时，仅返回 scheme + host。
 */
export function getBaseUrlPort(): string {
    const raw = getBaseUrl()
    if (!raw) return ''
    try {
        const u = new URL(raw)
        const portPart = u.port ? `:${u.port}` : ''
        return `${u.protocol}//${u.hostname}${portPart}`
    } catch {
        return raw
    }
}

/**
 * 判断响应体是否为后端统一信封
 */
function isApiResponse(x: unknown): x is ApiResponse {
    return (
        typeof x === 'object' &&
        x !== null &&
        typeof (x as ApiResponse).code === 'number' &&
        'data' in (x as object) &&
        typeof (x as ApiResponse).message === 'string'
    )
}

/**
 * 统一 axios 实例。
 */
const instance: AxiosInstance = axios.create({
    baseURL: getBaseUrl(),
    timeout: 15000,
})

// =========================
// 请求拦截器：token + dedup + loading
// =========================
instance.interceptors.request.use((config) => {
    const opts = config.__opts ?? {}

    // 1) 自动携带 token
    try {
        const adminInfo = useAdminInfo()
        if (adminInfo.token) {
            if (!config.headers) {
                config.headers = new AxiosHeaders()
            } else if (!(config.headers instanceof AxiosHeaders)) {
                config.headers = new AxiosHeaders(config.headers)
            }
            config.headers.set('Authorization', `Bearer ${adminInfo.token}`)
        }
    } catch {
        // pinia 未注册 / SSR 环境静默忽略
    }

    // 2) 重复请求自动取消
    if (opts.dedup) {
        addPending(config as DedupableConfig)
    }

    // 3) 全屏 loading
    if (opts.showLoading) {
        startLoading(opts.loadingText)
    }

    return config
})

// =========================
// 响应拦截器：清理 + 信封解包 + 提示
// =========================
instance.interceptors.response.use(
    (res: AxiosResponse) => {
        const opts = res.config.__opts ?? {}

        if (opts.showLoading) {
            stopLoading()
        }
        if (opts.dedup) {
            removePending(res.config)
        }

        // 信封解包：code === 0 → 把 res.data 替换为真正的 data
        const body = res.data
        if (isApiResponse(body)) {
            if (body.code === 0) {
                if (opts.showSuccessMessage) {
                    ElMessage.success('操作成功')
                }
                res.data = body.data
                return res
            }
            // 业务错误：弹提示 + reject
            if (opts.showErrorMessage !== false) {
                ElMessage.error(body.message || '请求失败')
            }
            return Promise.reject(new Error(body.message || 'Business error'))
        }

        // 非信封响应（健康检查等）原样返回
        return res
    },
    (error: AxiosError) => {
        const config = (error.config ?? {}) as AxiosRequestConfig & { __opts?: RequestOptions }
        const opts = config.__opts ?? {}

        if (opts.showLoading) {
            stopLoading()
        }
        if (opts.dedup) {
            removePending(config)
        }

        // dedup 主动取消的请求：静默抛出，不弹任何提示
        if (error?.name === 'CanceledError' || (error as any)?.code === 'ERR_CANCELED') {
            return Promise.reject(error)
        }

        // 401：清理 token，由路由守卫 / 调用方决定跳转
        if (error.response?.status === 401) {
            try {
                useAdminInfo().removeToken()
            } catch {
                /* noop */
            }
            return Promise.reject(error)
        }

        // HTTP / 网络错误（4xx/5xx、断网、超时）：弹提示
        if (opts.showHttpErrorMessage !== false) {
            const status = error.response?.status
            const msg = status ? `请求错误 (${status})` : error.message || '网络异常'
            ElMessage.error(msg)
        }

        return Promise.reject(error)
    }
)

export default instance