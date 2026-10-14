import { defineConfig } from 'vite'
import uni from '@dcloudio/vite-plugin-uni'

export default defineConfig({
  plugins: [uni()],
  server: {
    port: 5176,
    // 端口必须固定：后端的 CORS 白名单按 origin 精确匹配（含端口），
    // 自动 +1 换端口会让所有非简单请求（POST）被 gin-contrib/cors 直接 403。
    // false = 端口被占用时直接报错退出，而不是静默漂移到 5177。
    strictPort: true,
    proxy: {
      // 开发期把 /api 前缀剥掉后转发到本地后端（默认 8080）
      '/api': {
        target: process.env.VITE_DEV_PROXY_TARGET || 'http://127.0.0.1:8080',
        changeOrigin: true,
        rewrite: (path) => path.replace(/^\/api/, ''),
      },
    },
  },
})
