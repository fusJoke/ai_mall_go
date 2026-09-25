<template>
    <el-config-provider :value-on-clear="() => null" :locale="elLocale">
        <router-view></router-view>
    </el-config-provider>
</template>

<script setup lang="ts">
import elEn from 'element-plus/es/locale/lang/en'
import elZhCn from 'element-plus/es/locale/lang/zh-cn'
import { computed } from 'vue'
import { useConfig } from '/@/stores/config'

const config = useConfig()

const elLocales: Record<string, typeof elEn> = {
    en: elEn,
    'zh-cn': elZhCn,
}

const elLocale = computed(() => elLocales[config.lang.defaultLang] || elLocales[config.lang.fallbackLang])
</script>
