<template>
    <div class="left-split-menus">
        <div class="left-split-primary-menus-scrollbar-wrap">
            <el-scrollbar ref="layoutMenuScrollbarRef" class="left-split-primary-menus-scrollbar">
                <el-menu
                    class="layouts-menu-vertical primary-menus"
                    :collapse-transition="false"
                    :unique-opened="config.layout.menuUniqueOpened"
                    :default-active="state.primaryDefaultActive"
                    :collapse="true"
                >
                    <el-menu-item
                        v-for="menu in navTabs.state.tabsViewRoutes"
                        :key="getMenuKey(menu)"
                        :index="getMenuKey(menu)"
                        @click="onClickPrimaryMenu(menu)"
                    >
                        <Icon :color="config.getColorVal('menuColor')" :name="menu.meta?.icon ? menu.meta?.icon : config.layout.menuDefaultIcon" />
                        <span>{{ menu.meta?.title ? menu.meta?.title : $t('layouts.untitled') }}</span>
                    </el-menu-item>
                </el-menu>
            </el-scrollbar>
        </div>

        <div v-if="navTabs.state.childrenMenus.length" class="left-split-secondary-menus-scrollbar-wrap">
            <el-scrollbar ref="layoutSecondaryMenuScrollbarRef" class="left-split-secondary-menus-scrollbar">
                <el-menu
                    class="layouts-menu-vertical secondary-menus"
                    :collapse-transition="false"
                    :unique-opened="config.layout.menuUniqueOpened"
                    :default-active="state.secondaryDefaultActive"
                    :collapse="config.layout.menuCollapse"
                >
                    <MenuLeftSplitTree :menus="navTabs.state.childrenMenus" />
                </el-menu>
            </el-scrollbar>
            <AsideFooterToolbar />
        </div>
    </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, useTemplateRef } from 'vue'
import { onBeforeRouteUpdate, useRoute, type RouteLocationNormalizedLoaded, type RouteRecordRaw } from 'vue-router'
import AsideFooterToolbar from '/@/layouts/admin/components/asideToolbar/footer.vue'
import MenuLeftSplitTree from '/@/layouts/admin/components/menus/menuLeftSplitTree.vue'
import { useConfig } from '/@/stores/config'
import { useNavTabs } from '/@/stores/navTabs'
import { getMenuKey, routePush } from '/@/utils/router'

/**
 * LeftSplit（左分双栏）布局的主次菜单（对齐 buildadmin menus/menuLeftSplit.vue）：
 * 主菜单（一级，窄栏、常折叠）+ 次级菜单（当前一级菜单的子树）。
 */
const route = useRoute()
const config = useConfig()
const navTabs = useNavTabs()
const menuWidth = computed(() => config.menuWidth())
const layoutMenuScrollbarRef = useTemplateRef('layoutMenuScrollbarRef')
const layoutSecondaryMenuScrollbarRef = useTemplateRef('layoutSecondaryMenuScrollbarRef')

const state = reactive({
    primaryDefaultActive: '',
    secondaryDefaultActive: '',
})

const verticalSecondaryMenusScrollbarHeight = computed(() => {
    const asideFooterToolbarHeight = config.layout.menuCollapse ? 100 : 50
    return 'calc(100% - ' + asideFooterToolbarHeight + 'px)'
})

/**
 * 依据当前路由派生主菜单激活项与次级菜单数据
 */
const findRouteChildren = (currentRoute: RouteLocationNormalizedLoaded) => {
    const routeChildren = navTabs.getTabsViewDataByRoute(currentRoute, 'above')
    if (routeChildren) {
        state.primaryDefaultActive = getMenuKey(routeChildren)
    }
    if (routeChildren && routeChildren.children?.length) {
        navTabs.setChildrenMenus(routeChildren.children)
    } else {
        navTabs.setChildrenMenus([])
    }
}

/**
 * 一级菜单点击：目录（或有子菜单的菜单）切换次级菜单，叶子直接导航
 */
const onClickPrimaryMenu = (menu: RouteRecordRaw) => {
    if (menu.children?.length) {
        navTabs.setChildrenMenus(menu.children)
        state.primaryDefaultActive = getMenuKey(menu)
        return
    }
    routePush(menu.path)
}

/**
 * 激活当前路由对应的菜单
 */
const currentRouteActive = (currentRoute: RouteLocationNormalizedLoaded) => {
    const tabView = navTabs.getTabsViewDataByRoute(currentRoute)
    if (tabView) {
        state.secondaryDefaultActive = getMenuKey(tabView)
    }
    findRouteChildren(currentRoute)
}

/**
 * 滚动条滚动到激活菜单所在位置
 */
const verticalMenusScroll = () => {
    setTimeout(() => {
        const activeMenu: HTMLElement | null = document.querySelector('.primary-menus.layouts-menu-vertical li.is-active')
        if (activeMenu) {
            layoutMenuScrollbarRef.value?.setScrollTop(activeMenu.offsetTop)
        }

        const secondaryActiveMenu: HTMLElement | null = document.querySelector('.secondary-menus.layouts-menu-vertical li.is-active')
        if (secondaryActiveMenu) {
            layoutSecondaryMenuScrollbarRef.value?.setScrollTop(secondaryActiveMenu.offsetTop)
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
.left-split-menus {
    display: flex;
    height: 100%;
}
.left-split-primary-menus-scrollbar-wrap {
    width: 80px;
    background-color: v-bind('config.getColorVal("menuBackgroundPrimary")');
    .left-split-primary-menus-scrollbar {
        width: 100%;
        height: 100%;
    }
}
.left-split-secondary-menus-scrollbar-wrap {
    width: calc(v-bind(menuWidth) - 80px);
    background-color: v-bind('config.getColorVal("menuBackground")');
    .left-split-secondary-menus-scrollbar {
        width: 100%;
        padding: 8px;
        height: v-bind(verticalSecondaryMenusScrollbarHeight);
    }
}
.layouts-menu-vertical {
    border: 0;
}
.primary-menus {
    margin: 0 8px;
    --el-menu-bg-color: v-bind('config.getColorVal("menuBackgroundPrimary")');
    --el-menu-text-color: v-bind('config.getColorVal("menuColor")');
    --el-menu-active-color: v-bind('config.getColorVal("menuActiveColor")');
    --el-menu-hover-bg-color: v-bind('config.getColorVal("menuHoverBackgroundLeftSplit")');
    --el-menu-active-bg-color: v-bind('config.getColorVal("menuActiveBackgroundPrimary")');
    :deep(.el-menu-item) {
        margin: 8px 0;
        border-radius: var(--el-border-radius-base);
        .icon {
            vertical-align: middle;
            width: 24px;
            text-align: center;
            flex-shrink: 0;
        }
        &.is-active {
            background-color: var(--el-menu-active-bg-color);
        }
        &.is-active > .icon {
            color: var(--el-menu-active-color) !important;
        }
    }
}
.secondary-menus {
    --el-menu-bg-color: v-bind('config.getColorVal("menuBackground")');
    --el-menu-text-color: v-bind('config.getColorVal("menuColor")');
    --el-menu-active-color: v-bind('config.getColorVal("menuActiveColor")');
    --el-menu-hover-bg-color: v-bind('config.getColorVal("menuHoverBackgroundLeftSplit")');
    --el-menu-active-bg-color: v-bind('config.getColorVal("menuActiveBackground")');
}
</style>
