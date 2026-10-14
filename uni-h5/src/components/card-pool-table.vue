<script setup lang="ts">
/**
 * 概率公示表。
 *
 * 后端 `items[].weight` 同池合计 10000，换算为百分比展示；
 * 按稀有度（SSR → SR → R → N）聚合排序，与结果页 / 订单详情的配色保持一致。
 */

import { computed } from 'vue'

import { formatPercent } from '@/utils/format'
import { rarityColor, rarityIndex } from '@/utils/rarity'
import type { MallCardPoolItem } from '@/types'

const props = defineProps<{ items: MallCardPoolItem[] }>()

interface Row {
  rarity: string
  weight: number
  percent: number
  stock: number
  color: string
}

const rows = computed<Row[]>(() => {
  const grouped = new Map<string, Row>()

  for (const item of props.items) {
    const rarity = String(item.rarity || 'N').toUpperCase()
    const exist = grouped.get(rarity)
    if (exist) {
      exist.weight += item.weight || 0
      exist.stock += Math.max(0, item.stock || 0)
    } else {
      grouped.set(rarity, {
        rarity,
        weight: item.weight || 0,
        percent: 0,
        stock: Math.max(0, item.stock || 0),
        color: rarityColor(rarity),
      })
    }
  }

  const list = [...grouped.values()].sort((a, b) => rarityIndex(a.rarity) - rarityIndex(b.rarity))
  for (const row of list) row.percent = Number(((row.weight / 10000) * 100).toFixed(2))
  return list
})

/** 表中权重合计（用于自检：应恒为 10000）。 */
const weightSum = computed(() => rows.value.reduce((sum, row) => sum + row.weight, 0))
</script>

<template>
  <view class="ptable">
    <view v-if="!rows.length" class="ptable__empty">—</view>

    <template v-else>
      <view class="ptable__head">
        <text class="ptable__c1">稀有度</text>
        <text class="ptable__c2">概率</text>
        <text class="ptable__c3">剩余</text>
      </view>

      <view v-for="row in rows" :key="row.rarity" class="ptable__row">
        <view class="ptable__c1">
          <view class="ptable__dot" :style="{ backgroundColor: row.color }" />
          <text class="ptable__rarity" :style="{ color: row.color }">{{ row.rarity }}</text>
        </view>
        <text class="ptable__c2">{{ formatPercent(row.weight) }}</text>
        <text class="ptable__c3">{{ row.stock }}</text>
      </view>

      <view class="ptable__foot">
        <text class="ptable__foot-text">权重合计 {{ weightSum }}（万分比）</text>
      </view>
    </template>
  </view>
</template>

<style lang="scss" scoped>
.ptable__head,
.ptable__row {
  display: flex;
  align-items: center;
  padding: 14rpx 0;
}

.ptable__head {
  border-bottom: 1rpx solid var(--border);
}

.ptable__row + .ptable__row {
  border-top: 1rpx solid var(--border);
}

.ptable__c1 {
  flex: 1.2;
  display: flex;
  align-items: center;
  font-size: 24rpx;
  color: var(--muted);
}

.ptable__c2 {
  flex: 1;
  text-align: right;
  font-size: 26rpx;
  color: var(--text);
}

.ptable__c3 {
  flex: 0.8;
  text-align: right;
  font-size: 26rpx;
  color: var(--muted);
}

.ptable__dot {
  width: 12rpx;
  height: 12rpx;
  border-radius: 50%;
  margin-right: 8rpx;
}

.ptable__rarity {
  font-weight: 700;
  font-size: 26rpx;
  letter-spacing: 1rpx;
}

.ptable__foot {
  margin-top: var(--space-sm);
}

.ptable__foot-text {
  font-size: 22rpx;
  color: var(--dim);
}

.ptable__empty {
  font-size: 26rpx;
  color: var(--dim);
  padding: var(--space-md) 0;
}
</style>
