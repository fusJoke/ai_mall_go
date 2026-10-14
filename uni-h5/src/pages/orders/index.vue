<script setup lang="ts">
/**
 * P5 订单列表。
 *
 * 需登录。未登录时直接引导到登录页并携带回跳地址（不依赖 401 拦截，减少一次无效请求）。
 */

import { computed, ref } from 'vue'
import { onPullDownRefresh, onReachBottom, onShow } from '@dcloudio/uni-app'

import NavBar from '@/components/nav-bar.vue'
import StateView from '@/components/state-view.vue'
import { PAGE_SIZE, fetchOrders } from '@/api/order'
import { isLoggedIn } from '@/store/user'
import { formatPrice, formatTime } from '@/utils/format'
import type { OrderSummary } from '@/types'

/** 后端状态 → 文案的白名单，避免未知状态被渲染成空白。 */
const STATUS_TEXT: Record<string, string> = {
  pending: '待支付',
  paid: '已支付',
  drawn: '已开卡',
  failed: '已失败',
}

const STATUS_CLASS: Record<string, string> = {
  pending: 'tag--muted',
  paid: 'tag--gold',
  drawn: 'tag--success',
  failed: 'tag--danger',
}

const list = ref<OrderSummary[]>([])
const total = ref(0)
const page = ref(1)
const loading = ref(false)
const loaded = ref(false)
const finished = ref(false)
const hasError = ref(false)
let guardTimer: ReturnType<typeof setTimeout> | null = null

const isEmpty = computed(() => loaded.value && !list.value.length)

function statusText(status: string): string {
  return STATUS_TEXT[status] || status
}

function statusClass(status: string): string {
  return STATUS_CLASS[status] || 'tag--muted'
}

async function load(reset = false): Promise<void> {
  if (loading.value) return
  if (!reset && finished.value) return

  loading.value = true
  try {
    const target = reset ? 1 : page.value
    const res = await fetchOrders(target, PAGE_SIZE)
    const items = res.items || []
    list.value = reset ? items : [...list.value, ...items]
    total.value = res.total || 0
    page.value = target + 1
    finished.value = !items.length || list.value.length >= total.value
    hasError.value = false
    loaded.value = true
  } catch (err) {
    console.error('[orders] load failed:', err)
    if (!list.value.length) hasError.value = true
  } finally {
    loading.value = false
    uni.stopPullDownRefresh()
  }
}

function goDetail(order: OrderSummary): void {
  uni.navigateTo({ url: `/pages/orders/detail?id=${order.id}` })
}

function goHome(): void {
  uni.switchTab({ url: '/pages/home/index' })
}

function goLogin(): void {
  uni.navigateTo({ url: '/pages/login/index?redirect=%2Fpages%2Forders%2Findex' })
}

onShow(() => {
  if (!isLoggedIn()) {
    // 等页面挂载完成再跳，避免 onShow 期间导航被忽略。
    if (guardTimer) clearTimeout(guardTimer)
    guardTimer = setTimeout(goLogin, 0)
    return
  }
  void load(true)
})

onPullDownRefresh(() => {
  void load(true)
})

onReachBottom(() => {
  void load(false)
})
</script>

<template>
  <view class="page orders">
    <NavBar title="我的订单" />

    <view v-if="loaded && list.length" class="summary">
      <text class="summary__text">共 {{ total }} 单</text>
    </view>

    <StateView v-if="hasError && !list.length" type="error" title="加载失败"
      desc="网络异常，请稍后重试" action-text="重试" @action="load(true)" />

    <StateView v-else-if="isEmpty" type="empty" title="还没有订单"
      desc="去首页拆一盒试试" action-text="去首页逛逛" @action="goHome" />

    <template v-else>
      <view v-for="order in list" :key="order.id" class="card order" @tap="goDetail(order)">
        <view class="row--between order__head">
          <text class="order__no ellipsis">{{ order.order_no }}</text>
          <text class="tag" :class="statusClass(order.status)">
            {{ statusText(order.status) }}
          </text>
        </view>

        <view class="row--between order__body">
          <view class="col">
            <text class="order__meta">盲盒 #{{ order.blind_box_id }}</text>
            <text class="order__meta">{{ formatTime(order.created_at) }}</text>
          </view>
          <text class="price">
            <text class="price__symbol">¥</text>{{ formatPrice(order.price) }}
          </text>
        </view>
      </view>

      <view class="tail">
        <text v-if="loading" class="tail__text">加载中</text>
        <text v-else-if="finished" class="tail__text">没有更多了</text>
      </view>

      <view class="footer-space" />
    </template>
  </view>
</template>

<style lang="scss" scoped>
.orders {
  padding: 0 var(--space-md);
}

.summary {
  padding: var(--space-sm) 0 var(--space-md);
}

.summary__text {
  font-size: 24rpx;
  color: var(--dim);
}

.order {
  margin-bottom: var(--space-md);
}

.order__no {
  flex: 1;
  font-size: 26rpx;
  color: var(--muted);
  margin-right: var(--space-sm);
}

.order__body {
  margin-top: var(--space-md);
  align-items: flex-end;
}

.order__meta {
  display: block;
  font-size: 24rpx;
  color: var(--dim);
  line-height: 1.7;
}

.tail {
  padding: var(--space-md) 0;
  text-align: center;
}

.tail__text {
  font-size: 24rpx;
  color: var(--dim);
}
</style>
