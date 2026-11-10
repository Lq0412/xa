import { randomUUID } from 'node:crypto';
import { expect, type Page, test } from '@playwright/test';

// 真实服务回归沿用原有覆盖范围；主要管理流程已合并进 admin-smoke。
const email = process.env.XA_E2E_EMAIL;
const password = process.env.XA_E2E_PASSWORD;
if (!email || !password)
  throw new Error('请设置 XA_E2E_EMAIL 和 XA_E2E_PASSWORD');
const credentials = { email, password };

async function signIn(page: Page) {
  await page.goto('/agents');
  await expect(page).toHaveURL(/\/user\/login/);
  await page.getByLabel('邮箱', { exact: true }).fill(credentials.email);
  await page.getByLabel('密码', { exact: true }).fill(credentials.password);
  await page.getByRole('button', { name: '登录', exact: true }).click();
  await expect(page).toHaveURL(/\/agents$/);
  await expect(page.getByRole('link', { name: '新建 Agent' })).toBeVisible();
}

test('编辑保留头像和非默认策略，整页危险区删除真实 Agent', async ({
  page,
  request,
}) => {
  await signIn(page);
  const token = await page.evaluate(() =>
    localStorage.getItem('xa.web.session'),
  );
  const headers = { Authorization: `Bearer ${token}` };
  const name = `配置回归 ${randomUUID().slice(0, 8)}`;
  let agentId: string | undefined;
  try {
    const created = await request.post('/api/v1/agents', {
      headers,
      data: {
        name,
        avatar: '/default-agent-avatar.png',
        description: '配置保留回归',
        tags: ['验收'],
        visibility: 'public',
        configuration: {
          soul_md: '请使用中文回答。',
          memory_policy: { enabled: false },
          runtime_limits: { max_model_turns: 6, max_tool_calls: 10 },
        },
      },
    });
    expect(created.status()).toBe(201);
    agentId = (await created.json()).agent.id;
    const endpoint = `/api/v1/agents/${agentId}`;
    const before = await (
      await request.get(`${endpoint}/config`, { headers })
    ).json();
    await page.goto(`/agents/${agentId}`);
    await page.getByLabel('SOUL.md').fill('请用简洁中文回答。');
    await page.getByRole('button', { name: '保存', exact: true }).click();
    await expect(page.locator('.save-bar')).toHaveCount(0);
    const after = await (
      await request.get(`${endpoint}/config`, { headers })
    ).json();
    expect(after.configuration.soul_md).toBe('请用简洁中文回答。');
    expect(after.configuration.memory_policy).toEqual(
      before.configuration.memory_policy,
    );
    expect(after.configuration.runtime_limits).toEqual(
      before.configuration.runtime_limits,
    );
    expect(
      (await (await request.get(endpoint, { headers })).json()).avatar,
    ).toBe('/default-agent-avatar.png');
    await page.getByRole('button', { name: '删除', exact: true }).click();
    await page
      .getByRole('dialog')
      .getByRole('button', { name: '删除', exact: true })
      .click();
    await expect(page).toHaveURL(/\/agents$/);
    expect((await request.get(endpoint, { headers })).status()).toBe(404);
    agentId = undefined;
  } finally {
    if (agentId)
      expect(
        (
          await request.delete(`/api/v1/agents/${agentId}`, { headers })
        ).status(),
      ).toBe(204);
    expect([204, 401]).toContain(
      (await request.post('/api/v1/auth/logout', { headers })).status(),
    );
  }
});

test('登录错误、用户端注册入口和已撤销会话均有明确反馈', async ({
  page,
  request,
}) => {
  await page.goto('/user/login');
  await page.getByLabel('邮箱', { exact: true }).fill(credentials.email);
  await page.getByLabel('密码', { exact: true }).fill(randomUUID());
  await page.getByRole('button', { name: '登录', exact: true }).click();
  await expect(page.getByRole('alert')).toBeVisible();
  await expect(page).toHaveURL(/\/user\/login/);
  await page.getByRole('link', { name: '用户注册与使用入口' }).click();
  await expect(page).toHaveURL(/\/user\/register/);
  await expect(page.getByText('前往用户端创建账号')).toBeVisible();
  await expect(page.getByRole('link', { name: '前往注册' })).toHaveAttribute(
    'href',
    /:8002\/register$/,
  );
  await page.getByRole('link', { name: '返回管理员登录' }).click();
  await signIn(page);
  await page.goto('/missing-page');
  await expect(page.getByText('访问的页面不存在')).toBeVisible();
  await page.goto('/agents');
  const token = await page.evaluate(() =>
    localStorage.getItem('xa.web.session'),
  );
  expect(
    (
      await request.post('/api/v1/auth/logout', {
        headers: { Authorization: `Bearer ${token}` },
      })
    ).ok(),
  ).toBeTruthy();
  await page.reload();
  await expect(page).toHaveURL(/\/user\/login/);
  expect(
    await page.evaluate(() => localStorage.getItem('xa.web.session')),
  ).toBeNull();
  await page.setViewportSize({ width: 390, height: 844 });
  await expect(
    page.getByRole('button', { name: '登录', exact: true }),
  ).toBeVisible();
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  ).toBeTruthy();
});
