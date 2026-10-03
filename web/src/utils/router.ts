// web\src\utils\router.ts
// 后台菜单路由工具 —— 裁剪自 buildadmin utils/router.ts（openspec add-admin-layout-shell）：
// 去掉 memberCenter / siteConfig / pageShade 依赖，保留布局所需的最小集合。

import { ElNotification } from 'element-plus'
import { isNavigationFailure, NavigationFailureType } from 'vue-router'
import type { RouteLocationRaw, RouteRecordRaw } from 'vue-router'
import i18n from '/@/lang/index'
import router from '/@/router/index'

/**
 * 带导航失败提示的路由跳转
 */
export const routePush = async (to: RouteLocationRaw) => {
    try {
        const failure = await router.push(to)
        if (isNavigationFailure(failure, NavigationFailureType.aborted)) {
            ElNotification({
                message: i18n.global.t('layouts.Navigation failed, navigation guard intercepted!'),
                type: 'error',
            })
        } else if (isNavigationFailure(failure, NavigationFailureType.duplicated)) {
            ElNotification({
                message: i18n.global.t('layouts.Navigation failed, it is at the navigation target position!'),
                type: 'warning',
            })
        }
    } catch (error) {
        ElNotification({
            message: i18n.global.t('layouts.Navigation failed, invalid route!'),
            type: 'error',
        })
        console.error(error)
    }
}

/**
 * 获取菜单树里第一个可导航的菜单叶子（path 已注册进路由表）
 */
export const getFirstRoute = (routes: RouteRecordRaw[]): false | RouteRecordRaw => {
    const routerPaths: string[] = []
    const allRoutes = router.getRoutes()
    allRoutes.forEach((item) => {
        if (item.path) routerPaths.push(item.path)
    })
    let find: boolean | RouteRecordRaw = false
    for (const key in routes) {
        if (routes[key].meta?.menuType == 'menu' && routerPaths.indexOf(routes[key].path) !== -1) {
            return routes[key]
        } else if (routes[key].children && routes[key].children?.length) {
            find = getFirstRoute(routes[key].children!)
            if (find) return find
        }
    }
    return find
}

/**
 * el-menu 的 index key：优先 name，回退 path
 */
export const getMenuKey = (menu: RouteRecordRaw): string => {
    return String(menu.name || menu.path)
}
