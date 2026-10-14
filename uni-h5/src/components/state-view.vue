<script setup lang="ts">
/**
 * 加载 / 空 / 错误三态视图。
 *
 * 统一由页面复用，保证断网、空数据都不出现白屏。
 */

withDefaults(
  defineProps<{
    /** loading 时展示骨架文案；empty / error 时展示对应图标与引导。 */
    type?: 'loading' | 'empty' | 'error'
    title?: string
    desc?: string
    actionText?: string
  }>(),
  { type: 'empty', title: '', desc: '', actionText: '' }
)

const emit = defineEmits<{ (e: 'action'): void }>()
</script>

<template>
  <view class="state">
    <view v-if="type === 'loading'" class="state__spinner" />
    <view v-else class="state__icon" :class="`state__icon--${type}`">
      <text>{{ type === 'error' ? '!' : '—' }}</text>
    </view>

    <text v-if="title" class="state__title">{{ title }}</text>
    <text v-if="desc" class="state__desc">{{ desc }}</text>

    <view v-if="actionText" class="state__action" @tap="emit('action')">
      <text>{{ actionText }}</text>
    </view>
  </view>
</template>

<style lang="scss" scoped>
.state__spinner {
  width: 56rpx;
  height: 56rpx;
  margin-bottom: var(--space-md);
  border-radius: 50%;
  border: 4rpx solid var(--border);
  border-top-color: var(--gold);
  animation: state-spin 0.8s linear infinite;
}

@keyframes state-spin {
  to {
    transform: rotate(360deg);
  }
}

.state__icon {
  width: 88rpx;
  height: 88rpx;
  border-radius: 50%;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 44rpx;
  margin-bottom: var(--space-md);
  color: var(--dim);
  background-color: var(--surface-2);
}

.state__icon--error {
  color: var(--danger);
}
</style>
