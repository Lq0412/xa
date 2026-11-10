import { randomUUID } from 'node:crypto';
import { join } from 'node:path';
import { expect, type Page, test } from '@playwright/test';

/** 截图在填写登录凭证之前或进入业务页后采集，不保存 trace。 */
async function capture(page: Page, name: string, fullPage = false) {
  if (process.env.XA_ADMIN_SHOTS)
    await page.screenshot({
      path: join(process.env.XA_ADMIN_SHOTS, name),
      animations: 'disabled',
      fullPage,
    });
}

test('管理员登录、今天、Agent 配置、确认记录和退出', async ({
  page,
  request,
}) => {
  const email = process.env.XA_E2E_EMAIL;
  const password = process.env.XA_E2E_PASSWORD;
  if (!email || !password)
    throw new Error('请设置 XA_E2E_EMAIL 和 XA_E2E_PASSWORD');
  const errors: string[] = [];
  let expectingConflict = false;
  page.on('pageerror', (error) => errors.push(error.message));
  page.on('console', (message) => {
    if (
      message.type() === 'error' &&
      !(expectingConflict && message.text().includes('409'))
    )
      errors.push(message.text());
  });
  await page.goto('/overview');
  await expect(page).toHaveURL(/\/user\/login/);
  await expect(page.getByRole('heading', { name: '登录管理端' })).toBeVisible();
  await capture(page, '01-login.png');
  await page.getByLabel('邮箱', { exact: true }).fill(email);
  await page.getByLabel('密码', { exact: true }).fill(password);
  await page.getByRole('button', { name: '登录', exact: true }).click();
  await expect(page).toHaveURL(/\/overview$/);
  const token = await page.evaluate(() =>
    localStorage.getItem('xa.web.session'),
  );
  expect(token).toBeTruthy();
  const headers = { Authorization: `Bearer ${token}` };
  let agentId: string | undefined;
  try {
    await expect(
      page.getByRole('heading', { name: '今天', exact: true }),
    ).toBeVisible();
    await expect(page.locator('.metrics')).toBeVisible();
    await expect(page.getByRole('alert')).toHaveCount(0);
    await capture(page, '02-today.png');
    const nav = page.getByRole('navigation', { name: '主导航' });
    await nav.getByRole('link', { name: 'Agent', exact: true }).click();
    await expect(
      page.getByRole('heading', { name: 'Agent', exact: true }),
    ).toBeVisible();
    await expect(page.locator('[aria-busy=true]')).toHaveCount(0);
    await expect(page.getByRole('alert')).toHaveCount(0);
    await capture(page, '03-agents.png');
    // 使用真实创建的临时 Agent 验证保存，不改现有公共 Agent。
    await page.getByRole('link', { name: '新建 Agent' }).click();
    const name = `管理端验收 ${randomUUID().slice(0, 8)}`;
    await page.getByLabel('名称', { exact: true }).fill(name);
    await page
      .getByLabel('简介', { exact: true })
      .fill('用于验证管理端配置保存');
    await page
      .getByLabel('SOUL.md', { exact: true })
      .fill('使用中文，简洁回答。');
    const createdResponse = page.waitForResponse(
      (response) =>
        response.url().endsWith('/api/v1/agents') &&
        response.request().method() === 'POST',
    );
    await page.getByRole('button', { name: '创建', exact: true }).click();
    const created = await createdResponse;
    expect(created.status()).toBe(201);
    const result = await created.json();
    agentId = result.agent.id;
    await expect(page).toHaveURL(new RegExp(`/agents/${agentId}$`));
    await expect(page.getByLabel('SOUL.md')).toHaveValue(
      '使用中文，简洁回答。',
    );
    const before = await (
      await request.get(`/api/v1/agents/${agentId}/config`, { headers })
    ).json();
    const tools = await (
      await request.get('/api/v1/tools', { headers })
    ).json();
    const tool = tools.tools.find(
      (item: { available: boolean; approval: { overridable: boolean } }) =>
        item.available && item.approval.overridable,
    );
    expect(tool).toBeTruthy();
    const toggle = page.getByRole('switch', {
      name: `启用${tool.title}`,
      exact: true,
    });
    if ((await toggle.getAttribute('aria-checked')) === 'false')
      await toggle.click();
    const approval = page.getByRole('button', {
      name: new RegExp(`^${tool.title}：`),
    });
    const oldLabel = await approval.innerText();
    await approval.click();
    await expect(approval).toHaveText(
      oldLabel === '需确认' ? '自动运行' : '需确认',
    );
    await page.getByLabel('SOUL.md').fill('先给结论，再简短解释。');
    const alternativeModel = before.available_models.find(
      (model: string) => model !== before.configuration.model,
    );
    if (alternativeModel)
      await page
        .getByLabel('模型', { exact: true })
        .selectOption(alternativeModel);
    if (process.env.XA_ADMIN_SHOTS) {
      await page.setViewportSize({ width: 1440, height: 2100 });
      await page.evaluate(() => window.scrollTo(0, 0));
      await page.getByRole('heading', { name, exact: true }).click();
      await capture(page, '05-agent-edit.png');
      await page.setViewportSize({ width: 1440, height: 1000 });
    }
    await page.getByRole('button', { name: '保存', exact: true }).click();
    await expect(page.locator('.save-bar')).toHaveCount(0);
    const after = await (
      await request.get(`/api/v1/agents/${agentId}/config`, { headers })
    ).json();
    expect(after.id).not.toBe(before.id);
    expect(after.configuration.soul_md).toBe('先给结论，再简短解释。');
    if (alternativeModel)
      expect(after.configuration.model).toBe(alternativeModel);
    expect(
      after.configuration.tools.find(
        (item: { name: string }) => item.name === tool.name,
      ).approval,
    ).toBe(oldLabel === '需确认' ? 'never' : 'always');
    for (const key of [
      'models',
      'knowledge',
      'evaluations',
      'memory_policy',
      'runtime_limits',
    ])
      expect(after.configuration[key]).toEqual(before.configuration[key]);
    await page.reload();
    await expect(page.getByLabel('SOUL.md')).toHaveValue(
      '先给结论，再简短解释。',
    );
    // 验证未保存退出保护，不把草稿带到其他页面。
    await page.getByLabel('简介', { exact: true }).fill('尚未保存');
    page.once('dialog', (dialog) => dialog.dismiss());
    await nav.getByRole('link', { name: 'Agent', exact: true }).click();
    await expect(page.getByLabel('简介', { exact: true })).toHaveValue(
      '尚未保存',
    );
    await page.getByRole('button', { name: '取消', exact: true }).click();
    await expect(page.locator('.save-bar')).toHaveCount(0);
    // 真实并发版本冲突必须保留草稿，不能覆盖新版本。
    const concurrent = await request.put(`/api/v1/agents/${agentId}/config`, {
      headers,
      data: {
        name: result.agent.name,
        description: result.agent.description,
        avatar: result.agent.avatar,
        tags: result.agent.tags,
        visibility: result.agent.visibility,
        expected_version_id: after.id,
        configuration: {
          ...after.configuration,
          prompts: undefined,
          soul_md: '并发修改后的内容',
        },
      },
    });
    expect(concurrent.status()).toBe(200);
    await page.getByLabel('SOUL.md').fill('旧版本草稿');
    expectingConflict = true;
    const conflict = page.waitForResponse(
      (response) =>
        response.request().method() === 'PUT' &&
        response.url().endsWith(`/agents/${agentId}/config`),
    );
    await page.getByRole('button', { name: '保存', exact: true }).click();
    expect((await conflict).status()).toBe(409);
    await expect(page.getByRole('alert')).toContainText('配置已被更新');
    await expect(page.getByLabel('SOUL.md')).toHaveValue('旧版本草稿');
    await page.getByRole('button', { name: '取消', exact: true }).click();
    await page.reload();
    await expect(page.getByLabel('SOUL.md')).toHaveValue('并发修改后的内容');
    expectingConflict = false;
    await page.getByRole('button', { name: '停用', exact: true }).click();
    await expect(
      page.getByRole('button', { name: '启用', exact: true }),
    ).toBeVisible();
    await page.getByRole('button', { name: '启用', exact: true }).click();
    await expect(
      page.getByRole('button', { name: '停用', exact: true }),
    ).toBeVisible();
    await nav.getByRole('link', { name: '今天', exact: true }).click();
    await expect(page.locator('.metrics')).toBeVisible();
    await expect(
      page.getByRole('link', { name: new RegExp(name) }),
    ).toBeVisible();
    await capture(page, '02-today.png');
    await nav.getByRole('link', { name: 'Agent', exact: true }).click();
    await expect(page.locator('[aria-busy=true]')).toHaveCount(0);
    await capture(page, '03-agents.png');
    await page.getByRole('textbox', { name: '搜索 Agent' }).fill(name);
    await expect(page.getByRole('link', { name, exact: true })).toBeVisible();
    await page
      .getByRole('textbox', { name: '搜索 Agent' })
      .fill('不存在的 Agent 搜索词');
    await expect(page.getByText('没有符合条件的 Agent')).toBeVisible();
    await nav.getByRole('link', { name: '确认记录', exact: true }).click();
    await expect(
      page.getByRole('heading', { name: '确认记录', exact: true }),
    ).toBeVisible();
    await expect(
      page.getByRole('region', { name: '确认记录列表' }),
    ).toHaveAttribute('aria-busy', 'false');
    await page.getByRole('button', { name: '全部', exact: true }).click();
    await expect(
      page.getByRole('region', { name: '确认记录列表' }),
    ).toHaveAttribute('aria-busy', 'false');
    if (await page.locator('.approval-record').count()) {
      await page.locator('.approval-record').first().click();
      await expect(page.locator('.approval-detail h2')).toBeVisible();
      const title = await page.locator('.approval-detail h2').innerText();
      await page.reload();
      await expect(page.locator('.approval-detail h2')).toHaveText(title);
    }
    await expect(page.getByRole('alert')).toHaveCount(0);
    await capture(page, '04-approvals.png');
    await page.setViewportSize({ width: 1024, height: 850 });
    await page.getByRole('button', { name: '打开导航' }).click();
    await expect(page.getByRole('dialog', { name: '管理导航' })).toBeVisible();
    await page.getByRole('button', { name: '关闭导航' }).click();
    expect(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth,
      ),
    ).toBe(true);
    await capture(page, '06-narrow.png');
    await page.setViewportSize({ width: 1440, height: 1000 });
    await page.getByRole('button', { name: '账号菜单' }).click();
    await page.getByRole('menuitem', { name: '退出登录' }).click();
    await expect(page).toHaveURL(/\/user\/login$/);
    expect((await request.get('/api/v1/auth/me', { headers })).status()).toBe(
      401,
    );
    expect(errors).toEqual([]);
  } finally {
    // 使用独立会话清理本测试创建的资源，凭证与令牌不输出。
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
