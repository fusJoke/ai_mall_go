<script lang="ts">
import { createVNode, createCommentVNode, resolveComponent, defineComponent, computed, type CSSProperties } from 'vue'
import { Sparkles, Eye, EyeOff, Mail } from '@lucide/vue'
import Svg from '/@/components/icon/svg/index.vue'
import { isExternal } from '/@/utils/common'

// lucide 图标按名字查表。静态 import 让 bundler 能 tree-shake 未被引用的图标。
// 新增 lucide 图标：在这里追加一行 import + map 登记即可。
const lucideIcons: Record<string, any> = {
    Sparkles,
    Eye,
    EyeOff,
    Mail,
}

export default defineComponent({
    name: 'Icon',
    props: {
        name: {
            type: String,
            required: true,
        },
        size: {
            type: [String, Number],
            default: '18px',
        },
        color: {
            type: String,
            default: undefined,
        },
    },
    setup(props) {
        // 把 size 归一为 number，便于 CSS fontSize 与 lucide size prop 共用
        const numSize = (): number => {
            if (typeof props.size === 'number') return props.size
            const n = parseInt(props.size, 10)
            return Number.isFinite(n) ? n : 18
        }
        const iconStyle = computed((): CSSProperties => {
            const style: CSSProperties = { fontSize: `${numSize()}px` }
            if (props.color) style.color = props.color
            return style
        })

        if (props.name.indexOf('lucide:') === 0) {
            const iconName = props.name.slice('lucide:'.length)
            const lucideComponent = lucideIcons[iconName]
            return () => {
                if (lucideComponent) {
                    // color 仅在调用方显式传入时转发 — 让 lucide 默认走 CSS currentColor 继承
                    const lucideProps: Record<string, any> = { size: numSize() }
                    if (props.color) lucideProps.color = props.color
                    return createVNode(lucideComponent, lucideProps)
                }
                return createCommentVNode(`lucide icon not found: ${iconName}`)
            }
        } else if (props.name.indexOf('el-icon-') === 0) {
            return () => createVNode('el-icon', { class: 'icon el-icon', style: iconStyle.value }, [createVNode(resolveComponent(props.name))])
        } else if (props.name.indexOf('local-') === 0 || isExternal(props.name)) {
            // Svg 子组件仍要求 string size
            const sizeStr = typeof props.size === 'number' ? `${props.size}px` : props.size
            return () => createVNode(Svg, { name: props.name, size: sizeStr, color: props.color })
        } else {
            return () => createVNode('i', { class: [props.name, 'icon'], style: iconStyle.value })
        }
    },
})
</script>
