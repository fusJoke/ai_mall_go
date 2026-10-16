import { createSSRApp } from 'vue'

import App from './App.vue'
import { assertProductionHygiene } from './config'

// 生产构建卫生门禁：USE_MOCK / VITE_DEV_TOKEN 一旦混进生产包就立刻抛出，
// 而不是静默跑假数据或把真实令牌内联进 JS（OCR 审查 P2-3 / P2-4）。
// 放在模块顶层而非 createApp 内，保证在挂载任何 UI 之前就失败。
assertProductionHygiene()

export function createApp() {
  const app = createSSRApp(App)
  return { app }
}
