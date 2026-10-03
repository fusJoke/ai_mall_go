// src\router\static\userBase.ts

import type { RouteRecordRaw } from 'vue-router'

/**
 * C 端基础路由路径
 */
export const userBaseRoutePath = '/user'

/**
 * C 端登录页（独立于布局，同 /admin/login 模式）
 */
const userLoginRoute: RouteRecordRaw = {
    path: '/user/login',
    name: 'userLogin',
    component: () => import('/@/views/user/login.vue'),
    meta: {
        title: 'pageTitles.Login',
    },
}

/**
 * C 端基础静态路由（布局 + 子页面）
 */
const userBaseRoute: RouteRecordRaw = {
    path: userBaseRoutePath,
    name: 'user',
    component: () => import('/@/layouts/user/index.vue'),
    redirect: userBaseRoutePath + '/home',
    children: [
        {
            // 首页（ES feed）
            path: 'home',
            name: 'userHome',
            component: () => import('/@/views/user/home/index.vue'),
            meta: {
                title: 'pageTitles.userHome',
            },
        },
        {
            // 盲盒详情（概率公示 + 抽卡）
            path: 'blindbox/:id',
            name: 'userBlindboxDetail',
            component: () => import('/@/views/user/blindbox/detail.vue'),
            meta: {
                title: 'pageTitles.userBlindbox',
            },
        },
        {
            // 秒杀活动列表（限时倒计时）
            path: 'seckill',
            name: 'userSeckill',
            component: () => import('/@/views/user/seckill/index.vue'),
            meta: {
                title: 'pageTitles.userSeckill',
            },
        },
        {
            // 秒杀活动详情（限购 + 实时剩余名额 + 秒杀按钮）
            path: 'seckill/:id',
            name: 'userSeckillDetail',
            component: () => import('/@/views/user/seckill/detail.vue'),
            meta: {
                title: 'pageTitles.userSeckillDetail',
            },
        },
        {
            // 我的订单列表（登录态由页面内自检 + 接口 401 兜底）
            path: 'orders',
            name: 'userOrders',
            component: () => import('/@/views/user/orders/index.vue'),
            meta: {
                title: 'pageTitles.userOrders',
            },
        },
        {
            // 订单详情
            path: 'orders/:id',
            name: 'userOrderDetail',
            component: () => import('/@/views/user/orders/detail.vue'),
            meta: {
                title: 'pageTitles.userOrderDetail',
            },
        },
        {
            // 我的关注（任务 18.11）
            path: 'follow',
            name: 'userFollow',
            component: () => import('/@/views/user/follow/index.vue'),
            meta: {
                title: 'pageTitles.userFollow',
            },
        },
        {
            // 开卡结果页（普通抽卡与秒杀抽卡共用，结果经 sessionStorage 传递）
            path: 'draw/result',
            name: 'userDrawResult',
            component: () => import('/@/views/user/draw/result.vue'),
            meta: {
                title: 'pageTitles.userDrawResult',
            },
        },
        {
            // /user 子路径兜底
            path: ':path(.*)*',
            redirect: '/404',
        },
    ],
}

export default [userLoginRoute, userBaseRoute]
