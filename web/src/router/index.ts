// src\router\index.ts 文件，即 createRouter 的文件

import NProgress from 'nprogress'
import 'nprogress/nprogress.css'
import { createRouter, createWebHashHistory } from 'vue-router'
import staticRoutes from '/@/router/static'

const router = createRouter({
    history: createWebHashHistory(),
    routes: staticRoutes,
})

// 路由加载前
router.beforeEach(() => {
    // 显示进度条
    NProgress.configure({ showSpinner: false })
    NProgress.start()
})

// 路由加载后
router.afterEach(() => {
    // 隐藏进度条
    NProgress.done()
})

export default router
