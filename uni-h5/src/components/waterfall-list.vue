<script setup lang="ts">
/**
 * 双列瀑布流。
 *
 * 选型结论（design.md D7）：**不引虚拟列表**。
 * 首页单次返回量在几十到一两百张卡，远低于虚拟列表的收益阈值（通常 1000+ 节点），
 * 而动态高度虚拟列表在 uni-app H5 端有滚动位置错乱与图片复用闪烁的已知问题。
 *
 * 因此对外只暴露 `list` / `loading` / `finished` 与 `load-more` 事件；
 * 将来量级上来时替换本文件内部实现即可，调用方零改动。
 *
 * 派发算法：按估算高度贪心投入较矮的一列。封面固定 3:4，差异来自标题行数。
 */

import { computed, getCurrentInstance, onBeforeUnmount, onMounted } from 'vue'

import BlindBoxCard from './blindbox-card.vue'
import type { BlindBoxFeedItem } from '@/types'

const props = withDefaults(
  defineProps<{
    list: BlindBoxFeedItem[]
    loading?: boolean
    finished?: boolean
  }>(),
  { loading: false, finished: false }
)

const emit = defineEmits<{
  (e: 'select', item: BlindBoxFeedItem): void
  (e: 'load-more'): void
}>()

/** 估算高度（单位任意，仅用于列间比较）。 */
function estimateHeight(item: BlindBoxFeedItem): number {
  const titleLines = Math.max(1, Math.min(2, Math.ceil((item.name?.length || 0) / 9)))
  return 400 + titleLines * 40 + (item.rarity_summary ? 40 : 0)
}

const columns = computed<BlindBoxFeedItem[][]>(() => {
  const left: BlindBoxFeedItem[] = []
  const right: BlindBoxFeedItem[] = []
  let leftH = 0
  let rightH = 0

  for (const item of props.list) {
    const h = estimateHeight(item)
    if (leftH <= rightH) {
      left.push(item)
      leftH += h
    } else {
      right.push(item)
      rightH += h
    }
  }
  return [left, right]
})

const instance = getCurrentInstance()
let observer: UniApp.IntersectionObserver | null = null

onMounted(() => {
  if (!instance) return
  try {
    observer = uni.createIntersectionObserver(instance.proxy as never, { thresholds: [0] })
    // 提前 200px 触发，滚动更顺滑。
    observer.relativeToViewport({ bottom: 200 }).observe('.wf__sentinel', () => {
      if (!props.loading && !props.finished) emit('load-more')
    })
  } catch (err) {
    // 观察器不可用时不阻塞渲染：页面自身的 onReachBottom 会兜底。
    console.warn('[waterfall-list] intersection observer unavailable:', err)
  }
})

onBeforeUnmount(() => {
  observer?.disconnect()
  observer = null
})
</script>

<template>
  <view class="wf">
    <view class="wf__cols">
      <view class="wf__col">
        <BlindBoxCard
          v-for="item in columns[0]"
          :key="item.id"
          :item="item"
          class="wf__cell"
          @select="emit('select', item)"
        />
      </view>
      <view class="wf__col">
        <BlindBoxCard
          v-for="item in columns[1]"
          :key="item.id"
          :item="item"
          class="wf__cell"
          @select="emit('select', item)"
        />
      </view>
    </view>

    <!-- 触底哨兵：进入视口 200px 内时触发 load-more -->
    <view class="wf__sentinel" />
  </view>
</template>

<style lang="scss" scoped>
.wf {
  padding: 0 var(--space-md);
}

.wf__cols {
  display: flex;
  align-items: flex-start;
}

.wf__col {
  flex: 1;
  min-width: 0;
}

.wf__col + .wf__col {
  margin-left: var(--space-sm);
}

.wf__cell {
  display: block;
  margin-bottom: var(--space-sm);
}

.wf__sentinel {
  height: 2rpx;
  width: 100%;
}
</style>
