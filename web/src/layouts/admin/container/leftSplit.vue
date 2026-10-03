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
 * LeftSplit（左分双栏）布局容器（对齐 buildadmin container/leftSplit.vue）：
 * 侧边栏分主（一级菜单窄栏）+ 次（子菜单树）两栏，顶栏为悬浮标签样式。
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
