// 仅供开发服务器使用；部署 dist 时应由反向代理转发相同路径。
const target = process.env.XA_API_PROXY || 'http://127.0.0.1:18080';

export default {
  '/api/': { target, changeOrigin: true },
  '/health/': { target, changeOrigin: true },
};
