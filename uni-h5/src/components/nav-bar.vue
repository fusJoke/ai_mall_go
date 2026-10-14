<script setup lang="ts">
/**
 * 自定义导航栏。
 *
 * pages.json 中所有页面均为 `navigationStyle: custom`，导航栏由本组件承担。
 * 采用**文档流内**布局（非 fixed），避免 H5 端吸顶抖动与高度占位偏差。
 */

withDefaults(
  defineProps<{
    title?: string
    showBack?: boolean
  }>(),
  { title: '', showBack: false }
)

/** H5 端通常为 0；真机浏览器可能有刘海区域。 */
const statusBarHeight = (() => {
  try {
    return uni.getSystemInfoSync().statusBarHeight || 0
  } catch {
    return 0
  }
})()

function goBack(): void {
  const pages = getCurrentPages()
  if (pages.length > 1) {
    uni.navigateBack()
    return
  }
  uni.switchTab({ url: '/pages/home/index' })
}
</script>

<template>
  <view class="nav">
    <view v-if="statusBarHeight > 0" :style="{ height: statusBarHeight + 'px' }" />
    <view class="nav__bar">
      <view v-if="showBack" class="nav__back" @tap="goBack">
        <text class="nav__back-icon">‹</text>
      </view>
      <text class="nav__title ellipsis">{{ title }}</text>
      <view class="nav__right">
        <slot name="right" />
      </view>
    </view>
  </view>
</template>

<style lang="scss" scoped>
.nav {
  background-color: var(--bg);
}

.nav__bar {
  height: 88rpx;
  display: flex;
  align-items: center;
  padding: 0 var(--space-md);
}

.nav__back {
  width: 64rpx;
  height: 64rpx;
  display: flex;
  align-items: center;
  justify-content: center;
  margin-left: calc(var(--space-md) * -1);
}

.nav__back-icon {
  font-size: 56rpx;
  line-height: 1;
  color: var(--text);
  margin-top: -6rpx;
}

.nav__title {
  flex: 1;
  font-size: 34rpx;
  font-weight: 600;
  color: var(--text);
}

.nav__right {
  display: flex;
  align-items: center;
}
</style>
