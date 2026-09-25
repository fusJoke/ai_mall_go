import type { CSSProperties } from 'vue'

/**
 * 计算布局主体的可用高度样式。
 * 默认减去顶栏、标签栏等占位高度（约 120px），可在后续按布局模式做差异化计算。
 */
export const mainHeight = (): CSSProperties => {
    return {
        height: 'calc(100vh - 120px)',
    }
}