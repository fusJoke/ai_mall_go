// src\router\static.ts

import type { RouteRecordRaw } from 'vue-router'

/*
 * 静态路由（支持自动扩展）
 * 系统会自动加载 ./static 目录及其子目录中的所有 .ts 文件
 * 每个模块的 default 导出可以是 RouteRecordRaw 或 RouteRecordRaw[]，自动 push 到以下 staticRoutes 数组
 */
const staticRoutes: Array<RouteRecordRaw> = [
    {
        // 首页
        path: '/',
        name: '/',
        component: () => import('/@/views/index.vue'),
        meta: {
            title: `pageTitles.Home`,
        },
    },
    {
        // 登录页
        path: '/admin/login',
        name: 'login',
        component: () => import('/@/views/admin/login.vue'),
        meta: {
            title: `pageTitles.Login`,
        },
    },
    {
        // 404 页
        path: '/404',
        name: 'notFound',
        component: () => import('/@/views/404.vue'),
        meta: {
            title: `pageTitles.NotFound`,
        },
    },
    {
        // 未知路由通配跳转 - 必须放在最后
        path: '/:path(.*)*',
        redirect: '/404',
    },
]

// 静态路由自动扩展逻辑
const staticFiles = import.meta.glob('./static/**/*.ts', { eager: true })
for (const path in staticFiles) {
    const module = staticFiles[path] as any
    if (!module.default) {
        console.warn(`[Router] Static route module ${path} does not export default, skipped`)
        continue
    }
    const route = module.default as RouteRecordRaw | RouteRecordRaw[]
    if (Array.isArray(route)) {
        staticRoutes.push(...route)
    } else {
        staticRoutes.push(route)
    }
}

export default staticRoutes
