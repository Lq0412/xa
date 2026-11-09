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
  antd: false,
  mock: false,
  tailwindcss: {},
  define: {
    'process.env.XA_ADMIN_WEB_URL':
      process.env.XA_ADMIN_WEB_URL ||
      (process.env.NODE_ENV === 'development' ? 'http://localhost:8000' : ''),
    // 开发代理会缓冲整条 SSE；设此项时消息流/续传直连 Go API（需 XA_HTTP_CORS_ORIGINS）。
    'process.env.XA_API_STREAM_BASE': process.env.XA_API_STREAM_BASE || '',
  },
  proxy: {
    '/api/': {
      target: process.env.XA_API_PROXY || 'http://127.0.0.1:18080',
      changeOrigin: true,
      proxyTimeout: 0,
      timeout: 0,
      // 开发代理必须逐块转发 SSE，否则 Playwright / 浏览器无法实时渲染思考与工具态。
      selfHandleResponse: true,
      onProxyRes: (proxyRes, _req, res) => {
        res.writeHead(proxyRes.statusCode ?? 502, proxyRes.headers);
        proxyRes.pipe(res);
      },
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
