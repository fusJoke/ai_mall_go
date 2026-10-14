<script setup lang="ts">
/**
 * 滑块验证码。
 *
 * 交互（design.md D8）：拖动 → 松手**自动预检**（无需再点确认）→ 通过则向上抛出
 * `{key, x, w, h}` 供登录接口做消费型二次校验；失败复位；连续 3 次失败自动重建图片。
 *
 * 坐标系：创建时给出的是**原图坐标**（w×h），界面按显示宽度等比缩放，
 * 提交前换算回原图坐标 —— 与 `slider-captcha` spec 的「w/h 必须与创建时一致，
 * 不得按缩放折算」一致。
 */

import { computed, getCurrentInstance, nextTick, onBeforeUnmount, onMounted, ref } from 'vue'

import { createSlider, verifySlider } from '@/api/auth'
import type { SliderCreateResponse } from '@/types'

const emit = defineEmits<{
  (e: 'success', payload: { key: string; x: number; w: number; h: number }): void
  (e: 'fail'): void
}>()

/** 连续失败达到该次数即重建图片。 */
const MAX_FAIL = 3

/** 拼图块宽度缺省占比（服务端未返回 block_w 时的视觉退化值）。 */
const FALLBACK_BLOCK_RATIO = 0.16

const challenge = ref<SliderCreateResponse | null>(null)
const loading = ref(false)
const verified = ref(false)
const checking = ref(false)
const dragging = ref(false)

/** 手柄像素位移。 */
const handleX = ref(0)
/** 图片显示宽度（测出后填充）。 */
const trackWidth = ref(0)

let failCount = 0
let dragStart = 0
let resizeTimer: ReturnType<typeof setTimeout> | null = null

/** 视口变化后重新量宽（桌面浏览器拖拽窗口、旋转屏幕等场景）。 */
function onWindowResize(): void {
  if (resizeTimer) clearTimeout(resizeTimer)
  resizeTimer = setTimeout(() => {
    resizeTimer = null
    measure()
  }, 150)
}

const scale = computed(() => {
  const c = challenge.value
  if (!c || !trackWidth.value) return 1
  return trackWidth.value / c.w
})

/** 图片显示高度，由原图比例推出。 */
const imageHeight = computed(() => {
  const c = challenge.value
  if (!c || !trackWidth.value) return 0
  return (c.h / c.w) * trackWidth.value
})

const blockWidth = computed(() => {
  const c = challenge.value
  if (!c) return 0
  return c.block_w ? c.block_w * scale.value : trackWidth.value * FALLBACK_BLOCK_RATIO
})

const blockHeight = computed(() => {
  const c = challenge.value
  if (!c) return 0
  return c.block_h ? c.block_h * scale.value : blockWidth.value
})

/** 手柄最大可拖距离。 */
const maxHandle = computed(() => Math.max(0, trackWidth.value - blockWidth.value))

/** 拼图块在图片内的位置。 */
const blockStyle = computed(() => {
  const c = challenge.value
  if (!c) return {}
  return {
    width: `${blockWidth.value}px`,
    height: `${blockHeight.value}px`,
    left: `${handleX.value}px`,
    top: `${(c.y || 0) * scale.value}px`,
  }
})

const barFillStyle = computed(() => ({
  width: `${handleX.value + blockWidth.value / 2}px`,
}))

function measure(): void {
  const instance = getCurrentInstance()
  uni.createSelectorQuery()
    .in(instance?.proxy as never)
    .select('.slider__track')
    .boundingClientRect((rect) => {
      const r = rect as { width?: number } | null
      if (r && r.width) trackWidth.value = r.width
    })
    .exec()
}

async function refresh(): Promise<void> {
  loading.value = true
  verified.value = false
  handleX.value = 0
  try {
    challenge.value = await createSlider()
    await nextTick()
    // 图片以 data URI 内联，无需等 onload；下一帧即可量宽。
    setTimeout(measure, 0)
  } catch (err) {
    console.error('[slider] create failed:', err)
  } finally {
    loading.value = false
  }
}

function onTouchStart(e: TouchEvent): void {
  if (verified.value || checking.value || !challenge.value) return
  dragging.value = true
  dragStart = e.touches[0].clientX - handleX.value
}

function onTouchMove(e: TouchEvent): void {
  if (!dragging.value) return
  const next = e.touches[0].clientX - dragStart
  handleX.value = Math.min(Math.max(0, next), maxHandle.value)
}

async function onTouchEnd(): Promise<void> {
  if (!dragging.value) return
  dragging.value = false
  await submit()
}

async function submit(): Promise<void> {
  const c = challenge.value
  if (!c || !trackWidth.value || checking.value) return

  checking.value = true
  // 换算回原图坐标提交。
  const nativeX = Math.round(handleX.value / scale.value)

  try {
    await verifySlider({ key: c.key, x: nativeX, w: c.w, h: c.h })
    verified.value = true
    failCount = 0
    emit('success', { key: c.key, x: nativeX, w: c.w, h: c.h })
  } catch (err) {
    console.warn('[slider] verify failed:', err)
    failCount += 1
    emit('fail')
    if (failCount >= MAX_FAIL) {
      failCount = 0
      await refresh()
    } else {
      handleX.value = 0
    }
  } finally {
    checking.value = false
  }
}

/** 供父组件在登录失败（如验证码失效）时重置。 */
function reset(): void {
  failCount = 0
  verified.value = false
  void refresh()
}

defineExpose({ reset })

onMounted(() => {
  void refresh()
  uni.onWindowResize?.(onWindowResize)
})

onBeforeUnmount(() => {
  uni.offWindowResize?.(onWindowResize)
  if (resizeTimer) clearTimeout(resizeTimer)
})
</script>

<template>
  <view class="slider">
    <view class="slider__track" :style="{ height: imageHeight ? imageHeight + 'px' : '300rpx' }">
      <image
        v-if="challenge"
        class="slider__bg"
        :src="challenge.image"
        mode="scaleToFill"
        @load="measure"
      />
      <view v-if="challenge" class="slider__block" :style="blockStyle">
        <image class="slider__block-img" :src="challenge.block" mode="scaleToFill" />
      </view>

      <view v-if="loading" class="slider__mask">
        <text class="slider__mask-text">加载中</text>
      </view>
    </view>

    <view
      class="slider__bar"
      :class="{ 'slider__bar--done': verified }"
      @touchstart="onTouchStart"
      @touchmove.stop.prevent="onTouchMove"
      @touchend="onTouchEnd"
      @touchcancel="onTouchEnd"
    >
      <view class="slider__fill" :style="barFillStyle" />
      <text class="slider__hint">
        {{ verified ? '验证通过' : '拖动滑块完成拼图' }}
      </text>
      <view class="slider__handle" :style="{ left: handleX + 'px', width: blockWidth + 'px' }">
        <text class="slider__handle-icon">{{ verified ? '✓' : '»' }}</text>
      </view>
    </view>
  </view>
</template>

<style lang="scss" scoped>
.slider {
  width: 100%;
}

.slider__track {
  position: relative;
  width: 100%;
  border-radius: var(--radius-md);
  overflow: hidden;
  background-color: var(--surface-2);
}

.slider__bg {
  width: 100%;
  height: 100%;
}

.slider__block {
  position: absolute;
  left: 0;
  top: 0;
}

.slider__block-img {
  width: 100%;
  height: 100%;
}

.slider__mask {
  position: absolute;
  inset: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  background-color: rgba(11, 13, 18, 0.72);
}

.slider__mask-text {
  font-size: 26rpx;
  color: var(--muted);
}

.slider__bar {
  position: relative;
  height: 88rpx;
  margin-top: var(--space-sm);
  border-radius: var(--radius-pill);
  background-color: var(--surface-2);
  border: 1rpx solid var(--border);
  overflow: hidden;
  display: flex;
  align-items: center;
  justify-content: center;
}

.slider__bar--done {
  border-color: var(--gold);
}

.slider__fill {
  position: absolute;
  left: 0;
  top: 0;
  bottom: 0;
  background-color: rgba(245, 181, 68, 0.16);
}

.slider__hint {
  position: relative;
  z-index: 2;
  font-size: 26rpx;
  color: var(--muted);
  text-shadow: 0 0 8rpx rgba(11, 13, 18, 0.8);
}

.slider__handle {
  position: absolute;
  left: 0;
  top: 0;
  bottom: 0;
  min-width: 88rpx;
  display: flex;
  align-items: center;
  justify-content: center;
  background: linear-gradient(135deg, var(--gold-2), var(--gold));
  border-radius: var(--radius-pill);
}

.slider__handle-icon {
  color: var(--bg);
  font-size: 34rpx;
  font-weight: 700;
  line-height: 1;
}
</style>
