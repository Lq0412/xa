import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// 开发服务器把 API 转发给本地 Go server，页面无需处理跨域配置。
export default defineConfig({
  plugins: [react()],
  server: {
    port: 5173,
    proxy: {
      '/api': 'http://127.0.0.1:18080',
      '/health': 'http://127.0.0.1:18080',
    },
  },
})
