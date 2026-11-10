import { defineConfig } from 'umi';
import proxy from './proxy';
import routes from './routes';

// 使用 Umi 的构建、路由和开发代理，界面及会话由应用管理。
export default defineConfig({
  hash: true,
  // 开发和生产输出分开，构建时可以继续验收开发页面。
  outputPath:
    process.env.NODE_ENV === 'development' ? '../.output/web-dev' : 'dist',
  publicPath: '/',
  title: '管理',
  metas: [{ name: 'viewport', content: 'width=device-width, initial-scale=1' }],
  mfsu: false,
  esbuildMinifyIIFE: true,
  define: {
    'process.env.XA_USER_WEB_URL':
      process.env.XA_USER_WEB_URL ||
      (process.env.NODE_ENV === 'development' ? 'http://localhost:8002' : ''),
  },
  routes,
  proxy,
  fastRefresh: true,
  chainWebpack(memo) {
    // Tailwind 4 只处理应用的样式入口。
    memo.module
      .rule('tailwind')
      .enforce('pre')
      .test(/[/\\]tailwind\.css$/)
      .use('tailwind')
      .loader(require.resolve('@tailwindcss/webpack'));
    return memo;
  },
  mock: false,
});
