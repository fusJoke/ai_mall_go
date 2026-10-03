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
start()
