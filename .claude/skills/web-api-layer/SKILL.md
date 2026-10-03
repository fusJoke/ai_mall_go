---
name: web-api-layer
description: 在 web/ 下新增或修改接口调用（src/api/）时使用。规定文件位置、request 调用写法、DTO 类型、token 传递与提示选项。涉及接口联调、新增 API 函数、改请求参数、定义响应类型时加载。
---

# 前端 API 层规范

## 何时用

新增/修改 `web/src/api/` 下任何文件；前端要调一个新后端接口；改请求参数或响应类型。

## 定位规则

- **按身份分目录**：`src/api/user/`（C 端）、`src/api/supplier/`（商家端）、`src/api/admin/`（平台端）
- **按业务域拆文件**：`order.ts` / `blindbox.ts` / `product.ts`……不要把新接口堆进 `index.ts`
- 文件名小写；接口函数 camelCase 动词开头（`orderList` / `orderDetail` / `createOrder`）

## 文件骨架（照抄）

```ts
// src/api/user/order.ts — C 端订单查询
//
// 后端实现见 internal/handler/user/order.go；均需登录态。
import request from '/@/utils/request'
import { useUserInfo } from '/@/stores/user/userInfo'

export interface OrderListResponse {
    items: DrawOrder[]
    total: number
    page: number
    page_size: number
}

export function orderList(params: { page?: number; page_size?: number }) {
    const userInfo = useUserInfo()
    return request.request<OrderListResponse>({
        url: '/user/orders/list',
        method: 'GET',
        params,
        headers: { Authorization: `Bearer ${userInfo.token}` },
    })
}
```

## 硬规则

1. 用 `import request from '/@/utils/request'`——默认导出就是 axios 实例。**禁止**再 `axios.create()` 或自己包一层 fetch。
2. 调用一律 `request.request<T>({ url, method, params/data, headers })`（axios config 对象风格），与现有代码保持一致。不要混用 `request.get(url)` 简写风格。
3. 响应已被拦截器解包：后端统一返回 `{code, message, data}`，`code === 0` 时 `data` 直接作为返回值，`code !== 0` 自动弹错并 reject。因此泛型 `T` 写的是 **data 的形状**，不是外层信封。
4. **类型与函数同文件**：请求/响应的 `interface` 就写在该 api 文件里并导出，供组件 import。跨端共用类型才放 `web/types/`。禁止在 `.vue` 里重复定义接口类型。
5. token 从对应身份 store 取：`useUserInfo()` / `useSupplierInfo()` / `useAdminInfo()`，放 `Authorization: Bearer <token>`。
   > 更优做法是由请求拦截器按当前路由身份统一注入，避免每个函数重复取 token；当前保持与现有代码一致，改造需单独提案。
6. 提示行为用 `__opts`（直接挂在 config 上，模块声明合并已扩展 AxiosRequestConfig）：
   - `showErrorMessage: false` — 轮询/后台静默请求失败不弹窗，由调用方自行处理
   - `showSuccessMessage: true` — 保存类操作成功提示
   - `showLoading: true` + `loadingText` — 需要全屏 loading 的长请求
   - `dedup: true` — 可重复触发的查询（搜索输入等）自动取消前一次请求
7. URL 不带 `/api` 前缀——baseURL 已在 env 中配置，写 `/user/orders/list`。
8. **字段命名保持 snake_case 对齐后端 JSON**，前端不做驼峰转换（`order_no` / `page_size` / `snapshot_name`）。改字段名必须同步改后端 DTO。

## 收尾自检

- [ ] `pnpm -C web typecheck` 通过
- [ ] 新函数放在正确身份目录
- [ ] `.vue` 中没有手写 URL 字符串（一律走 api 函数）
- [ ] 静默/轮询请求都加了 `showErrorMessage: false`
