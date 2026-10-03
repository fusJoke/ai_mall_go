<template>
    <div class="layout-iframe">
        <iframe :src="url" frameborder="0" allowfullscreen />
    </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import { adminBaseRoutePath } from '/@/router/static/adminBase'

/**
 * iframe 菜单视图（对齐 buildadmin common/router-view/iframe.vue）：
 * 菜单 store 已把 iframe 菜单的 path 编码为 `{adminBaseRoutePath}/iframe/{encodeURIComponent(url)}`，
 * 本视图从路由 path 中解出原始 url 嵌入 iframe。
 */
const route = useRoute()

const url = computed(() => {
    const prefix = `${adminBaseRoutePath}/iframe/`
    if (route.path.startsWith(prefix)) {
        return decodeURIComponent(route.path.slice(prefix.length))
    }
    const param = route.params.url
    return decodeURIComponent(String(Array.isArray(param) ? param[0] : (param ?? '')))
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
}
</style>
