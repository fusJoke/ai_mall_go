<template>
    <el-scrollbar class="vertical-menus-scrollbar">
        <el-menu
            class="layouts-menu-vertical"
            :collapse-transition="false"
            :unique-opened="config.layout.menuUniqueOpened"
            :default-active="state.defaultActive"
            :collapse="config.layout.menuCollapse"
        >
            <MenuTree :menus="navTabs.state.tabsViewRoutes" />
        </el-menu>
    </el-scrollbar>
</template>

<script setup lang="ts">
import { reactive, watch } from 'vue'
import { useRoute, type RouteRecordRaw } from 'vue-router'
import MenuTree from '/@/layouts/admin/components/menus/menuTree.vue'
import { useConfig } from '/@/stores/config'
import { useNavTabs } from '/@/stores/navTabs'
import { getMenuKey } from '/@/utils/router'

/**
 * 竖向菜单（openspec add-admin-layout-shell），对齐 buildadmin menuVertical.vue。
 * 激活项以「解析后的完整路径」匹配当前路由。
 */
const config = useConfig()
const navTabs = useNavTabs()
const route = useRoute()

const state = reactive({
    defaultActive: '',
})

/**
 * 在菜单树中按 path 找到当前路由对应节点，取其 el-menu index
 */
function currentRouteActive() {
    const find = (menus: RouteRecordRaw[]): RouteRecordRaw | null => {
        for (const menu of menus) {
            if (menu.path === route.path) return menu
            if (menu.children?.length) {
                const hit = find(menu.children)
                if (hit) return hit
            }
        }
        return null
    }
    const hit = find(navTabs.state.tabsViewRoutes)
    state.defaultActive = hit ? getMenuKey(hit) : ''
}

watch(() => route.path, currentRouteActive, { immediate: true })
</script>

<style scoped lang="scss">
.vertical-menus-scrollbar {
    height: calc(100% - 100px);
    background-color: v-bind('config.getColorVal("menuBackground")');
}
.layouts-menu-vertical {
    border: 0;
    --el-menu-bg-color: v-bind('config.getColorVal("menuBackground")');
    --el-menu-text-color: v-bind('config.getColorVal("menuColor")');
    --el-menu-active-color: v-bind('config.getColorVal("menuActiveColor")');
    --el-menu-hover-bg-color: v-bind('config.getColorVal("menuHoverBackground")');
    --el-menu-active-bg-color: v-bind('config.getColorVal("menuActiveBackground")');
}
</style>
