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
            // 静态子路由：agUpload 组件测试页（不走后端动态菜单加载）
            // 必须放在 :path(.*)* 兜底路由之前，vue-router 才能优先命中。
            path: 'agInput',
            name: 'adminAgInput',
            component: () => import('/@/views/agInput/index.vue'),
            meta: {
                title: 'agInput 测试',
                // 不依赖后端菜单，便于在 admin 登录态下直接访问
                noAuth: false,
            },
        },
        {
            // 静态子路由：admin 管理页（add-admin-management）
            // 同样不走后端动态菜单加载；admin_rule 表里有同名菜单种子，
            // 走两条独立的链路便于前后端各自刷新。
            path: 'manager',
            name: 'adminManager',
            component: () => import('/@/views/admin/manager/index.vue'),
            meta: {
                title: 'pageTitles.adminManager',
                noAuth: false,
            },
        },
        {
            // 静态子路由：菜单规则管理页（add-admin-rule-management）
            // 同 manager：不在 dynamic 路由表，admin_rule 表里有同名菜单种子，
            // 这里挂静态子路由便于在 admin 登录态下直接访问 /admin/rule。
            path: 'rule',
            name: 'adminRule',
            component: () => import('/@/views/admin/rule/index.vue'),
            meta: {
                title: 'pageTitles.adminRule',
                noAuth: false,
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
