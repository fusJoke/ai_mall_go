<script setup lang="ts">
/**
 * P1 首页。
 *
 * 数据源：`GET /user/home/feed`（一次返回全量 feed，非无限分页流）。
 * 该接口强依赖 ES，ES 不可用时由**服务端**降级到 MySQL —— 客户端不做任何降级分支，
 * 也不感知数据来源（见能力 `home-feed-fallback` 与 tasks 5.6）。
 */

import { ref } from 'vue'
import { onPullDownRefresh, onReachBottom, onShow } from '@dcloudio/uni-app'

import NavBar from '@/components/nav-bar.vue'
import StateView from '@/components/state-view.vue'
import WaterfallList from '@/components/waterfall-list.vue'
import { fetchFeed } from '@/api/blindbox'
import type { BlindBoxFeedItem, SupplierFeedItem } from '@/types'

/** 分类胶囊（本期为占位，不参与筛选）。 */
const CATEGORIES = ['全部', '热门', '新品', '高爆率']

const list = ref<BlindBoxFeedItem[]>([])
const suppliers = ref<SupplierFeedItem[]>([])
const loading = ref(false)
const loaded = ref(false)
const hasError = ref(false)

async function load(): Promise<void> {
  if (loading.value) return
  loading.value = true
  try {
    const res = await fetchFeed()
    list.value = res.blind_boxes || []
    suppliers.value = res.suppliers || []
    hasError.value = false
    loaded.value = true
  } catch (err) {
    console.error('[home] feed failed:', err)
    // 已有数据时保留旧数据，只有首次加载失败才整页报错。
    if (!list.value.length) hasError.value = true
  } finally {
    loading.value = false
    uni.stopPullDownRefresh()
  }
}

function goDetail(item: BlindBoxFeedItem): void {
  uni.navigateTo({ url: `/pages/blindbox/detail?id=${item.id}` })
}

onShow(() => {
  if (!loaded.value) void load()
})

onPullDownRefresh(() => {
  void load()
})

onReachBottom(() => {
  // feed 一次返回全量，不存在下一页；此处的 load-more 由组件哨兵触发时同样短路。
  if (loaded.value) return
  void load()
})
</script>

<template>
  <view class="page">
    <NavBar title="拆卡" />

    <!-- 搜索入口：本期占位，不做跳转 -->
    <view class="search">
      <view class="search__box">
        <text class="search__icon">⌕</text>
        <text class="search__placeholder">搜索盲盒 / 卡牌</text>
      </view>
    </view>

    <!-- 分类胶囊：本期占位 -->
    <scroll-view class="cats" scroll-x :show-scrollbar="false">
      <view class="cats__inner">
        <view
          v-for="(cat, index) in CATEGORIES"
          :key="cat"
          class="cats__item"
          :class="{ 'cats__item--active': index === 0 }"
        >
          <text>{{ cat }}</text>
        </view>
      </view>
    </scroll-view>

    <view class="banner">
      <text class="banner__title">新客首拆立减</text>
      <text class="banner__desc">精选卡池 · 限量开拆</text>
    </view>

    <StateView v-if="hasError && !list.length" type="error" title="加载失败"
      desc="网络异常，请稍后重试" action-text="重试" @action="load" />

    <StateView v-else-if="!loaded && loading" type="loading" title="加载中" />

    <StateView v-else-if="loaded && !list.length" type="empty" title="暂无可拆盲盒"
      desc="精选卡池 · 限量开拆" action-text="重试" @action="load" />

    <template v-else>
      <WaterfallList :list="list" :loading="loading" :finished="true" @select="goDetail" />
      <view class="footer-space" />
    </template>
  </view>
</template>

<style lang="scss" scoped>
.search {
  padding: var(--space-sm) var(--space-md) 0;
}

.search__box {
  height: 72rpx;
  border-radius: var(--radius-pill);
  background-color: var(--surface);
  border: 1rpx solid var(--border);
  display: flex;
  align-items: center;
  padding: 0 var(--space-md);
}

.search__icon {
  font-size: 32rpx;
  color: var(--dim);
  margin-right: 8rpx;
}

.search__placeholder {
  font-size: 26rpx;
  color: var(--dim);
}

.cats {
  white-space: nowrap;
  margin-top: var(--space-sm);
}

.cats__inner {
  display: inline-flex;
  padding: 0 var(--space-md);
}

.cats__item {
  padding: 8rpx 24rpx;
  border-radius: var(--radius-pill);
  background-color: var(--surface);
  border: 1rpx solid var(--border);
  margin-right: var(--space-sm);
  font-size: 26rpx;
  color: var(--muted);
}

.cats__item--active {
  color: var(--gold);
  border-color: var(--gold);
  background-color: rgba(245, 181, 68, 0.12);
}

.banner {
  margin: var(--space-md);
  padding: var(--space-lg) var(--space-md);
  border-radius: var(--radius-lg);
  background: linear-gradient(135deg, rgba(245, 181, 68, 0.18), rgba(185, 140, 255, 0.12));
  border: 1rpx solid var(--border);
}

.banner__title {
  display: block;
  font-size: 34rpx;
  font-weight: 700;
  color: var(--gold-2);
}

.banner__desc {
  display: block;
  margin-top: 6rpx;
  font-size: 24rpx;
  color: var(--muted);
}
</style>
