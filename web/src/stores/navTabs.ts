import { reactive } from 'vue'
import type { RouteLocationNormalized, RouteRecordRaw } from 'vue-router'
import { adminBaseRoutePath } from '/@/router/static/adminBase'
import { STORE_TAB_VIEW_CONFIG } from '/@/stores/constant/cacheKey'
import { useMenu } from '/@/stores/menu'

/**
 * 后台标签页导航状态（openspec add-admin-layout-shell）。
 *
 * 状态为模块级单例：aside/header/tabs/main 多个布局组件必须共享同一份，
 * 原「每次 useNavTabs() 新建 state」的实现会让组件间各持一份，故重构。
 * 保留 useNavTabs() 的调用形态；字段命名对齐 buildadmin 便于后续对照移植。
 *
 * - tabsViewRoutes：菜单路由树（来自 useMenu.rawData，含目录节点），驱动侧边菜单
 * - tabsView：已打开的标签页（路由驱动增删），驱动顶栏标签
 * - activeRoute / activeIndex：当前激活路由与标签下标
 * - tabFullScreen：当前标签全屏态（右键菜单触发）
 * - keepAliveTabs：参与 keep-alive 的视图组件名（= 路由 name）列表
 */
const state = reactive({
    tabs: [] as RouteRecordRaw[],
    activeTab: '' as string,
    childrenMenus: [] as RouteRecordRaw[],
    tabsViewRoutes: [] as RouteRecordRaw[],
    tabsView: [] as RouteLocationNormalized[],
    activeRoute: null as RouteLocationNormalized | null,
    activeIndex: 0,
    tabFullScreen: false,
    keepAliveTabs: [] as string[],
})

/**
 * 给路由树打标签页标记（meta.addtab）：仅菜单叶子（非目录）参与标签页。
 * 相对 path（不以 / 开头）统一补全为后台绝对路径（/admin/xxx），
 * 与 vue-router 注册后的最终路径一致 —— 侧边菜单/横向菜单按 path 匹配
 * 当前路由、叶子点击 routePush(menu.path) 都依赖绝对路径（对齐 buildadmin
 * handleAdminRoute 的 path 前缀化处理）。
 */
function withAddTab(routes: RouteRecordRaw[]): RouteRecordRaw[] {
    return routes.map((item) => {
        const path = item.path.startsWith('/') ? item.path : `${adminBaseRoutePath}/${item.path}`
        const clone = { ...item, path, meta: { ...item.meta, addtab: item.meta?.menuType !== 'dir' } } as RouteRecordRaw
        if (item.children?.length) {
            clone.children = withAddTab(item.children)
        }
        return clone
    })
}

/**
 * 从 useMenu.rawData 派生菜单路由树（含目录节点）
 */
function setTabsViewRoutes(data: RouteRecordRaw[]) {
    state.tabsViewRoutes = withAddTab(data)
}

/**
 * 添加标签页（路由变化时调用；按 fullPath 去重），并把路由 name 记入
 * keep-alive 名单（视图组件名与路由 name 一致，见各视图 defineOptions）。
 */
function addTab(route: RouteLocationNormalized) {
    if (!route.meta?.addtab) return
    for (const item of state.tabsView) {
        if (item.fullPath === route.fullPath) {
            return
        }
    }
    state.tabsView.push({ ...route, meta: { ...route.meta } })
    if (typeof route.name === 'string' && !state.keepAliveTabs.includes(route.name)) {
        state.keepAliveTabs.push(route.name)
    }
}

/**
 * 在菜单树中查找当前路由对应的节点（多容器布局的菜单激活态依据）。
 *
 * - mode='children'：返回与 route.path / route.fullPath 匹配的最深层菜单节点；
 * - mode='above'：返回包含该路由的顶级（一级）菜单节点，用于次级菜单派生
 *   （Double 布局的侧边子菜单 / LeftSplit 布局的次级菜单）。
 * 无匹配返回 null。
 */
function getTabsViewDataByRoute(
    route: Pick<RouteLocationNormalized, 'path' | 'fullPath'>,
    mode: 'children' | 'above' = 'children'
): RouteRecordRaw | null {
    const isMatch = (menu: RouteRecordRaw) => menu.path === route.path || menu.path === route.fullPath
    const findDeep = (menus: RouteRecordRaw[]): RouteRecordRaw | null => {
        for (const menu of menus) {
            if (isMatch(menu)) return menu
            if (menu.children?.length) {
                const hit = findDeep(menu.children)
                if (hit) return hit
            }
        }
        return null
    }
    if (mode === 'children') {
        return findDeep(state.tabsViewRoutes)
    }
    for (const top of state.tabsViewRoutes) {
        if (isMatch(top) || (top.children?.length && findDeep(top.children))) {
            return top
        }
    }
    return null
}

/**
 * 设置激活路由（路由变化时调用），并同步激活标签下标
 */
function setActiveRoute(route: RouteLocationNormalized) {
    state.activeRoute = route
    state.activeIndex = state.tabsView.findIndex((item) => item.fullPath === route.fullPath)
    if (state.activeIndex === -1) {
        state.activeIndex = 0
    }
}

/**
 * 关闭标签（右键菜单/关闭图标），并同步 keep-alive 名单
 */
function closeTab(route: RouteLocationNormalized) {
    const idx = state.tabsView.findIndex((item) => item.fullPath === route.fullPath)
    if (idx > -1) {
        state.tabsView.splice(idx, 1)
    }
    syncKeepAliveTabs()
}

/**
 * 关闭其他标签（保留指定标签）
 */
function closeOtherTabs(route: RouteLocationNormalized) {
    state.tabsView = state.tabsView.filter((item) => item.fullPath === route.fullPath)
    syncKeepAliveTabs()
}

/**
 * 关闭全部标签
 */
function closeAllTabs() {
    state.tabsView = []
    syncKeepAliveTabs()
}

/**
 * 依据 tabsView 现存标签重建 keep-alive 名单（视图组件名 = 路由 name）
 */
function syncKeepAliveTabs() {
    state.keepAliveTabs = state.tabsView.map((item) => item.name).filter((name): name is string => typeof name === 'string')
}

function setTabs(data: RouteRecordRaw[]) {
    state.tabs = data
}

function setActiveTab(path: string) {
    state.activeTab = path
}

function setChildrenMenus(data: RouteRecordRaw[]) {
    state.childrenMenus = data
}

function setTabFullScreen(full: boolean) {
    state.tabFullScreen = full
}

/**
 * 标签页关闭后的跳转兜底路径：后台主页
 */
function homePath(): string {
    return adminBaseRoutePath + '/dashboard'
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

/**
 * 刷新派生数据：菜单数据就绪（init 完成）后由布局根组件调用
 */
function refreshMenus() {
    const menu = useMenu()
    setTabsViewRoutes(menu.rawData as RouteRecordRaw[])
}

export { state as navTabsState, setTabsViewRoutes, withAddTab, refreshMenus, homePath }

export const useNavTabs = () => {
    return {
        state,
        setTabs,
        setActiveTab,
        setChildrenMenus,
        setTabFullScreen,
        addTab,
        setActiveRoute,
        closeTab,
        closeOtherTabs,
        closeAllTabs,
        getTabsViewDataByRoute,
        hydrate,
    }
}
