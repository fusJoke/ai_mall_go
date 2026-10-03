// web\src\mock\uploadDrivers.ts
// 上传驱动示例数据 —— 多驱动上传（本地 / 对象存储）前端测试用的驱动清单。
//
// 约定（对齐 buildadmin 的服务端上传驱动概念）：
// - 前端只声明「期望使用哪个驱动」，实际驱动由服务端 POST /admin/ajax/upload
//   的 `driver` 参数决定；驱动未配置时服务端回落本地存储；
// - `configured` 表示该驱动在服务端是否已完成配置（示例数据约定，
//   真实状态由服务端下发，此处仅为前端展示示例）。

export type UploadDriverValue = 'local' | 'aliyun' | 'tencent' | 'qiniu'

export interface UploadDriver {
    /** 驱动标识（随上传请求发送的 driver 参数值） */
    value: UploadDriverValue
    /** 驱动名称 */
    name: string
    /** 说明 */
    description: string
    /** 服务端是否已配置（示例数据） */
    configured: boolean
    /** 是否为默认驱动 */
    isDefault: boolean
}

export const exampleUploadDrivers: UploadDriver[] = [
    {
        value: 'local',
        name: '本地存储',
        description: '文件保存到服务器本地磁盘（当前服务端已实现的驱动）',
        configured: true,
        isDefault: true,
    },
    {
        value: 'aliyun',
        name: '阿里云 OSS',
        description: '示例数据：直传阿里云对象存储，需服务端配置 AccessKey / Bucket',
        configured: false,
        isDefault: false,
    },
    {
        value: 'tencent',
        name: '腾讯云 COS',
        description: '示例数据：直传腾讯云对象存储，需服务端配置 SecretId / Bucket',
        configured: false,
        isDefault: false,
    },
    {
        value: 'qiniu',
        name: '七牛云 Kodo',
        description: '示例数据：直传七牛对象存储，需服务端配置 AccessKey / Bucket',
        configured: false,
        isDefault: false,
    },
]
