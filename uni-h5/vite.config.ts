import { defineConfig, loadEnv } from 'vite'
import uni from '@dcloudio/vite-plugin-uni'

/**
 * 生产构建的「开发配置」门禁（OCR 审查 P2-3 / P2-4）。
 *
 * ## 为什么必须在构建期拦，而不是在产物里扫
 *
 * `import.meta.env.VITE_*` 会被 Vite 在构建期**静态内联**：源码里的
 * `import.meta.env.VITE_USE_MOCK === 'true'` 在产物里变成 `!0`，
 * `VITE_DEV_TOKEN` 变成一段裸字符串字面量。也就是说：
 *   - `USE_MOCK=true` 在产物里只表现为一个布尔字面量，事后按值 grep **认不出来**；
 *   - 令牌虽然形状特殊、事后能扫出来，但那时包已经产出，靠人记得跑扫描太脆。
 * 所以判定必须发生在「env 还有名字」的时刻，即这里。
 *
 * ## 实测事实（避免重新走一遍弯路）
 *
 * - 把 `VITE_DEV_TOKEN` 写进 `.env` 或 `.env.local` 后执行 `uni build`，
 *   令牌**确实**会以明文出现在 `dist/build/h5/assets/index-*.js` 里（已验证）；
 * - 只写在 `.env.development.local`（dev 模式专用）时，生产构建读不到它，产物干净（已验证）。
 *
 * 运行时还有第二道防线：`src/config/index.ts` 的 `assertProductionHygiene()`
 * 会在浏览器里抛错；`scripts/verify-prod-bundle.mjs` 是第三道（凭证形状扫描）。
 */
function assertProdBuildHygiene(mode: string, env: Record<string, string>): void {
  const problems: string[] = []

  if (env.VITE_USE_MOCK === 'true') {
    problems.push(
      '  · VITE_USE_MOCK=true —— 生产包会注册 mock 路由，登录返回假令牌、抽卡/订单全 401；\n' +
        '    表现为「界面显示已登录但功能半瘫」，且不会有人第一时间看出根因。'
    )
  }
  if (env.VITE_DEV_TOKEN) {
    problems.push(
      '  · VITE_DEV_TOKEN 非空 —— 它是能直接读写真实用户数据的令牌，会被明文内联进产物 JS，\n' +
        '    任何访客 view-source 即可取走并调用需鉴权接口。'
    )
  }

  if (problems.length === 0) return

  throw new Error(
    `[uni-h5] 生产构建被拒绝（mode=${mode}）：检测到开发期配置。\n` +
      problems.join('\n') +
      '\n' +
      '  正确做法：把这两项放进 `.env.development.local`（仅 dev 加载、已 gitignore），\n' +
      '  不要放进 `.env` 或 `.env.local`（Vite 在 production 构建里同样会加载它们）。\n' +
      '  说明见 README「环境文件放哪里」。\n' +
      '\n' +
      '  ⚠ 若这次「被拒绝」之前，同一枚 VITE_DEV_TOKEN 曾经成功产出过任何包\n' +
      '    （本地 dist、CI artifact、已上线的静态站都算），该令牌必须视为**已泄漏**，\n' +
      '    请立刻在服务端轮换，仅从 env 里删掉它是不够的。'
  )
}

export default defineConfig(({ command, mode }) => {
  // loadEnv 第三参传 'VITE_' 才会把带 VITE_ 前缀的变量读进来（默认只读无前缀的）。
  const env = loadEnv(mode, process.cwd(), 'VITE_')

  if (command === 'build') {
    assertProdBuildHygiene(mode, env)
  }

  return {
    plugins: [uni()],
    server: {
      port: 5176,
      // 端口必须固定：后端的 CORS 白名单按 origin 精确匹配（含端口），
      // 自动 +1 换端口会让所有非简单请求（POST）被 gin-contrib/cors 直接 403。
      // false = 端口被占用时直接报错退出，而不是静默漂移到 5177。
      strictPort: true,
      proxy: {
        // 开发期把 /api 前缀剥掉后转发到本地后端（默认 8080）。
        //
        // 注意这里是「上下文 -> 选项」的映射：`'/api'` 这层键不能省，
        // 否则选项对象本身会被当成 map 迭代，Vite 会拿 `changeOrigin: true`
        // 去 createProxyServer(true)，报
        // `TypeError: Cannot create property 'prependPath' on boolean 'true'`。
        //
        // 用 loadEnv 而不是 process.env：Vite 只把**不含**前缀的变量写进
        // process.env，而 `VITE_DEV_PROXY_TARGET` 带前缀，因此原先的
        // `process.env.VITE_DEV_PROXY_TARGET` 永远读不到 .env 文件里的值
        // （只有 shell 显式导出时才生效）—— 即该配置项此前是个静默 no-op。
        '/api': {
          target: env.VITE_DEV_PROXY_TARGET || 'http://127.0.0.1:8080',
          changeOrigin: true,
          rewrite: (path) => path.replace(/^\/api/, ''),
        },
      },
    },
  }
})
