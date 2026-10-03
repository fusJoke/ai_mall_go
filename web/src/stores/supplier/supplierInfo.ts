// src\stores\supplier\supplierInfo.ts
//
// B 端供应商登录态。与 stores/adminInfo.ts 同构：
// supplier_id 是供应商后台一切业务操作的归属 ID（后端从 token 身份推导，
// 前端仅用于展示）。
import { defineStore } from 'pinia'
import { SUPPLIER_INFO } from '/@/stores/constant/cacheKey'

export interface SupplierInfoState {
    id: number
    supplier_id: number
    username: string
    token: string
}

export const useSupplierInfo = defineStore('supplierInfo', {
    state: (): SupplierInfoState => {
        return {
            id: 0,
            supplier_id: 0,
            username: '',
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
        reset() {
            this.removeToken()
            this.$patch({
                id: 0,
                supplier_id: 0,
                username: '',
            })
        },
    },
    persist: {
        key: SUPPLIER_INFO,
    },
})
