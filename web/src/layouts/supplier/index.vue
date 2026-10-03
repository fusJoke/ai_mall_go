<!-- src\layouts\supplier\index.vue — B 端供应商布局（design D11）。
     与 user 布局同构的轻布局：顶部导航（商品 / 活动 / 登录态），篮球橙蓝主题。 -->
<template>
    <div class="supplier-layout">
        <header class="topbar">
            <div class="topbar-inner">
                <RouterLink to="/supplier/products" class="brand">
                    <span class="brand-icon">🏪</span>
                    <span class="brand-name">供应商中心</span>
                </RouterLink>
                <nav class="nav">
                    <RouterLink to="/supplier/products" class="nav-link">{{ t('supplier.products.title') }}</RouterLink>
                    <RouterLink to="/supplier/promotions" class="nav-link">{{ t('supplier.promotions.title') }}</RouterLink>
                    <RouterLink to="/supplier/seckill" class="nav-link">{{ t('supplier.seckill.title') }}</RouterLink>
                </nav>
                <div class="user-area">
                    <template v-if="supplierInfo.token">
                        <span class="who">{{ supplierInfo.username }}</span>
                        <button type="button" class="logout-btn" @click="doLogout">{{ t('supplier.login.title') === 'Supplier Login' ? 'Logout' : '退出' }}</button>
                    </template>
                    <RouterLink v-else to="/supplier/login" class="login-btn">
                        {{ t('supplier.login.submit') }}
                    </RouterLink>
                </div>
            </div>
        </header>
        <main class="content">
            <RouterView />
        </main>
    </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { useSupplierInfo } from '/@/stores/supplier/supplierInfo'
import { supplierLogout } from '/@/api/supplier/auth'

const { t } = useI18n()
const router = useRouter()
const supplierInfo = useSupplierInfo()

async function doLogout() {
    try {
        await supplierLogout(supplierInfo.token)
    } catch {
        // 幂等，失败不阻塞本地清理
    }
    supplierInfo.reset()
    router.push('/supplier/login')
}
</script>

<style scoped lang="scss">
/* 与 user 布局同一主题族，主色调偏稳重蓝（B 端） */
.supplier-layout {
    min-height: 100vh;
    display: flex;
    flex-direction: column;
    background: #f8fafc;
}
.topbar {
    background: linear-gradient(90deg, #0f172a, #1e3a8a);
    color: #fff;
    position: sticky;
    top: 0;
    z-index: 100;
}
.topbar-inner {
    max-width: 1280px;
    margin: 0 auto;
    padding: 0 24px;
    height: 60px;
    display: flex;
    align-items: center;
    gap: 32px;
}
.brand {
    display: flex;
    align-items: center;
    gap: 8px;
    color: #fff;
    text-decoration: none;
    font-weight: 700;
    font-size: 18px;
}
.nav {
    display: flex;
    gap: 8px;
    flex: 1;
}
.nav-link {
    color: rgba(255, 255, 255, 0.75);
    text-decoration: none;
    padding: 6px 14px;
    border-radius: 999px;
    font-size: 14px;
    transition: all 0.2s;

    &:hover,
    &.router-link-active {
        color: #fff;
        background: rgba(249, 115, 22, 0.85);
    }
}
.user-area {
    display: flex;
    align-items: center;
    gap: 12px;
}
.who {
    font-size: 13px;
    color: rgba(255, 255, 255, 0.85);
}
.logout-btn {
    background: none;
    border: 1px solid rgba(255, 255, 255, 0.4);
    color: #fff;
    border-radius: 999px;
    padding: 4px 14px;
    font-size: 13px;
    cursor: pointer;

    &:hover {
        border-color: #f97316;
        color: #f97316;
    }
}
.login-btn {
    background: #f97316;
    color: #fff;
    text-decoration: none;
    border-radius: 999px;
    padding: 6px 20px;
    font-size: 14px;
}
.content {
    flex: 1;
    width: 100%;
    max-width: 1280px;
    margin: 0 auto;
    padding: 24px;
    box-sizing: border-box;
}
</style>
