import { defineConfig } from '@playwright/test';

// 验收通过真实 server 和生产构建完成，不记录截图、视频或 trace。
export default defineConfig({
  testDir: './tests',
  workers: 1,
  retries: 0,
  timeout: 180000,
  expect: { timeout: 10000 },
  reporter: 'list',
  outputDir: '../.output/user-layout-e2e',
  use: {
    baseURL: process.env.XA_WEB_URL,
    browserName: 'chromium',
    viewport: { width: 1440, height: 1000 },
    screenshot: 'off',
    video: 'off',
    trace: 'off',
  },
});
