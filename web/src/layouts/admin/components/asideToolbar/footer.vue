<template>
    <div>
        <div class="aside-footer-toolbar-wrap">
            <div class="aside-footer-toolbar">
                <Icon
                    @click="onMenuCollapse"
                    :name="config.layout.menuCollapse ? 'lucide:PanelLeftOpen' : 'lucide:PanelLeftClose'"
                    :color="config.getColorVal('menuToolBarColor')"
                    size="16"
                    class="footer-toolbar-item"
                    role="button"
                    :title="config.layout.menuCollapse ? $t('layouts.expandMenu') : $t('layouts.collapseMenu')"
                />
                <Icon
                    @click="onMenuSearch"
                    name="lucide:Search"
                    :color="config.getColorVal('menuToolBarColor')"
                    size="16"
                    class="footer-toolbar-item"
                    role="button"
                    :title="$t('layouts.menuSearch')"
                />
            </div>
        </div>

        <MenuSearchDialog v-model="menuSearchDialogVisible" />
    </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import MenuSearchDialog from '/@/layouts/admin/components/asideToolbar/menuSearch/dialog.vue'
import { useConfig } from '/@/stores/config'

/**
 * 侧边菜单底部工具栏（openspec add-admin-layout-shell）：
 * 折叠切换 + 菜单搜索入口，对齐 buildadmin asideToolbar/footer.vue 的精简形态。
 */
const config = useConfig()
const menuSearchDialogVisible = ref(false)

const onMenuSearch = function () {
    menuSearchDialogVisible.value = true
}

const onMenuCollapse = function () {
    config.setLayout('menuCollapse', !config.layout.menuCollapse)
}
</script>

<style scoped lang="scss">
.aside-footer-toolbar-wrap {
    position: relative;
    height: 50px;
    background-color: v-bind('config.getColorVal("menuBackground")');
    .aside-footer-toolbar {
        position: absolute;
        display: flex;
        align-items: center;
        justify-content: space-between;
        height: 50px;
        width: 100%;
        padding: 0 20px;
        box-sizing: border-box;
        transition: all 0.2s ease;
        .footer-toolbar-item {
            padding: 10px;
            border-radius: 50%;
            cursor: pointer;
            box-sizing: content-box;

            &:hover {
                color: v-bind('config.getColorVal("menuToolBarHoverColor")') !important;
                background-color: v-bind('config.getColorVal("menuToolBarHoverBackground")');
            }
        }
    }
}
</style>
