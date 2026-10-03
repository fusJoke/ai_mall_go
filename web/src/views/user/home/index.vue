<!-- src\views\user\home\index.vue — C 端首页（任务 6.7）。
     ES 混合 feed：featured 盲盒卡片网格 + featured 供应商列表；
     ES 不可用（500 search_unavailable）时显示可重试的空态。 -->
<template>
    <div class="home">
        <section class="section">
            <h2 class="section-title">{{ t('user.home.title') }}</h2>
            <div v-if="loading" class="state">Loading...</div>
            <div v-else-if="error" class="state error">
                {{ t('user.home.searchUnavailable') }}
                <button type="button" class="retry" @click="load">↻</button>
            </div>
            <div v-else-if="feed.blind_boxes.length === 0" class="state">{{ t('user.home.empty') }}</div>
            <div v-else class="grid">
                <div v-for="box in feed.blind_boxes" :key="box.id" class="box-card" @click="goDetail(box.id)">
                    <div class="cover">
                        <img v-if="box.cover" :src="box.cover" :alt="box.name" loading="lazy" />
                        <span v-else class="cover-fallback">🏀</span>
                        <span v-if="box.promo_price" class="promo-badge">SALE</span>
                    </div>
                    <div class="body">
                        <h3 class="name">{{ box.name }}</h3>
                        <p class="supplier">{{ box.supplier_name }}</p>
                        <p v-if="box.rarity_summary" class="rarity">{{ box.rarity_summary }}</p>
                        <div class="price-row">
                            <span class="price">¥{{ formatPrice(box.promo_price ?? box.price) }}</span>
                            <span v-if="box.promo_price" class="price-origin">¥{{ formatPrice(box.price) }}</span>
                            <span class="detail-link">{{ t('user.home.viewDetail') }} →</span>
                        </div>
                    </div>
                </div>
            </div>
        </section>

        <section v-if="!error && feed.suppliers.length > 0" class="section">
            <h2 class="section-title">{{ t('user.home.suppliers') }}</h2>
            <div class="supplier-row">
                <div v-for="sup in feed.suppliers" :key="sup.id" class="supplier-card">
                    <img v-if="sup.logo" class="logo" :src="sup.logo" :alt="sup.name" />
                    <span v-else class="logo logo-fallback">🏪</span>
                    <div class="supplier-info">
                        <h4>{{ sup.name }}</h4>
                        <p>{{ sup.blind_box_count }} {{ t('user.home.blindBoxCount') }}</p>
                    </div>
                </div>
            </div>
        </section>
    </div>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { homeFeed, type FeedResponse } from '/@/api/user/home'

const { t } = useI18n()
const router = useRouter()

const feed = reactive<FeedResponse>({ blind_boxes: [], suppliers: [], fetched_at: '' })
const loading = ref(true)
const error = ref(false)

function formatPrice(v: number): string {
    return v.toFixed(2)
}

async function load() {
    loading.value = true
    error.value = false
    try {
        const { data } = await homeFeed()
        feed.blind_boxes = data.blind_boxes ?? []
        feed.suppliers = data.suppliers ?? []
        feed.fetched_at = data.fetched_at
    } catch {
        error.value = true
    } finally {
        loading.value = false
    }
}

function goDetail(id: number) {
    router.push(`/user/blindbox/${id}`)
}

onMounted(load)
</script>

<style scoped lang="scss">
.home {
    display: flex;
    flex-direction: column;
    gap: 32px;
}
.section-title {
    font-size: 20px;
    color: #0f172a;
    margin: 0 0 16px;
}
.grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(240px, 1fr));
    gap: 16px;
}
.box-card {
    background: #fff;
    border-radius: 12px;
    overflow: hidden;
    cursor: pointer;
    box-shadow: 0 1px 3px rgba(15, 23, 42, 0.1);
    transition: transform 0.2s, box-shadow 0.2s;

    &:hover {
        transform: translateY(-3px);
        box-shadow: 0 8px 24px rgba(30, 58, 138, 0.15);
    }
}
.cover {
    position: relative;
    aspect-ratio: 4 / 3;
    background: #e2e8f0;
    display: flex;
    align-items: center;
    justify-content: center;

    img {
        width: 100%;
        height: 100%;
        object-fit: cover;
    }
}
.cover-fallback {
    font-size: 56px;
}
.promo-badge {
    position: absolute;
    top: 10px;
    left: 10px;
    background: #f97316;
    color: #fff;
    font-size: 12px;
    font-weight: 700;
    padding: 2px 10px;
    border-radius: 999px;
}
.body {
    padding: 14px;
}
.name {
    margin: 0 0 4px;
    font-size: 15px;
    color: #0f172a;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
}
.supplier {
    margin: 0 0 6px;
    font-size: 12px;
    color: #64748b;
}
.rarity {
    margin: 0 0 8px;
    font-size: 11px;
    color: #1e40af;
}
.price-row {
    display: flex;
    align-items: baseline;
    gap: 8px;
}
.price {
    color: #ea580c;
    font-size: 18px;
    font-weight: 700;
}
.price-origin {
    color: #94a3b8;
    font-size: 13px;
    text-decoration: line-through;
}
.detail-link {
    margin-left: auto;
    font-size: 12px;
    color: #1e40af;
}
.supplier-row {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(220px, 1fr));
    gap: 12px;
}
.supplier-card {
    display: flex;
    align-items: center;
    gap: 12px;
    background: #fff;
    border-radius: 12px;
    padding: 14px;
    box-shadow: 0 1px 3px rgba(15, 23, 42, 0.1);
}
.logo {
    width: 44px;
    height: 44px;
    border-radius: 10px;
    object-fit: cover;
}
.logo-fallback {
    display: flex;
    align-items: center;
    justify-content: center;
    background: #eff6ff;
    font-size: 22px;
}
.supplier-info {
    h4 {
        margin: 0 0 2px;
        font-size: 14px;
        color: #0f172a;
    }
    p {
        margin: 0;
        font-size: 12px;
        color: #64748b;
    }
}
.state {
    padding: 48px 0;
    text-align: center;
    color: #64748b;
    background: #fff;
    border-radius: 12px;
}
.state.error {
    color: #dc2626;
}
.retry {
    margin-left: 8px;
    border: none;
    background: none;
    color: #1e40af;
    font-size: 16px;
    cursor: pointer;
}
</style>
