import { randomUUID } from 'node:crypto';
import { expect, test } from '@playwright/test';

test('确认记录失效链接与窄屏详情重试', async ({ context, page, request }) => {
  const email = process.env.XA_E2E_EMAIL;
  const password = process.env.XA_E2E_PASSWORD;
  if (!email || !password)
    throw new Error('请设置 XA_E2E_EMAIL 和 XA_E2E_PASSWORD');

  const selected = randomUUID();
  const detail = page.getByRole('article', { name: '记录详情' });
  const list = page.getByRole('region', { name: '确认记录列表' });
  const missing = detail.getByText('未找到这条确认记录');

  try {
    await page.goto('/user/login');
    await page.getByLabel('邮箱', { exact: true }).fill(email);
    await page.getByLabel('密码', { exact: true }).fill(password);
    await page.getByRole('button', { name: '登录', exact: true }).click();
    await expect(page).toHaveURL(/\/overview$/);

    await page.goto(`/approvals?status=all&selected=${selected}`);
    await expect(missing).toBeVisible();
    await expect(list.locator('[aria-current=true]')).toHaveCount(0);
    await expect(detail.getByText('记录编号', { exact: true })).toHaveCount(0);

    await page.setViewportSize({ width: 375, height: 812 });
    await detail.getByRole('button', { name: '确认记录', exact: true }).click();
    await expect(list).toHaveAttribute('aria-busy', 'false');

    // 用真实断网让浏览器返回详情时的请求失败。
    await context.setOffline(true);
    await page.goBack();
    await expect(detail.getByRole('alert')).toContainText('无法连接服务');
    await expect(missing).toHaveCount(0);

    await context.setOffline(false);
    await detail.getByRole('button', { name: '重试', exact: true }).click();
    await expect(missing).toBeVisible();
    await expect(detail.getByRole('alert')).toHaveCount(0);
    await expect(page).toHaveURL(new RegExp(`selected=${selected}$`));
  } finally {
    await context.setOffline(false);
    const token = await page.evaluate(() =>
      localStorage.getItem('xa.web.session'),
    );
    if (token) {
      const headers = { Authorization: `Bearer ${token}` };
      expect(
        (await request.post('/api/v1/auth/logout', { headers })).status(),
      ).toBe(204);
      expect((await request.get('/api/v1/auth/me', { headers })).status()).toBe(
        401,
      );
    }
  }
});
