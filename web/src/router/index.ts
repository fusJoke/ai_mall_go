// src\router\index.ts 文件，即 createRouter 的文件

import NProgress from 'nprogress'
import 'nprogress/nprogress.css'
import { createRouter, createWebHashHistory } from 'vue-router'
import staticRoutes from '/@/router/static'
import { useTitle } from '@vueuse/core'
import { loading } from '/@/utils/loading'
import i18n from '/@/lang/index'

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
router.afterEach((to) => {
    if (window.loading) {
        loading.hide()
    }
    NProgress.done()

    // 设置浏览器标题
    const titleKey = to?.meta?.title as string | undefined
    const title = titleKey && i18n.global.te(titleKey) ? i18n.global.t(titleKey) : ''
    useTitle().value = title ? `${title} - AI GO MALL` : 'AI GO MALL'
})



export default router
