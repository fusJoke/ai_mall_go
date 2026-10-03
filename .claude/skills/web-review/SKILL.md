---
name: web-review
description: 评审前端改动（.vue / .ts）时使用。提供针对本项目的前端评审清单：响应式正确性、三态、类型安全、请求层一致性、性能与内存、工程约定。做代码评审、提交前自查、写 PR 描述时加载。
---

# 前端评审清单

按顺序看，**前四节是高频事故区**。

## 1. 响应式正确性（最高频）

- [ ] 解构 props / store 是否丢了响应性？（Pinia 解构需 `storeToRefs`；props 解构需 `toRefs` 或直接 `props.x`）
- [ ] `reactive` 是否被整体替换过？解构 `reactive` 对象是否丢响应？
- [ ] 该用 `computed` 的地方是否误用了 `watch`？`watch` 的 `immediate`/`deep` 是否真有必要？

## 2. 三态与异常路径

- [ ] loading 是否在 `finally` 中复位（异常路径也要复位）？
- [ ] 空数据有 `el-empty` 吗？请求失败有提示/重试吗？
- [ ] 下单/支付/提交类操作，是否有防重复点击（disabled 或 debounce）？

## 3. 类型安全

- [ ] 有无 `any` / `as any` / `@ts-ignore`？（项目 eslint 启用 ts recommended）
- [ ] 接口类型是否来自 `src/api/` 而非在 `.vue` 里重复定义？
- [ ] `pnpm -C web typecheck` 是否通过？

## 4. 请求层一致性

- [ ] 是否走 `src/api/` 函数，而非在 `.vue` 里手写 url 或直接 axios？
- [ ] 静默请求（轮询、预检）是否加了 `__opts: { showErrorMessage: false }`？
- [ ] 可重复触发的查询是否用了 `dedup`？
- [ ] token 是否从对应身份 store 取？

## 5. 性能与内存

- [ ] `v-for` 是否有稳定 `:key`（可变列表禁用 index 作 key）？
- [ ] 大列表是否分页/虚拟滚动？大对象是否用 `shallowRef`？
- [ ] `addEventListener` / `setInterval` / `watch` 是否在 `onUnmounted` 清理？
- [ ] 弹窗内的大组件是否用 `v-if` 惰性渲染？

## 6. 工程约定

- [ ] 文件是否放对身份目录（user/supplier/admin）？组件 PascalCase、页面小写？
- [ ] 路由改动是否落在 `src/router/static/*Base.ts`，没散落在页面里？
- [ ] 文案是否走 i18n？样式是否 `scoped`？
- [ ] `pnpm -C web lint` 是否通过？

## 输出格式

每条问题写 `[P0-P3] 文件:行号 — 问题 — 建议`：

- **P0**：会导致线上事故（死循环、内存泄漏、下单/支付逻辑错）
- **P1**：明显 bug（响应式丢失、三态缺失、异常路径卡死）
- **P2**：规范偏离（any、手写 url、类型重复定义）
- **P3**：改进建议

没有问题就明确说"未发现问题"，**不要为凑数编问题**。
