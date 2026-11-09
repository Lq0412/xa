import path from 'node:path';
import { expect, test } from '@playwright/test';

if (process.env.XA_P4_CHAT_SCREENSHOTS !== '1') {
  throw new Error('请设置 XA_P4_CHAT_SCREENSHOTS=1');
}

const artifactDir = '/opt/cursor/artifacts';

test('P4 流式：思考折叠、工具 chip、刷新续传', async ({ page, context }) => {
  test.setTimeout(420000);
  await context.grantPermissions(['clipboard-read', 'clipboard-write']);
  await page.goto('/login', { waitUntil: 'networkidle' });
  await expect(page.locator('input[name=email]')).toBeVisible({
    timeout: 60000,
  });
  await page.locator('input[name=email]').fill(process.env.XA_E2E_EMAIL || '');
  await page
    .locator('input[name=password]')
    .fill(process.env.XA_E2E_PASSWORD || '');
  await page.getByRole('button', { name: '登录', exact: true }).click();
  await expect(page).toHaveURL(/\/chat/, { timeout: 60000 });

  const composer = page.getByPlaceholder('发消息');
  await expect(composer).toBeEnabled({ timeout: 60000 });
  await composer.fill(
    '必须调用 current_time 工具。先展示工具执行过程，再写两段中文：第一段可多句推理，第二段一句话总结当前时间。',
  );
  await composer.press('Enter');

  await page.waitForTimeout(8000);
  const reasoning = page.locator('[data-testid="reasoning-fold"]');
  if ((await reasoning.count()) > 0) {
    await reasoning
      .first()
      .getByRole('button')
      .click()
      .catch(() => undefined);
  }
  await page.screenshot({
    path: path.join(artifactDir, 'p4-chat-reasoning-fold.png'),
    fullPage: true,
  });
  await page.screenshot({
    path: path.join(artifactDir, 'p4-chat-tool-running.png'),
    fullPage: true,
  });

  await expect(composer).toBeEnabled({ timeout: 300000 });
  await page.screenshot({
    path: path.join(artifactDir, 'p4-chat-tool-done.png'),
    fullPage: true,
  });

  await page.reload({ waitUntil: 'networkidle' });
  await expect(composer).toBeVisible();
  await page.waitForTimeout(5000);
  await page.screenshot({
    path: path.join(artifactDir, 'p4-chat-resume.png'),
    fullPage: true,
  });
});
