<template>
    <div :id="uuid">
        <div class="ba-click-captcha" :class="props.class">
            <div v-if="state.loading" class="loading">{{ i18n.global.t('utils.Loading') }}</div>
            <div v-else class="captcha-img-box">
                <img
                    ref="imgRef"
                    class="captcha-img"
                    @click.prevent="onRecord($event)"
                    :src="state.captcha.image"
                    :alt="i18n.global.t('validate.Captcha loading failed, please click refresh button')"
                />
                <span
                    v-for="(pt, index) in state.points"
                    :key="index"
                    class="step"
                    @click="onCancelRecord(index)"
                    :style="`left:${pt.x - 13}px;top:${pt.y - 13}px`"
                >
                    {{ index + 1 }}
                </span>
            </div>
            <div class="captcha-prompt" v-if="state.tip">
                {{ state.tip }}
            </div>
            <div v-else class="captcha-prompt">
                {{ i18n.global.t('validate.Please click') }}
                <span
                    v-for="(text, index) in state.captcha.elements"
                    :key="index"
                    :class="state.points.length > index ? 'clicaptcha-clicked' : ''"
                >
                    {{ text }}
                </span>
            </div>
            <div class="captcha-refresh-box">
                <div class="captcha-refresh-line captcha-refresh-line-l"></div>
                <i
                    class="fa fa-refresh captcha-refresh-btn"
                    :title="i18n.global.t('Refresh')"
                    @click="load"
                ></i>
                <div class="captcha-refresh-line captcha-refresh-line-r"></div>
            </div>
        </div>
        <div class="ba-layout-shade" @click="onClose"></div>
    </div>
</template>

<script setup lang="ts">
import { computed, reactive, ref } from 'vue'
import { Props } from './index'
import { checkClickCaptcha, getClickCaptcha } from '/@/api/common'
import i18n from '/@/lang'
import { SYSTEM_ZINDEX } from '/@/stores/constant/common'

const props = withDefaults(defineProps<Props>(), {
    uuid: '',
    callback: () => {},
    class: '',
    error: i18n.global.t('validate.The correct area is not clicked, please try again!'),
    success: i18n.global.t('validate.Verification is successful!'),
    apiBaseURL: '',
})

// Captcha 与 clicks 的 state —— 与后端契约对齐：
//   captcha:  { key, elements: string[], image: dataURL, width, height }
//   points:   按 elements 顺序累计的 { x, y }（图片原始像素坐标）
const state: {
    loading: boolean
    points: { x: number; y: number }[]
    tip: string
    captcha: {
        key: string
        elements: string[]
        image: string
        width: number
        height: number
    }
} = reactive({
    loading: true,
    points: [],
    tip: '',
    captcha: {
        key: '',
        elements: [],
        image: '',
        // 默认 fallback；service 真实返回后会被覆盖
        width: 350,
        height: 200,
    },
})

const imgRef = ref<HTMLImageElement | null>(null)

const emits = defineEmits<{
    (e: 'destroy'): void
}>()

const load = () => {
    state.loading = true
    state.points = []
    state.tip = ''
    getClickCaptcha(props.apiBaseURL)
        .then((res) => {
            const data = res.data || {}
            state.captcha.key = data.key || ''
            state.captcha.elements = Array.isArray(data.elements) ? data.elements : []
            state.captcha.image = data.image || ''
            state.captcha.width = data.width || 350
            state.captcha.height = data.height || 200
            state.loading = false
        })
        .catch(() => {
            state.loading = false
        })
}

/**
 * 把点击的 DOM offsetX/Y 归一化到图片原始像素坐标（naturalWidth/Height）。
 *
 * 关键：CSS `width: 350px` 与 naturalWidth=350 一致时系数为 1；
 * 但若 CSS 做了缩放，必须按比例放大到图片原始坐标，否则后端按 350×200 比对会
 * 偏离容差半径。组件 CSS 用 v-bind 引用 state.captcha.width/height，理论上 1:1，
 * 这里仍按比例计算以防后续容器尺寸调整。
 */
const normalizePoint = (event: MouseEvent): { x: number; y: number } => {
    const img = event.target as HTMLImageElement
    const rect = img.getBoundingClientRect()
    // Click X/Y 在视口 → 转回容器内的坐标；
    // 用 clientX/Y - rect.left = DOM 内坐标，与 offsetX 等价但更稳。
    const localX = event.clientX - rect.left
    const localY = event.clientY - rect.top
    const scaleX = img.naturalWidth ? img.naturalWidth / rect.width : 1
    const scaleY = img.naturalHeight ? img.naturalHeight / rect.height : 1
    return {
        x: Math.round(localX * scaleX),
        y: Math.round(localY * scaleY),
    }
}

const onRecord = (event: MouseEvent) => {
    const need = state.captcha.elements.length
    if (state.points.length >= need) {
        return
    }
    const pt = normalizePoint(event)
    state.points.push(pt)

    if (state.points.length === need) {
        checkClickCaptcha(
            {
                key: state.captcha.key,
                points: state.points,
                w: state.captcha.width,
                h: state.captcha.height,
            },
            props.apiBaseURL
        )
            .then(() => {
                state.tip = props.success
                setTimeout(() => {
                    // 回调透传 captchaKey + points —— 由调用方（如 login.vue）负责把它们带给 /admin/login。
                    // 拍一份 points 避免 setTimeout + 后续 load() 改动 state.points 时引用漂移。
                    const pointsSnapshot = state.points.map((p) => ({ x: p.x, y: p.y }))
                    props.callback?.(state.captcha.key, pointsSnapshot)
                    onClose()
                }, 1500)
            })
            .catch(() => {
                state.tip = props.error
                setTimeout(() => {
                    load()
                }, 1500)
            })
    }
}

const onCancelRecord = (index: number) => {
    state.points.splice(index, 1)
}

const onClose = () => {
    emits('destroy')
}

const captchaBoxTop = computed(() => (state.captcha.height + 200) / 2 + 'px')
const captchaBoxLeft = computed(() => (state.captcha.width + 24) / 2 + 'px')

load()
</script>

<style scoped lang="scss">
.ba-click-captcha {
    padding: 12px;
    border: 1px solid var(--el-border-color-extra-light);
    background-color: var(--el-color-white);
    position: fixed;
    z-index: v-bind('SYSTEM_ZINDEX');
    left: calc(50% - v-bind('captchaBoxLeft'));
    top: calc(50% - v-bind('captchaBoxTop'));
    border-radius: 10px;
    box-shadow:
        0 0 0 1px hsla(0, 0%, 100%, 0.3) inset,
        0 0.5em 1em rgba(0, 0, 0, 0.6);
    .loading {
        color: var(--el-color-info);
        width: 350px;
        text-align: center;
        line-height: 200px;
    }
    .captcha-img-box {
        position: relative;
        .captcha-img {
            width: v-bind('state.captcha.width') px;
            height: v-bind('state.captcha.height') px;
            border: none;
            cursor: pointer;
        }
        .step {
            box-sizing: border-box;
            position: absolute;
            width: 20px;
            height: 20px;
            line-height: 20px;
            font-size: var(--el-font-size-small);
            font-weight: bold;
            text-align: center;
            color: var(--el-color-white);
            border: 1px solid var(--el-border-color-extra-light);
            background-color: var(--el-color-primary);
            border-radius: 30px;
            box-shadow: 0 0 10px var(--el-color-white);
            user-select: none;
            cursor: pointer;
        }
    }
    .captcha-prompt {
        height: 40px;
        line-height: 40px;
        font-size: var(--el-font-size-base);
        text-align: center;
        color: var(--el-color-info);
        span {
            margin-left: 10px;
            font-size: var(--el-font-size-medium);
            font-weight: bold;
            color: var(--el-color-error);
            &.clicaptcha-clicked {
                color: var(--el-color-primary);
            }
        }
    }
    .captcha-refresh-box {
        position: relative;
        margin-top: 10px;
        .captcha-refresh-line {
            position: absolute;
            top: 16px;
            width: 140px;
            height: 1px;
            background-color: #ccc;
        }
        .captcha-refresh-line-l {
            left: 5px;
        }
        .captcha-refresh-line-r {
            right: 5px;
        }
        .captcha-refresh-btn {
            cursor: pointer;
            display: block;
            margin: 0 auto;
            width: 32px;
            height: 32px;
            font-size: 32px;
            color: var(--el-color-info);
        }
    }
}
</style>
