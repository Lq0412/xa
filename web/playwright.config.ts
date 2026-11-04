import { defineConfig } from '@playwright/test';

// 验收依赖已经启动的真实服务；截图和 trace 关闭，避免记录凭证。
export default defineConfig({
  testDir: './tests',
  timeout: 60000,
  expect: { timeout: 10000 },
  workers: 1,
  retries: 0,
  reporter: 'list',
  outputDir: '../.output/web-e2e',
  use: {
    baseURL: process.env.XA_WEB_URL || 'http://127.0.0.1:8000',
    browserName: 'chromium',
    viewport: { width: 1440, height: 1000 },
    actionTimeout: 10000,
    screenshot: 'off',
    trace: 'off',
    video: 'off',
  },
});
