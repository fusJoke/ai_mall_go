// src\stores\interface\index.ts

import type { RouteRecordRaw } from 'vue-router'

/**
 * 管理员信息
 */
export interface AdminInfo {
    id: number
    username: string
    nickname: string
    avatar: string
    last_login_at: string
    last_login_ip: string
    token: string
    // 是否是 superAdmin（用于判定是否显示超管级按钮，不做任何权限判断）
    super: boolean
}

/**
 * 管理员登录请求体 —— 与后端 internal/handler/admin/admin.go LoginRequest 对齐。
 *
 * 点选验证码集成（click-captcha change）后，登录链路为：
 *   1. 前端弹窗组件预检通过 → 拿到 captchaKey
 *   2. 预检的同时也拿到用户点击的 points（图片原始 350×200 坐标）
 *   3. login() 同时携带 captchaKey + points；后端做 consume 二次校验
 *
 * 缺 captchaKey/points 会触发 400 login.invalid_input。
 */
export interface LoginRequest {
    username: string
    password: string
    captcha_key: string
    points: { x: number; y: number }[]
    remember: boolean
}

/**
 * 管理员登录响应 —— 与后端 internal/handler/admin/admin.go LoginResponse 对齐。
 */
export interface LoginResponse {
    admin: {
        id: number
        username: string
        nickname: string
        avatar: string
        email?: string
        mobile?: string
        last_login_at?: string
        last_login_ip?: string
        bio?: string
        status?: number
    }
    token: string
}

/**
 * 管理员登出响应 —— 与后端 internal/handler/admin/admin.go Logout 信封对齐。
 *
 * 后端 Logout 始终返回 `{code, message}` 信封：
 * - message="ok" 表示 token 被软删除（有效 / 已过期统一处理）
 * - message="no active session" 表示 header 缺失 / 格式错误 / token 不存在
 *
 * 调用方一般不消费 message 文案，登出失败由 axios 拦截器静默处理。
 */
export interface LogoutResponse {
    code: string
    message: string
}

/**
 * CRUD 列表模块配置
 */
export interface Crud {
    syncType: 'manual' | 'auto'
    syncedUpdate: 'yes' | 'no'
    syncAutoPublic: 'yes' | 'no'
}

/**
 * 多语言配置
 */
export interface Lang {
    defaultLang: string
    fallbackLang: string
    langArray: { name: string; value: string }[]
}

/**
 * 布局配置（颜色对：[亮色, 暗色]）
 */
export interface Layout {
    // 全局
    showDrawer: boolean
    shrink: boolean
    layoutMode: string
    mainAnimation: string
    isDark: boolean

    // 菜单栏
    menuBackground: string[]
    menuColor: string[]
    menuActiveBackground: string[]
    menuActiveColor: string[]
    menuHoverBackground: string[]
    menuWidth: number
    menuDefaultIcon: string
    menuCollapse: boolean
    menuUniqueOpened: boolean
    menuShowTopBar: boolean
    menuTopBarBackground: string[]
    menuTopBarColor: string[]
    menuTopBarCenter: boolean
    menuTopBarLogo: boolean
    menuToolBarAutoHide: boolean
    menuToolBarColor: string[]
    menuToolBarHoverColor: string[]
    menuToolBarHoverBackground: string[]

    // 主菜单栏额外配置（部分布局存在主次两个菜单栏）
    menuBackgroundPrimary: string[]
    menuActiveBackgroundPrimary: string[]

    // 左分布局独有
    menuWidthLeftSplit: number
    menuHoverBackgroundLeftSplit: string[]

    // 顶栏
    headerBarTabColor: string[]
    headerBarTabActiveColor: string[]
    headerBarBackground: string[]
    headerBarHoverBackground: string[]
    headerBarTabActiveBackground: string[]
    headerBarTabActiveBackgroundFloating: string[]

    // 布局漫游式引导
    layoutTour: boolean
    layoutTourUnfinished: boolean
}

/**
 * 菜单数据
 */
export interface Menu {
    rawData: RouteRecordRaw[]
    children: RouteRecordRaw[]
    authNode: Map<string, string[]>
}

/**
 * 站点基础配置 —— 与后端 `GET /admin/init` 响应里的 `site_config` 字段对齐。
 *
 * 字段名严格 snake_case 以匹配后端 JSON tag；缺失配置对应空字符串而非 undefined。
 */
export interface SiteConfig {
    name: string
    record_number: string
    version: string
}

/**
 * 菜单规则 —— 与后端 `internal/model/admin.go` 的 `AdminRule` 对齐，
 * 仅列出 init 响应会用到的字段。
 *
 * - `type` 后端返回字符串（dir/menu/node），前端按值路由：
 *   - `dir`：规则目录，作为菜单分组，不注册为路由
 *   - `menu`：菜单项，注册为路由
 *   - `node`：纯权限节点，仅展示，不注册为路由
 * - `open_type` 可空 —— 后端 nil 不写字段。
 */
export interface MenuRule {
    id: number
    pid: number
    type: 'dir' | 'menu' | 'node'
    title: string
    name: string
    path: string
    icon: string
    open_type?: 'tab' | 'link' | 'iframe'
    url: string
    component: string
    keepalive: boolean
    extend: string
    weigh: number
}

/**
 * 后台初始化响应 —— 与后端 `internal/service/admin/init.go` 的 InitResponse 对齐。
 *
 * 字段含义：
 *   - `admin`：当前登录管理员基本信息 + 是否超管（决定前端 UI 是否展示超管按钮）
 *   - `site_config`：站点基础配置三项（site name / record number / version）
 *   - `menus`：当前管理员持有的菜单规则全集（含 dir / menu / node 三种类型）
 */
export interface InitResponse {
    admin: AdminInfo
    site_config: SiteConfig
    menus: MenuRule[]
}