// src\lang\index.ts
// 实现了语言包懒加载、子目录语言包加载、当前语言动态修改函数
import { merge, set } from 'lodash-es'
import type { App } from 'vue'
import { createI18n } from 'vue-i18n'
import { parse as parseYaml } from 'yaml'
import { useConfig } from '/@/stores/config'

/**
 * 支持的语言类型
 */
export type LangKey = 'zh-cn' | 'en'

/**
 * 支持的语言列表
 */
export const langs: LangKey[] = ['zh-cn', 'en']

/**
 * 语言显示名称
 */
export const langNames: Record<LangKey, string> = {
    en: 'English',
    'zh-cn': '简体中文',
}

/**
 * i18n 实例
 */
const i18n = createI18n({
    legacy: false,
    locale: 'zh-cn',
    fallbackLocale: 'zh-cn',
    messages: {},
})

// 使用 vite import.meta.glob 批量导入 lang 目录下所有 .yaml 文件（包括子目录）
// 以 raw 文本形式读取，再交给 yaml.parse 解析为 JS 对象
const langGlobs: Record<LangKey, Record<string, () => Promise<string>>> = {
    en: import.meta.glob('./en/**/*.yaml', { as: 'raw' }) as Record<string, () => Promise<string>>,
    'zh-cn': import.meta.glob('./zh-cn/**/*.yaml', { as: 'raw' }) as Record<string, () => Promise<string>>,
}

/**
 * 设置 i18n，并为 vue 安装 i18n 插件
 */
export async function setupI18n(app: App): Promise<void> {
    const config = useConfig()
    i18n.global.fallbackLocale.value = config.lang.fallbackLang

    // 初始化当前语言包
    await setLang(config.lang.defaultLang as LangKey)

    app.use(i18n)
}

/**
 * 设置语言
 * @param lang 语言标识
 */
export async function setLang(lang: LangKey): Promise<void> {
    await loadMessages(lang)

    const config = useConfig()
    i18n.global.locale.value = lang
    config.setLang(lang)
}

/**
 * 懒加载语言包
 * @param lang 语言标识
 */
export async function loadMessages(lang: LangKey): Promise<void> {
    // 如果已加载则跳过
    if (i18n.global.availableLocales.includes(lang)) {
        return
    }

    try {
        // 批量加载 lang 目录下所有 .yaml 文件
        const glob = langGlobs[lang]
        const promises = Object.entries(glob).map(async ([path, loader]) => {
            const raw = await loader()
            const parsed = parseYaml(raw) ?? {}
            return { path, default: parsed }
        })
        const modules = await Promise.all(promises)

        // 按文件路径构建嵌套的 messages 结构
        const mergedMessages: Record<string, any> = {}
        for (const { path, default: moduleData } of modules) {
            if (typeof moduleData !== 'object' || moduleData === null) {
                continue
            }
            const keys = filePathToKeys(lang, path)
            if (keys.length === 0) {
                // 合并到顶层
                merge(mergedMessages, moduleData)
            } else {
                // 子模块 — 按路径嵌套
                merge(mergedMessages, set({}, keys, moduleData))
            }
        }

        i18n.global.setLocaleMessage(lang, mergedMessages)
    } catch (error) {
        console.error(`Failed to load lang: ${lang}`, error)
    }
}

const filePathToKeys = (lang: LangKey, path: string) => {
    const langPathPrefix = `/${lang}`
    const pathName = path.slice(path.lastIndexOf(langPathPrefix) + (langPathPrefix.length + 1), path.lastIndexOf('.'))
    const keys = pathName.split('/')

    // index.yaml 作为顶层，其余按目录层级拆分
    if (keys.length === 1 && keys[0] === 'index') {
        return []
    }
    return keys
}

export default i18n
