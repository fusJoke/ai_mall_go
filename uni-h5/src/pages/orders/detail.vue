<script setup lang="ts">
/**
 * P6 订单详情。
 *
 * 三区块：订单信息 / 支付信息 / 卡牌明细。
 * 只取白名单字段，不引用 `deleted_at` 等内部字段（tasks 8.4）。
 */

import { computed, ref } from 'vue'
import { onLoad } from '@dcloudio/uni-app'

import NavBar from '@/components/nav-bar.vue'
import StateView from '@/components/state-view.vue'
import { fetchOrderDetail } from '@/api/order'
import { formatPrice, formatTime } from '@/utils/format'
import { rarityColor } from '@/utils/rarity'
import type { OrderDetail } from '@/types'

/** 后端状态 → 文案的白名单，未知状态原样透出。 */
const STATUS_TEXT: Record<string, string> = {
  pending: '待支付',
  paid: '已支付',
  drawn: '已开卡',
  failed: '已失败',
}

/** 下单来源 → 文案的白名单。 */
const SOURCE_TEXT: Record<string, string> = {
  normal: '普通',
  seckill: '秒杀',
}

const orderId = ref(0)
const detail = ref<OrderDetail | null>(null)
const loading = ref(false)
const hasError = ref(false)

const order = computed(() => detail.value?.order || null)
const items = computed(() => detail.value?.items || [])

function statusText(status?: string): string {
  if (!status) return '-'
  return STATUS_TEXT[status] || status
}

function sourceText(source?: string): string {
  if (!source) return '-'
  return SOURCE_TEXT[source] || source
}

async function load(): Promise<void> {
  if (!orderId.value) return
  loading.value = true
  try {
    detail.value = await fetchOrderDetail(orderId.value)
    hasError.value = false
  } catch (err) {
    console.error('[order-detail] load failed:', err)
    hasError.value = true
  } finally {
    loading.value = false
  }
}

onLoad((query) => {
  const id = Number((query as Record<string, string>)?.id || 0)
  orderId.value = Number.isFinite(id) && id > 0 ? id : 0
  void load()
})
</script>

<template>
  <view class="page odetail">
    <NavBar title="订单详情" show-back />

    <StateView v-if="loading && !detail" type="loading" title="加载中" />

    <StateView v-else-if="hasError && !detail" type="error" title="网络异常，请稍后重试"
      action-text="重试" @action="load" />

    <template v-else-if="order">
      <view class="card">
        <text class="card__title">订单信息</text>
        <view class="kv">
          <text class="kv__k">订单号</text>
          <text class="kv__v ellipsis">{{ order.order_no }}</text>
        </view>
        <view class="kv">
          <text class="kv__k">订单状态</text>
          <text class="kv__v">{{ statusText(order.status) }}</text>
        </view>
        <view class="kv">
          <text class="kv__k">来源</text>
          <text class="kv__v">{{ sourceText(order.source) }}</text>
        </view>
        <view class="kv">
          <text class="kv__k">盲盒</text>
          <text class="kv__v">#{{ order.blind_box_id }}</text>
        </view>
        <view class="kv">
          <text class="kv__k">供应商</text>
          <text class="kv__v">#{{ order.supplier_id }}</text>
        </view>
      </view>

      <view class="card">
        <text class="card__title">支付信息</text>
        <view class="kv">
          <text class="kv__k">实付金额</text>
          <text class="price">
            <text class="price__symbol">¥</text>{{ formatPrice(order.price) }}
          </text>
        </view>
        <view class="kv">
          <text class="kv__k">下单时间</text>
          <text class="kv__v">{{ formatTime(order.created_at) }}</text>
        </view>
        <view class="kv">
          <text class="kv__k">更新时间</text>
          <text class="kv__v">{{ formatTime(order.updated_at) }}</text>
        </view>
      </view>

      <view class="card">
        <text class="card__title">卡牌明细</text>

        <text v-if="!items.length" class="empty-text">该订单暂无卡牌</text>

        <view v-for="item in items" :key="item.id" class="card-row">
          <image
            class="card-row__img"
            :src="item.snapshot_image"
            mode="aspectFill"
            :style="{ borderColor: rarityColor(item.rarity) }"
          />
          <view class="card-row__meta">
            <text class="rarity" :style="{ color: rarityColor(item.rarity) }">
              {{ item.rarity }}
            </text>
            <text class="card-row__name ellipsis">{{ item.snapshot_name }}</text>
          </view>
        </view>
      </view>

      <view class="footer-space" />
    </template>
  </view>
</template>

<style lang="scss" scoped>
.odetail {
  padding: 0 var(--space-md);
}

.kv {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 12rpx 0;
}

.kv__k {
  font-size: 26rpx;
  color: var(--muted);
}

.kv__v {
  flex: 1;
  text-align: right;
  font-size: 26rpx;
  color: var(--text);
  margin-left: var(--space-md);
}

.empty-text {
  font-size: 26rpx;
  color: var(--dim);
  padding: var(--space-sm) 0;
}

.card-row {
  display: flex;
  align-items: center;
  padding: var(--space-sm) 0;
}

.card-row + .card-row {
  border-top: 1rpx solid var(--border);
}

.card-row__img {
  width: 120rpx;
  height: 160rpx;
  border-radius: var(--radius-sm);
  border: 2rpx solid var(--border);
  background-color: var(--surface-2);
}

.card-row__meta {
  flex: 1;
  min-width: 0;
  margin-left: var(--space-md);
}

.card-row__name {
  display: block;
  font-size: 26rpx;
  color: var(--text);
  margin-top: 6rpx;
}
</style>
