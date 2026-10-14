<script setup lang="ts">
/**
 * 开卡动画（全屏遮罩）。
 *
 * 只在下单**成功后**播放；下单失败不播放（见 tasks 7.3 验证口径）。
 * 动画结束由组件自己 emit `finish`，页面据此跳转结果页 —— 把时长控制收敛在一处。
 */

import { onBeforeUnmount, watch } from 'vue'

const props = withDefaults(
  defineProps<{
    visible: boolean
    /** 播放时长，毫秒。 */
    duration?: number
    text?: string
  }>(),
  { duration: 1600, text: '' }
)

const emit = defineEmits<{ (e: 'finish'): void }>()

let timer: ReturnType<typeof setTimeout> | null = null

function clear(): void {
  if (timer) {
    clearTimeout(timer)
    timer = null
  }
}

watch(
  () => props.visible,
  (visible) => {
    clear()
    if (visible) {
      timer = setTimeout(() => {
        timer = null
        emit('finish')
      }, props.duration)
    }
  }
)

onBeforeUnmount(clear)
</script>

<template>
  <view v-if="visible" class="draw-anim">
    <view class="draw-anim__glow" />
    <view class="draw-anim__card">
      <view class="draw-anim__card-inner">
        <text class="draw-anim__logo">卡</text>
      </view>
    </view>
    <text class="draw-anim__text">{{ text || '开卡中' }}</text>
  </view>
</template>

<style lang="scss" scoped>
.draw-anim {
  position: fixed;
  inset: 0;
  z-index: 999;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  background-color: rgba(11, 13, 18, 0.94);
}

.draw-anim__glow {
  position: absolute;
  width: 520rpx;
  height: 520rpx;
  border-radius: 50%;
  background: radial-gradient(circle, rgba(245, 181, 68, 0.28), rgba(245, 181, 68, 0) 68%);
  animation: draw-pulse 1.2s ease-in-out infinite;
}

.draw-anim__card {
  width: 260rpx;
  height: 360rpx;
  border-radius: var(--radius-lg);
  background: linear-gradient(160deg, var(--surface-2), var(--surface));
  border: 2rpx solid var(--gold);
  display: flex;
  align-items: center;
  justify-content: center;
  animation: draw-flip 1.2s ease-in-out infinite;
}

.draw-anim__card-inner {
  width: 200rpx;
  height: 300rpx;
  border-radius: var(--radius-md);
  border: 1rpx dashed rgba(245, 181, 68, 0.5);
  display: flex;
  align-items: center;
  justify-content: center;
}

.draw-anim__logo {
  font-size: 72rpx;
  font-weight: 700;
  color: var(--gold);
  letter-spacing: 4rpx;
}

.draw-anim__text {
  position: relative;
  margin-top: var(--space-xl);
  font-size: 28rpx;
  color: var(--muted);
  letter-spacing: 4rpx;
}

@keyframes draw-pulse {
  0%,
  100% {
    transform: scale(0.9);
    opacity: 0.7;
  }
  50% {
    transform: scale(1.1);
    opacity: 1;
  }
}

@keyframes draw-flip {
  0% {
    transform: rotateY(0deg) scale(1);
  }
  50% {
    transform: rotateY(180deg) scale(1.06);
  }
  100% {
    transform: rotateY(360deg) scale(1);
  }
}
</style>
