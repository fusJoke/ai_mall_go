---
name: web-test
description: 为 web/ 前端补写测试、搭建 Vitest 环境时使用。当前项目无任何前端测试框架，本 skill 定义最小可用引入方案与测试优先级。写测试、改测试配置、在 CI 加前端测试步时加载。
---

# 前端测试（Vitest）

## 现状

`web/package.json` 目前**没有任何测试框架**，没有 `pnpm test`。前端质量只靠 `typecheck` + `lint` 兜底——这两者查不出逻辑错误。

## 最小引入（一次性）

```bash
pnpm -C web add -D vitest @vitest/coverage-v8 @vue/test-utils @testing-library/vue jsdom msw
```

- 用 **Vitest**（与 Vite 共享配置、零额外构建链路）；**不要装 Jest**（ESM/Vue3 配置成本高）
- `@testing-library/vue` 面向用户可见行为；`msw` 拦 HTTP，让测试不依赖后端

`vite.config.ts` 增加：

```ts
/// <reference types="vitest/config" />
test: {
    environment: 'jsdom',
    globals: true,
    setupFiles: ['./src/test/setup.ts'],
}
```

`package.json` scripts 增加：

```json
"test": "vitest",
"test:run": "vitest run",
"test:cov": "vitest run --coverage"
```

## 测试优先级（按性价比，别追覆盖率数字）

| 优先级 | 测什么 | 说明 |
| ------ | ------ | ---- |
| P0 | `src/utils/` 纯函数（request 拦截器逻辑、权限判断、格式化） | 无 DOM 依赖，最便宜，回归价值最高 |
| P0 | `src/stores/` 的 action（登录态、菜单、购物车） | 状态逻辑是 bug 高发区 |
| P1 | 关键业务组件（下单表单校验、拆单结果展示） | 用 Testing Library 查可见文本/role |
| P2 | 黄金路径 E2E（登录→下单→支付→出卡） | 交给 Playwright，独立一套，不进单测 |

**明确不做**：给每个页面写快照测试；断言组件内部 data/私有方法；为覆盖率数字写无意义用例。

## 硬规则

1. 查元素用 `getByRole` / `findByText`（用户视角），**禁止** `wrapper.vm.xxx` 断言内部状态。
2. **禁止 `setTimeout` 等异步**——用 `await findBy*` 或 `await flushPromises()`。
3. 网络一律走 `msw` mock，测试不许打真实后端。
4. 只测行为契约，不锁实现；重构实现时不应改测试。
5. CI 中跑 `pnpm -C web test:run`（`vitest` 不带 `run` 会 watch，挂住流水线）。
