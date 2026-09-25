import { reactive } from 'vue'
import type { RouteRecordRaw } from 'vue-router'
import { STORE_TAB_VIEW_CONFIG } from '/@/stores/constant/cacheKey'

/**
 * 后台标签页导航状态。
 * - tabs：当前打开的标签页列表
 * - activeTab：当前激活的标签页 path
 * - childrenMenus：左分布局下的次级菜单数据
 *
 * 以组合式函数形式暴露（不挂到 pinia），保持 `useNavTabs().state.x` 的访问风格。
 */
export const useNavTabs = () => {
    const state = reactive({
        tabs: [] as RouteRecordRaw[],
        activeTab: '' as string,
        childrenMenus: [] as RouteRecordRaw[],
    })

    function setTabs(data: RouteRecordRaw[]) {
        state.tabs = data
    }
    function setActiveTab(path: string) {
        state.activeTab = path
    }
    function setChildrenMenus(data: RouteRecordRaw[]) {
        state.childrenMenus = data
    }

    /**
     * 从 localStorage 恢复持久化数据。
     * 项目实际接入持久化中间件时可移除此处的硬编码。
     */
    function hydrate() {
        try {
            const raw = localStorage.getItem(STORE_TAB_VIEW_CONFIG)
            if (!raw) return
            const parsed = JSON.parse(raw)
            if (Array.isArray(parsed.tabs)) state.tabs = parsed.tabs
            if (typeof parsed.activeTab === 'string') state.activeTab = parsed.activeTab
            if (Array.isArray(parsed.childrenMenus)) state.childrenMenus = parsed.childrenMenus
        } catch (error) {
            console.warn('[navTabs] hydrate failed:', error)
        }
    }

    return {
        state,
        setTabs,
        setActiveTab,
        setChildrenMenus,
        hydrate,
    }
}