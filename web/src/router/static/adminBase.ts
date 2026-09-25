// src\router\static\adminBase.ts

import type { RouteRecordRaw } from 'vue-router'

/**
 * 后台基础路由路径
 */
export const adminBaseRoutePath = '/admin'

/*
 * 后台基础静态路由
 */
const adminBaseRoute: RouteRecordRaw = {
    path: adminBaseRoutePath,
    name: 'admin',
    component: () => import('/@/layouts/admin/index.vue'),
    // 直接重定向到 loading 路由
    redirect: adminBaseRoutePath + '/loading',
    meta: {
        title: `pageTitles.Loading`,
    },
    children: [
        {
            path: 'loading/:to?',
            name: 'adminMainLoading',
            component: () => import('/@/layouts/common/loading.vue'),
            meta: {
                title: `pageTitles.Loading`,
            },
        },
        {
            // 后台子路径兜底 — 走 loading 路由，让 loading 页面尝试从后端懒加载目标路由
            path: ':path(.*)*',
            redirect: (to) => {
                return {
                    name: 'adminMainLoading',
                    params: {
                        to: JSON.stringify({
                            path: to.path,
                            query: to.query,
                        }),
                    },
                }
            },
        },
    ],
}

export default adminBaseRoute
