<template>
    <div class="admin-layout">
        <router-view />
    </div>
</template>

<script setup lang="ts">
/**
 * 后台根布局 —— 挂载即触发后台初始化流程。
 *
 * 流程（spec admin-init「前端 init 流程与登录落地」）：
 *  1. 调 GET /admin/init（api/admin/init）
 *  2. 失败 → 清空 adminInfo + router.replace('/admin/login')
 *  3. 成功 → 写 useAdminInfo（不含 token）+ useConfig.siteConfig + useMenu.rawData
 *  4. 遍历 menus：type !== 'node' 的注册为动态路由 —— component 字符串经
 *     import.meta.glob 解析为懒加载组件；相对 path 注册为 admin 布局的子路由
 *     （vue-router 拼接为 /admin/xxx），绝对 path 顶层注册（同名按 hasRoute 去重）
 *  5. 仅当当前停在 /admin/loading 占位页时跳转：loading 路由的 to 深链参数
 *     （catch-all 兜底带来）优先，否则落地后台主页 /admin/dashboard
 *
 * 组件本身只 mount 一次（layout 路由），后续内部路由切换不会重复 mount，
 * 但若用户刷新页面或外层 layout 被替换，会再次触发 init —— 数据填充
 * 是覆盖式（setRawData / dataFill 都会替换而非追加），addRoute 前先 hasRoute
 * 去重，所以重复 mount 不会累积脏路由。
 */
import { onMounted, watch } from 'vue'
import { useRoute, useRouter, type RouteComponent, type RouteRecordRaw } from 'vue-router'
import { init as adminInit } from '/@/api/admin'
import { useAdminInfo } from '/@/stores/adminInfo'
import { useConfig } from '/@/stores/config'
import { useMenu } from '/@/stores/menu'

const router = useRouter()
const route = useRoute()
const adminInfo = useAdminInfo()
const config = useConfig()
const menu = useMenu()

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
    const candidates = raw.startsWith('/src/views/')
        ? [raw]
        : raw
          ? [`/src/views/${raw}.vue`, `/src/views/${raw}/index.vue`]
          : []
    for (const key of candidates) {
        const mod = viewModules[key]
        if (mod) {
            return mod as RouteComponent
        }
    }
    return viewModules['/src/views/404.vue'] as RouteComponent
}

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

/**
 * 把菜单规则注册为 vue-router 路由（type === 'node' 跳过）。
 *
 * - component 字符串先经 resolveViewComponent 解析为懒加载组件（后端契约只传字符串）；
 * - 相对 path（不以 / 开头，后端 routeFromRule 的约定）注册为 admin 布局的子路由，
 *   由 vue-router 拼接为 /admin/xxx；绝对 path 保持顶层注册；
 * - 注册前先 hasRoute(name) 检测：重复 mount 时若已存在则 removeRoute 再 add，
 *   保证不会累积重复的同名路由。
 */
function registerMenuRoutes(menus: RouteRecordRaw[]): void {
    for (const item of menus) {
        if (item.meta?.menuType === 'node') {
            // 权限节点不进路由，但保留在 useMenu.rawData 里供前端展示权限点标签。
            continue
        }
        const name = typeof item.name === 'string' ? item.name : undefined
        if (name && router.hasRoute(name)) {
            router.removeRoute(name)
        }
        // 显式构造 RouteRecordSingleView：item 是联合类型的 RouteRecordRaw，
        // 直接 spread 再覆写 component 会产生非法组合。动态菜单按后端契约
        // （routeFromRule）只产出叶子视图节点，path/name/component/meta 足够。
        const record: RouteRecordRaw = {
            path: typeof item.path === 'string' ? item.path : '',
            name: name,
            component: resolveViewComponent(item.component),
            meta: item.meta,
        }
        if (record.path.startsWith('/')) {
            router.addRoute(record)
        } else {
            router.addRoute('admin', record)
        }
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

    // 3) menu rawData（覆盖式：useMenu.setRawData 是赋值而非追加）
    const routes = resp.menus as unknown as RouteRecordRaw[]
    menu.setRawData(routes)

    // 4) 注册路由（node 跳过，同名去重，component 解析为真实组件）
    registerMenuRoutes(routes)

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

<style scoped lang="scss">
.admin-layout {
    width: 100vw;
    height: 100vh;
}
</style>