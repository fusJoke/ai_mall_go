<!-- src\views\user\draw\result.vue — 开卡结果页（add-user-draw-result-page）。
     嗨卡式开盒仪式感：氛围区 + 卡牌网格（稀有度边框色）+ 订单信息条 + 三条动线出口。
     数据经 sessionStorage 传递（普通抽卡与秒杀抽卡共用本页），读取即清除；
     刷新 / 直达无数据时 replace 订单列表兜底，防丢参白屏。 -->
<template>
    <div class="result">
        <template v-if="result">
            <div class="celebrate">
                <span class="icon">🎉</span>
                <h1>{{ t('user.drawResult.title') }}</h1>
                <p>{{ t('user.drawResult.subtitle') }}</p>
            </div>

            <div class="cards">
                <div v-for="card in result.cards" :key="card.item_id" class="card" :class="`rarity-${card.rarity}`">
                    <div class="card-face">
                        <img v-if="card.snapshot_image" :src="card.snapshot_image" :alt="card.snapshot_name" />
                        <span v-else class="face-fallback">🃏</span>
                    </div>
                    <span class="rarity-chip">{{ card.rarity }}</span>
                    <p class="card-name">{{ card.snapshot_name }}</p>
                </div>
            </div>
            <p v-if="!result.cards?.length" class="state">{{ t('user.drawResult.noCards') }}</p>

            <div class="order-bar">
                <span>{{ t('user.drawResult.orderNo') }}: {{ result.order_no }}</span>
                <span>{{ t('user.drawResult.actualPrice') }}: ¥{{ result.actual_price.toFixed(2) }}</span>
            </div>

            <div class="actions">
                <button type="button" class="btn primary" @click="router.push('/user/orders')">
                    {{ t('user.drawResult.viewOrders') }}
                </button>
                <button type="button" class="btn" @click="router.push(result.source_path || '/user/home')">
                    {{ t('user.drawResult.drawAgain') }}
                </button>
                <button type="button" class="btn ghost" @click="router.push('/user/home')">
                    {{ t('user.drawResult.backHome') }}
                </button>
            </div>
        </template>
    </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { takeDrawResult, type DrawResultPayload } from '/@/utils/drawResult'

const { t } = useI18n()
const router = useRouter()

const result = ref<DrawResultPayload | null>(null)

onMounted(() => {
    result.value = takeDrawResult()
    if (!result.value) {
        // 刷新 / 未抽卡直达：无数据则回订单列表，不白屏。
        router.replace('/user/orders')
    }
})
</script>

<style scoped lang="scss">
.result {
    max-width: 720px;
    margin: 0 auto;
    display: flex;
    flex-direction: column;
    gap: 20px;
}
.celebrate {
    text-align: center;
    padding: 24px 0 4px;

    .icon {
        font-size: 52px;
    }
    h1 {
        margin: 8px 0 4px;
        font-size: 26px;
        color: #0f172a;
    }
    p {
        margin: 0;
        color: #64748b;
        font-size: 14px;
    }
}
.cards {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(150px, 1fr));
    gap: 16px;
}
.card {
    background: #fff;
    border: 2px solid #e2e8f0;
    border-radius: 14px;
    padding: 14px;
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 8px;
}
.card-face {
    width: 100%;
    aspect-ratio: 3 / 4;
    border-radius: 10px;
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
.face-fallback {
    font-size: 44px;
}
.rarity-chip {
    font-size: 12px;
    font-weight: 700;
    padding: 2px 10px;
    border-radius: 999px;
}
.card-name {
    margin: 0;
    font-size: 13px;
    color: #334155;
    text-align: center;
    word-break: break-all;
}
.rarity-SSR {
    border-color: #f59e0b;
    box-shadow: 0 4px 16px rgba(245, 158, 11, 0.35);

    .rarity-chip {
        background: rgba(245, 158, 11, 0.14);
        color: #b45309;
    }
}
.rarity-SR {
    border-color: #8b5cf6;

    .rarity-chip {
        background: rgba(139, 92, 246, 0.12);
        color: #7c3aed;
    }
}
.rarity-R {
    border-color: #3b82f6;

    .rarity-chip {
        background: rgba(59, 130, 246, 0.12);
        color: #1d4ed8;
    }
}
.rarity-N {
    border-color: #cbd5e1;

    .rarity-chip {
        background: #f1f5f9;
        color: #64748b;
    }
}
.order-bar {
    background: #fff;
    border-radius: 12px;
    padding: 14px 20px;
    display: flex;
    justify-content: space-between;
    flex-wrap: wrap;
    gap: 8px;
    font-size: 14px;
    color: #475569;
}
.actions {
    display: flex;
    gap: 12px;
    flex-wrap: wrap;
    justify-content: center;
}
.btn {
    min-width: 132px;
    height: 44px;
    border: 1px solid #cbd5e1;
    border-radius: 10px;
    background: #fff;
    color: #334155;
    font-size: 14px;
    cursor: pointer;
    transition: all 0.15s ease;

    &:hover {
        background: #f8fafc;
    }
}
.btn.primary {
    border: none;
    background: linear-gradient(90deg, #f97316, #ea580c);
    color: #fff;
    font-weight: 700;
    box-shadow: 0 4px 14px rgba(249, 115, 22, 0.35);

    &:hover {
        opacity: 0.92;
        background: linear-gradient(90deg, #f97316, #ea580c);
    }
}
.btn.ghost {
    border-color: transparent;
    background: transparent;
    color: #64748b;
}
.state {
    text-align: center;
    color: #64748b;
}
</style>
