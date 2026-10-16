import { createApp } from 'vue'
import ElementPlus from 'element-plus'
import 'element-plus/dist/index.css'
import App from './App.vue'
import router from './router'
import pinia from '/@/stores/index'
import { setupI18n } from '/@/lang/index'
import Icon from '/@/components/icon/index.vue'
import '/@/styles/index.scss'

// modules import mark, Please do not remove.

async function start() {
    const app = createApp(App)
    app.component('Icon', Icon)
    app.use(ElementPlus)
    app.use(pinia)
    await setupI18n(app)
    app.use(router)
    app.mount('#app')
}

// 启动链路必须自己兜错，原因有两条：
//  1. start() 是 fire-and-forget 的 —— 没有 await 也没有 catch，
//     任何一步 reject 都只会变成 unhandled rejection，页面停在空的 #app 上，
//     用户看到的就是一整页空白且没有任何提示；
//  2. 失败点大多在「渲染之前」——setupI18n 要动态 import 语言包（网络/解析都可能失败）、
//     createApp 之后的挂载也可能抛，此时 Vue 的 errorHandler / app.config.errorHandler
//     都还没生效，指望不上框架层的兜底。
// 因此这里渲染一段纯 DOM 提示（不经 Vue、也不经 i18n —— i18n 本身就可能是失败原因）。
start().catch((err) => {
    console.error('[web] 应用启动失败', err)

    const el = document.getElementById('app')
    if (!el) {
        return
    }
    el.innerHTML =
        '<div style="padding:48px 24px;text-align:center;color:#606266;font-size:14px;line-height:2">' +
        '<div style="font-size:16px;color:#303133;margin-bottom:8px">应用启动失败</div>' +
        '<div>请刷新页面重试；若持续失败，请检查网络或联系管理员。</div>' +
        '</div>'
})
