import { randomUUID } from 'node:crypto';
import { expect, test } from '@playwright/test';

// 两个真实前端共享后端测试账号，各自创建并撤销会话，测试不替换任何响应。
const email = process.env.XA_E2E_EMAIL;
const password = process.env.XA_E2E_PASSWORD;
if (!email || !password)
  throw new Error('请设置管理员 XA_E2E_EMAIL 和 XA_E2E_PASSWORD');
const userUrl = process.env.XA_USER_WEB_URL || 'http://127.0.0.1:8002';

test('管理端仅展示可用模块，已删除页面返回 404', async ({ page, request }) => {
  const errors: string[] = [];
  page.on('pageerror', (error) => errors.push(error.message));
  await page.goto('/overview');
  await page.getByRole('textbox', { name: '邮箱' }).fill(email);
  await page.getByLabel('密码', { exact: true }).fill(password);
  await page.getByRole('button', { name: /登\s*录/ }).click();
  await expect(page).toHaveURL(/\/overview$/);
  const token = await page.evaluate(() =>
    localStorage.getItem('xa.web.session'),
  );
  try {
    const menu = page.getByRole('navigation', { name: '主导航' });
    await expect(menu.getByRole('link')).toHaveCount(3);
    await expect(menu.getByRole('link', { name: '今天' })).toBeVisible();
    await expect(menu.getByRole('link', { name: '确认记录' })).toBeVisible();
    await menu.getByRole('link', { name: 'Agent', exact: true }).click();
    await expect(page).toHaveURL(/\/agents$/);
    await page.reload();
    await expect(page.getByRole('link', { name: '新建 Agent' })).toBeVisible();
    for (const path of [
      '/models',
      '/knowledge',
      '/tools',
      '/evaluation',
      '/tasks',
      '/users',
      '/operations',
    ]) {
      await page.goto(path);
      await expect(page.getByText('404', { exact: true })).toBeVisible();
    }
    expect(errors).toEqual([]);
  } finally {
    expect(
      (
        await request.post('/api/v1/auth/logout', {
          headers: { Authorization: `Bearer ${token}` },
        })
      ).status(),
    ).toBe(204);
  }
});

test('用户端真实 Agent 创建、独立会话、对话布局与可用入口', async ({
  page,
  request,
}) => {
  const errors: string[] = [];
  page.on('pageerror', (error) => errors.push(error.message));
  await page.goto(`${userUrl}/explore`);
  await expect(
    page.getByRole('heading', { name: '发现 Agent', exact: true }),
  ).toBeVisible();
  await page
    .getByRole('navigation', { name: '主导航' })
    .getByRole('link', { name: '我的 Agent', exact: true })
    .click();
  await expect(page).toHaveURL(/\/login\?redirect=/);
  await page.getByLabel('邮箱', { exact: true }).fill(email);
  await page.getByLabel('密码', { exact: true }).fill(password);
  await page.getByRole('button', { name: /登\s*录/ }).click();
  await expect(page).toHaveURL(/\/my-agents$/);
  const token = await page.evaluate(() =>
    localStorage.getItem('xa.user.session'),
  );
  expect(token).toBeTruthy();
  expect(
    await page.evaluate(() => localStorage.getItem('xa.web.session')),
  ).toBeNull();
  const headers = { Authorization: `Bearer ${token}` };
  let agentId: string | undefined;
  try {
    const name = `用户端验收 ${randomUUID()}`;
    await page
      .getByRole('button', { name: '新建 Agent', exact: true })
      .first()
      .click();
    const dialog = page.getByRole('dialog');
    await dialog.getByLabel('名称', { exact: true }).fill(name);
    await dialog
      .getByLabel('简介', { exact: true })
      .fill('验证真实私有 Agent 与对话页面结构');
    await dialog
      .getByLabel('SOUL.md', { exact: true })
      .fill('请使用中文回答。');
    const created = page.waitForResponse(
      (response) =>
        response.url().endsWith('/api/v1/agents') &&
        response.request().method() === 'POST',
    );
    await dialog.getByRole('button', { name: /创\s*建/ }).click();
    const response = await created;
    expect(response.status()).toBe(201);
    const result = await response.json();
    agentId = result.agent.id;
    expect(result.agent.visibility).toBe('private');
    await expect(dialog).not.toBeVisible();
    await expect(page).toHaveURL(/\/chat\?agentId=/);
    await page
      .getByRole('navigation', { name: '主导航' })
      .getByRole('link', { name: '我的 Agent', exact: true })
      .click();
    await page.getByRole('textbox', { name: '搜索 Agent' }).fill(name);
    const card = page.getByRole('article').filter({ hasText: name });
    await expect(card).toBeVisible();
    await page.reload();
    await page.getByRole('textbox', { name: '搜索 Agent' }).fill(name);
    await card.getByRole('link', { name: /进入对话/ }).click();
    await expect(page).toHaveURL(/\/chat\?agentId=/);
    await expect(
      page.getByRole('heading', { name, exact: true }).first(),
    ).toBeVisible();
    await expect(page.getByRole('button', { name: '发送消息' })).toBeDisabled();
    await expect(
      page.getByRole('textbox', { name: '消息', exact: true }),
    ).toBeEnabled();
    const input = page.getByRole('textbox', { name: '消息', exact: true });
    await input.fill('草稿');
    await input.press('Shift+Enter');
    await input.press('End');
    await input.type('第二行');
    await expect(input).toHaveValue('草稿\n第二行');
    await expect(page.getByRole('button', { name: '发送消息' })).toBeEnabled();
    await input.fill('');
    const details = page.getByRole('region', { name: 'Agent 配置' });
    await expect(details).toContainText(name);
    await details.getByRole('button', { name: '收起 Agent 配置' }).click();
    await expect(details).toBeHidden();
    await page.getByRole('button', { name: '展开 Agent 配置' }).click();
    await expect(details).toContainText('请使用中文回答。');
    await page.setViewportSize({ width: 390, height: 844 });
    await expect(details).toBeHidden();
    await page.getByRole('button', { name: '打开 Agent 列表' }).click();
    await expect(
      page.getByRole('dialog', { name: '我的 Agent' }),
    ).toBeVisible();
    await page.getByRole('button', { name: '关闭我的 Agent' }).click();
    expect(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth,
      ),
    ).toBeTruthy();
    const composer = await page.locator('.message-composer').boundingBox();
    if (!composer) throw new Error('输入区不可见');
    expect(composer.y + composer.height).toBeLessThanOrEqual(844);
    for (const [path, title] of [
      ['history', '会话历史'],
      ['memory', '个人记忆'],
      ['feedback', '学习反馈'],
      ['settings', '账号设置'],
    ]) {
      await expect(
        page.getByRole('navigation', { name: '主导航' }).getByRole('link', {
          name: title,
          exact: true,
        }),
      ).toHaveCount(0);
      await page.goto(`${userUrl}/${path}`);
      await expect(page.getByText('页面不存在', { exact: true })).toBeVisible();
      expect(
        await page.evaluate(
          () => document.documentElement.scrollWidth <= innerWidth,
        ),
      ).toBeTruthy();
    }
    await page.setViewportSize({ width: 1440, height: 1000 });
    await page.getByRole('button', { name: '退出登录' }).click();
    await expect(page).toHaveURL(/\/explore$/);
    expect((await request.get('/api/v1/auth/me', { headers })).status()).toBe(
      401,
    );
    expect(errors).toEqual([]);
  } finally {
    // 页面退出后使用独立清理会话删除本测试创建的资源。
    const cleanup = await request.post('/api/v1/auth/login', {
      data: { email, password },
    });
    expect(cleanup.ok()).toBeTruthy();
    const auth = { Authorization: `Bearer ${(await cleanup.json()).token}` };
    if (agentId)
      expect(
        (
          await request.delete(`/api/v1/agents/${agentId}`, { headers: auth })
        ).status(),
      ).toBe(204);
    expect(
      (await request.post('/api/v1/auth/logout', { headers: auth })).status(),
    ).toBe(204);
    expect([204, 401]).toContain(
      (await request.post('/api/v1/auth/logout', { headers })).status(),
    );
  }
});

test('用户端重复注册与登录失败展示真实服务错误', async ({ page }) => {
  await page.goto(`${userUrl}/register`);
  await page.getByLabel('邮箱', { exact: true }).fill(email);
  await page.getByLabel('密码', { exact: true }).fill(randomUUID());
  await page.getByRole('button', { name: '注册并登录' }).click();
  await expect(page.getByRole('alert')).toBeVisible();
  await page.getByRole('link', { name: '已有账号？登录' }).click();
  await page.getByLabel('邮箱', { exact: true }).fill(email);
  await page.getByLabel('密码', { exact: true }).fill(randomUUID());
  await page.getByRole('button', { name: /登\s*录/ }).click();
  await expect(page.getByRole('alert')).toBeVisible();
});
