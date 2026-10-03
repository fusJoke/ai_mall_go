// src\router\static\supplierBase.ts

import type { RouteRecordRaw } from 'vue-router'

/**
 * B 端基础路由路径
 */
export const supplierBaseRoutePath = '/supplier'

/**
 * B 端登录页（独立于布局，同 /admin/login 模式）
 */
const supplierLoginRoute: RouteRecordRaw = {
    path: '/supplier/login',
    name: 'supplierLogin',
    component: () => import('/@/views/supplier/login.vue'),
    meta: {
        title: 'pageTitles.Login',
    },
}

/**
 * B 端基础静态路由（布局 + 子页面）
 */
const supplierBaseRoute: RouteRecordRaw = {
    path: supplierBaseRoutePath,
    name: 'supplier',
    component: () => import('/@/layouts/supplier/index.vue'),
    redirect: supplierBaseRoutePath + '/products',
    children: [
        {
            // 商品管理
            path: 'products',
            name: 'supplierProducts',
            component: () => import('/@/views/supplier/products/index.vue'),
            meta: {
                title: 'pageTitles.supplierProducts',
            },
        },
        {
            // 商品新建 / 编辑（:id 缺省 = 新建）
            path: 'products/edit/:id?',
            name: 'supplierProductEdit',
            component: () => import('/@/views/supplier/products/edit.vue'),
            meta: {
                title: 'pageTitles.supplierProductEdit',
            },
        },
        {
            // 限时特价管理
            path: 'promotions',
            name: 'supplierPromotions',
            component: () => import('/@/views/supplier/promotions/index.vue'),
            meta: {
                title: 'pageTitles.adminPromotion',
            },
        },
        {
            // 秒杀活动管理
            path: 'seckill',
            name: 'supplierSeckill',
            component: () => import('/@/views/supplier/seckill/index.vue'),
            meta: {
                title: 'pageTitles.supplierSeckill',
            },
        },
        {
            // 秒杀新建 / 编辑（:id 缺省 = 新建）
            path: 'seckill/edit/:id?',
            name: 'supplierSeckillEdit',
            component: () => import('/@/views/supplier/seckill/edit.vue'),
            meta: {
                title: 'pageTitles.supplierSeckillEdit',
            },
        },
        {
            // /supplier 子路径兜底
            path: ':path(.*)*',
            redirect: '/404',
        },
    ],
}

export default [supplierLoginRoute, supplierBaseRoute]
