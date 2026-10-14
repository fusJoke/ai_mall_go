<script setup lang="ts">
/**
 * 库存条。
 *
 * 设计说明：后端只返回 `pool_total_stock`（= Σ items.stock），**没有**「初始总量」，
 * 因此无法画出「已消耗 / 总量」的进度条 —— 硬凑一个分母就是编数据。
 * 这里改为展示**剩余库存的结构构成**：各稀有度剩余量占剩余总量的比例。
 * 数据全部来自接口，含义明确；剩余总量为 0 时条为空。
 */

import { computed } from 'vue'

import { rarityColor, rarityIndex } from '@/utils/rarity'
import type { MallCardPoolItem } from '@/types'

const props = defineProps<{ items: MallCardPoolItem[] }>()

/** 剩余总量。 */
const total = computed(() =>
  props.items.reduce((sum, item) => sum + Math.max(0, item.stock || 0), 0)
)

const segments = computed(() => {
  const grouped = new Map<string, number>()
  for (const item of props.items) {
    const rarity = String(item.rarity || 'N').toUpperCase()
    grouped.set(rarity, (grouped.get(rarity) || 0) + Math.max(0, item.stock || 0))
  }

  return [...grouped.entries()]
    .map(([rarity, stock]) => ({
      rarity,
      stock,
      percent: total.value > 0 ? (stock / total.value) * 100 : 0,
      color: rarityColor(rarity),
    }))
    .sort((a, b) => rarityIndex(a.rarity) - rarityIndex(b.rarity))
    .filter((seg) => seg.stock > 0)
})

defineExpose({ total })
</script>

<template>
  <view class="sbar">
    <view class="sbar__track">
      <view
        v-for="seg in segments"
        :key="seg.rarity"
        class="sbar__seg"
        :style="{ width: seg.percent + '%', backgroundColor: seg.color }"
      />
    </view>

    <view class="sbar__legend">
      <view v-for="seg in segments" :key="seg.rarity" class="sbar__item">
        <view class="sbar__dot" :style="{ backgroundColor: seg.color }" />
        <text class="sbar__rarity" :style="{ color: seg.color }">{{ seg.rarity }}</text>
        <text class="sbar__count">{{ seg.stock }}</text>
      </view>
      <text v-if="!segments.length" class="sbar__empty">库存已空</text>
    </view>
  </view>
</template>

<style lang="scss" scoped>
.sbar__track {
  display: flex;
  height: 16rpx;
  border-radius: var(--radius-pill);
  overflow: hidden;
  background-color: var(--surface-2);
}

.sbar__seg {
  height: 100%;
}

.sbar__legend {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  margin-top: var(--space-sm);
}

.sbar__item {
  display: flex;
  align-items: center;
  margin-right: var(--space-md);
  margin-bottom: var(--space-xs);
}

.sbar__dot {
  width: 12rpx;
  height: 12rpx;
  border-radius: 50%;
  margin-right: 8rpx;
}

.sbar__rarity {
  font-size: 22rpx;
  font-weight: 600;
  margin-right: 6rpx;
}

.sbar__count {
  font-size: 22rpx;
  color: var(--muted);
}

.sbar__empty {
  font-size: 24rpx;
  color: var(--dim);
}
</style>
