<!-- src\views\user\seckill\detail.vue — 秒杀活动详情（任务 12.3）。
     限购规则 + 实时剩余名额（5s 轮询）+ 秒杀按钮；未开始显示开抢倒计时，
     进行中显示结束倒计时；秒杀错误码 → 业务文案（与 handler/user/seckill.go 对齐）。 -->
<template>
    <div v-if="detail" class="detail">
        <RouterLink to="/user/seckill" class="back">← {{ t('user.seckill.backToList') }}</RouterLink>

        <div class="main">
            <div class="cover">
                <img v-if="detail.blind_box_cover" :src="detail.blind_box_cover" :alt="detail.blind_box_name" />
                <span v-else class="cover-fallback">⚡</span>
            </div>
            <div class="info">
                <div class="title-row">
                    <h1>{{ detail.blind_box_name }}</h1>
                    <span class="flash-tag">{{ t('user.seckill.tag') }}</span>
                </div>
                <p class="supplier">{{ detail.supplier_name }}</p>
                <p v-if="detail.description" class="desc">{{ detail.description }}</p>

                <div class="price-block">
                    <span class="seckill-price">¥{{ detail.seckill_price.toFixed(2) }}</span>
                    <span class="origin-price">¥{{ detail.original_price.toFixed(2) }}</span>
                    <span class="save-tag">-{{ discountPercent }}%</span>
                </div>

                <div class="rules">
                    <div class="rule">
                        <span class="label">{{ t('user.seckill.perUserLimit') }}</span>
                        <b>{{ detail.per_user_limit }} {{ t('user.seckill.limitUnit') }}</b>
                    </div>
                    <div class="rule">
                        <span class="label">{{ t('user.seckill.totalStock') }}</span>
                        <b>{{ detail.total_stock }}</b>
                    </div>
                    <div class="rule">
                        <span class="label">{{ t('user.seckill.remaining') }}</span>
                        <b class="remaining" :class="{ out: remainingOut }">{{ remainingText }}</b>
                    </div>
                    <div class="rule">
                        <span class="label">{{ t('user.seckill.window') }}</span>
                        <b>{{ formatTime(detail.start_at) }} ~ {{ formatTime(detail.end_at) }}</b>
                    </div>
                </div>

                <div class="countdown" :class="phase">
                    <template v-if="phase === 'upcoming'">
                        {{ t('user.seckill.startsIn') }} <b>{{ countdownText }}</b>
                    </template>
                    <template v-else-if="phase === 'active'">
                        {{ t('user.seckill.endsIn') }} <b>{{ countdownText }}</b>
                    </template>
                    <template v-else>{{ t('user.seckill.ended') }}</template>
                </div>

                <button type="button" class="draw-btn" :disabled="!canDraw || drawing" @click="onDraw">
                    {{ buttonText }}
                </button>
                <p v-if="msg" :class="['msg', msgType]">{{ msg }}</p>
            </div>
        </div>

        <div class="link-panel">
            <RouterLink :to="`/user/blindbox/${detail.blind_box_id}`" class="blindbox-link">
                {{ t('user.seckill.viewBlindbox') }} →
            </RouterLink>
        </div>
    </div>
    <div v-else class="state">{{ loading ? 'Loading...' : t('user.seckill.notAvailable') }}</div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { seckillDetail, seckillDraw, type SeckillDetail as Detail } from '/@/api/user/seckill'
import { useUserInfo } from '/@/stores/user/userInfo'

const { t, locale } = useI18n()
const route = useRoute()
const router = useRouter()
const userInfo = useUserInfo()

const detail = ref<Detail | null>(null)
const loading = ref(true)
const drawing = ref(false)
const msg = ref('')
const msgType = ref<'error' | 'success'>('error')
const now = ref(Date.now())
let tickTimer: number | undefined
let pollTimer: number | undefined

const phase = computed<'upcoming' | 'active' | 'ended'>(() => {
    if (!detail.value) return 'ended'
    if (now.value < new Date(detail.value.start_at).getTime()) return 'upcoming'
    if (now.value >= new Date(detail.value.end_at).getTime()) return 'ended'
    return 'active'
})

const discountPercent = computed(() => {
    if (!detail.value || detail.value.original_price <= 0) return 0
    return Math.round((1 - detail.value.seckill_price / detail.value.original_price) * 100)
})

const remainingOut = computed(() => !!detail.value && detail.value.remaining_stock === 0)

const remainingText = computed(() => {
    if (!detail.value) return '-'
    // -1 = 后端 Redis / DB 兜底都不可知，显示占位不误导。
    if (detail.value.remaining_stock < 0) return t('user.seckill.stockUnknown')
    return String(detail.value.remaining_stock)
})

const canDraw = computed(() => phase.value === 'active' && !!detail.value && detail.value.remaining_stock !== 0)

const buttonText = computed(() => {
    if (drawing.value) return t('user.seckill.drawing')
    if (!detail.value) return t('user.seckill.draw')
    if (phase.value === 'upcoming') return t('user.seckill.notStarted')
    if (phase.value === 'ended') return t('user.seckill.ended')
    if (detail.value.remaining_stock === 0) return t('user.seckill.soldOut')
    return t('user.seckill.draw')
})

const countdownText = computed(() => {
    if (!detail.value) return ''
    const target = phase.value === 'upcoming' ? new Date(detail.value.start_at).getTime() : new Date(detail.value.end_at).getTime()
    const diff = target - now.value
    if (diff <= 0) return '00:00:00'
    const h = Math.floor(diff / 3600000)
    const m = Math.floor((diff % 3600000) / 60000)
    const s = Math.floor((diff % 60000) / 1000)
    const pad = (n: number) => String(n).padStart(2, '0')
    return `${h}:${pad(m)}:${pad(s)}`
})

function formatTime(iso: string): string {
    return new Date(iso).toLocaleString(locale.value)
}

function show(type: 'error' | 'success', text: string) {
    msgType.value = type
    msg.value = text
}

/** 秒杀错误码 → 文案（与 handler/user/seckill.go 的 mapSeckillErr 对齐）。 */
function describeSeckillError(err: unknown): string {
    const anyErr = err as { response?: { status?: number; data?: { code?: string } } } | undefined
    const code = anyErr?.response?.data?.code
    const map: Record<string, string> = {
        'seckill.sold_out': t('user.seckill.soldOutMsg'),
        'seckill.user_limit_exceeded': t('user.seckill.userLimitExceeded'),
        'seckill.insufficient_balance': t('user.seckill.insufficientBalance'),
        'seckill.not_in_window': t('user.seckill.notInWindow'),
        'seckill.not_available': t('user.seckill.notAvailable'),
        'seckill.blindbox_not_available': t('user.seckill.blindboxNotAvailable'),
        'seckill.user_not_available': t('user.seckill.notAvailable'),
        rate_limited: t('user.seckill.rateLimited'),
        'seckill.unauthorized': t('user.seckill.loginRequired'),
    }
    if (code && map[code]) return map[code]
    if (anyErr?.response?.status === 401) return t('user.seckill.loginRequired')
    return t('user.seckill.drawFailed')
}

async function onDraw() {
    if (!userInfo.token) {
        router.push('/user/login')
        return
    }
    drawing.value = true
    msg.value = ''
    try {
        const { data } = await seckillDraw(detail.value!.id)
        const first = data.cards?.[0]
        show(
            'success',
            `${t('user.blindbox.drawSuccess')} ${first ? `[${first.rarity}] ${first.snapshot_name}` : ''}（${t('user.orders.orderNo')} ${data.order_no}）`,
        )
        // 抢到了立刻刷新剩余名额，不等下一个轮询周期。
        load()
    } catch (err) {
        const anyErr = err as { response?: { status?: number } }
        if (anyErr?.response?.status === 401) {
            router.push('/user/login')
            return
        }
        show('error', describeSeckillError(err))
        load()
    } finally {
        drawing.value = false
    }
}

async function load() {
    try {
        const id = Number(route.params.id)
        const { data } = await seckillDetail(id)
        detail.value = data
    } catch {
        detail.value = null
    } finally {
        loading.value = false
    }
}

onMounted(() => {
    load()
    tickTimer = window.setInterval(() => (now.value = Date.now()), 1000)
    // 实时剩余名额：5s 轮询（后端读 Redis，单 GET 成本可忽略）。
    pollTimer = window.setInterval(load, 5000)
})

onBeforeUnmount(() => {
    if (tickTimer) window.clearInterval(tickTimer)
    if (pollTimer) window.clearInterval(pollTimer)
})
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
    background: #f1f5f9;
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
.title-row {
    display: flex;
    align-items: center;
    gap: 10px;
}
.flash-tag {
    background: linear-gradient(90deg, #f97316, #ea580c);
    color: #fff;
    font-size: 12px;
    font-weight: 700;
    padding: 3px 10px;
    border-radius: 999px;
    flex-shrink: 0;
}
.supplier {
    margin: 0;
    color: #1e40af;
    font-size: 14px;
}
.desc {
    margin: 0;
    color: #475569;
    font-size: 14px;
    line-height: 1.6;
}
.price-block {
    display: flex;
    align-items: baseline;
    gap: 10px;
}
.seckill-price {
    font-size: 32px;
    font-weight: 800;
    color: #ea580c;
}
.origin-price {
    color: #94a3b8;
    text-decoration: line-through;
    font-size: 16px;
}
.save-tag {
    background: rgba(220, 38, 38, 0.1);
    color: #dc2626;
    font-size: 12px;
    font-weight: 700;
    padding: 2px 8px;
    border-radius: 999px;
}
.rules {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
    gap: 10px;
    padding: 14px;
    background: #f8fafc;
    border-radius: 10px;
}
.rule {
    .label {
        display: block;
        font-size: 12px;
        color: #94a3b8;
        margin-bottom: 2px;
    }
    b {
        font-size: 14px;
        color: #0f172a;
    }
}
.remaining {
    color: #ea580c !important;
    &.out {
        color: #dc2626 !important;
    }
}
.countdown {
    font-size: 14px;
    color: #64748b;
    font-variant-numeric: tabular-nums;

    b {
        color: #ea580c;
        font-size: 16px;
    }
    &.ended {
        color: #94a3b8;
    }
}
.draw-btn {
    align-self: flex-start;
    min-width: 220px;
    height: 52px;
    border: none;
    border-radius: 12px;
    background: linear-gradient(90deg, #dc2626, #ea580c);
    color: #fff;
    font-size: 17px;
    font-weight: 700;
    cursor: pointer;
    box-shadow: 0 6px 18px rgba(234, 88, 12, 0.35);

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
.link-panel {
    background: #fff;
    border-radius: 16px;
    padding: 16px 24px;
}
.blindbox-link {
    color: #1e40af;
    text-decoration: none;
    font-size: 14px;
    font-weight: 600;

    &:hover {
        text-decoration: underline;
    }
}
.state {
    padding: 64px 0;
    text-align: center;
    color: #64748b;
    background: #fff;
    border-radius: 16px;
}
@media (max-width: 768px) {
    .main {
        grid-template-columns: 1fr;
    }
}
</style>
