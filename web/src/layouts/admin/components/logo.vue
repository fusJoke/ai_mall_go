<template>
    <div class="layout-title-bar">
        <div class="brand-icon">
            <Icon name="lucide:Sparkles" :size="16" />
        </div>
        <div v-if="!config.layout.menuCollapse" class="website-name">{{ siteName }}</div>
    </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useConfig } from '/@/stores/config'

/**
 * 侧边菜单顶栏（站点标题区），对齐 buildadmin logo.vue。
 * 站点名来自 init 的 site_config.name，缺失时兜底 'AI GO MALL'（示例数据约定）。
 */
const config = useConfig()
const siteConfig = useConfig().siteConfig
const siteName = computed(() => siteConfig.name || 'AI GO MALL')
</script>

<style scoped lang="scss">
.layout-title-bar {
    width: 100%;
    display: flex;
    align-items: center;
    box-sizing: border-box;
    padding: 10px 20px;
    height: 50px;
    background: v-bind('config.getColorVal("menuTopBarBackground")');
    gap: 8px;
}
.brand-icon {
    width: 28px;
    height: 28px;
    display: flex;
    align-items: center;
    justify-content: center;
    border-radius: var(--el-border-radius-base);
    background: var(--el-color-primary-light-8);
    color: var(--el-color-primary);
    flex-shrink: 0;
}
.website-name {
    display: block;
    padding: 0 2px;
    font-size: var(--el-font-size-large);
    font-weight: 600;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    color: v-bind('config.getColorVal("menuTopBarColor")');
}
</style>
