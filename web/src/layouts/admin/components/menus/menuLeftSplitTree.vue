<template>
    <template v-for="menu in props.menus">
        <template v-if="menu.children && menu.children.length > 0">
            <el-sub-menu :index="getMenuKey(menu)" :key="getMenuKey(menu)">
                <template #title>
                    <Icon :color="config.getColorVal('menuColor')" :name="menu.meta?.icon ? menu.meta?.icon : config.layout.menuDefaultIcon" />
                    <span>{{ menu.meta?.title ? menu.meta?.title : $t('layouts.untitled') }}</span>
                </template>
                <MenuLeftSplitTree :extends="{ ...props.extends, level: (props.extends?.level ?? 1) + 1 }" :menus="menu.children" />
            </el-sub-menu>
        </template>
        <template v-else>
            <el-menu-item :index="getMenuKey(menu)" :key="getMenuKey(menu)" @click="onClickMenu(menu)">
                <Icon :color="config.getColorVal('menuColor')" :name="menu.meta?.icon ? menu.meta?.icon : config.layout.menuDefaultIcon" />
                <span>{{ menu.meta?.title ? menu.meta?.title : $t('layouts.untitled') }}</span>
            </el-menu-item>
        </template>
    </template>
</template>

<script setup lang="ts">
import type { RouteRecordRaw } from 'vue-router'
import { useConfig } from '/@/stores/config'
import { getMenuKey, routePush } from '/@/utils/router'

/**
 * LeftSplit 布局次级菜单树（对齐 buildadmin menus/menuLeftSplitTree.vue）：
 * 圆角条目样式，叶子点击经 routePush 导航。
 */
const config = useConfig()

interface Props {
    menus: RouteRecordRaw[]
    extends?: {
        level?: number
        [key: string]: any
    }
}
const props = withDefaults(defineProps<Props>(), {
    menus: () => [],
    extends: () => ({ level: 1 }),
})

const onClickMenu = (menu: RouteRecordRaw) => {
    routePush(menu.path)
}
</script>

<style scoped lang="scss">
.el-sub-menu,
.el-menu-item {
    border-radius: var(--el-border-radius-base);
    :deep(.el-sub-menu__title) {
        border-radius: var(--el-border-radius-base);
    }
    .icon {
        vertical-align: middle;
        margin-right: 5px;
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
</style>
