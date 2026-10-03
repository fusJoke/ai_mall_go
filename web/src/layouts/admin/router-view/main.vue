<template>
    <el-main class="layout-main">
        <el-scrollbar class="layout-main-scrollbar">
            <router-view v-slot="{ Component }">
                <transition :name="config.layout.mainAnimation" mode="out-in">
                    <keep-alive :include="navTabs.state.keepAliveTabs">
                        <component :is="Component" :key="state.componentKey" />
                    </keep-alive>
                </transition>
            </router-view>
        </el-scrollbar>
    </el-main>
</template>

<script setup lang="ts">
import { provide, reactive } from 'vue'
import { useConfig } from '/@/stores/config'
import { useNavTabs } from '/@/stores/navTabs'

/**
 * 主内容区（openspec add-admin-layout-shell），对齐 buildadmin router-view/main.vue：
 * scrollbar + 页面过渡动画 + 刷新 key + keep-alive。
 *
 * - 「刷新」由标签页右键菜单触发：provide 的钩子递增 componentKey 强制重挂载当前视图；
 * - keep-alive include 取 navTabs.keepAliveTabs（现存标签页的路由 name 列表），
 *   视图组件名与路由 name 一致（见各视图 defineOptions）。
 */
const config = useConfig()
const navTabs = useNavTabs()

const state = reactive({
    componentKey: 0,
})

const refresh = () => {
    state.componentKey++
}

provide('layout-main-refresh', refresh)
</script>

<style scoped lang="scss">
.layout-main {
    width: 100%;
    padding: 0 var(--ba-main-space) calc(var(--ba-main-space) / 2) var(--ba-main-space);

    .layout-main-scrollbar {
        width: 100%;
        background: var(--ba-bg-color-overlay);
        border-radius: var(--el-border-radius-base);
        padding: 14px;
    }
}
</style>
