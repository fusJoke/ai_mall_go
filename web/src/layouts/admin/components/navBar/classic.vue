<template>
    <div class="nav-bar">
        <NavTabs />
        <NavMenus />
    </div>
</template>

<script setup lang="ts">
import NavMenus from '/@/layouts/admin/components/navMenus.vue'
import NavTabs from '/@/layouts/admin/components/navBar/tabs.vue'
import { useConfig } from '/@/stores/config'

/**
 * Classic 布局的顶栏（对齐 buildadmin navBar/classic.vue）：
 * 左侧标签页导航 + 右侧操作区，通栏背景样式（激活项整块高亮）。
 */
const configStore = useConfig()
</script>

<style lang="scss" scoped>
.nav-bar {
    display: flex;
    align-items: center;
    justify-content: space-between;
    height: 50px;
    width: 100%;
    background-color: v-bind('configStore.getColorVal("headerBarBackground")');
    gap: 12px;

    :deep(.nav-tabs) {
        display: flex;
        height: 100%;
        position: relative;
        align-items: center;

        .ba-nav-tab {
            display: flex;
            align-items: center;
            justify-content: center;
            padding: 0 20px;
            height: 100%;
            cursor: pointer;
            z-index: 1;
            user-select: none;
            color: v-bind('configStore.getColorVal("headerBarTabColor")');
            transition: all 0.2s;
            white-space: nowrap;

            .close-icon {
                padding: 2px;
                margin: 2px 0 0 4px;
                border-radius: 50%;

                &:hover {
                    background: var(--ba-color-primary-light);
                    color: var(--el-border-color) !important;
                }
            }

            &.active {
                color: v-bind('configStore.getColorVal("headerBarTabActiveColor")');
            }

            &:hover {
                background-color: v-bind('configStore.getColorVal("headerBarHoverBackground")');
            }
        }

        .nav-tabs-active-box {
            position: absolute;
            height: 50px;
            background-color: v-bind('configStore.getColorVal("headerBarTabActiveBackground")');
            transition: all 0.2s;
        }
    }
}
</style>
