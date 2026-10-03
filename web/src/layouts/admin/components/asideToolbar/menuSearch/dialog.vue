<template>
    <el-dialog v-model="visible" class="layout-menu-search-dialog" :title="$t('layouts.menuSearch')" width="420px" append-to-body>
        <el-select
            v-model="selected"
            filterable
            remote
            clearable
            default-first-option
            :remote-method="onSearch"
            :placeholder="$t('layouts.menuSearchPlaceholder')"
            size="large"
            style="width: 100%"
            @change="onSelect"
        >
            <el-option v-for="item in searchResult" :key="item.path" :label="String(item.meta?.title ?? '')" :value="item.path">
                <div class="menu-search-option">
                    <Icon :name="String(item.meta?.icon || '') || config.layout.menuDefaultIcon" size="14" />
                    <span>{{ item.meta?.title }}</span>
                    <span class="menu-search-path">{{ item.path }}</span>
                </div>
            </el-option>
        </el-select>
    </el-dialog>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import type { RouteRecordRaw } from 'vue-router'
import { useConfig } from '/@/stores/config'
import { useNavTabs } from '/@/stores/navTabs'
import { routePush } from '/@/utils/router'

/**
 * 菜单搜索弹窗（openspec add-admin-layout-shell）：
 * 按标题/路径过滤菜单树叶子，选中即跳转。数据来自 navTabs.tabsViewRoutes。
 */
const config = useConfig()
const navTabs = useNavTabs()

const visible = defineModel<boolean>({ default: false })
const selected = ref('')
const searchResult = ref<RouteRecordRaw[]>([])

/**
 * 收集菜单树的所有叶子
 */
function collectLeaves(menus: RouteRecordRaw[], out: RouteRecordRaw[]) {
    for (const menu of menus) {
        if (menu.children?.length) {
            collectLeaves(menu.children, out)
        } else if (menu.meta?.menuType !== 'dir') {
            out.push(menu)
        }
    }
}

const onSearch = (keyword: string) => {
    const leaves: RouteRecordRaw[] = []
    collectLeaves(navTabs.state.tabsViewRoutes, leaves)
    const kw = keyword.trim().toLowerCase()
    searchResult.value = kw
        ? leaves.filter(
              (item) =>
                  String(item.meta?.title || '')
                      .toLowerCase()
                      .includes(kw) ||
                  String(item.path || '')
                      .toLowerCase()
                      .includes(kw)
          )
        : leaves
}

const onSelect = (path: string) => {
    if (!path) return
    visible.value = false
    selected.value = ''
    routePush(path)
}

// 打开时预加载全部叶子
watch(visible, (val) => {
    if (val) onSearch('')
})
</script>

<style scoped lang="scss">
.menu-search-option {
    display: flex;
    align-items: center;
    gap: 8px;
    .menu-search-path {
        margin-left: auto;
        color: var(--el-text-color-secondary);
        font-size: 12px;
    }
}
</style>
