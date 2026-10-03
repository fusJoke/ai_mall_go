<template>
    <el-container class="layout-container">
        <el-container class="content-wrapper">
            <Header />
            <Main />
        </el-container>
        <CloseFullScreen v-if="navTabs.state.tabFullScreen" />
    </el-container>
</template>

<script setup lang="ts">
import CloseFullScreen from '/@/layouts/admin/components/closeFullScreen.vue'
import Header from '/@/layouts/admin/components/header.vue'
import Main from '/@/layouts/admin/router-view/main.vue'
import { useNavTabs } from '/@/stores/navTabs'

/**
 * Streamline（单栏）布局容器（对齐 buildadmin container/streamline.vue）：
 * 无侧边栏，顶栏承载 Logo + 横向菜单 + 右侧操作区。
 * 标签页的路由驱动（增删/激活）统一在 layouts/admin/index.vue 处理。
 */
const navTabs = useNavTabs()
</script>

<style scoped>
.layout-container {
    height: 100%;
    width: 100%;
}
.content-wrapper {
    flex-direction: column;
    width: 100%;
    height: 100%;
}
</style>
