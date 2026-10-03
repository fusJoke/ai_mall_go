<!-- src\views\user\seckill\index.vue — C 端秒杀活动列表（任务 12.2）。
     当前生效的秒杀活动（后端已过滤时间窗 / 下架 / 禁用供应商），
     每张卡片带限时倒计时 + 抢购进度；30s 轮询刷新剩余名额。 -->
<template>
    <div class="seckill-page">
        <div class="head">
            <h1>{{ t('user.seckill.title') }}</h1>
            <span class="sub">{{ t('user.seckill.subtitle') }}</span>
        </div>

        <div v-if="loading" class="state">Loading...</div>
        <div v-else-if="items.length === 0" class="state">{{ t('user.seckill.empty') }}</div>
        <div v-else class="grid">
            <RouterLink v-for="item in items" :key="item.id" :to="`/user/seckill/${item.id}`" class="card">
                <div class="cover">
                    <img v-if="item.blind_box_cover" :src="item.blind_box_cover" :alt="item.blind_box_name" />
                    <span v-else>⚡</span>
                    <span class="flash-tag">{{ t('user.seckill.tag') }}</span>
                </div>
                <div class="body">
                    <p class="name">{{ item.blind_box_name }}</p>
                    <p class="supplier">{{ item.supplier_name }}</p>
                    <div class="prices">
                        <span class="seckill">¥{{ item.seckill_price.toFixed(2) }}</span>
                        <span class="origin">¥{{ item.original_price.toFixed(2) }}</span>
                    </div>
                    <div class="progress">
                        <div class="bar">
                            <div class="bar-fill" :style="{ width: soldPercent(item) + '%' }" />
                        </div>
                        <span class="sold-text">{{ soldText(item) }}</span>
                    </div>
                    <div class="foot">
                        <span class="countdown" :class="{ urgent: isUrgent(item) }">
                            {{ countdownText(item) }}
                        </span>
                        <span class="limit">×{{ item.per_user_limit }} {{ t('user.seckill.limitUnit') }}</span>
                    </div>
                </div>
            </RouterLink>
        </div>
    </div>
</template>

<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { seckillList, type SeckillActivity } from '/@/api/user/seckill'

const { t, locale } = useI18n()

const items = ref<SeckillActivity[]>([])
const loading = ref(true)
const now = ref(Date.now())
let tickTimer: number | undefined
let pollTimer: number | undefined

/** 已抢百分比：remaining 未知（-1）时不显示进度。 */
function soldPercent(item: SeckillActivity): number {
    if (item.remaining_stock < 0 || item.total_stock <= 0) return 0
    const sold = item.total_stock - item.remaining_stock
    return Math.min(100, Math.round((sold / item.total_stock) * 100))
}

function soldText(item: SeckillActivity): string {
    if (item.remaining_stock < 0) return t('user.seckill.stockUnknown')
    const sold = item.total_stock - item.remaining_stock
    return t('user.seckill.sold', { sold: String(sold), total: String(item.total_stock) })
}

function isUrgent(item: SeckillActivity): boolean {
    return new Date(item.end_at).getTime() - now.value < 10 * 60 * 1000
}

/** 倒计时文案：>1h 显示 H:mm:ss，否则 mm:ss。 */
function countdownText(item: SeckillActivity): string {
    const diff = new Date(item.end_at).getTime() - now.value
    if (diff <= 0) return t('user.seckill.ended')
    const h = Math.floor(diff / 3600000)
    const m = Math.floor((diff % 3600000) / 60000)
    const s = Math.floor((diff % 60000) / 1000)
    const pad = (n: number) => String(n).padStart(2, '0')
    const prefix = locale.value.startsWith('zh') ? '距结束 ' : 'Ends in '
    return h > 0 ? `${prefix}${h}:${pad(m)}:${pad(s)}` : `${prefix}${pad(m)}:${pad(s)}`
}

async function load() {
    try {
        const { data } = await seckillList({ page: 1, page_size: 50 })
        items.value = data.items ?? []
    } catch {
        items.value = []
    } finally {
        loading.value = false
    }
}

onMounted(() => {
    load()
    tickTimer = window.setInterval(() => (now.value = Date.now()), 1000)
    pollTimer = window.setInterval(load, 30000)
})

onBeforeUnmount(() => {
    if (tickTimer) window.clearInterval(tickTimer)
    if (pollTimer) window.clearInterval(pollTimer)
})
</script>

<style scoped lang="scss">
.seckill-page {
    display: flex;
    flex-direction: column;
    gap: 20px;
}
.head {
    h1 {
        margin: 0 0 4px;
        font-size: 24px;
        color: #0f172a;
    }
}
.sub {
    font-size: 13px;
    color: #94a3b8;
}
.grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(260px, 1fr));
    gap: 18px;
}
.card {
    background: #fff;
    border-radius: 16px;
    overflow: hidden;
    text-decoration: none;
    border: 1px solid transparent;
    transition: all 0.2s;

    &:hover {
        transform: translateY(-2px);
        border-color: rgba(249, 115, 22, 0.4);
        box-shadow: 0 8px 24px rgba(249, 115, 22, 0.15);
    }
}
.cover {
    position: relative;
    aspect-ratio: 16 / 10;
    background: #f1f5f9;
    display: flex;
    align-items: center;
    justify-content: center;
    font-size: 48px;
    overflow: hidden;

    img {
        width: 100%;
        height: 100%;
        object-fit: cover;
    }
}
.flash-tag {
    position: absolute;
    top: 10px;
    left: 10px;
    background: linear-gradient(90deg, #f97316, #ea580c);
    color: #fff;
    font-size: 12px;
    font-weight: 700;
    padding: 3px 10px;
    border-radius: 999px;
}
.body {
    padding: 14px 16px 16px;
    display: flex;
    flex-direction: column;
    gap: 8px;
}
.name {
    margin: 0;
    font-size: 16px;
    font-weight: 700;
    color: #0f172a;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
}
.supplier {
    margin: 0;
    font-size: 12px;
    color: #1e40af;
}
.prices {
    display: flex;
    align-items: baseline;
    gap: 8px;
}
.seckill {
    font-size: 22px;
    font-weight: 800;
    color: #ea580c;
}
.origin {
    font-size: 13px;
    color: #94a3b8;
    text-decoration: line-through;
}
.progress {
    display: flex;
    align-items: center;
    gap: 8px;
}
.bar {
    flex: 1;
    height: 8px;
    border-radius: 999px;
    background: #fee2e2;
    overflow: hidden;
}
.bar-fill {
    height: 100%;
    border-radius: 999px;
    background: linear-gradient(90deg, #f97316, #ea580c);
    transition: width 0.4s;
}
.sold-text {
    font-size: 12px;
    color: #64748b;
    white-space: nowrap;
}
.foot {
    display: flex;
    align-items: center;
    justify-content: space-between;
}
.countdown {
    font-size: 13px;
    font-weight: 700;
    color: #b45309;
    font-variant-numeric: tabular-nums;

    &.urgent {
        color: #dc2626;
        animation: blink 1s infinite;
    }
}
@keyframes blink {
    50% {
        opacity: 0.55;
    }
}
.limit {
    font-size: 12px;
    color: #94a3b8;
}
.state {
    padding: 64px 0;
    text-align: center;
    color: #64748b;
    background: #fff;
    border-radius: 16px;
}
</style>
