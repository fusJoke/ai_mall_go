<template>
    <div class="nav-menus">
        <!-- 站点主页（新窗口） -->
        <router-link class="h100" target="_blank" :title="$t('layouts.home')" to="/">
            <div class="nav-menu-item">
                <Icon :color="config.getColorVal('headerBarTabColor')" class="nav-menu-icon" name="lucide:Monitor" size="18" />
            </div>
        </router-link>

        <!-- 语言切换 -->
        <el-dropdown class="h100" size="large" :hide-timeout="50" placement="bottom" trigger="click">
            <div class="nav-menu-item" :title="$t('layouts.language')">
                <Icon :color="config.getColorVal('headerBarTabColor')" class="nav-menu-icon" name="lucide:Languages" size="18" />
            </div>
            <template #dropdown>
                <el-dropdown-menu>
                    <el-dropdown-item
                        v-for="item in config.lang.langArray"
                        :key="item.name"
                        :disabled="item.name == config.lang.defaultLang"
                        @click="onSwitchLang(item.name)"
                    >
                        {{ item.value }}
                    </el-dropdown-item>
                </el-dropdown-menu>
            </template>
        </el-dropdown>

        <!-- 布局配置 -->
        <div class="nav-menu-item" :title="$t('layouts.layoutConfiguration')" @click="config.setLayout('showDrawer', true)">
            <Icon :color="config.getColorVal('headerBarTabColor')" class="nav-menu-icon" name="lucide:Settings2" size="18" />
        </div>

        <!-- 暗色切换 -->
        <div class="nav-menu-item" :title="$t('layouts.darkMode')" @click="onToggleDark">
            <Icon
                :color="config.getColorVal('headerBarTabColor')"
                class="nav-menu-icon"
                :name="config.layout.isDark ? 'lucide:Moon' : 'lucide:Sun'"
                size="18"
            />
        </div>

        <!-- 全屏切换 -->
        <div class="nav-menu-item" :title="state.isFullScreen ? $t('layouts.exitFullscreen') : $t('layouts.FullScreen')" @click="onFullScreen">
            <Icon
                :color="config.getColorVal('headerBarTabColor')"
                class="nav-menu-icon"
                :name="state.isFullScreen ? 'lucide:Minimize' : 'lucide:Maximize'"
                size="18"
            />
        </div>

        <!-- 管理员信息 -->
        <el-popover placement="bottom-end" :width="260" trigger="click" popper-class="admin-info-box">
            <template #reference>
                <div class="admin-info">
                    <el-avatar :size="25">{{ adminInfo.nickname.slice(0, 1) }}</el-avatar>
                    <div class="admin-name">{{ adminInfo.nickname }}</div>
                </div>
            </template>
            <div>
                <div class="admin-info-base">
                    <el-avatar :size="70">{{ adminInfo.nickname.slice(0, 1) }}</el-avatar>
                    <div class="admin-info-other">
                        <div class="admin-info-name">{{ adminInfo.nickname }}</div>
                        <div class="admin-info-lasttime">{{ adminInfo.last_login_at || '-' }}</div>
                    </div>
                </div>
                <div class="admin-info-footer">
                    <el-button @click="onOpenProfile">{{ $t('layouts.profile') }}</el-button>
                    <el-button @click="onLogout" type="danger" plain>{{ $t('layouts.logout') }}</el-button>
                </div>
            </div>
        </el-popover>

        <!-- 账号资料抽屉（示例数据约定：编辑仅写本地 store） -->
        <BaAccount v-model="accountDrawerVisible" />
    </div>
</template>

<script setup lang="ts">
import { ElMessage } from 'element-plus'
import { useDark, useToggle } from '@vueuse/core'
import { onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { logout } from '/@/api/admin'
import { setLang, type LangKey } from '/@/lang/index'
import BaAccount from '/@/layouts/admin/components/baAccount.vue'
import { useAdminInfo } from '/@/stores/adminInfo'
import { useConfig } from '/@/stores/config'

/**
 * 顶栏右侧操作区（openspec add-admin-layout-shell）：
 * 站点主页外链 / 语言切换 / 布局配置 / 暗色开关 / 全屏 / 用户信息弹层（账号资料 + 退出登录）。
 * 对齐 buildadmin navMenus.vue + darkSwitch.vue + config 入口的精简合并形态。
 */
const router = useRouter()
const adminInfo = useAdminInfo()
const config = useConfig()
const { t } = useI18n()

const accountDrawerVisible = ref(false)

const onOpenProfile = () => {
    accountDrawerVisible.value = true
}

const isDark = useDark()
const toggleDark = useToggle(isDark)
// useDark 与布局取色共用 config.layout.isDark 数据源
const onToggleDark = () => {
    toggleDark()
    config.setLayout('isDark', !config.layout.isDark)
}

const state = reactive({
    isFullScreen: false,
})

const onFullScreenChange = () => {
    state.isFullScreen = !!document.fullscreenElement
}
onMounted(() => {
    document.addEventListener('fullscreenchange', onFullScreenChange)
})
onBeforeUnmount(() => {
    document.removeEventListener('fullscreenchange', onFullScreenChange)
})

const onFullScreen = () => {
    if (!document.fullscreenEnabled) {
        ElMessage.warning(t('layouts.fullscreenNotSupported'))
        return
    }
    if (document.fullscreenElement) {
        document.exitFullscreen()
    } else {
        document.documentElement.requestFullscreen()
    }
}

// 语言切换：config.lang + i18n 实例同步
const onSwitchLang = (name: string) => {
    config.setLang(name)
    void setLang(name as LangKey)
}

const loggingOut = ref(false)
const onLogout = async () => {
    if (loggingOut.value) return
    loggingOut.value = true
    try {
        await logout()
    } catch {
        // 登出接口幂等，网络失败也不阻塞本地清理
    }
    adminInfo.reset()
    loggingOut.value = false
    router.push('/admin/login')
}
</script>

<style scoped lang="scss">
.nav-menus {
    display: flex;
    align-items: center;
    height: 100%;

    .h100 {
        height: 100%;
        text-decoration: none;
    }

    .nav-menu-item {
        display: flex;
        align-items: center;
        justify-content: center;
        height: 100%;
        min-width: 40px;
        padding: 0 8px;
        cursor: pointer;
        border-radius: var(--el-border-radius-base);

        &:hover {
            background-color: v-bind('config.getColorVal("headerBarHoverBackground")');
        }
    }

    .admin-info {
        display: flex;
        align-items: center;
        gap: 6px;
        height: 100%;
        padding: 0 8px;
        cursor: pointer;
        border-radius: var(--el-border-radius-base);

        &:hover {
            background-color: v-bind('config.getColorVal("headerBarHoverBackground")');
        }

        .admin-name {
            font-size: var(--el-font-size-base);
            color: var(--el-text-color-primary);
        }
    }
}

:global(.admin-info-box) {
    .admin-info-base {
        display: flex;
        align-items: center;
        gap: 12px;
        padding: 4px 0 12px;

        .admin-info-name {
            font-weight: 600;
            margin-bottom: 4px;
        }
        .admin-info-lasttime {
            font-size: 12px;
            color: var(--el-text-color-secondary);
        }
    }
    .admin-info-footer {
        display: flex;
        justify-content: space-around;
        padding: 10px 0 2px;
        border-top: 1px solid var(--el-border-color-lighter);
    }
}
</style>
