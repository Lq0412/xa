import { join } from 'node:path';
import { defineConfig } from '@umijs/max';
import defaultSettings from './defaultSettings';
import proxy from './proxy';
import routes from './routes';

// 基于 Ant Design Pro v6.0.3，使用 Umi Max 管理布局、认证状态和请求。
export default defineConfig({
  hash: true,
  // Utoo 开发资源也写入磁盘，独立目录避免构建 dist 时破坏正在运行的页面。
  outputPath:
    process.env.NODE_ENV === 'development' ? '../.output/web-dev' : 'dist',
  publicPath: '/',
  title: 'xa Agent',
  routes,
  proxy,
  fastRefresh: true,
  model: {},
  initialState: {},
  access: {},
  layout: { locale: true, ...defaultSettings },
  locale: { default: 'zh-CN', antd: true, baseNavigator: false },
  antd: {
    appConfig: {},
    configProvider: { theme: { token: { colorPrimary: '#1677ff' } } },
  },
  request: {},
  // 开发环境直接访问 xa 后端，禁止使用模板演示接口。
  mock: false,
  tailwindcss: {},
  utoopack: { root: join(__dirname, '../..') },
  headScripts: [{ src: '/scripts/loading.js', async: true }],
});
