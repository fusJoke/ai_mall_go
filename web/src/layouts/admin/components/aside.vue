<template>
    <el-aside v-if="!navTabs.state.tabFullScreen" :class="['layout-aside-' + config.layout.layoutMode]">
        <Logo v-if="config.layout.menuShowTopBar && config.layout.layoutMode != 'LeftSplit'" />

        <MenuVerticalChildren v-if="config.layout.layoutMode == 'Double'" />
        <MenuLeftSplit v-else-if="config.layout.layoutMode == 'LeftSplit'" />
        <MenuVertical v-else />

        <AsideFooterToolbar v-if="['Default', 'Classic', 'Double'].includes(config.layout.layoutMode)" />
    </el-aside>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import AsideFooterToolbar from '/@/layouts/admin/components/asideToolbar/footer.vue'
import Logo from '/@/layouts/admin/components/logo.vue'
import MenuLeftSplit from '/@/layouts/admin/components/menus/menuLeftSplit.vue'
import MenuVertical from '/@/layouts/admin/components/menus/menuVertical.vue'
import MenuVerticalChildren from '/@/layouts/admin/components/menus/menuVerticalChildren.vue'
import { useConfig } from '/@/stores/config'
import { useNavTabs } from '/@/stores/navTabs'

/**
 * 后台侧边菜单栏（openspec add-admin-layout-shell），对齐 buildadmin aside.vue 的多模式形态：
 * - Default / Classic / Streamline(无侧栏)：普通竖向菜单
 * - Double：仅展示当前激活一级菜单的子菜单（顶栏为横向菜单）
 * - LeftSplit：主（一级窄栏）+ 次（子菜单树）双栏
 * 菜单宽度由 config.menuWidth() 按布局模式统一计算。
 */
const config = useConfig()
const navTabs = useNavTabs()
const menuWidth = computed(() => config.menuWidth())
</script>

<style scoped lang="scss">
.layout-aside-Default,
.layout-aside-LeftSplit {
    width: v-bind(menuWidth);
    background: var(--ba-bg-color-overlay);
    margin: 16px 0 16px 16px;
    height: calc(100% - 32px);
    box-shadow: var(--el-box-shadow-light);
    border-radius: var(--el-border-radius-base);
    overflow: hidden;
    transition: width 0.3s ease;
}
.layout-aside-Classic,
.layout-aside-Double {
    width: v-bind(menuWidth);
    background: var(--ba-bg-color-overlay);
    margin: 0;
    height: 100%;
    overflow: hidden;
    transition: width 0.3s ease;
}
</style>
