<template>
    <el-header v-if="!navTabs.state.tabFullScreen" class="layout-header">
        <component :is="navBarComponent" />
    </el-header>
</template>

<script setup lang="ts">
import { computed, type Component } from 'vue'
import ClassicNavBar from '/@/layouts/admin/components/navBar/classic.vue'
import DefaultNavBar from '/@/layouts/admin/components/navBar/default.vue'
import DoubleNavBar from '/@/layouts/admin/components/navBar/double.vue'
import LeftSplitNavBar from '/@/layouts/admin/components/navBar/leftSplit.vue'
import StreamlineNavBar from '/@/layouts/admin/components/menus/menuHorizontal.vue'
import { useConfig } from '/@/stores/config'
import { useNavTabs } from '/@/stores/navTabs'

/**
 * 后台顶栏（openspec add-admin-layout-shell），对齐 buildadmin header.vue：
 * 按当前布局模式渲染对应的 navBar 变体，标签页全屏态下整体隐藏。
 * - Default / LeftSplit：悬浮标签 + 操作区
 * - Classic：通栏背景标签
 * - Double：横向菜单 + 操作区
 * - Streamline：Logo + 横向菜单 + 操作区（无侧栏）
 */
const config = useConfig()
const navTabs = useNavTabs()

const navBarMap: Record<string, Component> = {
    Default: DefaultNavBar,
    Classic: ClassicNavBar,
    LeftSplit: LeftSplitNavBar,
    Double: DoubleNavBar,
    Streamline: StreamlineNavBar,
}

const navBarComponent = computed(() => navBarMap[config.layout.layoutMode] ?? DefaultNavBar)
</script>

<style scoped lang="scss">
.layout-header {
    height: auto;
    padding: 0;
}
</style>
