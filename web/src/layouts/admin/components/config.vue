<template>
    <el-drawer
        :model-value="config.layout.showDrawer"
        :title="t('layouts.layoutConfiguration')"
        size="410px"
        :append-to-body="true"
        @close="config.setLayout('showDrawer', false)"
    >
        <div class="layout-config">
            <!-- 布局模式 -->
            <div class="config-block">
                <div class="config-block-title">{{ t('layouts.layoutMode') }}</div>
                <div class="layout-mode-list">
                    <div
                        v-for="mode in layoutModes"
                        :key="mode.value"
                        class="layout-mode-item"
                        :class="{ active: config.layout.layoutMode == mode.value }"
                        @click="onSelectMode(mode.value)"
                    >
                        <div class="mode-thumb">
                            <span v-if="mode.thumb.side" class="t-side"></span>
                            <span v-if="mode.thumb.side2" class="t-side2"></span>
                            <div class="t-right">
                                <span v-if="mode.thumb.header" class="t-header"></span>
                                <span class="t-main"></span>
                            </div>
                        </div>
                        <div class="mode-name">{{ mode.label }}</div>
                    </div>
                </div>
            </div>

            <!-- 全局 -->
            <div class="config-block">
                <div class="config-block-title">{{ t('layouts.overallSituation') }}</div>
                <div class="config-row">
                    <span>{{ t('layouts.darkMode') }}</span>
                    <el-switch :model-value="config.layout.isDark" @change="onToggleDark" />
                </div>
                <div class="config-row">
                    <span>{{ t('layouts.backgroundPageSwitchingAnimation') }}</span>
                    <el-select :model-value="config.layout.mainAnimation" style="width: 150px" @change="onSetLayout('mainAnimation', $event)">
                        <el-option v-for="name in animationNames" :key="name" :label="name" :value="name" />
                    </el-select>
                </div>
            </div>

            <!-- 侧边菜单栏 -->
            <div class="config-block">
                <div class="config-block-title">{{ t('layouts.sidebar') }}</div>
                <div class="config-row">
                    <span>{{ t('layouts.sideMenuBarBackgroundColor') }}</span>
                    <el-color-picker :model-value="colorVal('menuBackground')" @change="onColorChange('menuBackground', $event)" />
                </div>
                <div class="config-row">
                    <span>{{ t('layouts.sideMenuTextColor') }}</span>
                    <el-color-picker :model-value="colorVal('menuColor')" @change="onColorChange('menuColor', $event)" />
                </div>
                <div class="config-row">
                    <span>{{ t('layouts.sideMenuActiveItemBackgroundColor') }}</span>
                    <el-color-picker :model-value="colorVal('menuActiveBackground')" @change="onColorChange('menuActiveBackground', $event)" />
                </div>
                <div class="config-row">
                    <span>{{ t('layouts.sideMenuActiveItemTextColor') }}</span>
                    <el-color-picker :model-value="colorVal('menuActiveColor')" @change="onColorChange('menuActiveColor', $event)" />
                </div>
                <div class="config-row">
                    <span>{{ t('layouts.sideMenuHoverBackgroundColor') }}</span>
                    <el-color-picker :model-value="colorVal('menuHoverBackground')" @change="onColorChange('menuHoverBackground', $event)" />
                </div>
                <div class="config-row">
                    <span>{{ t('layouts.sideMenuWidth') }}</span>
                    <el-slider
                        :model-value="config.layout.menuWidth"
                        :min="160"
                        :max="320"
                        :step="10"
                        style="width: 150px"
                        @update:model-value="onSetLayout('menuWidth', $event as number)"
                    />
                </div>
                <div class="config-row">
                    <span>{{ t('layouts.sideMenuHorizontalCollapse') }}</span>
                    <el-switch :model-value="config.layout.menuCollapse" @change="onSetLayout('menuCollapse', $event as boolean)" />
                </div>
                <div class="config-row">
                    <span>{{ t('layouts.sideMenuAccordion') }}</span>
                    <el-switch :model-value="config.layout.menuUniqueOpened" @change="onSetLayout('menuUniqueOpened', $event as boolean)" />
                </div>

                <template v-if="config.layout.layoutMode == 'LeftSplit'">
                    <div class="config-subtitle">{{ t('layouts.mainMenu') }}</div>
                    <div class="config-row">
                        <span>{{ t('layouts.primaryMenuBackgroundColor') }}</span>
                        <el-color-picker :model-value="colorVal('menuBackgroundPrimary')" @change="onColorChange('menuBackgroundPrimary', $event)" />
                    </div>
                    <div class="config-row">
                        <span>{{ t('layouts.primaryMenuActiveItemBackgroundColor') }}</span>
                        <el-color-picker
                            :model-value="colorVal('menuActiveBackgroundPrimary')"
                            @change="onColorChange('menuActiveBackgroundPrimary', $event)"
                        />
                    </div>
                    <div class="config-row">
                        <span>{{ t('layouts.primaryMenuHoverBackgroundColor') }}</span>
                        <el-color-picker
                            :model-value="colorVal('menuHoverBackgroundLeftSplit')"
                            @change="onColorChange('menuHoverBackgroundLeftSplit', $event)"
                        />
                    </div>
                    <div class="config-row">
                        <span>{{ t('layouts.leftSplitSideMenuWidth') }}</span>
                        <el-slider
                            :model-value="config.layout.menuWidthLeftSplit"
                            :min="140"
                            :max="260"
                            :step="10"
                            style="width: 150px"
                            @update:model-value="onSetLayout('menuWidthLeftSplit', $event as number)"
                        />
                    </div>
                </template>
            </div>

            <!-- 侧边栏的顶部与底部 -->
            <div class="config-block">
                <div class="config-block-title">{{ t('layouts.sidebarTopAndBottom') }}</div>
                <div class="config-row">
                    <span>{{ t('layouts.showSideMenuTopBar') }}</span>
                    <el-switch :model-value="config.layout.menuShowTopBar" @change="onSetLayout('menuShowTopBar', $event as boolean)" />
                </div>
                <template v-if="config.layout.menuShowTopBar">
                    <div class="config-row">
                        <span>{{ t('layouts.sideMenuTopBarBackgroundColor') }}</span>
                        <el-color-picker :model-value="colorVal('menuTopBarBackground')" @change="onColorChange('menuTopBarBackground', $event)" />
                    </div>
                    <div class="config-row">
                        <span>{{ t('layouts.sideMenuTopBarTextColor') }}</span>
                        <el-color-picker :model-value="colorVal('menuTopBarColor')" @change="onColorChange('menuTopBarColor', $event)" />
                    </div>
                    <div class="config-row">
                        <span>{{ t('layouts.sideMenuTopBarCenterContent') }}</span>
                        <el-switch :model-value="config.layout.menuTopBarCenter" @change="onSetLayout('menuTopBarCenter', $event as boolean)" />
                    </div>
                    <div class="config-row">
                        <span>{{ t('layouts.sideMenuTopBarDisplayLogo') }}</span>
                        <el-switch :model-value="config.layout.menuTopBarLogo" @change="onSetLayout('menuTopBarLogo', $event as boolean)" />
                    </div>
                </template>
            </div>

            <!-- 顶栏 -->
            <div class="config-block">
                <div class="config-block-title">{{ t('layouts.topBar') }}</div>
                <div class="config-row">
                    <span>{{ t('layouts.topBarBackgroundColor') }}</span>
                    <el-color-picker :model-value="colorVal('headerBarBackground')" @change="onColorChange('headerBarBackground', $event)" />
                </div>
                <div class="config-row">
                    <span>{{ t('layouts.topBarTextColor') }}</span>
                    <el-color-picker :model-value="colorVal('headerBarTabColor')" @change="onColorChange('headerBarTabColor', $event)" />
                </div>
                <div class="config-row">
                    <span>{{ t('layouts.topBarHoverBackgroundColor') }}</span>
                    <el-color-picker
                        :model-value="colorVal('headerBarHoverBackground')"
                        @change="onColorChange('headerBarHoverBackground', $event)"
                    />
                </div>
                <div class="config-row">
                    <span>{{ t('layouts.topBarMenuActiveItemTextColor') }}</span>
                    <el-color-picker :model-value="colorVal('headerBarTabActiveColor')" @change="onColorChange('headerBarTabActiveColor', $event)" />
                </div>
                <div class="config-row">
                    <span>{{ t('layouts.topBarMenuActiveItemBackgroundColor') }}</span>
                    <el-color-picker
                        :model-value="colorVal('headerBarTabActiveBackground')"
                        @change="onColorChange('headerBarTabActiveBackground', $event)"
                    />
                </div>
                <div class="config-row">
                    <span>{{ t('layouts.topBarMenuActiveItemBackgroundFloatingColor') }}</span>
                    <el-color-picker
                        :model-value="colorVal('headerBarTabActiveBackgroundFloating')"
                        @change="onColorChange('headerBarTabActiveBackgroundFloating', $event)"
                    />
                </div>
            </div>

            <!-- 恢复默认 -->
            <div class="config-footer">
                <el-button type="danger" plain @click="onRestoreDefault">{{ t('layouts.restoreDefault') }}</el-button>
            </div>
        </div>
    </el-drawer>
</template>

<script setup lang="ts">
import { ElMessageBox } from 'element-plus'
import { useI18n } from 'vue-i18n'
import type { Layout } from '/@/stores/interface'
import { useConfig } from '/@/stores/config'

/**
 * 布局设置抽屉（对齐 buildadmin backend/components/config.vue 的精简移植）：
 * 布局模式切换（5 容器）、暗黑模式、页面切换动画、侧边菜单栏 / 顶栏配色与开关项。
 * 全部状态写入 useConfig（pinia persist 自动持久化），布局组件响应式联动。
 */
const { t } = useI18n()
const config = useConfig()

const layoutModes = [
    { value: 'Default', label: t('layouts.default'), thumb: { side: true, side2: false, header: true } },
    { value: 'Classic', label: t('layouts.classic'), thumb: { side: true, side2: false, header: true } },
    { value: 'Streamline', label: t('layouts.singleColumn'), thumb: { side: false, side2: false, header: true } },
    { value: 'Double', label: t('layouts.doubleColumn'), thumb: { side: true, side2: false, header: true } },
    { value: 'LeftSplit', label: t('layouts.leftSplit'), thumb: { side: true, side2: true, header: true } },
]

const animationNames = ['slide-right', 'slide-left', 'slide-top', 'slide-bottom', 'fade']

const onSelectMode = (mode: string) => {
    config.setLayoutMode(mode)
}

const onSetLayout = (name: keyof Layout, value: unknown) => {
    config.setLayout(name, value as never)
}

/**
 * 暗黑切换：与 navMenus 的暗黑开关共用 config.layout.isDark 数据源
 */
const onToggleDark = (val: string | number | boolean) => {
    config.setLayout('isDark', Boolean(val))
    const root = document.documentElement
    root.classList.toggle('dark', Boolean(val))
}

/**
 * 取当前主题（亮/暗）下实际生效的颜色
 */
const colorVal = (name: keyof Layout): string => {
    return config.getColorVal(name)
}

/**
 * 颜色变更：写入当前主题对应的色位（亮色写 [0]，暗色写 [1]），另一侧保持不变
 */
const onColorChange = (name: keyof Layout, val: string | null) => {
    if (!val) return
    const colors = config.layout[name] as string[]
    const pair = [...colors]
    if (config.layout.isDark) {
        pair[1] = val
    } else {
        pair[0] = val
    }
    config.setLayout(name, pair)
}

/**
 * 恢复默认配置（对齐 config store 的初始值）
 */
const onRestoreDefault = async () => {
    try {
        await ElMessageBox.confirm(t('layouts.restoreConfigConfirm'), '', { type: 'warning' })
    } catch {
        return
    }
    const defaults: Partial<Layout> = {
        layoutMode: 'Default',
        mainAnimation: 'slide-right',
        menuBackground: ['#ffffff', '#1d1e1f'],
        menuColor: ['#303133', '#CFD3DC'],
        menuActiveBackground: ['#ffffff', '#1d1e1f'],
        menuActiveColor: ['#409eff', '#3375b9'],
        menuHoverBackground: ['#ecf5ff', '#18222c'],
        menuWidth: 260,
        menuCollapse: false,
        menuUniqueOpened: false,
        menuShowTopBar: true,
        menuTopBarBackground: ['#fcfcfc', '#1d1e1f'],
        menuTopBarColor: ['#409eff', '#3375b9'],
        menuTopBarCenter: false,
        menuTopBarLogo: false,
        menuBackgroundPrimary: ['#f5f5f5', '#18222c'],
        menuActiveBackgroundPrimary: ['#c6e2ff', '#1d1e1f'],
        menuWidthLeftSplit: 180,
        menuHoverBackgroundLeftSplit: ['#ebebeb', '#213d5b'],
        headerBarTabColor: ['#000000', '#CFD3DC'],
        headerBarTabActiveColor: ['#000000', '#409EFF'],
        headerBarBackground: ['#ffffff', '#1d1e1f'],
        headerBarHoverBackground: ['#f5f5f5', '#18222c'],
        headerBarTabActiveBackground: ['#f5f5f5', '#141414'],
        headerBarTabActiveBackgroundFloating: ['#ffffff', '#1d1e1f'],
    }
    for (const key in defaults) {
        config.setLayout(key as keyof Layout, (defaults as Record<string, unknown>)[key])
    }
}
</script>

<style scoped lang="scss">
.layout-config {
    padding: 0 4px;
}
.config-block {
    margin-bottom: 20px;
}
.config-block-title {
    font-weight: 600;
    margin-bottom: 12px;
}
.config-subtitle {
    font-weight: 600;
    font-size: 13px;
    color: var(--el-text-color-secondary);
    margin: 12px 0 8px;
}
.config-row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 7px 0;
    font-size: 13px;
    color: var(--el-text-color-regular);
}
.config-footer {
    display: flex;
    justify-content: center;
    padding: 10px 0 20px;
}

.layout-mode-list {
    display: flex;
    flex-wrap: wrap;
    gap: 10px;
}
.layout-mode-item {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 6px;
    padding: 8px;
    border: 1px solid var(--el-border-color-lighter);
    border-radius: var(--el-border-radius-base);
    cursor: pointer;
    transition: all 0.2s;

    &:hover {
        border-color: var(--el-color-primary-light-5);
    }
    &.active {
        border-color: var(--el-color-primary);
        box-shadow: 0 0 0 1px var(--el-color-primary);
    }
}
.mode-name {
    font-size: 12px;
    color: var(--el-text-color-regular);
}

.mode-thumb {
    display: flex;
    width: 56px;
    height: 40px;
    padding: 4px;
    gap: 3px;
    background: var(--el-fill-color-light);

    .t-side {
        width: 10px;
        height: 100%;
        background: var(--el-color-primary-light-7);
        border-radius: 2px;
    }
    .t-side2 {
        width: 6px;
        height: 100%;
        background: var(--el-color-primary-light-5);
        border-radius: 2px;
    }
    .t-right {
        display: flex;
        flex-direction: column;
        flex: 1;
        gap: 3px;

        .t-header {
            height: 8px;
            background: var(--el-color-primary-light-3);
            border-radius: 2px;
        }
        .t-main {
            flex: 1;
            background: var(--el-fill-color-darker);
            border-radius: 2px;
        }
    }
}
</style>
