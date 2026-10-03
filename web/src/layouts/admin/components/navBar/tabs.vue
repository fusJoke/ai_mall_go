<template>
    <div class="nav-tabs" ref="tabScrollbarRef">
        <div
            v-for="(item, idx) in navTabs.state.tabsView"
            :key="item.fullPath"
            class="ba-nav-tab"
            :class="navTabs.state.activeIndex == idx ? 'active' : ''"
            :ref="(el) => setTabRef(el as HTMLDivElement, idx)"
            @click="onTab(item)"
            @contextmenu.prevent="onContextmenu(item, $event)"
        >
            {{ item.meta?.title }}
            <Icon v-if="navTabs.state.tabsView.length > 1" class="close-icon" size="14" name="lucide:X" @click.stop="onCloseTab(item)" />
        </div>
        <div :style="activeBoxStyle" class="nav-tabs-active-box"></div>

        <!-- 内联右键菜单（openspec add-admin-layout-shell：简化自 buildadmin 通用 contextmenu 组件） -->
        <div v-if="contextmenu.visible" class="nav-tabs-contextmenu" :style="{ left: contextmenu.x + 'px', top: contextmenu.y + 'px' }">
            <div class="contextmenu-item" @click="onContextmenuAction('refresh')">
                <Icon name="lucide:RefreshCw" size="13" />{{ $t('layouts.refresh') }}
            </div>
            <div class="contextmenu-item" @click="onContextmenuAction('close')"><Icon name="lucide:X" size="13" />{{ $t('layouts.closeTab') }}</div>
            <div class="contextmenu-item" @click="onContextmenuAction('fullScreen')">
                <Icon name="lucide:Maximize" size="13" />{{ $t('layouts.tabFullscreen') }}
            </div>
            <div class="contextmenu-item" @click="onContextmenuAction('closeOther')">
                <Icon name="lucide:Minimize" size="13" />{{ $t('layouts.closeOtherTabs') }}
            </div>
            <div class="contextmenu-item" @click="onContextmenuAction('closeAll')">
                <Icon name="lucide:X" size="13" />{{ $t('layouts.closeAllTabs') }}
            </div>
        </div>
    </div>
</template>

<script setup lang="ts">
import { inject, nextTick, onBeforeUnmount, onMounted, reactive, ref, watch, type ComponentPublicInstance } from 'vue'
import { useRoute, useRouter, type RouteLocationNormalized } from 'vue-router'
import { useNavTabs, homePath } from '/@/stores/navTabs'

/**
 * 顶栏标签页（openspec add-admin-layout-shell），对齐 buildadmin navBar/tabs.vue 的精简形态：
 * - 标签增删由容器（container/default.vue）路由监听驱动
 * - 右键菜单为内联简化实现（刷新/关闭/关闭其他/关闭全部）
 * - 「刷新」经 main.vue provide 的布局刷新钩子重载当前视图
 */
const route = useRoute()
const router = useRouter()
const navTabs = useNavTabs()

const tabScrollbarRef = ref<HTMLDivElement | null>(null)
const tabRefs = ref<HTMLDivElement[]>([])
const setTabRef = (el: Element | ComponentPublicInstance | null, idx: number) => {
    if (el) tabRefs.value[idx] = el as HTMLDivElement
}

const refreshMain = inject<() => void>('layout-main-refresh', () => {})

const activeBoxStyle = reactive({
    width: '0',
    transform: 'translateX(0px)',
})

const contextmenu = reactive({
    visible: false,
    x: 0,
    y: 0,
    route: null as RouteLocationNormalized | null,
})

// 全局点击收起右键菜单
const onGlobalClick = () => {
    contextmenu.visible = false
}
onMounted(() => {
    window.addEventListener('click', onGlobalClick)
    nextTick(() => selectNavTab())
})
onBeforeUnmount(() => {
    window.removeEventListener('click', onGlobalClick)
})

/**
 * 激活色块位移 + 滚动跟随
 */
const selectNavTab = function (idx = navTabs.state.activeIndex) {
    const dom = tabRefs.value[idx]
    if (!dom) return
    activeBoxStyle.width = dom.clientWidth + 'px'
    activeBoxStyle.transform = `translateX(${dom.offsetLeft}px)`

    if (tabScrollbarRef.value) {
        const scrollLeft = dom.offsetLeft + dom.clientWidth - tabScrollbarRef.value.clientWidth
        if (dom.offsetLeft < tabScrollbarRef.value.scrollLeft) {
            tabScrollbarRef.value.scrollTo(dom.offsetLeft, 0)
        } else if (scrollLeft > tabScrollbarRef.value.scrollLeft) {
            tabScrollbarRef.value.scrollTo(scrollLeft, 0)
        }
    }
}

const onTab = (menu: RouteLocationNormalized) => {
    router.push(menu.fullPath)
}

const onCloseTab = (menu: RouteLocationNormalized) => {
    closeTab(menu)
}

/**
 * 关闭标签：关闭后若激活标签被关，跳最后一个标签或主页
 */
function closeTab(route: RouteLocationNormalized) {
    const isActive = navTabs.state.activeRoute?.fullPath === route.fullPath
    navTabs.closeTab(route)
    if (isActive) {
        const lastTab = navTabs.state.tabsView.slice(-1)[0]
        router.push(lastTab ? lastTab.fullPath : homePath())
    }
    nextTick(() => selectNavTab())
}

function closeOtherTabs(route: RouteLocationNormalized) {
    navTabs.closeOtherTabs(route)
    router.push(route.fullPath)
    nextTick(() => selectNavTab(0))
}

function closeAllTabs() {
    navTabs.closeAllTabs()
    router.push(homePath())
    nextTick(() => selectNavTab())
}

const onContextmenu = (item: RouteLocationNormalized, e: MouseEvent) => {
    contextmenu.visible = true
    contextmenu.x = e.clientX
    contextmenu.y = e.clientY
    contextmenu.route = item
}

const onContextmenuAction = (name: string) => {
    const item = contextmenu.route
    contextmenu.visible = false
    if (!item) return
    switch (name) {
        case 'refresh':
            if (navTabs.state.activeRoute?.fullPath === item.fullPath) {
                refreshMain()
            } else {
                router.push(item.fullPath)
            }
            break
        case 'close':
            closeTab(item)
            break
        case 'fullScreen':
            if (route.fullPath !== item.fullPath) {
                router.push(item.fullPath)
            }
            navTabs.setTabFullScreen(true)
            break
        case 'closeOther':
            closeOtherTabs(item)
            break
        case 'closeAll':
            closeAllTabs()
            break
    }
}

// 激活路由变化时移动色块
watch(
    () => navTabs.state.activeIndex,
    () => nextTick(() => selectNavTab())
)
</script>

<style scoped lang="scss">
.nav-tabs {
    display: flex;
    overflow-x: auto;
    scrollbar-width: none;

    &::-webkit-scrollbar {
        display: none;
    }
}

.nav-tabs-contextmenu {
    position: fixed;
    z-index: 2100;
    min-width: 130px;
    background: var(--el-bg-color-overlay);
    border-radius: var(--el-border-radius-base);
    box-shadow: var(--el-box-shadow-light);
    padding: 4px 0;

    .contextmenu-item {
        display: flex;
        align-items: center;
        gap: 8px;
        padding: 7px 16px;
        font-size: 13px;
        color: var(--el-text-color-primary);
        cursor: pointer;

        &:hover {
            background: var(--el-fill-color-light);
            color: var(--el-color-primary);
        }
    }
}
</style>
