<!-- src\views\user\follow\index.vue — 我的关注（任务 18.11）
     登录态自检：无 token 直接引导登录；接口 401 同样清理本地态并跳登录。
     单个取关按钮直接调 DELETE /user/follow；分页器与 orders/index.vue 同构。 -->
<template>
    <div class="follow">
        <h2 class="title">{{ t('user.follow.title') }}</h2>
        <div v-if="loading" class="state">Loading...</div>
        <div v-else-if="items.length === 0" class="state">{{ t('user.follow.empty') }}</div>
        <ul v-else class="list">
            <li v-for="sup in items" :key="sup.id" class="card">
                <div class="card-left">
                    <img v-if="sup.logo" :src="sup.logo" :alt="sup.name" class="avatar" />
                    <span v-else class="avatar-fallback">🏬</span>
                </div>
                <div class="card-body">
                    <h3 class="name">{{ sup.name }}</h3>
                    <p v-if="sup.bio" class="bio">{{ sup.bio }}</p>
                </div>
                <button type="button" class="unfollow-btn" :disabled="pendingId === sup.id" @click="onUnfollow(sup.id)">
                    {{ t('user.blindbox.followBtnActive') }}
                </button>
            </li>
        </ul>
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
import { useRouter } from 'vue-router'
import { followingList, unfollowSupplier, type FollowingSupplier } from '/@/api/user/follow'
import { useUserInfo } from '/@/stores/user/userInfo'

const { t } = useI18n()
const router = useRouter()
const userInfo = useUserInfo()

const items = ref<FollowingSupplier[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = 10
const loading = ref(true)
const pendingId = ref(0)

const totalPages = computed(() => Math.max(1, Math.ceil(total.value / pageSize)))

function requireLogin(): boolean {
    if (userInfo.token) return true
    router.push('/user/login')
    return false
}

async function load() {
    if (!requireLogin()) {
        loading.value = false
        return
    }
    loading.value = true
    try {
        const { data } = await followingList({ page: page.value, page_size: pageSize })
        items.value = data.items
        total.value = data.total
    } catch {
        items.value = []
        total.value = 0
    } finally {
        loading.value = false
    }
}

function go(p: number) {
    if (p < 1 || p > totalPages.value) return
    page.value = p
    load()
}

async function onUnfollow(supplierId: number) {
    pendingId.value = supplierId
    try {
        await unfollowSupplier(supplierId)
        // 乐观移除（接口幂等，无需回填）。
        items.value = items.value.filter((s) => s.id !== supplierId)
        total.value = Math.max(0, total.value - 1)
    } catch {
        // 静默失败：用户场景下一次重新请求即可
    } finally {
        pendingId.value = 0
    }
}

onMounted(load)
</script>

<style scoped lang="scss">
.follow {
    display: flex;
    flex-direction: column;
    gap: 16px;
}
.title {
    margin: 0 0 8px;
    font-size: 22px;
    color: #0f172a;
}
.state {
    padding: 64px 0;
    text-align: center;
    color: #94a3b8;
}
.list {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: 10px;
}
.card {
    display: grid;
    grid-template-columns: 64px 1fr auto;
    gap: 16px;
    align-items: center;
    background: #fff;
    border-radius: 12px;
    padding: 14px 18px;
    box-shadow: 0 1px 2px rgba(15, 23, 42, 0.04);
}
.avatar {
    width: 56px;
    height: 56px;
    border-radius: 12px;
    object-fit: cover;
}
.avatar-fallback {
    width: 56px;
    height: 56px;
    border-radius: 12px;
    background: #e2e8f0;
    display: flex;
    align-items: center;
    justify-content: center;
    font-size: 30px;
}
.card-body {
    display: flex;
    flex-direction: column;
    gap: 4px;
}
.name {
    margin: 0;
    font-size: 16px;
    color: #0f172a;
}
.bio {
    margin: 0;
    font-size: 13px;
    color: #64748b;
    line-height: 1.5;
}
.unfollow-btn {
    border: 1px solid #cbd5e1;
    background: #fff;
    color: #475569;
    padding: 8px 14px;
    border-radius: 8px;
    font-size: 13px;
    cursor: pointer;

    &:hover:not(:disabled) {
        background: #f1f5f9;
    }
    &:disabled {
        opacity: 0.55;
        cursor: not-allowed;
    }
}
.pager {
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 12px;
    padding-top: 8px;

    button {
        border: 1px solid #cbd5e1;
        background: #fff;
        color: #475569;
        width: 32px;
        height: 32px;
        border-radius: 6px;
        cursor: pointer;
    }
    button:disabled {
        opacity: 0.45;
        cursor: not-allowed;
    }
}
</style>