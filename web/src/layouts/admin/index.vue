<template>
    <component :is="layoutContainer" />
    <LayoutConfig />
</template>

<script setup lang="ts">
/**
 * 后台根布局 —— 挂载即触发后台初始化流程，按 config.layout.layoutMode 渲染布局容器
 * （Default / Classic / Streamline / Double / LeftSplit，对齐 buildadmin backend/index.vue）。
 *
 * 流程（spec admin-init「前端 init 流程与登录落地」）：
 *  1. 调 GET /admin/init（api/admin/init）
 *  2. 失败 → 清空 adminInfo + router.replace('/admin/login')
 *  3. 成功 → 写 useAdminInfo（不含 token）+ useConfig.siteConfig + useMenu.rawData；
 *     menus 为空数组时回落示例菜单（web/src/mock/menus.ts，示例数据约定）
 *  4. 遍历菜单树：dir 目录不注册路由、递归其 menu 叶子；叶子 component 字符串经
 *     import.meta.glob 解析为懒加载组件；相对 path 注册为 admin 布局的子路由
 *     （vue-router 拼接为 /admin/xxx），绝对 path 顶层注册（同名按 hasRoute 去重）；
 *     iframe 菜单（meta.menuType == 'iframe'）改挂 iframe 视图组件
 *  5. 仅当当前停在 /admin/loading 占位页时跳转：loading 路由的 to 深链参数
 *     （catch-all 兜底带来）优先，否则落地后台主页 /admin/dashboard
 *
 * 标签页的增删/激活由本组件对路由的 watch 统一驱动（所有容器共享同一逻辑）。
 *
 * 组件本身只 mount 一次（layout 路由），后续内部路由切换不会重复 mount，
 * 但若用户刷新页面或外层 layout 被替换，会再次触发 init —— 数据填充
 * 是覆盖式（setRawData / dataFill 都会替换而非追加），addRoute 前先 hasRoute
 * 去重，所以重复 mount 不会累积脏路由。
 */
import { computed, onMounted, watch } from 'vue'
import { useRoute, useRouter, type RouteComponent, type RouteRecordRaw } from 'vue-router'
import { init as adminInit } from '/@/api/admin'
import ClassicContainer from '/@/layouts/admin/container/classic.vue'
import DefaultContainer from '/@/layouts/admin/container/default.vue'
import DoubleContainer from '/@/layouts/admin/container/double.vue'
import LeftSplitContainer from '/@/layouts/admin/container/leftSplit.vue'
import StreamlineContainer from '/@/layouts/admin/container/streamline.vue'
import LayoutConfig from '/@/layouts/admin/components/config.vue'
import { exampleAdminMenus } from '/@/mock/menus'
import { refreshMenus, useNavTabs } from '/@/stores/navTabs'
import { useAdminInfo } from '/@/stores/adminInfo'
import { useConfig } from '/@/stores/config'
import { useMenu } from '/@/stores/menu'
import type { Component } from 'vue'

const router = useRouter()
const route = useRoute()
const adminInfo = useAdminInfo()
const config = useConfig()
const menu = useMenu()
const navTabs = useNavTabs()

/** 布局模式 → 容器组件映射 */
const containerMap: Record<string, Component> = {
    Default: DefaultContainer,
    Classic: ClassicContainer,
    Streamline: StreamlineContainer,
    Double: DoubleContainer,
    LeftSplit: LeftSplitContainer,
}

const layoutContainer = computed(() => containerMap[config.layout.layoutMode] ?? DefaultContainer)

/** 后台主页：init 完成且无深链目标时的落地页（静态子路由，见 adminBase.ts） */
const adminHomePagePath = '/admin/dashboard'

/** init 是否已完成：完成后布局内再次进入 loading 占位页时直接跳走，不再重复 init */
let initDone = false

/**
 * 把后端 menu 的 component 字符串解析为懒加载组件（routeFromRule 契约：
 * 形如 '/src/views/admin/manager/index.vue'）。短形式 'admin/manager' 依次尝试
 * '/src/views/{c}.vue' 与 '/src/views/{c}/index.vue'；全不命中兜底 404 页。
 */
const viewModules = import.meta.glob('/src/views/**/*.vue')

function resolveViewComponent(component: unknown): RouteComponent {
    const raw = typeof component === 'string' ? component : ''
    const candidates = raw.startsWith('/src/views/') ? [raw] : raw ? [`/src/views/${raw}.vue`, `/src/views/${raw}/index.vue`] : []
    for (const key of candidates) {
        const mod = viewModules[key]
        if (mod) {
            return mod as RouteComponent
        }
    }
    return viewModules['/src/views/404.vue'] as RouteComponent
}

/**
 * iframe 视图组件（菜单 store 已把 iframe 菜单的 path 编码为 /admin/iframe/{encoded}）
 */
const iframeViewModule = () => import('/@/layouts/admin/router-view/iframe.vue')

/**
 * 从错误对象提取 HTTP status（兼容 axios 错误形状）。
 *
 * axios 错误链：AxiosError → response.status；业务错误（非 HTTP 错误）没 response 字段。
 */
function getStatus(err: unknown): number | undefined {
    const anyErr = err as { response?: { status?: number } } | undefined
    return anyErr?.response?.status
}

/**
 * 解析 loading 路由携带的 to 深链参数（adminBase 兜底路由重定向时 JSON 化的目标）。
 * 非法（非 JSON / path 非字符串 / 不以 / 开头 / 指向 loading 自身）→ undefined，回落主页。
 */
function resolveToTarget(): string {
    const raw = route.params.to
    const json = Array.isArray(raw) ? raw[0] : raw
    if (!json) {
        return adminHomePagePath
    }
    try {
        const parsed = JSON.parse(json) as { path?: unknown }
        const path = typeof parsed.path === 'string' ? parsed.path : ''
        if (path.startsWith('/') && path !== '/admin/loading') {
            return path
        }
    } catch {
        // 非法 JSON —— 按无深链处理，回落主页
    }
    return adminHomePagePath
}

// init 完成后布局内再进入 loading 占位页（如手动改址 / 兜底重定向），直接跳目标页。
// 跳转动作与 onMounted 第 5 步一致：深链 to 参数优先，否则后台主页。
watch(
    () => route.name,
    (name) => {
        if (initDone && name === 'adminMainLoading') {
            const target = resolveToTarget()
            router.replace(target).catch(() => router.replace(adminHomePagePath))
        }
    }
)

// 标签页的增删与激活态统一由本组件驱动（所有布局容器共享，替代原 container 内的监听）
watch(
    () => route.fullPath,
    () => {
        navTabs.addTab(route as never)
        navTabs.setActiveRoute(route as never)
    },
    { immediate: true }
)

/**
 * 注册单个菜单叶子为路由。
 *
 * - component 字符串先经 resolveViewComponent 解析为懒加载组件（后端契约只传字符串）；
 * - 相对 path（不以 / 开头，后端 routeFromRule 的约定）注册为 admin 布局的子路由，
 *   由 vue-router 拼接为 /admin/xxx；绝对 path 保持顶层注册；
 * - iframe 菜单（meta.menuType == 'iframe'，path 已被菜单 store 编码为
 *   /admin/iframe/{encoded}）改挂 iframe 视图组件；
 * - 注册前先 hasRoute(name) 检测：重复 mount 时若已存在则 removeRoute 再 add，
 *   保证不会累积重复的同名路由。
 */
function registerLeafRoute(item: { path?: unknown; name?: unknown; component?: unknown; meta?: unknown }): void {
    const name = typeof item.name === 'string' ? item.name : undefined
    if (name && router.hasRoute(name)) {
        router.removeRoute(name)
    }
    const menuType = (item.meta as RouteRecordRaw['meta'])?.menuType
    // 显式构造 RouteRecordSingleView：菜单 item 是普通对象而非合法 RouteRecordRaw
    // （后端只给字符串 component），直接 spread 会产生非法组合。
    const record: RouteRecordRaw = {
        path: typeof item.path === 'string' ? item.path : '',
        name: name,
        component: menuType === 'iframe' ? iframeViewModule : resolveViewComponent(item.component),
        meta: item.meta as RouteRecordRaw['meta'],
    }
    if (record.path.startsWith('/')) {
        router.addRoute(record)
    } else {
        router.addRoute('admin', record)
    }
}

/**
 * 把菜单树注册为 vue-router 路由（type === 'node' 权限节点跳过）。
 *
 * 目录（menuType === 'dir'）不注册路由，递归其 children；
 * 叶子（menuType === 'menu'）逐个注册。
 */
function registerMenuRoutes(menus: RouteRecordRaw[]): void {
    for (const item of menus) {
        const menuType = item.meta?.menuType
        if (menuType === 'node') {
            // 权限节点不进路由，但保留在 useMenu.rawData 里供前端展示权限点标签。
            continue
        }
        if (menuType === 'dir') {
            if (item.children?.length) {
                registerMenuRoutes(item.children)
            }
            continue
        }
        registerLeafRoute(item as never)
    }
}

onMounted(async () => {
    let resp
    try {
        const result = await adminInit()
        resp = result.data
    } catch (err) {
        const status = getStatus(err)
        // 401：token 失效 / 类型不符 / 已过期 —— 中间件已清 token，这里再清 store。
        // 403：admin 被禁用 / 已被删除 —— 同样清空 + 跳登录页。
        // 5xx：后端故障 —— 也走兜底跳登录，避免卡在 loading。
        if (status === 401 || status === 403 || (status !== undefined && status >= 500)) {
            adminInfo.reset()
            router.replace('/admin/login')
        }
        // 其他未知错误（如网络断开）—— 静默留在 loading，由全局兜底或下次访问处理。
        return
    }

    // 1) admin info（dataFill 默认排除 token，避免覆盖）
    adminInfo.dataFill({
        id: resp.admin.id,
        username: resp.admin.username,
        nickname: resp.admin.nickname,
        avatar: resp.admin.avatar,
        last_login_at: resp.admin.last_login_at ?? '',
        last_login_ip: resp.admin.last_login_ip ?? '',
        super: resp.admin.super,
    })

    // 2) site config
    config.setSiteConfig(resp.site_config)

    // 3) menu rawData（覆盖式）：服务端 menus 为空时使用示例菜单兜底
    //    （openspec add-admin-layout-shell：示例数据约定，root 无分组权限即空）
    let routes = resp.menus as unknown as RouteRecordRaw[]
    if (!routes.length) {
        routes = exampleAdminMenus as unknown as RouteRecordRaw[]
    }
    menu.setRawData(routes)

    // 4) 注册路由（node/dir 跳过，同名去重，component 解析为真实组件）
    registerMenuRoutes(routes)

    // 4.1) 派生菜单路由树给侧边菜单/标签页（navTabs 模块单例）
    refreshMenus()

    // 5) 跳转：仅当当前停在 loading 占位页时执行（深链直达 manager/rule 等已注册
    //    路由时布局同样 mount + init，但不该被拽走）。目标 = to 深链参数优先，
    //    否则后台主页。用 replace 而非 push：避免浏览器历史留下 loading 页这一层。
    initDone = true
    if (route.name === 'adminMainLoading') {
        const target = resolveToTarget()
        router.replace(target).catch(() => router.replace(adminHomePagePath))
    }
})
</script>
