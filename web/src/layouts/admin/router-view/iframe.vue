<template>
    <div class="layout-iframe">
        <div v-if="!url" class="layout-iframe__blocked">{{ t('admin.layout.iframeBlocked') }}</div>
        <iframe v-else :src="url" frameborder="0" allowfullscreen />
    </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute } from 'vue-router'
import { adminBaseRoutePath } from '/@/router/static/adminBase'

/**
 * iframe 菜单视图（对齐 buildadmin common/router-view/iframe.vue）：
 * 菜单 store 已把 iframe 菜单的 path 编码为 `{adminBaseRoutePath}/iframe/{encodeURIComponent(url)}`，
 * 本视图从路由 path 中解出原始 url 嵌入 iframe。
 */
const route = useRoute()
const { t } = useI18n()

/**
 * iframe src 允许的协议白名单（OCR 审查 P3）。
 *
 * 为什么需要它：菜单里的 url 来自数据库（`adminRule` 表的菜单配置），
 * 本视图把它 `decodeURIComponent` 后**直接**塞进 `iframe[src]`，中间没有任何校验。
 * 菜单可写者（或库被篡改）能借此把任意字符串变成框架源。
 *
 * 为什么白名单放在「协议」这一层，而不是「host」：
 *   - 这不是一个封闭系统，iframe 菜单的用途就是挂外部系统/文档站，host 天然是开放的，
 *     硬编码 host 白名单只会逼运维每次新增菜单都改代码；真正的**载体**是协议；
 *   - 需要拦的是 `javascript:`（脚本执行）、`data:` / `blob:`（opaque origin 的页面）、
 *     `file:`（本地文件探测）这类非网络协议；
 *   - `http:` / `https:` 页面受同源策略约束，拿不到父窗口的 DOM 与 localStorage，
 *     跨域 XSS 面为零。
 *
 * 注：现代浏览器已默认禁止 iframe 加载 `javascript:`，所以这是纵深防御而非补漏；
 * 但 `data:` 目前仍可加载，白名单能把它一并挡掉。
 */
const ALLOWED_IFRAME_PROTOCOLS = new Set(['http:', 'https:'])

/** 从路由 path / params 中还原菜单配置的原始 url（不做任何信任假设）。 */
const rawUrl = computed(() => {
    const prefix = `${adminBaseRoutePath}/iframe/`
    if (route.path.startsWith(prefix)) {
        return decodeURIComponent(route.path.slice(prefix.length))
    }
    const param = route.params.url
    return decodeURIComponent(String(Array.isArray(param) ? param[0] : (param ?? '')))
})

/**
 * 校验后的 iframe src。校验不通过返回空串，模板据此不渲染 iframe
 * （空 src 会让 iframe 回退加载当前页面，所以必须显式不渲染）。
 */
const url = computed(() => {
    const raw = rawUrl.value.trim()
    if (raw === '') {
        return ''
    }

    let parsed: URL
    try {
        // 相对地址按当前站点解析，绝对地址按自身协议解析。
        parsed = new URL(raw, window.location.origin)
    } catch {
        console.error('[admin/iframe] 菜单地址无法解析为 URL，已拒绝加载：', raw)
        return ''
    }

    if (!ALLOWED_IFRAME_PROTOCOLS.has(parsed.protocol)) {
        console.error(`[admin/iframe] 菜单地址协议 ${parsed.protocol} 不在白名单内，已拒绝加载：`, raw)
        return ''
    }

    return parsed.href
})
</script>

<style scoped lang="scss">
.layout-iframe {
    width: 100%;
    height: calc(100vh - 130px);

    iframe {
        width: 100%;
        height: 100%;
        border: none;
        border-radius: var(--el-border-radius-base);
        background: var(--ba-bg-color-overlay);
    }

    &__blocked {
        padding: 48px 24px;
        text-align: center;
        line-height: 2;
        color: var(--el-text-color-secondary);
        background: var(--ba-bg-color-overlay);
        border-radius: var(--el-border-radius-base);
    }
}
</style>
