<!-- src\views\user\orders\index.vue — 我的订单列表（任务 6.9）。
     登录态自检：无 token 直接引导登录；接口 401 同样清理本地态并跳登录。 -->
<template>
    <div class="orders">
        <h2 class="title">{{ t('user.orders.title') }}</h2>
        <div v-if="loading" class="state">Loading...</div>
        <div v-else-if="items.length === 0" class="state">{{ t('user.orders.empty') }}</div>
        <table v-else class="table">
            <thead>
                <tr>
                    <th>{{ t('user.orders.orderNo') }}</th>
                    <th>{{ t('user.orders.price') }}</th>
                    <th>{{ t('user.orders.status') }}</th>
                    <th>{{ t('user.orders.source') }}</th>
                    <th>{{ t('user.orders.createdAt') }}</th>
                    <th>{{ t('user.orders.action') }}</th>
                </tr>
            </thead>
            <tbody>
                <tr v-for="order in items" :key="order.id">
                    <td class="mono">{{ order.order_no }}</td>
                    <td>¥{{ order.price.toFixed(2) }}</td>
                    <td><span class="status" :class="`status-${order.status}`">{{ statusText(order.status) }}</span></td>
                    <td>{{ order.source === 'seckill' ? t('user.orders.sourceSeckill') : t('user.orders.sourceNormal') }}</td>
                    <td>{{ formatTime(order.created_at) }}</td>
                    <td>
                        <RouterLink :to="`/user/orders/${order.id}`" class="link">{{ t('user.orders.viewDetail') }}</RouterLink>
                    </td>
                </tr>
            </tbody>
        </table>
        <div class="pager">
            <button type="button" :disabled="page <= 1" @click="go(page - 1)">‹</button>
            <span>{{ page }} / {{ totalPages }}</span>
            <button type="button" :disabled="page >= totalPages" @click="go(page + 1)">›</button>
        </div>
    </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { orderList, type DrawOrder } from '/@/api/user/order'
import { useUserInfo } from '/@/stores/user/userInfo'
import { useRouter } from 'vue-router'

const { t } = useI18n()
const router = useRouter()
const userInfo = useUserInfo()

const items = ref<DrawOrder[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = 10
const loading = ref(true)

const totalPages = computed(() => Math.max(1, Math.ceil(total.value / pageSize)))

function statusText(status: string): string {
    const map: Record<string, string> = {
        paid: t('user.orders.statusPaid'),
        drawn: t('user.orders.statusDrawn'),
        failed: t('user.orders.statusFailed'),
        pending: t('user.orders.statusPending'),
    }
    return map[status] ?? status
}

function formatTime(iso: string): string {
    return new Date(iso).toLocaleString()
}

function requireLogin(): boolean {
    if (userInfo.token) return true
    router.push('/user/login')
    return false
}

async function load() {
    if (!requireLogin()) return
    loading.value = true
    try {
        const { data } = await orderList({ page: page.value, page_size: pageSize })
        items.value = data.items ?? []
        total.value = data.total
    } catch (err) {
        const anyErr = err as { response?: { status?: number } }
        if (anyErr?.response?.status === 401) {
            userInfo.reset()
            router.push('/user/login')
            return
        }
        items.value = []
    } finally {
        loading.value = false
    }
}

function go(p: number) {
    page.value = p
    load()
}

onMounted(load)
</script>

<style scoped lang="scss">
.orders {
    background: #fff;
    border-radius: 16px;
    padding: 24px;
}
.title {
    margin: 0 0 16px;
    font-size: 20px;
    color: #0f172a;
}
.table {
    width: 100%;
    border-collapse: collapse;

    th,
    td {
        text-align: left;
        padding: 12px;
        border-bottom: 1px solid #f1f5f9;
        font-size: 14px;
    }
    th {
        color: #64748b;
        font-weight: 500;
    }
}
.mono {
    font-family: ui-monospace, monospace;
    font-size: 13px;
}
.status {
    padding: 2px 10px;
    border-radius: 999px;
    font-size: 12px;
}
.status-drawn {
    background: rgba(22, 163, 74, 0.12);
    color: #16a34a;
}
.status-paid {
    background: rgba(30, 64, 175, 0.1);
    color: #1e40af;
}
.status-pending {
    background: rgba(234, 88, 12, 0.12);
    color: #ea580c;
}
.status-failed {
    background: rgba(220, 38, 38, 0.1);
    color: #dc2626;
}
.link {
    color: #1e40af;
    text-decoration: none;

    &:hover {
        text-decoration: underline;
    }
}
.pager {
    display: flex;
    align-items: center;
    justify-content: flex-end;
    gap: 12px;
    margin-top: 16px;
    color: #64748b;
    font-size: 13px;

    button {
        border: 1px solid #e2e8f0;
        background: #fff;
        border-radius: 6px;
        width: 28px;
        height: 28px;
        cursor: pointer;

        &:disabled {
            opacity: 0.4;
            cursor: not-allowed;
        }
    }
}
.state {
    padding: 48px 0;
    text-align: center;
    color: #64748b;
}
</style>
