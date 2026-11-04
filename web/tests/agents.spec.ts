import { randomUUID } from 'node:crypto';
import { expect, type Page, test } from '@playwright/test';

// 测试账号由调用环境提供；不内置默认密码，也不替换任何后端响应。
const email = process.env.XA_E2E_EMAIL;
const password = process.env.XA_E2E_PASSWORD;
if (!email || !password)
  throw new Error('请设置 XA_E2E_EMAIL 和 XA_E2E_PASSWORD');
const credentials = { email, password };

async function signIn(page: Page) {
  await page.goto('/agents');
  await expect(page).toHaveURL(/\/user\/login/);
  await page.getByRole('textbox', { name: '邮箱' }).fill(credentials.email);
  await page.getByLabel('密码', { exact: true }).fill(credentials.password);
  await page.getByRole('button', { name: /登\s*录/ }).click();
  await expect(page).toHaveURL(/\/agents$/);
  await expect(page.getByRole('button', { name: '新建 Agent' })).toBeVisible();
}

test('真实会话与 Agent 创建、筛选、编辑、版本冲突、启停和删除', async ({
  page,
  request,
}) => {
  const errors: string[] = [];
  page.on('pageerror', (error) => errors.push(error.message));
  await signIn(page);
  const token = await page.evaluate(() =>
    localStorage.getItem('xa.web.session'),
  );
  expect(token).toBeTruthy();
  const headers = { Authorization: `Bearer ${token}` };
  const name = `Web 验收 ${randomUUID()}`;
  let agentId: string | undefined;
  let deleted = false;
  try {
    await page.reload();
    await expect(
      page.getByRole('button', { name: '新建 Agent' }),
    ).toBeVisible();
    await page.getByRole('button', { name: '新建 Agent' }).click();
    const dialog = page.getByRole('dialog');
    await dialog.getByLabel('名称', { exact: true }).fill(name);
    await dialog
      .getByLabel('描述', { exact: true })
      .fill('通过真实接口验证 Pro 管理页面');
    await dialog.getByLabel('标签', { exact: true }).fill('验收，Web');
    await dialog
      .getByLabel('系统提示词', { exact: true })
      .fill('请使用中文回答。');
    const createdResponse = page.waitForResponse(
      (response) =>
        response.url().endsWith('/api/v1/agents') &&
        response.request().method() === 'POST',
    );
    await dialog.getByRole('button', { name: /保\s*存/ }).click();
    const created = await createdResponse;
    expect(created.status()).toBe(201);
    const saved = (await created.json()) as { agent: XA.Agent };
    agentId = saved.agent.id;
    const endpoint = `/api/v1/agents/${agentId}`;
    const row = page.getByRole('row').filter({ hasText: name });
    await expect(dialog).not.toBeVisible();
    await expect(row).toBeVisible();

    // 使用实际保存的配置追加另一个提示词，验证表单不会覆盖未展示内容。
    const before = (await (
      await request.get(`${endpoint}/config`, { headers })
    ).json()) as XA.AgentVersion;
    const extra: XA.PromptSlot = {
      name: 'context',
      content: '保留上下文',
      enabled: false,
      order: 1,
      variables: [],
      scope: 'agent',
    };
    const configuration = {
      ...before.configuration,
      prompts: [...(before.configuration.prompts ?? []), extra],
      runtime_limits: { max_turns: 12 },
    };
    const payload = {
      name,
      avatar: '/logo.svg',
      description: saved.agent.description,
      tags: saved.agent.tags,
      visibility: saved.agent.visibility,
      configuration,
    };
    expect(
      (
        await request.put(`${endpoint}/config`, {
          headers,
          data: { ...payload, expected_version_id: before.id },
        })
      ).ok(),
    ).toBeTruthy();

    await row.getByRole('button', { name: /编\s*辑/ }).click();
    await expect(dialog.getByLabel('系统提示词', { exact: true })).toHaveValue(
      '请使用中文回答。',
    );
    await dialog
      .getByLabel('系统提示词', { exact: true })
      .fill('请用简洁中文回答。');
    await dialog.getByRole('button', { name: /保\s*存/ }).click();
    await expect(dialog).not.toBeVisible();
    const after = (await (
      await request.get(`${endpoint}/config`, { headers })
    ).json()) as XA.AgentVersion;
    expect(
      after.configuration.prompts?.map((prompt) => prompt.content),
    ).toEqual(['请用简洁中文回答。', '保留上下文']);
    expect(after.configuration.runtime_limits).toEqual({ max_turns: 12 });
    expect(
      (await (await request.get(endpoint, { headers })).json()).avatar,
    ).toBe('/logo.svg');

    // 在编辑窗打开后通过真实接口保存新版本，旧编辑必须被拒绝。
    await row.getByRole('button', { name: /编\s*辑/ }).click();
    await expect(dialog.getByLabel('名称', { exact: true })).toHaveValue(name);
    expect(
      (
        await request.put(`${endpoint}/config`, {
          headers,
          data: {
            ...payload,
            configuration: after.configuration,
            expected_version_id: after.id,
          },
        })
      ).ok(),
    ).toBeTruthy();
    await dialog
      .getByLabel('描述', { exact: true })
      .fill('过期编辑不得覆盖配置');
    await dialog.getByRole('button', { name: /保\s*存/ }).click();
    await expect(dialog.getByRole('alert')).toContainText('配置已被更新');
    await dialog.getByRole('button', { name: /取\s*消/ }).click();
    await expect(dialog).not.toBeVisible();

    await page
      .getByLabel('名称', { exact: true })
      .fill(`不存在 ${randomUUID()}`);
    await page.getByRole('button', { name: /查\s*询/ }).click();
    await expect(row).not.toBeVisible();
    await page.getByLabel('名称', { exact: true }).fill(name);
    await page.getByRole('button', { name: /查\s*询/ }).click();
    await expect(row).toBeVisible();
    await row.getByRole('button', { name: /停\s*用/ }).click();
    await expect(row).toContainText('已停用');
    await row.getByRole('button', { name: /^启\s*用$/ }).click();
    await expect(row).toContainText('已启用');
    await row.getByRole('button', { name: /删\s*除/ }).click();
    await page
      .getByRole('button', { name: /删\s*除/ })
      .last()
      .click();
    await expect(row).not.toBeVisible();
    expect((await request.get(endpoint, { headers })).status()).toBe(404);
    deleted = true;

    await page.getByRole('button', { name: '账号菜单' }).click();
    await page.getByRole('menuitem', { name: '退出登录' }).click();
    await expect(page).toHaveURL(/\/user\/login/);
    expect((await request.get('/api/v1/auth/me', { headers })).status()).toBe(
      401,
    );
    expect(errors).toEqual([]);
  } finally {
    // 只删除本用例创建的 Agent；即便中途断言失败，也撤销测试会话。
    if (agentId && !deleted)
      expect(
        (await request.delete(`/api/v1/agents/${agentId}`, { headers })).ok(),
      ).toBeTruthy();
    // 页面已经退出时认证中间件返回 401，也表示该测试会话已被撤销。
    const cleanup = await request.post('/api/v1/auth/logout', { headers });
    expect([204, 401]).toContain(cleanup.status());
  }
});

test('登录错误、用户端注册入口和已撤销会话均有明确反馈', async ({
  page,
  request,
}) => {
  await page.goto('/user/login');
  await page.getByRole('textbox', { name: '邮箱' }).fill(credentials.email);
  await page.getByLabel('密码', { exact: true }).fill(randomUUID());
  await page.getByRole('button', { name: /登\s*录/ }).click();
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
  await expect(page.getByRole('button', { name: /登\s*录/ })).toBeVisible();
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= window.innerWidth,
    ),
  ).toBeTruthy();
});
