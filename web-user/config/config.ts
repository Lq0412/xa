import { defineConfig } from '@umijs/max';

// 用户端独立构建和部署，只通过 HTTP 与公共 Go 后端交互。
export default defineConfig({
  title: 'xa · Agent',
  hash: true,
  mfsu: false,
  // Umi 的压缩隔离选项防止异步页面的 esbuild 辅助函数互相覆盖。
  esbuildMinifyIIFE: true,
  model: {},
  initialState: {},
  antd: {
    appConfig: {},
    configProvider: {
      theme: {
        token: {
          colorPrimary: '#262626',
          borderRadius: 10,
          fontFamily:
            '-apple-system, BlinkMacSystemFont, "Segoe UI", "PingFang SC", "Microsoft YaHei", sans-serif',
        },
      },
    },
  },
  locale: { default: 'zh-CN', antd: true, baseNavigator: false },
  mock: false,
  define: {
    'process.env.XA_ADMIN_WEB_URL':
      process.env.XA_ADMIN_WEB_URL ||
      (process.env.NODE_ENV === 'development' ? 'http://localhost:8000' : ''),
  },
  proxy: {
    '/api/': {
      target: process.env.XA_API_PROXY || 'http://127.0.0.1:18080',
      changeOrigin: true,
    },
    '/health/': {
      target: process.env.XA_API_PROXY || 'http://127.0.0.1:18080',
      changeOrigin: true,
    },
  },
  routes: [
    { path: '/login', component: './auth' },
    { path: '/register', component: './auth' },
    {
      path: '/',
      component: '@/layouts/AppLayout',
      routes: [
        { path: '/', redirect: '/explore' },
        { path: '/explore', component: './agents' },
        { path: '/my-agents', component: './agents' },
        { path: '/chat', component: './chat' },
        { path: '*', component: './not-found' },
      ],
    },
  ],
});
