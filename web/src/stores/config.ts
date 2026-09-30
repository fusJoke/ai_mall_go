import { defineStore } from 'pinia'
import { reactive, ref } from 'vue'
import { STORE_CONFIG } from '/@/stores/constant/cacheKey'
import type { Crud, Lang, Layout, SiteConfig } from '/@/stores/interface'
import { useNavTabs } from '/@/stores/navTabs'

export const useConfig = defineStore(
    'config',
    () => {
        const layout: Layout = reactive({
            // 全局
            showDrawer: false,
            shrink: false,
            layoutMode: 'Default',
            mainAnimation: 'slide-right',
            isDark: false,

            // 菜单栏
            menuBackground: ['#ffffff', '#1d1e1f'],
            menuColor: ['#303133', '#CFD3DC'],
            menuActiveBackground: ['#ffffff', '#1d1e1f'],
            menuActiveColor: ['#409eff', '#3375b9'],
            menuHoverBackground: ['#ecf5ff', '#18222c'],
            menuWidth: 260,
            menuDefaultIcon: 'fa fa-circle-o',
            menuCollapse: false,
            menuUniqueOpened: false,
            menuShowTopBar: true,
            menuTopBarBackground: ['#fcfcfc', '#1d1e1f'],
            menuTopBarColor: ['#409eff', '#3375b9'],
            menuTopBarCenter: false,
            menuTopBarLogo: false,
            menuToolBarAutoHide: false,
            menuToolBarColor: ['#303133', '#CFD3DC'],
            menuToolBarHoverColor: ['#409eff', '#CFD3DC'],
            menuToolBarHoverBackground: ['#ecf5ff', '#18222c'],

            // 主菜单栏额外配置项（一些布局存在主次两个菜单栏）
            menuBackgroundPrimary: ['#f5f5f5', '#18222c'],
            menuActiveBackgroundPrimary: ['#c6e2ff', '#1d1e1f'],

            // 左分布局独有菜单栏配置
            menuWidthLeftSplit: 180,
            menuHoverBackgroundLeftSplit: ['#ebebeb', '#213d5b'],

            // 顶栏
            headerBarTabColor: ['#000000', '#CFD3DC'],
            headerBarTabActiveColor: ['#000000', '#409EFF'],
            headerBarBackground: ['#ffffff', '#1d1e1f'],
            headerBarHoverBackground: ['#f5f5f5', '#18222c'],
            headerBarTabActiveBackground: ['#f5f5f5', '#141414'],
            headerBarTabActiveBackgroundFloating: ['#ffffff', '#1d1e1f'],

            // tour
            // 布局漫游式引导
            layoutTour: false,
            layoutTourUnfinished: true,
        })

        const lang: Lang = reactive({
            defaultLang: 'zh-cn',
            fallbackLang: 'zh-cn',
            langArray: [
                { name: 'zh-cn', value: '中文简体' },
                { name: 'en', value: 'English' },
            ],
        })

        const crud: Crud = reactive({
            syncType: 'manual',
            syncedUpdate: 'yes',
            syncAutoPublic: 'no',
        })

        function menuWidth() {
            // 菜单折叠时基本宽度
            const menuCollapseBaseWidth = 64

            // 左分布局特有
            if (layout.layoutMode == 'LeftSplit') {
                const navTabs = useNavTabs()

                // 本布局带来的额外菜单宽度，主菜单宽度 80 + 次级菜单的左右内边距 16
                const modeMenuWidth = 96
                // 最终菜单宽度
                let leftSplitMenuWidth = layout.menuCollapse
                    ? menuCollapseBaseWidth + modeMenuWidth
                    : parseInt(layout.menuWidthLeftSplit.toString()) + modeMenuWidth

                // 无次级菜单，固定宽度
                if (!navTabs.state.childrenMenus.length) {
                    leftSplitMenuWidth = 80
                }

                // 小屏模式
                if (layout.shrink) {
                    return layout.menuCollapse ? 0 : `${leftSplitMenuWidth}px`
                }

                return `${leftSplitMenuWidth}px`
            }

            // 小屏模式
            if (layout.shrink) {
                return layout.menuCollapse ? 0 : `${layout.menuWidth}px`
            }

            // 菜单是否折叠
            return layout.menuCollapse ? `${menuCollapseBaseWidth}px` : `${layout.menuWidth}px`
        }

        function setLang(val: string) {
            lang.defaultLang = val
        }

        function setLayoutMode(data: string) {
            layout.layoutMode = data
        }

        const setLayout = (name: keyof Layout, value: any) => {
            ;(layout[name] as any) = value
        }

        const getColorVal = function (name: keyof Layout): string {
            const colors = layout[name] as string[]
            if (layout.isDark) {
                return colors[1]
            } else {
                return colors[0]
            }
        }

        const setCrud = (name: keyof Crud, value: any) => {
            ;(crud[name] as any) = value
        }

        // 站点基础配置（来自后端 `GET /admin/init` 响应 site_config 字段）。
        // 用 reactive 包裹便于单独 watch；调用方通过 setSiteConfig 覆盖。
        const siteConfig = reactive<SiteConfig>({
            name: '',
            record_number: '',
            version: '',
        })

        // CDN 配置（来自 config.yaml 中的 server.cdn_url / server.cdn_url_params）。
        // 与 siteConfig 平级，单独 ref 暴露 —— fullURL 等 util 需要在 store 内部访问。
        const cdnUrl = ref<string>('')
        const cdnUrlParams = ref<string>('')

        const setSiteConfig = (data: Partial<SiteConfig>) => {
            Object.assign(siteConfig, data)
        }

        // setCDNConfig 由启动期 / 配置同步入口调用（当前直接读 yaml 注入；
        // 后续若改为后端下发，可在此集中入口覆盖）。
        const setCDNConfig = (url: string, params: string) => {
            cdnUrl.value = url
            cdnUrlParams.value = params
        }

        return {
            layout,
            lang,
            crud,
            siteConfig,
            cdnUrl,
            cdnUrlParams,
            menuWidth,
            setLang,
            setLayoutMode,
            setLayout,
            getColorVal,
            setCrud,
            setSiteConfig,
            setCDNConfig,
        }
    },
    {
        persist: {
            key: STORE_CONFIG,
        },
    }
)
