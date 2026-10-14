<script setup lang="ts">
/**
 * P4 抽卡结果。
 *
 * 数据经 `store/draw` 一次性传递（不入 URL）。直接访问或刷新时存储为空，
 * 兜底跳订单列表，不出现白屏（tasks 7.9）。
 */

import { computed, ref } from 'vue'
import { onLoad } from '@dcloudio/uni-app'

import NavBar from '@/components/nav-bar.vue'
import StateView from '@/components/state-view.vue'
import { consumeDrawResult } from '@/store/draw'
import { formatPrice } from '@/utils/format'
import { isHighRarity, rarityColor } from '@/utils/rarity'
import type { DrawCard } from '@/types'

const payload = ref<ReturnType<typeof consumeDrawResult>>(null)

const result = computed(() => payload.value?.result || null)
const cards = computed<DrawCard[]>(() => result.value?.cards || [])
const hasHighRarity = computed(() => cards.value.some((card) => isHighRarity(card.rarity)))

function goAgain(): void {
  const id = payload.value?.blindBoxId
  if (id) {
    uni.redirectTo({ url: `/pages/blindbox/detail?id=${id}` })
    return
  }
  uni.switchTab({ url: '/pages/home/index' })
}

function goOrders(): void {
  uni.switchTab({ url: '/pages/orders/index' })
}

onLoad(() => {
  const data = consumeDrawResult()
  if (!data) {
    // 无数据兜底：直接回订单列表。
    uni.switchTab({ url: '/pages/orders/index' })
    return
  }
  payload.value = data
})
</script>

<template>
  <view class="page result">
    <NavBar title="开卡结果" show-back />

    <StateView v-if="!result" type="loading" title="加载中" />

    <template v-else>
      <view class="order-bar">
        <view class="order-bar__item">
          <text class="order-bar__label">订单号</text>
          <text class="order-bar__value ellipsis">{{ result.order_no }}</text>
        </view>
        <view class="order-bar__item order-bar__item--right">
          <text class="order-bar__label">实付</text>
          <text class="price">
            <text class="price__symbol">¥</text>{{ formatPrice(result.actual_price) }}
          </text>
        </view>
      </view>

      <view v-if="hasHighRarity" class="rare-tip">
        <text>稀有卡入手</text>
      </view>

      <view class="grid">
        <view
          v-for="card in cards"
          :key="card.item_id"
          class="grid__cell"
          :style="{ borderColor: rarityColor(card.rarity) }"
        >
          <image class="grid__img" :src="card.snapshot_image" mode="aspectFill" />
          <view class="grid__meta">
            <text class="rarity" :style="{ color: rarityColor(card.rarity) }">
              {{ card.rarity }}
            </text>
            <text class="grid__name ellipsis">{{ card.snapshot_name }}</text>
          </view>
        </view>
      </view>

      <view class="actions">
        <view class="btn btn--ghost actions__btn" @tap="goAgain">
          <text>再拆一盒</text>
        </view>
        <view class="btn btn--primary actions__btn" @tap="goOrders">
          <text>查看订单</text>
        </view>
      </view>
    </template>
  </view>
</template>

<style lang="scss" scoped>
.result {
  padding: 0 var(--space-md);
}

.order-bar {
  display: flex;
  align-items: center;
  padding: var(--space-md);
  border-radius: var(--radius-md);
  background-color: var(--surface);
  border: 1rpx solid var(--border);
  margin-bottom: var(--space-md);
}

.order-bar__item {
  flex: 1;
  min-width: 0;
}

.order-bar__item--right {
  text-align: right;
}

.order-bar__label {
  display: block;
  font-size: 22rpx;
  color: var(--dim);
}

.order-bar__value {
  display: block;
  font-size: 26rpx;
  color: var(--text);
  margin-top: 4rpx;
}

.rare-tip {
  margin-bottom: var(--space-sm);
  font-size: 24rpx;
  color: var(--gold-2);
  letter-spacing: 2rpx;
}

.grid {
  display: flex;
  flex-wrap: wrap;
}

.grid__cell {
  width: 48%;
  margin: 0 1% var(--space-sm);
  border-radius: var(--radius-md);
  border: 2rpx solid var(--border);
  overflow: hidden;
  background-color: var(--surface);
}

.grid__img {
  width: 100%;
  height: 320rpx;
  background-color: var(--surface-2);
}

.grid__meta {
  padding: var(--space-sm);
}

.grid__name {
  display: block;
  font-size: 24rpx;
  color: var(--text);
  margin-top: 4rpx;
}

.actions {
  display: flex;
  margin-top: var(--space-lg);
}

.actions__btn {
  flex: 1;
}

.actions__btn + .actions__btn {
  margin-left: var(--space-md);
}
</style>
