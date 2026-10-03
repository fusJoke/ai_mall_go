<!-- src\views\user\blindbox\detail.vue — 盲盒详情（任务 6.8）。
     封面 + 概率公示表 + 限时特价信息 + 抽卡按钮 + 卡池预览。
     抽卡需登录态：无 token 提示登录；402/409/404/429 分别映射业务文案（user.blindbox.*）。 -->
<template>
    <div v-if="detail" class="detail">
        <div class="main">
            <div class="cover">
                <img v-if="detail.blind_box.cover" :src="detail.blind_box.cover" :alt="detail.blind_box.name" />
                <span v-else class="cover-fallback">🏀</span>
            </div>
            <div class="info">
                <h1>{{ detail.blind_box.name }}</h1>
                <div class="supplier-row">
                    <p class="supplier">{{ detail.supplier?.name }}</p>
                    <button
                        v-if="detail.supplier?.id"
                        type="button"
                        class="follow-btn"
                        :class="{ active: following }"
                        :disabled="followPending"
                        @click="onFollowToggle"
                    >
                        {{ following ? t('user.blindbox.followBtnActive') : t('user.blindbox.followBtn') }}
                    </button>
                </div>
                <p v-if="detail.blind_box.description" class="desc">{{ detail.blind_box.description }}</p>

                <div class="price-block">
                    <template v-if="detail.active_promotion">
                        <span class="promo-price">¥{{ formatPrice(detail.effective_price) }}</span>
                        <span class="origin-price">¥{{ formatPrice(detail.blind_box.price) }}</span>
                        <span class="promo-tag">{{ t('user.blindbox.promotionPrice') }}</span>
                        <p class="promo-ends">
                            {{ t('user.blindbox.promotionEnds') }}: {{ formatTime(detail.active_promotion.end_at) }}
                        </p>
                    </template>
                    <template v-else>
                        <span class="promo-price">¥{{ formatPrice(detail.effective_price) }}</span>
                    </template>
                    <span class="stock">{{ t('user.blindbox.stock') }}: {{ detail.pool_total_stock }}</span>
                </div>

                <button
                    type="button"
                    class="draw-btn"
                    :disabled="drawing || detail.pool_total_stock === 0"
                    @click="onDraw"
                >
                    {{ buttonText }}
                </button>
                <p v-if="msg" :class="['msg', msgType]">{{ msg }}</p>
            </div>
        </div>

        <section class="panel">
            <h2>{{ t('user.blindbox.probability') }}</h2>
            <table class="table">
                <thead>
                    <tr>
                        <th>{{ t('user.blindbox.rarity') || 'Rarity' }}</th>
                        <th>Probability</th>
                        <th>{{ t('user.blindbox.stock') }}</th>
                    </tr>
                </thead>
                <tbody>
                    <tr v-for="row in rarityRows" :key="row.rarity" :class="`rarity-${row.rarity}`">
                        <td>{{ row.rarity }}</td>
                        <td>{{ row.percent }}%</td>
                        <td>{{ row.stock }}</td>
                    </tr>
                </tbody>
            </table>
        </section>

        <section class="panel">
            <h2>{{ t('user.blindbox.poolPreview') }}</h2>
            <div class="pool-grid">
                <div v-for="item in detail.items" :key="item.id" class="pool-item" :class="`rarity-${item.rarity}`">
                    <span class="rarity-chip">{{ item.rarity }}</span>
                    <span class="pool-weight">P {{ (item.weight / 100).toFixed(0) }}%</span>
                </div>
            </div>
        </section>
    </div>
    <div v-else class="state">{{ loading ? 'Loading...' : t('user.home.empty') }}</div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { blindBoxDetail, draw, type BlindBoxDetail as Detail } from '/@/api/user/blindbox'
import { followSupplier, unfollowSupplier } from '/@/api/user/follow'
import { useUserInfo } from '/@/stores/user/userInfo'

const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const userInfo = useUserInfo()

const detail = ref<Detail | null>(null)
const loading = ref(true)
const drawing = ref(false)
const msg = ref('')
const msgType = ref<'error' | 'success'>('error')

// 关注状态：未登录为 null；登录态切换由 onFollowToggle 触发；详情页不主动拉「是否已关注」。
// MVP 简化：默认 false，用户点关注后才置 true；不再单独调 GET 校验态（与 spec 一致）。
const following = ref(false)
const followPending = ref(false)

const buttonText = computed(() => {
    if (drawing.value) return t('user.blindbox.drawing')
    if (!detail.value) return t('user.blindbox.draw')
    if (detail.value.pool_total_stock === 0) return t('user.blindbox.soldOut')
    return t('user.blindbox.draw')
})

/** 概率公示行：按稀有度聚合 weight（同稀有度可能多行）。 */
const rarityRows = computed(() => {
    if (!detail.value) return []
    const total = detail.value.items.reduce((acc, it) => acc + it.weight, 0) || 1
    const agg = new Map<string, { weight: number; stock: number }>()
    for (const it of detail.value.items) {
        const prev = agg.get(it.rarity) ?? { weight: 0, stock: 0 }
        prev.weight += it.weight
        prev.stock += it.stock
        agg.set(it.rarity, prev)
    }
    const order = ['SSR', 'SR', 'R', 'N']
    return order
        .filter((r) => agg.has(r))
        .map((r) => {
            const row = agg.get(r)!
            return {
                rarity: r,
                percent: ((row.weight / total) * 100).toFixed(1),
                stock: row.stock,
            }
        })
})

function formatPrice(v: number): string {
    return v.toFixed(2)
}

function formatTime(iso: string): string {
    return new Date(iso).toLocaleString()
}

function show(type: 'error' | 'success', text: string) {
    msgType.value = type
    msg.value = text
}

/** 抽卡错误码 → 文案映射（与后端 handler/user/draw.go 的错误码对齐）。 */
function describeDrawError(err: unknown): string {
    const anyErr = err as { response?: { status?: number; data?: { code?: string } } } | undefined
    const code = anyErr?.response?.data?.code
    const map: Record<string, string> = {
        'draw.sold_out': t('user.blindbox.soldOutMsg'),
        'draw.insufficient_balance': t('user.blindbox.insufficientBalance'),
        'draw.blindbox_not_available': t('user.blindbox.notAvailable'),
        'draw.user_not_available': t('user.blindbox.notAvailable'),
        rate_limited: t('user.blindbox.rateLimited'),
        'draw.unauthorized': t('user.blindbox.loginRequired'),
    }
    if (code && map[code]) return map[code]
    if (anyErr?.response?.status === 401) return t('user.blindbox.loginRequired')
    return t('user.blindbox.drawFailed')
}

async function onDraw() {
    if (!userInfo.token) {
        router.push('/user/login')
        return
    }
    drawing.value = true
    msg.value = ''
    try {
        const { data } = await draw(detail.value!.blind_box.id)
        const first = data.cards?.[0]
        show(
            'success',
            `${t('user.blindbox.drawSuccess')} ${first ? `[${first.rarity}] ${first.snapshot_name}` : ''}（${t('user.orders.orderNo')} ${data.order_no}）`,
        )
    } catch (err) {
        const anyErr = err as { response?: { status?: number } }
        if (anyErr?.response?.status === 401) {
            router.push('/user/login')
            return
        }
        show('error', describeDrawError(err))
    } finally {
        drawing.value = false
    }
}

/** 切换关注状态：未登录跳登录；登录态乐观翻转 + 失败回滚。 */
async function onFollowToggle() {
    if (!detail.value?.supplier?.id) return
    if (!userInfo.token) {
        router.push('/user/login')
        return
    }
    const supplierId = detail.value.supplier.id
    const prev = following.value
    following.value = !prev
    followPending.value = true
    msg.value = ''
    try {
        if (prev) {
            await unfollowSupplier(supplierId)
        } else {
            await followSupplier(supplierId)
        }
    } catch (err) {
        following.value = prev
        const anyErr = err as { response?: { data?: { code?: string } } }
        const code = anyErr?.response?.data?.code
        if (code === 'follow.unauthorized') {
            router.push('/user/login')
            return
        }
        show('error', t('user.blindbox.drawFailed'))
    } finally {
        followPending.value = false
    }
}

async function load() {
    loading.value = true
    try {
        const id = Number(route.params.id)
        const { data } = await blindBoxDetail(id)
        detail.value = data
    } catch {
        detail.value = null
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
    gap: 24px;
}
.main {
    display: grid;
    grid-template-columns: 380px 1fr;
    gap: 24px;
    background: #fff;
    border-radius: 16px;
    padding: 24px;
}
.cover {
    aspect-ratio: 1;
    border-radius: 12px;
    background: #e2e8f0;
    display: flex;
    align-items: center;
    justify-content: center;
    overflow: hidden;

    img {
        width: 100%;
        height: 100%;
        object-fit: cover;
    }
}
.cover-fallback {
    font-size: 96px;
}
.info {
    display: flex;
    flex-direction: column;
    gap: 12px;

    h1 {
        margin: 0;
        font-size: 26px;
        color: #0f172a;
    }
}
.supplier {
    margin: 0;
    color: #1e40af;
    font-size: 14px;
}
.supplier-row {
    display: flex;
    align-items: center;
    gap: 10px;
}
.follow-btn {
    border: 1px solid #cbd5e1;
    background: #fff;
    color: #1e40af;
    font-size: 13px;
    padding: 5px 12px;
    border-radius: 999px;
    cursor: pointer;
    transition: all 0.15s ease;

    &:hover:not(:disabled) {
        background: #eff6ff;
    }
    &:disabled {
        opacity: 0.55;
        cursor: not-allowed;
    }
}
.follow-btn.active {
    background: #1e40af;
    color: #fff;
    border-color: #1e40af;
}
.desc {
    margin: 0;
    color: #475569;
    font-size: 14px;
    line-height: 1.6;
}
.price-block {
    display: flex;
    align-items: center;
    gap: 12px;
    flex-wrap: wrap;
}
.promo-price {
    font-size: 30px;
    font-weight: 800;
    color: #ea580c;
}
.origin-price {
    color: #94a3b8;
    text-decoration: line-through;
    font-size: 16px;
}
.promo-tag {
    background: rgba(249, 115, 22, 0.12);
    color: #ea580c;
    font-size: 12px;
    font-weight: 700;
    padding: 3px 10px;
    border-radius: 999px;
}
.promo-ends {
    width: 100%;
    margin: 0;
    font-size: 12px;
    color: #94a3b8;
}
.stock {
    font-size: 13px;
    color: #64748b;
}
.draw-btn {
    align-self: flex-start;
    min-width: 200px;
    height: 52px;
    border: none;
    border-radius: 12px;
    background: linear-gradient(90deg, #f97316, #ea580c);
    color: #fff;
    font-size: 17px;
    font-weight: 700;
    cursor: pointer;
    box-shadow: 0 6px 18px rgba(249, 115, 22, 0.35);

    &:hover:not(:disabled) {
        transform: translateY(-1px);
    }
    &:disabled {
        opacity: 0.55;
        cursor: not-allowed;
        box-shadow: none;
    }
}
.msg {
    margin: 0;
    font-size: 14px;
    padding: 10px 14px;
    border-radius: 8px;
}
.msg.error {
    background: rgba(220, 38, 38, 0.08);
    color: #dc2626;
}
.msg.success {
    background: rgba(22, 163, 74, 0.1);
    color: #16a34a;
}
.panel {
    background: #fff;
    border-radius: 16px;
    padding: 20px 24px;

    h2 {
        margin: 0 0 14px;
        font-size: 17px;
        color: #0f172a;
    }
}
.table {
    width: 100%;
    border-collapse: collapse;

    th,
    td {
        text-align: left;
        padding: 10px 12px;
        border-bottom: 1px solid #f1f5f9;
        font-size: 14px;
    }
    th {
        color: #64748b;
        font-weight: 500;
    }
}
.rarity-SSR td,
.rarity-SSR {
    color: #b45309;
    font-weight: 600;
}
.rarity-SR {
    color: #7c3aed;
}
.rarity-R {
    color: #1d4ed8;
}
.rarity-N {
    color: #64748b;
}
.pool-grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(120px, 1fr));
    gap: 10px;
}
.pool-item {
    border: 1px solid #e2e8f0;
    border-radius: 10px;
    padding: 12px;
    display: flex;
    align-items: center;
    justify-content: space-between;
}
.rarity-chip {
    font-weight: 700;
    font-size: 13px;
}
.pool-weight {
    font-size: 12px;
    color: #94a3b8;
}
.state {
    padding: 64px 0;
    text-align: center;
    color: #64748b;
}
@media (max-width: 768px) {
    .main {
        grid-template-columns: 1fr;
    }
}
</style>
