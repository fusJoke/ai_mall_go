<template>
    <template v-for="menu in props.menus">
        <template v-if="menu.children && menu.children.length > 0">
            <el-sub-menu :index="getMenuKey(menu)" :key="getMenuKey(menu)" @click="onClickSubMenu(menu)">
                <template #title>
                    <Icon color="var(--el-menu-text-color)" :name="menu.meta?.icon ? menu.meta?.icon : config.layout.menuDefaultIcon" />
                    <span>{{ menu.meta?.title ? menu.meta?.title : $t('layouts.untitled') }}</span>
                </template>
                <MenuTree :extends="{ ...props.extends, level: (props.extends?.level ?? 1) + 1 }" :menus="menu.children" />
            </el-sub-menu>
        </template>
        <template v-else>
            <el-menu-item :index="getMenuKey(menu)" :key="getMenuKey(menu)" @click="onClickMenu(menu)">
                <Icon color="var(--el-menu-text-color)" :name="menu.meta?.icon ? menu.meta?.icon : config.layout.menuDefaultIcon" />
                <span>{{ menu.meta?.title ? menu.meta?.title : $t('layouts.untitled') }}</span>
            </el-menu-item>
        </template>
    </template>
</template>

<script setup lang="ts">
import { ElNotification } from 'element-plus'
import { useI18n } from 'vue-i18n'
import type { RouteRecordRaw } from 'vue-router'
import { useConfig } from '/@/stores/config'
import { getFirstRoute, getMenuKey, routePush } from '/@/utils/router'

/**
 * 菜单树递归渲染（openspec add-admin-layout-shell），对齐 buildadmin menuTree.vue。
 * 叶子点击经 routePush 导航（解析后的完整路径）。
 *
 * extends.position == 'horizontal' 时用于顶栏横向菜单（Streamline/Double 布局）：
 * 一级 sub-menu 被点击时跳转其第一个可导航子菜单（el-menu 会把 sub-menu
 * 的 click 冒泡到内部 menu-item，以 extends.level 区分层级避免误触发）。
 */
const { t } = useI18n()
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

const onClickSubMenu = (menu: RouteRecordRaw) => {
    if (props.extends?.position == 'horizontal' && (props.extends?.level ?? 1) <= 1 && menu.children?.length) {
        const firstRoute = getFirstRoute(menu.children)
        if (firstRoute) {
            routePush(firstRoute.path)
        } else {
            ElNotification({
                type: 'error',
                message: t('layouts.noChildMenu'),
            })
        }
    }
}
</script>

<style scoped lang="scss">
.el-sub-menu,
.el-menu-item {
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
}
</style>
