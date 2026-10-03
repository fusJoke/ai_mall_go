<template>
    <el-container class="layout-container">
        <Aside />
        <el-container class="content-wrapper">
            <Header />
            <Main />
        </el-container>
        <CloseFullScreen v-if="navTabs.state.tabFullScreen" />
    </el-container>
</template>

<script setup lang="ts">
import Aside from '/@/layouts/admin/components/aside.vue'
import CloseFullScreen from '/@/layouts/admin/components/closeFullScreen.vue'
import Header from '/@/layouts/admin/components/header.vue'
import Main from '/@/layouts/admin/router-view/main.vue'
import { useNavTabs } from '/@/stores/navTabs'

/**
 * Classic（经典）布局容器（对齐 buildadmin container/classic.vue）：
 * 左侧通栏菜单 + 右侧（顶栏 + 主内容区），顶栏为通栏背景样式。
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
