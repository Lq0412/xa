import { randomUUID } from 'node:crypto';
import { expect, test } from '@playwright/test';

test('基础字段保存保留原始标签，版本不一致时重试读取', async ({
  page,
  request,
}) => {
  const email = process.env.XA_E2E_EMAIL;
  const password = process.env.XA_E2E_PASSWORD;
  if (!email || !password)
    throw new Error('请设置 XA_E2E_EMAIL 和 XA_E2E_PASSWORD');

  await page.goto('/agents');
  await page.getByLabel('邮箱', { exact: true }).fill(email);
  await page.getByLabel('密码', { exact: true }).fill(password);
  await page.getByRole('button', { name: '登录', exact: true }).click();
  await expect(page).toHaveURL(/\/agents$/);
  const token = await page.evaluate(() =>
    localStorage.getItem('xa.web.session'),
  );
  expect(Boolean(token)).toBe(true);
  const headers = { Authorization: `Bearer ${token}` };
  let endpoint: string | undefined;
  try {
    const name = `配置保存回归 ${randomUUID()}`;
    const tags = ['销售，售前', '华东,华南'];
    const avatar = '/default-agent-avatar.png';
    const created = await request.post('/api/v1/agents', {
      headers,
      data: {
        name,
        description: '初始简介',
        tags,
        avatar,
        visibility: 'public',
        configuration: {
          soul_md: '初始 SOUL',
          enable_thinking: false,
          memory_policy: { enabled: false },
          runtime_limits: { max_model_turns: 6, max_tool_calls: 10 },
        },
      },
    });
    expect(created.status()).toBe(201);
    const { agent } = await created.json();
    endpoint = `/api/v1/agents/${agent.id}`;
    const configEndpoint = `${endpoint}/config`;
    const configBefore = await request.get(configEndpoint, { headers });
    expect(configBefore.status()).toBe(200);
    const before = await configBefore.json();
    const description = page.getByLabel('简介', { exact: true });
    const soul = page.getByLabel('SOUL.md', { exact: true });
    const save = page.getByRole('button', { name: '保存', exact: true });
    await page.goto(`/agents/${agent.id}`);
    await description.fill('只改简介');
    const saving = page.waitForResponse(
      (response) =>
        response.request().method() === 'PUT' &&
        new URL(response.url()).pathname === configEndpoint,
    );
    await save.click();
    expect((await saving).status()).toBe(200);
    await expect(save).toHaveCount(0);
    await expect(description).toHaveValue('只改简介');
    const agentAfter = await request.get(endpoint, { headers });
    const configAfter = await request.get(configEndpoint, { headers });
    expect(agentAfter.status()).toBe(200);
    expect(configAfter.status()).toBe(200);
    expect(await agentAfter.json()).toMatchObject({ tags, avatar });
    const after = await configAfter.json();
    expect(after.configuration).toEqual(before.configuration);

    // 先接收真实元数据，再保存并发更新，最后放行真实配置请求。
    const metadata = page.waitForResponse(
      (response) =>
        response.request().method() === 'GET' &&
        new URL(response.url()).pathname === endpoint,
    );
    await page.route(
      (url) => url.pathname === configEndpoint,
      async (route) => {
        expect((await metadata).status()).toBe(200);
        const concurrent = await request.put(configEndpoint, {
          headers,
          data: {
            name,
            avatar,
            tags,
            visibility: 'public',
            description: '并发修改的简介',
            expected_version_id: after.id,
            configuration: {
              ...after.configuration,
              soul_md: '并发修改的 SOUL',
              prompts: undefined,
            },
          },
        });
        expect(concurrent.status()).toBe(200);
        await route.continue();
      },
      { times: 1 },
    );
    await page.reload();
    await expect(page.getByRole('alert')).toHaveText(
      '配置在读取时发生更新，请重试。',
    );
    await expect(description).toHaveCount(0);
    await expect(page.getByRole('button', { name: '创建' })).toHaveCount(0);
    await page.getByRole('button', { name: '重试', exact: true }).click();
    await expect(description).toHaveValue('并发修改的简介');
    await expect(soul).toHaveValue('并发修改的 SOUL');
  } finally {
    try {
      if (endpoint) {
        expect((await request.delete(endpoint, { headers })).status()).toBe(
          204,
        );
        expect((await request.get(endpoint, { headers })).status()).toBe(404);
      }
    } finally {
      expect(
        (await request.post('/api/v1/auth/logout', { headers })).status(),
      ).toBe(204);
    }
  }
});
