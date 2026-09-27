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