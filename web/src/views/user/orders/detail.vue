<!-- src\views\user\orders\detail.vue — 订单详情（任务 6.9 的 [id] 页）。 -->
<template>
    <div class="detail">
        <RouterLink to="/user/orders" class="back">← {{ t('user.orders.backToList') }}</RouterLink>
        <div v-if="loading" class="state">Loading...</div>
        <div v-else-if="!order" class="state">{{ t('user.orders.notFound') }}</div>
        <template v-else>
            <div class="card">
                <h2>{{ t('user.orders.title') }} <span class="mono">#{{ order.order_no }}</span></h2>
                <div class="meta">
                    <p><span>{{ t('user.orders.price') }}</span><b>¥{{ order.price.toFixed(2) }}</b></p>
                    <p><span>{{ t('user.orders.status') }}</span>{{ order.status }}</p>
                    <p>
                        <span>{{ t('user.orders.source') }}</span>
                        <span class="source-badge" :class="order.source === 'seckill' ? 'seckill' : 'normal'">
                            {{ order.source === 'seckill' ? `⚡ ${t('user.orders.sourceSeckill')}` : t('user.orders.sourceNormal') }}
                        </span>
                    </p>
                    <p><span>{{ t('user.orders.createdAt') }}</span>{{ new Date(order.created_at).toLocaleString() }}</p>
                </div>
            </div>
            <div class="card">
                <h3>{{ t('user.orders.items') }}</h3>
                <div class="cards">
                    <div v-for="item in items" :key="item.id" class="card-item" :class="`rarity-${item.rarity}`">
                        <div class="card-img">
                            <img v-if="item.snapshot_image" :src="item.snapshot_image" :alt="item.snapshot_name" />
                            <span v-else>🃏</span>
                        </div>
                        <p class="card-name">{{ item.snapshot_name }}</p>
                        <span class="chip">{{ item.rarity }}</span>
                    </div>
                </div>
            </div>
        </template>
    </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { orderDetail, type DrawOrder, type DrawOrderItem } from '/@/api/user/order'
import { useUserInfo } from '/@/stores/user/userInfo'

const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const userInfo = useUserInfo()

const order = ref<DrawOrder | null>(null)
const items = ref<DrawOrderItem[]>([])
const loading = ref(true)

async function load() {
    if (!userInfo.token) {
        router.push('/user/login')
        return
    }
    loading.value = true
    try {
        const id = Number(route.params.id)
        const { data } = await orderDetail(id)
        order.value = data.order
        items.value = data.items ?? []
    } catch {
        order.value = null
    } finally {
        loading.value = false
    }
}

onMounted(load)
</script>

<style scoped lang="scss">
.detail {
    display: flex;
    flex-direction: column;
    gap: 16px;
}
.back {
    color: #1e40af;
    text-decoration: none;
    font-size: 14px;

    &:hover {
        text-decoration: underline;
    }
}
.card {
    background: #fff;
    border-radius: 16px;
    padding: 24px;

    h2 {
        margin: 0 0 16px;
        font-size: 18px;
        color: #0f172a;
    }
    h3 {
        margin: 0 0 14px;
        font-size: 16px;
        color: #0f172a;
    }
}
.mono {
    font-family: ui-monospace, monospace;
    font-size: 14px;
    color: #64748b;
}
.meta {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
    gap: 12px;

    p {
        margin: 0;
        font-size: 14px;
        color: #334155;

        span {
            display: block;
            font-size: 12px;
            color: #94a3b8;
            margin-bottom: 4px;
        }
    }
}
.source-badge {
    display: inline-block !important;
    margin-bottom: 0 !important;
    padding: 2px 10px;
    border-radius: 999px;
    font-size: 12px !important;
    font-weight: 700;

    &.seckill {
        background: rgba(234, 88, 12, 0.12);
        color: #ea580c;
    }
    &.normal {
        background: #eff6ff;
        color: #1e40af;
    }
}
.cards {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(150px, 1fr));
    gap: 14px;
}
.card-item {
    border: 1px solid #e2e8f0;
    border-radius: 12px;
    padding: 12px;
    text-align: center;
}
.card-img {
    aspect-ratio: 3 / 4;
    background: #f1f5f9;
    border-radius: 8px;
    display: flex;
    align-items: center;
    justify-content: center;
    font-size: 40px;
    margin-bottom: 8px;
    overflow: hidden;

    img {
        width: 100%;
        height: 100%;
        object-fit: cover;
    }
}
.card-name {
    margin: 0 0 6px;
    font-size: 13px;
    color: #0f172a;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
}
.chip {
    display: inline-block;
    padding: 2px 10px;
    border-radius: 999px;
    background: #eff6ff;
    color: #1e40af;
    font-size: 12px;
    font-weight: 700;
}
.rarity-SSR .chip {
    background: rgba(180, 83, 9, 0.12);
    color: #b45309;
}
.rarity-SR .chip {
    background: rgba(124, 58, 237, 0.1);
    color: #7c3aed;
}
.state {
    padding: 48px 0;
    text-align: center;
    color: #64748b;
    background: #fff;
    border-radius: 16px;
}
</style>
