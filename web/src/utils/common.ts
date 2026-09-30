import { camelCase, snakeCase, trimStart } from 'lodash-es'
import { getCurrentInstance } from 'vue'
import { RouteLocationNormalized, RouteRecordRaw } from 'vue-router'
import router from '/@/router/index'
import { adminBaseRoutePath } from '/@/router/static/adminBase'
import { useMenu } from '/@/stores/menu'
import { getBaseUrlPort } from '/@/utils/request'
import { useConfig } from '/@/stores/config'

/**
 * 获取 globalProperties 对象
 */
export const getGlobalProperties = () => {
    if (!getCurrentInstance()) {
        throw new Error('getGlobalProperties() can only be used inside setup() or functional components!')
    }
    const instance = getCurrentInstance()
    if (instance) {
        const { appContext } = instance
        return appContext.config.globalProperties
    } else {
        return null
    }
}

/**
 * 复制文本到剪贴板
 * @param text 要复制的文本
 * @returns 复制成功返回 true，失败返回 false
 */
export async function copy(text: string): Promise<boolean> {
    try {
        await navigator.clipboard.writeText(text)
        return true
    } catch {
        // 降级方案：使用 execCommand（兼容旧浏览器或非 HTTPS 环境）
        const textarea = document.createElement('textarea')
        textarea.value = text
        textarea.style.position = 'fixed'
        textarea.style.left = '-9999px'
        textarea.style.top = '-9999px'
        document.body.appendChild(textarea)
        textarea.focus()
        textarea.select()
        try {
            document.execCommand('copy')
            return true
        } catch {
            return false
        } finally {
            document.body.removeChild(textarea)
        }
    }
}

/**
 * 获取路由 path
 */
export const getCurrentRoutePath = () => {
    let path = router.currentRoute.value.path
    if (path == '/') path = trimStart(window.location.hash, '#')
    if (path.indexOf('?') !== -1) path = path.replace(/\?.*/, '')
    return path
}

/**
 * 是否在后台应用内
 * @param path 不传递则通过当前路由 path 检查
 */
export const isAdminApp = (path = '') => {
    const regex = new RegExp(`^${adminBaseRoutePath}`)
    if (path) {
        return regex.test(path)
    }
    if (regex.test(getCurrentRoutePath())) {
        return true
    }
    return false
}

/**
 * 递归的寻找路由路径在菜单中的数据
 * @param path 路由路径
 * @param menus 菜单数据（只有 path 代表完整 url，没有 fullPath）
 * @param returnType 返回值要求:normal=返回被搜索的路径对应的菜单数据,above=返回被搜索的路径对应的上一级菜单数组
 */
export const getMenuDataByPath = (path: string, menus: RouteRecordRaw[], returnType: 'normal' | 'above'): RouteRecordRaw | false => {
    for (const key in menus) {
        // 找到目标
        if (menus[key].path === path) {
            return menus[key]
        }
        // 从子级继续寻找
        if (menus[key].children && menus[key].children.length) {
            const find = getMenuDataByPath(path, menus[key].children, returnType)
            if (find) {
                return returnType == 'above' ? menus[key] : find
            }
        }
    }
    return false
}

/**
 * 寻找路由在菜单中的数据
 * @param route 路由
 * @param returnType 返回值要求:normal=返回被搜索的路径对应的菜单数据,above=返回被搜索的路径对应的上一级菜单数组
 */
export const getMenuDataByRoute = (
    route: RouteLocationNormalized | RouteRecordRaw,
    returnType: 'normal' | 'above' = 'normal'
): RouteRecordRaw | false => {
    const menu = useMenu()
    let found: RouteRecordRaw | false = false
    const fullPath = (route as RouteLocationNormalized).fullPath
    if (fullPath) {
        // 以完整路径寻找
        found = getMenuDataByPath(fullPath, menu.rawData, returnType)
        if (found) {
            found.meta!.matched = fullPath
            return found
        }
    }

    // 以路径寻找
    found = getMenuDataByPath(route.path, menu.rawData, returnType)
    if (found) {
        found.meta!.matched = route.path
        return found
    }

    return false
}

/**
 * 递归将对象 key 从 snake_case 转为 camelCase
 */
export function keysToCamelCase(obj: any): any {
    if (Array.isArray(obj)) {
        return obj.map((item) => keysToCamelCase(item))
    }
    if (obj !== null && typeof obj === 'object') {
        const result: Record<string, any> = {}
        for (const key of Object.keys(obj)) {
            result[camelCase(key)] = keysToCamelCase(obj[key])
        }
        return result
    }
    return obj
}

/**
 * 递归将对象 key 从 camelCase 转为 snake_case
 */
export function keysToSnakeCase(obj: any): any {
    if (Array.isArray(obj)) {
        return obj.map((item) => keysToSnakeCase(item))
    }
    if (obj !== null && typeof obj === 'object') {
        const result: Record<string, any> = {}
        for (const key of Object.keys(obj)) {
            result[snakeCase(key)] = keysToSnakeCase(obj[key])
        }
        return result
    }
    return obj
}

/**
 * 判断路径是否为外部链接（http/https/mailto/tel 等协议）
 */
export const isExternal = (path: string): boolean => {
    return /^(https?:|mailto:|tel:)/.test(path)
}

/**
 * 把资源路径拼成完整 URL。与后端 kit/urlx.FullURL 行为对齐。
 *
 * 分支：
 *  1. 空串 → ""
 *  2. base64 data URI → 原样
 *  3. http:// / https:// 绝对地址 → 原样
 *  4. useConfig().cdnUrl 非空 → cdnUrl + (ensureLeadingSlash cdnUrlParams) + resource
 *  5. 否则 → getBaseUrlPort() + resource
 */
export const fullURL = (resource: string): string => {
    if (!resource) return ''
    if (resource.startsWith('data:')) return resource
    if (resource.startsWith('http://') || resource.startsWith('https://')) return resource

    const cfg = useConfig()
    if (cfg.cdnUrl) {
        const params = cfg.cdnUrlParams
        const sep = params && !params.startsWith('/') ? '/' : ''
        return cfg.cdnUrl + sep + params + resource
    }

    return getBaseUrlPort() + resource
}

/**
 * fullURL 的批量版：对数组内每个元素逐个走 fullURL。
 * 用于 agUpload 等组件一次性返回多个完整 URL。
 */
export const fullURLArray = (arr: string[]): string[] => {
    return arr.map((s) => fullURL(s))
}

/**
 * 把字符串 / 数组统一为字符串数组：
 *   - 数组 → 原样（去掉空元素）
 *   - 字符串 → 按逗号拆（去掉空元素）
 *   - 其它 → []
 */
export const stringToArray = (val: string | string[]): string[] => {
    if (Array.isArray(val)) return val.filter((s) => s !== '')
    if (typeof val === 'string') {
        return val
            .split(',')
            .map((s) => s.trim())
            .filter((s) => s !== '')
    }
    return []
}

/**
 * 从 URL / 路径中提取文件名（去掉 query / hash / 目录）。
 */
export const getFileNameFromPath = (path: string): string => {
    if (!path) return ''
    // 去掉 query 和 hash
    let cleanPath = path.split('?')[0].split('#')[0]
    // 取最后一段
    const parts = cleanPath.split('/')
    return parts[parts.length - 1] || ''
}

/**
 * 在对象数组 arr 中按 key/value 查找下标，未命中返回 false。
 */
export const getArrayKey = (arr: anyObj[], key: string, value: any): number | false => {
    for (let i = 0; i < arr.length; i++) {
        if (arr[i] && arr[i][key] === value) return i
    }
    return false
}