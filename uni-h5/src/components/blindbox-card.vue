<script setup lang="ts">
/**
 * 商品卡片。
 *
 * 展示规则：活动价优先（有 `promo_price` 时展示活动价 + 原价删除线），
 * 稀有度摘要作为标签展示。
 */

import { computed } from 'vue'

import { formatPrice } from '@/utils/format'
import type { BlindBoxFeedItem } from '@/types'

const props = defineProps<{
  item: BlindBoxFeedItem
  /** 是否展示供应商名（首页瀑布流不展示，搜索结果等场景可开）。 */
  showSupplier?: boolean
}>()

const emit = defineEmits<{ (e: 'select', item: BlindBoxFeedItem): void }>()

const hasPromo = computed(() => {
  const promo = props.item.promo_price
  return typeof promo === 'number' && promo > 0 && promo < props.item.price
})

const displayPrice = computed(() =>
  formatPrice(hasPromo.value ? (props.item.promo_price as number) : props.item.price)
)
</script>

<template>
  <view class="bcard" @tap="emit('select', item)">
    <view class="bcard__cover">
      <image class="bcard__img" :src="item.cover" mode="aspectFill" lazy-load />
      <view v-if="hasPromo" class="bcard__badge">
        <text>活动价</text>
      </view>
    </view>

    <view class="bcard__body">
      <text class="bcard__name ellipsis-2">{{ item.name }}</text>

      <text v-if="showSupplier && item.supplier_name" class="bcard__supplier ellipsis">
        {{ item.supplier_name }}
      </text>

      <view class="bcard__meta">
        <text v-if="item.rarity_summary" class="tag tag--muted">{{ item.rarity_summary }}</text>
      </view>

      <view class="bcard__price row">
        <text class="price">
          <text class="price__symbol">¥</text>{{ displayPrice }}
        </text>
        <text v-if="hasPromo" class="price--origin">¥{{ formatPrice(item.price) }}</text>
      </view>
    </view>
  </view>
</template>

<style lang="scss" scoped>
.bcard {
  background-color: var(--surface);
  border-radius: var(--radius-md);
  border: 1rpx solid var(--border);
  overflow: hidden;
}

.bcard__cover {
  position: relative;
  width: 100%;
  /* 3:4 封面比例，用 padding 撑高以兼容不支持的 aspect-ratio 的 WebView */
  padding-top: 133.33%;
  background-color: var(--surface-2);
}

.bcard__img {
  position: absolute;
  top: 0;
  left: 0;
  width: 100%;
  height: 100%;
}

.bcard__badge {
  position: absolute;
  top: var(--space-xs);
  left: var(--space-xs);
  padding: 2rpx 10rpx;
  border-radius: var(--radius-sm);
  font-size: 20rpx;
  color: var(--bg);
  background: linear-gradient(135deg, var(--gold-2), var(--gold));
}

.bcard__body {
  padding: var(--space-sm);
}

.bcard__name {
  font-size: 26rpx;
  color: var(--text);
  line-height: 1.35;
  min-height: 70rpx;
}

.bcard__supplier {
  display: block;
  font-size: 22rpx;
  color: var(--dim);
  margin-top: 4rpx;
}

.bcard__meta {
  margin-top: var(--space-xs);
}

.bcard__price {
  margin-top: var(--space-xs);
  align-items: baseline;
}
</style>
