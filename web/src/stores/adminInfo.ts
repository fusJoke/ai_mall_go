// src\stores\adminInfo.ts
import { defineStore } from 'pinia'
import { ADMIN_INFO } from '/@/stores/constant/cacheKey'
import type { AdminInfo } from '/@/stores/interface'

export const useAdminInfo = defineStore('adminInfo', {
    state: (): AdminInfo => {
        return {
            id: 0,
            username: '',
            nickname: '',
            avatar: '',
            last_login_at: '',
            last_login_ip: '',
            token: '',
            super: false,
        }
    },
    actions: {
        /**
         * 状态批量填充
         * @param state 新状态数据
         * @param [exclude=true] 是否排除某些字段（忽略填充），默认值 true 排除 token，传递 false 则不排除，还可传递 string[] 指定排除字段列表
         */
        dataFill(state: Partial<AdminInfo>, exclude: boolean | string[] = true) {
            if (exclude === true) {
                exclude = ['token']
            } else if (exclude === false) {
                exclude = []
            }

            if (Array.isArray(exclude)) {
                exclude.forEach((item) => {
                    delete state[item as keyof AdminInfo]
                })
            }

            this.$patch(state)
        },
        setToken(token: string) {
            this.token = token
        },
        removeToken() {
            this.token = ''
        },
        /**
         * 登出时清空整个 store —— 不仅清 token，也清其他持久化字段。
         *
         * 为什么不调 Pinia 内置 `$reset()`：它依赖 state factory 的初始化结果，
         * 未来加新字段时容易遗漏。显式列字段更可控，且新字段加入时编辑器会立刻报红。
         *
         * 配合 pinia-plugin-persistedstate 使用：清完即触发 localStorage 写入，
         * 旧值不会再残留。
         */
        reset() {
            this.removeToken()
            this.$patch({
                id: 0,
                username: '',
                nickname: '',
                avatar: '',
                last_login_at: '',
                last_login_ip: '',
                super: false,
            })
        },
    },
    persist: {
        key: ADMIN_INFO,
    },
})
