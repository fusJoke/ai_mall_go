<template>
    <div class="admin-layout">
        <router-view />
    </div>
</template>

<script setup lang="ts">
/**
 * 后台根布局 —— 挂载即触发后台初始化流程。
 *
 * 流程（spec admin-init "前端 init() 调用流程" 6 步）：
 *  1. 调 GET /admin/init（api/admin/init）
 *  2. 失败 → 清空 adminInfo + router.replace('/admin/login')
 *  3. 成功 → 写 useAdminInfo（不含 token）+ useConfig.siteConfig + useMenu.rawData
 *  4. 遍历 menus，type !== 'node' 的 router.addRoute（同名按 hasRoute 去重）
 *  5. 取排序后第一个菜单 router.replace 到其 path
 *  6. menus 为空 → 保留在 /admin/loading（兜底）
 *
 * 组件本身只 mount 一次（layout 路由），后续内部路由切换不会重复 mount，
 * 但若用户刷新页面或外层 layout 被替换，会再次触发 init —— 数据填充
 * 是覆盖式（setRawData / dataFill 都会替换而非追加），addRoute 前先 hasRoute
 * 去重，所以重复 mount 不会累积脏路由。
 */
import { onMounted } from 'vue'
import { useRouter, type RouteRecordRaw } from 'vue-router'
import { init as adminInit } from '/@/api/admin'
import { useAdminInfo } from '/@/stores/adminInfo'
import { useConfig } from '/@/stores/config'
import { useMenu } from '/@/stores/menu'

const router = useRouter()
const adminInfo = useAdminInfo()
const config = useConfig()
const menu = useMenu()

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
 * 把菜单规则注册为 vue-router 路由（type === 'node' 跳过）。
 *
 * 注册前先 hasRoute(name) 检测：重复 mount 时若已存在则 removeRoute 再 add，
 * 保证不会累积重复的同名路由。
 */
function registerMenuRoutes(menus: RouteRecordRaw[]): void {
    for (const route of menus) {
        if (route.meta?.menuType === 'node') {
            // 权限节点不进路由，但保留在 useMenu.rawData 里供前端展示权限点标签。
            continue
        }
        const name = typeof route.name === 'string' ? route.name : undefined
        if (name && router.hasRoute(name)) {
            router.removeRoute(name)
        }
        router.addRoute(route)
    }
}

/**
 * 从菜单规则集合里挑排序后的第一个菜单（按 weigh ASC, id ASC）。
 *
 * 规则：
 *   - 只考虑 type === 'menu'（dir / node 不参与跳转）
 *   - 空集合返回 undefined —— 由调用方决定兜底
 */
function pickFirstMenu(menus: { id: number; weigh: number; type: string; path: string }[]): string | undefined {
    const candidates = menus
        .filter((m) => m.type === 'menu' && m.path)
        .slice() // 复制避免修改传入数组
        .sort((a, b) => (a.weigh !== b.weigh ? a.weigh - b.weigh : a.id - b.id))
    return candidates[0]?.path
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

    // 4) 注册路由（node 跳过，同名去重）
    registerMenuRoutes(routes)

    // 5) 跳排序后的第一个菜单（无菜单时保留在 /admin/loading）
    const firstPath = pickFirstMenu(resp.menus)
    if (firstPath) {
        // 用 replace 而非 push：避免浏览器历史留下 loading 页这一层
        router.replace(firstPath)
    }
})
</script>

<style scoped lang="scss">
.admin-layout {
    width: 100vw;
    height: 100vh;
}
</style>