// types\common.d.ts
//
// 全局通用类型声明

interface anyObj {
    [key: string]: any
}

/**
 * 后端统一响应结构。
 *
 * 对应服务端内部规划中的 internal/response/response.go：
 *
 * ```go
 * type Response struct {
 *     Code    int         `json:"code"`     // 0 = 成功；其他 = 业务错误码
 *     Message string      `json:"message"` // 人可读的错误 / 成功提示
 *     Data    interface{} `json:"data"`     // 业务载荷
 * }
 * ```
 *
 * 前端默认在响应拦截器中自动解包：
 * - code === 0 → res.data 被改写为 Response.Data，调用方按 T 直接读取
 * - code !== 0 → 触发业务错误提示（受 showErrorMessage 控制），抛出 Error(message)
 */
interface ApiResponse<T = any> {
    code: number
    message: string
    data: T
}

interface Window {
    loading: boolean
}

type Writeable<T> = { -readonly [P in keyof T]: T[P] }