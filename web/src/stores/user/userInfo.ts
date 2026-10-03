// src\stores\user\userInfo.ts
//
// C 端会员登录态。与 stores/adminInfo.ts 同构，仅字段与 cacheKey 不同：
// token 由登录接口签发（type="user"），后续 user API 调用时自动作为 Bearer 携带。
import { defineStore } from 'pinia'
import { USER_INFO } from '/@/stores/constant/cacheKey'

export interface UserInfoState {
    id: number
    username: string
    nickname: string
    avatar: string
    balance: string
    token: string
}

export const useUserInfo = defineStore('userInfo', {
    state: (): UserInfoState => {
        return {
            id: 0,
            username: '',
            nickname: '',
            avatar: '',
            balance: '0.00',
            token: '',
        }
    },
    actions: {
        setToken(token: string) {
            this.token = token
        },
        removeToken() {
            this.token = ''
        },
        /** 登出清空（显式列字段，理由同 adminInfo.reset）。 */
        reset() {
            this.removeToken()
            this.$patch({
                id: 0,
                username: '',
                nickname: '',
                avatar: '',
                balance: '0.00',
            })
        },
    },
    persist: {
        key: USER_INFO,
    },
})
