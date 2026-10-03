<!-- src\layouts\user\index.vue — C 端会员布局（design D11）。
     独立目录 + 篮球橙蓝主题；顶部导航（首页 / 我的订单 / 登录态），内容区 router-view。
     不复用 admin 的动态菜单 / init 流程 —— C 端是公开浏览为主的轻布局。 -->
<template>
    <div class="user-layout">
        <header class="topbar">
            <div class="topbar-inner">
                <RouterLink to="/user/home" class="brand">
                    <span class="brand-icon">🏀</span>
                    <span class="brand-name">AI Go Mall</span>
                </RouterLink>
                <nav class="nav">
                    <RouterLink to="/user/home" class="nav-link">{{ t('user.home.title') }}</RouterLink>
                    <RouterLink to="/user/seckill" class="nav-link">{{ t('user.seckill.title') }}</RouterLink>
                    <RouterLink to="/user/orders" class="nav-link">{{ t('user.orders.title') }}</RouterLink>
                </nav>
                <div class="user-area">
                    <template v-if="userInfo.token">
                        <span class="balance">{{ userInfo.nickname || userInfo.username }} · ¥{{ userInfo.balance }}</span>
                        <button type="button" class="logout-btn" @click="doLogout">{{ t('user.login.submit') === 'Sign In' ? 'Logout' : '退出' }}</button>
                    </template>
                    <RouterLink v-else to="/user/login" class="login-btn">
                        {{ t('user.login.submit') }}
                    </RouterLink>
                </div>
            </div>
        </header>
        <main class="content">
            <RouterView />
        </main>
        <footer class="footer">AI GO MALL · 球星卡盲盒商城</footer>
    </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { useUserInfo } from '/@/stores/user/userInfo'
import { userLogout } from '/@/api/user/auth'
import { useRouter } from 'vue-router'

const { t } = useI18n()
const router = useRouter()
const userInfo = useUserInfo()

async function doLogout() {
    try {
        await userLogout(userInfo.token)
    } catch {
        // 后端登出幂等，失败不阻塞本地清理
    }
    userInfo.reset()
    router.push('/user/login')
}
</script>

<style scoped lang="scss">
/* 篮球橙蓝主题（D11）：主橙 #f97316，副蓝 #1e3a8a */
.user-layout {
    min-height: 100vh;
    display: flex;
    flex-direction: column;
    background: #f8fafc;
}
.topbar {
    background: linear-gradient(90deg, #1e3a8a, #1e40af);
    color: #fff;
    position: sticky;
    top: 0;
    z-index: 100;
}
.topbar-inner {
    max-width: 1200px;
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
.brand-icon {
    font-size: 22px;
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
.balance {
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

    &:hover {
        opacity: 0.9;
    }
}
.content {
    flex: 1;
    width: 100%;
    max-width: 1200px;
    margin: 0 auto;
    padding: 24px;
    box-sizing: border-box;
}
.footer {
    text-align: center;
    padding: 24px;
    color: #94a3b8;
    font-size: 13px;
}
</style>
