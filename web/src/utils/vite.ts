/**
 * 判断当前 Vite 构建模式是否为生产环境
 */
export const isProd = (mode: string): boolean => {
    return mode === 'production'
}

/**
 * 自定义 Vite HMR 钩子。
 * 后续如需对 .vue / .ts 文件做精细化热更新控制，在此扩展 handleHotUpdate 即可。
 */
export const customHotUpdate = () => {
    return {
        name: 'custom-hot-update',
    }
}