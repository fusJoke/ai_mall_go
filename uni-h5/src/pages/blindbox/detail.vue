<script setup lang="ts">
/**
 * P2 商品详情。
 *
 * 游客可完整浏览（不做登录预检），只在点击「立即拆卡」时检查登录态 ——
 * 保证浏览不被拦截，拦截只发生在真正的写操作上。
 */

import { computed, onUnmounted, ref } from 'vue'
import { onLoad } from '@dcloudio/uni-app'

import CardPoolTable from '@/components/card-pool-table.vue'
import DrawAnimation from '@/components/draw-animation.vue'
import NavBar from '@/components/nav-bar.vue'
import StateView from '@/components/state-view.vue'
import StockBar from '@/components/stock-bar.vue'
import { drawBlindBox, fetchBlindBoxDetail } from '@/api/blindbox'
import { resolveDrawError } from '@/config/draw-errors'
import { setDrawResult } from '@/store/draw'
import { clearAuth, isLoggedIn } from '@/store/user'
import { formatPrice } from '@/utils/format'
import { currentFullPath } from '@/utils/request'
import { toastError } from '@/utils/toast'
import type { ApiError, BlindBoxDetailResponse } from '@/types'

const boxId = ref(0)
const detail = ref<BlindBoxDetailResponse | null>(null)
const loading = ref(false)
const hasError = ref(false)

const drawing = ref(false)
const animating = ref(false)
/** 429 后的按钮禁用倒计时（秒）。 */
const cooldown = ref(0)
let cooldownTimer: ReturnType<typeof setInterval> | null = null

const box = computed(() => detail.value?.blind_box || null)
const items = computed(() => detail.value?.items || [])
const totalStock = computed(() => detail.value?.pool_total_stock || 0)
const soldOut = computed(() => totalStock.value <= 0)

const hasPromo = computed(() => {
  const promo = detail.value?.active_promotion
  const base = box.value?.price
  return !!promo && !!base && promo.promo_price < base
})

const ctaDisabled = computed(() => soldOut.value || drawing.value || cooldown.value > 0)

const ctaText = computed(() => {
  if (soldOut.value) return '该盲盒已拆完'
  if (cooldown.value > 0) return `${cooldown.value}s`
  if (drawing.value) return '加载中'
  return '立即拆卡'
})

async function load(): Promise<void> {
  if (!boxId.value) return
  loading.value = true
  try {
    detail.value = await fetchBlindBoxDetail(boxId.value)
    hasError.value = false
  } catch (err) {
    console.error('[detail] load failed:', err)
    hasError.value = true
  } finally {
    loading.value = false
  }
}

function startCooldown(seconds: number): void {
  stopCooldown()
  cooldown.value = seconds
  cooldownTimer = setInterval(() => {
    cooldown.value -= 1
    if (cooldown.value <= 0) stopCooldown()
  }, 1000)
}

function stopCooldown(): void {
  if (cooldownTimer) {
    clearInterval(cooldownTimer)
    cooldownTimer = null
  }
  cooldown.value = 0
}

function goLogin(): void {
  clearAuth()
  const redirect = currentFullPath()
  uni.navigateTo({
    url: `/pages/login/index${redirect ? `?redirect=${encodeURIComponent(redirect)}` : ''}`,
  })
}

/** 错误码 → 文案与动作（8 条规则，见 config/draw-errors.ts）。 */
async function handleDrawError(err: ApiError): Promise<void> {
  const rule = resolveDrawError(err)
  const message = rule.message

  switch (rule.action) {
    case 'login':
      goLogin()
      return
    case 'balance':
      toastError(message)
      setTimeout(() => uni.switchTab({ url: '/pages/mine/index' }), 900)
      return
    case 'home':
      toastError(message)
      setTimeout(() => uni.switchTab({ url: '/pages/home/index' }), 900)
      return
    case 'refresh-stock':
      toastError(message)
      await load()
      return
    case 'rate-limit':
      toastError(message)
      startCooldown(err.retryAfter && err.retryAfter > 0 ? err.retryAfter : 3)
      return
    default:
      toastError(message)
  }
}

async function onDraw(): Promise<void> {
  if (ctaDisabled.value || !box.value) return

  if (!isLoggedIn()) {
    goLogin()
    return
  }

  drawing.value = true
  try {
    const result = await drawBlindBox(box.value.id)
    setDrawResult({ result, blindBoxId: box.value.id })
    animating.value = true
  } catch (err) {
    await handleDrawError(err as ApiError)
  } finally {
    drawing.value = false
  }
}

function onAnimationFinish(): void {
  animating.value = false
  uni.redirectTo({ url: '/pages/draw/result' })
}

onLoad((query) => {
  const id = Number((query as Record<string, string>)?.id || 0)
  boxId.value = Number.isFinite(id) && id > 0 ? id : 0
  void load()
})

onUnmounted(stopCooldown)
</script>

<template>
  <view class="page detail">
    <NavBar title="商品详情" show-back />

    <StateView v-if="loading && !detail" type="loading" title="加载中" />

    <StateView v-else-if="hasError && !detail" type="error" title="网络异常，请稍后重试"
      action-text="重试" @action="load" />

    <template v-else-if="detail && box">
      <view class="hero">
        <image class="hero__img" :src="box.cover" mode="aspectFill" />
      </view>

      <view class="card">
        <text class="title">{{ box.name }}</text>
        <view class="row--between supplier">
          <text class="supplier__name">供应商 · {{ detail.supplier.name }}</text>
          <text class="tag tag--muted">{{ box.lifecycle }}</text>
        </view>
        <text v-if="box.description" class="desc">{{ box.description }}</text>
      </view>

      <view class="card">
        <view class="row--between">
          <view class="row">
            <text class="price">
              <text class="price__symbol">¥</text>{{ formatPrice(detail.effective_price) }}
            </text>
            <text v-if="hasPromo" class="price--origin">¥{{ formatPrice(box.price) }}</text>
          </view>
          <text v-if="hasPromo" class="tag tag--gold">活动价</text>
        </view>
      </view>

      <view class="card">
        <view class="row--between">
          <text class="card__title">库存进度</text>
          <text class="stock-num">
            剩余 <text class="stock-num__value">{{ totalStock }}</text>
          </text>
        </view>
        <StockBar :items="items" />
        <text class="pool-summary">概率公示 · {{ items.length }} 项</text>
      </view>

      <view class="card">
        <text class="card__title">概率公示</text>
        <CardPoolTable :items="items" />
      </view>

      <view class="footer-space" />
    </template>

    <!-- 底部固定 CTA -->
    <view v-if="detail && box" class="cta">
      <view class="cta__price">
        <text class="cta__label">实付</text>
        <text class="price">
          <text class="price__symbol">¥</text>{{ formatPrice(detail.effective_price) }}
        </text>
      </view>
      <view
        class="btn btn--primary cta__btn"
        :class="{ 'btn--disabled': ctaDisabled }"
        @tap="onDraw"
      >
        <text>{{ ctaText }}</text>
      </view>
    </view>

    <DrawAnimation :visible="animating" @finish="onAnimationFinish" />
  </view>
</template>

<style lang="scss" scoped>
.detail {
  padding-bottom: 180rpx;
}

.hero {
  width: 100%;
  padding-top: 100%;
  position: relative;
  background-color: var(--surface-2);
}

.hero__img {
  position: absolute;
  inset: 0;
  width: 100%;
  height: 100%;
}

.card {
  margin: var(--space-md);
}

.title {
  display: block;
  font-size: 36rpx;
  font-weight: 700;
  color: var(--text);
}

.supplier {
  margin-top: var(--space-sm);
}

.supplier__name {
  font-size: 26rpx;
  color: var(--muted);
}

.desc {
  display: block;
  margin-top: var(--space-sm);
  font-size: 26rpx;
  color: var(--muted);
  line-height: 1.6;
}

.stock-num {
  font-size: 24rpx;
  color: var(--muted);
}

.stock-num__value {
  color: var(--gold);
  font-weight: 700;
  font-size: 30rpx;
}

.pool-summary {
  display: block;
  margin-top: var(--space-sm);
  font-size: 22rpx;
  color: var(--dim);
}

.cta {
  position: fixed;
  left: 0;
  right: 0;
  bottom: 0;
  z-index: 20;
  display: flex;
  align-items: center;
  padding: var(--space-sm) var(--space-md);
  padding-bottom: calc(var(--space-sm) + env(safe-area-inset-bottom));
  background-color: var(--surface);
  border-top: 1rpx solid var(--border);
}

.cta__price {
  flex: 1;
}

.cta__label {
  display: block;
  font-size: 22rpx;
  color: var(--dim);
}

.cta__btn {
  width: 320rpx;
}
</style>
