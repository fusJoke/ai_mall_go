<template>
    <el-scrollbar ref="layoutMenuScrollbarRef" class="children-vertical-menus-scrollbar">
        <el-menu
            class="layouts-menu-vertical-children"
            :collapse-transition="false"
            :unique-opened="config.layout.menuUniqueOpened"
            :default-active="state.defaultActive"
            :collapse="config.layout.menuCollapse"
        >
            <MenuTree v-if="navTabs.state.childrenMenus.length > 0" :menus="navTabs.state.childrenMenus" />
        </el-menu>
    </el-scrollbar>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, useTemplateRef } from 'vue'
import { onBeforeRouteUpdate, useRoute, type RouteLocationNormalizedLoaded } from 'vue-router'
import MenuTree from '/@/layouts/admin/components/menus/menuTree.vue'
import { useConfig } from '/@/stores/config'
import { useNavTabs } from '/@/stores/navTabs'
import { getMenuKey } from '/@/utils/router'

/**
 * Double 布局的侧边子菜单（对齐 buildadmin menus/menuVerticalChildren.vue）：
 * 仅展示当前激活一级菜单的子菜单树（由 getTabsViewDataByRoute 'above' 模式派生）。
 * 无子菜单时回退为该一级菜单自身（单叶子场景）。
 */
const config = useConfig()
const navTabs = useNavTabs()
const route = useRoute()
const layoutMenuScrollbarRef = useTemplateRef('layoutMenuScrollbarRef')

const state = reactive({
    defaultActive: '',
})

const verticalMenusScrollbarHeight = computed(() => {
    const menuTopBarHeight = config.layout.menuShowTopBar ? 60 : 0
    const asideFooterToolbarHeight = config.layout.menuCollapse ? 100 : 50
    return 'calc(100% - ' + (menuTopBarHeight + asideFooterToolbarHeight) + 'px)'
})

/**
 * 激活当前路由的菜单，并派生次级菜单数据
 */
const currentRouteActive = (currentRoute: RouteLocationNormalizedLoaded) => {
    const tabView = navTabs.getTabsViewDataByRoute(currentRoute)
    if (tabView) {
        state.defaultActive = getMenuKey(tabView)
    }

    const routeChildren = navTabs.getTabsViewDataByRoute(currentRoute, 'above')
    if (routeChildren) {
        if (routeChildren.children && routeChildren.children.length > 0) {
            navTabs.setChildrenMenus(routeChildren.children)
        } else {
            navTabs.setChildrenMenus([routeChildren])
        }
    } else {
        navTabs.setChildrenMenus([])
    }
}

/**
 * 侧栏菜单滚动条滚动到激活菜单所在位置
 */
const verticalMenusScroll = () => {
    setTimeout(() => {
        const activeMenu: HTMLElement | null = document.querySelector('.el-menu.layouts-menu-vertical-children li.is-active')
        if (activeMenu) {
            layoutMenuScrollbarRef.value?.setScrollTop(activeMenu.offsetTop)
        }
    }, 500)
}

onMounted(() => {
    currentRouteActive(route)
    verticalMenusScroll()
})

onBeforeRouteUpdate((to) => {
    currentRouteActive(to)
})
</script>

<style scoped lang="scss">
.children-vertical-menus-scrollbar {
    height: v-bind(verticalMenusScrollbarHeight);
    background-color: v-bind('config.getColorVal("menuBackground")');
}
.layouts-menu-vertical-children {
    border: 0;
    --el-menu-bg-color: v-bind('config.getColorVal("menuBackground")');
    --el-menu-text-color: v-bind('config.getColorVal("menuColor")');
    --el-menu-active-color: v-bind('config.getColorVal("menuActiveColor")');
    --el-menu-hover-bg-color: v-bind('config.getColorVal("menuHoverBackground")');
    --el-menu-active-bg-color: v-bind('config.getColorVal("menuActiveBackground")');
}
</style>
