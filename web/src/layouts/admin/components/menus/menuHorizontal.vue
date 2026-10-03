<template>
    <div class="layouts-menu-horizontal">
        <div v-if="config.layout.menuShowTopBar" class="menu-horizontal-logo">
            <Logo />
        </div>
        <el-scrollbar ref="layoutMenuScrollbarRef" class="horizontal-menus-scrollbar">
            <el-menu
                class="menu-horizontal"
                mode="horizontal"
                :default-active="state.defaultActive"
                :popper-style="{
                    '--el-menu-bg-color': config.getColorVal('headerBarBackground'),
                    '--el-menu-text-color': config.getColorVal('headerBarTabColor'),
                    '--el-menu-active-color': config.getColorVal('headerBarTabActiveColor'),
                    '--el-menu-hover-bg-color': config.getColorVal('headerBarHoverBackground'),
                    '--el-menu-active-bg-color': config.getColorVal('headerBarTabActiveBackground'),
                }"
            >
                <MenuTree :extends="{ position: 'horizontal', level: 1 }" :menus="navTabs.state.tabsViewRoutes" />
            </el-menu>
        </el-scrollbar>
        <NavMenus />
    </div>
</template>

<script setup lang="ts">
import { onMounted, reactive, useTemplateRef } from 'vue'
import { onBeforeRouteUpdate, useRoute, type RouteLocationNormalizedLoaded } from 'vue-router'
import Logo from '/@/layouts/admin/components/logo.vue'
import MenuTree from '/@/layouts/admin/components/menus/menuTree.vue'
import NavMenus from '/@/layouts/admin/components/navMenus.vue'
import { useConfig } from '/@/stores/config'
import { useNavTabs } from '/@/stores/navTabs'
import { getMenuKey } from '/@/utils/router'

/**
 * 横向菜单（对齐 buildadmin menus/menuHorizontal.vue）：
 * Streamline（单栏）布局的顶栏 —— Logo + 横向菜单 + 右侧操作区。
 * 激活项按当前路由在菜单树中匹配（fullPath 优先，path 回退）。
 */
const config = useConfig()
const navTabs = useNavTabs()
const route = useRoute()
const layoutMenuScrollbarRef = useTemplateRef('layoutMenuScrollbarRef')

const state = reactive({
    defaultActive: '',
})

/**
 * 激活当前路由对应的菜单
 */
const currentRouteActive = (currentRoute: RouteLocationNormalizedLoaded) => {
    const tabView = navTabs.getTabsViewDataByRoute(currentRoute)
    if (tabView) {
        state.defaultActive = getMenuKey(tabView)
    }
}

/**
 * 滚动条横向滚动到激活菜单所在位置
 */
const horizontalMenusScroll = () => {
    setTimeout(() => {
        const activeMenu: HTMLElement | null = document.querySelector('.el-menu.menu-horizontal li.is-active')
        if (activeMenu) {
            layoutMenuScrollbarRef.value?.setScrollLeft(activeMenu.offsetLeft)
        }
    }, 500)
}

onMounted(() => {
    currentRouteActive(route)
    horizontalMenusScroll()
})

onBeforeRouteUpdate((to) => {
    currentRouteActive(to)
})
</script>

<style scoped lang="scss">
.layouts-menu-horizontal {
    display: flex;
    align-items: center;
    width: 100%;
    height: var(--el-header-height);
    background-color: v-bind('config.getColorVal("headerBarBackground")');
    border-bottom: 1px solid var(--el-color-info-light-8);
}
.menu-horizontal-logo {
    width: 180px;
    background-color: v-bind('config.getColorVal("headerBarBackground")');
}
.horizontal-menus-scrollbar {
    flex: 1;
    height: var(--el-header-height);
}
.menu-horizontal {
    border: none;
    --el-menu-bg-color: v-bind('config.getColorVal("headerBarBackground")');
    --el-menu-text-color: v-bind('config.getColorVal("headerBarTabColor")');
    --el-menu-active-color: v-bind('config.getColorVal("headerBarTabActiveColor")');
    --el-menu-hover-bg-color: v-bind('config.getColorVal("headerBarHoverBackground")');
    --el-menu-active-bg-color: v-bind('config.getColorVal("headerBarTabActiveBackground")');
}

:deep(.el-sub-menu),
:deep(.el-menu-item) {
    .icon {
        vertical-align: middle;
        margin-right: 5px;
        width: 24px;
        text-align: center;
        flex-shrink: 0;
    }
    .el-sub-menu__title {
        background-color: var(--el-menu-bg-color);
        &:hover {
            background-color: var(--el-menu-hover-bg-color);
        }
    }
    &:hover {
        color: var(--el-menu-text-color) !important;
        background-color: var(--el-menu-hover-bg-color);
    }
    &.is-active {
        background-color: var(--el-menu-active-bg-color);
        .el-sub-menu__title {
            background-color: var(--el-menu-active-bg-color);
            .icon {
                color: var(--el-menu-active-color) !important;
            }
        }
        &:hover {
            color: var(--el-menu-active-color) !important;
        }
    }
    &.is-active > .icon {
        color: var(--el-menu-active-color) !important;
    }
}
</style>
