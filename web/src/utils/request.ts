import axios, { type AxiosRequestConfig, type AxiosResponse } from 'axios'
import { ElMessage } from 'element-plus'
import { useAdminInfo } from '/@/stores/adminInfo'

/**
 * request 的可配置行为选项
 */
export interface RequestOptions {
    /** 是否在业务错误时弹错误提示，默认 true */
    showErrorMessage?: boolean
    /** 是否在成功时弹成功提示，默认 false */
    showSuccessMessage?: boolean
    /** 自定义成功提示文案 */
    successMessage?: string
}

const instance = axios.create({
    baseURL: import.meta.env.VITE_AXIOS_BASE_URL || '',
    timeout: 15000,
})

// 请求拦截器：附带 token
instance.interceptors.request.use((config) => {
    try {
        const adminInfo = useAdminInfo()
        if (adminInfo.token) {
            config.headers = config.headers || {}
            ;(config.headers as any).Authorization = `Bearer ${adminInfo.token}`
        }
    } catch {
        // pinia 未注册时静默忽略
    }
    return config
})

// 响应拦截器：统一解出 data / 错误处理
instance.interceptors.response.use(
    (response: AxiosResponse) => response,
    (error) => {
        const status = error?.response?.status
        if (status === 401) {
            // token 失效：清理后跳转登录
            try {
                const adminInfo = useAdminInfo()
                adminInfo.removeToken()
            } catch {
                /* noop */
            }
        }
        return Promise.reject(error)
    }
)

/**
 * 统一请求入口。
 * - config：axios 标准配置（url / method / data / baseURL / headers ...）
 * - options：业务级选项（是否弹错误/成功提示）
 * 返回原始 axios Response，调用方自行取 response.data。
 */
export default function request<T = any>(
    config: AxiosRequestConfig,
    options: RequestOptions = {}
): Promise<AxiosResponse<T>> {
    const { showErrorMessage = true, showSuccessMessage = false, successMessage } = options

    return instance
        .request<T>(config)
        .then((res) => {
            if (showSuccessMessage) {
                ElMessage.success(successMessage || '操作成功')
            }
            return res
        })
        .catch((error) => {
            if (showErrorMessage) {
                const msg = error?.response?.data?.msg || error?.message || '请求失败'
                ElMessage.error(msg)
            }
            throw error
        })
}