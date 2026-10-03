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
 * Default / LeftSplit 布局的顶栏（对齐 buildadmin navBar/default.vue）：
 * 左侧标签页导航 + 右侧操作区，悬浮（带外边距）样式。
 */
const configStore = useConfig()
</script>

<style lang="scss" scoped>
.nav-bar {
    display: flex;
    align-items: center;
    justify-content: space-between;
    height: 50px;
    margin: 20px var(--ba-main-space) 0 var(--ba-main-space);
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
            padding: 0 16px;
            height: 40px;
            cursor: pointer;
            z-index: 1;
            user-select: none;
            opacity: 0.7;
            color: v-bind('configStore.getColorVal("headerBarTabColor")');
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
                opacity: 1;
            }

            &:hover {
                opacity: 1;
            }
        }

        .nav-tabs-active-box {
            position: absolute;
            height: 40px;
            border-radius: var(--el-border-radius-base);
            background-color: v-bind('configStore.getColorVal("headerBarTabActiveBackgroundFloating")');
            box-shadow: var(--el-box-shadow-light);
            transition: all 0.2s;
        }
    }
}
</style>
