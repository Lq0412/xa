import { mkdir } from 'node:fs/promises';
import { join } from 'node:path';
import { expect, type Locator, test } from '@playwright/test';

test.skip(
  process.env.XA_CHAT_INTEGRATION !== '1',
  '需要真实用户端、认证 API 和 PostgreSQL',
);

test('工具设置保留其他绑定字段，并拒绝过期版本覆盖', async ({
  page,
  request,
}) => {
  const shotDir = process.env.XA_P7_SHOTS;
  if (shotDir) await mkdir(shotDir, { recursive: true });
  const shot = async (name: string, target: Locator) => {
    if (shotDir)
      await target.screenshot({ path: join(shotDir, `${name}.png`) });
  };
  await page.goto('/login');
  await page
    .getByLabel('邮箱', { exact: true })
    .fill(process.env.XA_E2E_EMAIL || '');
  await page
    .getByLabel('密码', { exact: true })
    .fill(process.env.XA_E2E_PASSWORD || '');
  await page.getByRole('button', { name: '登录', exact: true }).click();
  await expect(page).toHaveURL(/\/chat/);
  const token = await page.evaluate(() =>
    localStorage.getItem('xa.user.session'),
  );
  const headers = { Authorization: `Bearer ${token}` };
  const created = await request.post('/api/v1/agents', {
    headers,
    data: {
      name: '工具配置验收 Agent',
      visibility: 'private',
      configuration: {
        soul_md: '请使用中文回答。',
        tools: [
          {
            name: 'current_time',
            enabled: true,
            allowed_roles: ['user', 'admin'],
            configuration: { timezone: 'Asia/Shanghai', delay_ms: 0 },
          },
          { name: 'memory_search', enabled: false },
        ],
        runtime_limits: { max_wall_time_ms: 90000 },
      },
    },
  });
  expect(created.status()).toBe(201);
  const { agent } = await created.json();
  const agentPath = `/api/v1/agents/${encodeURIComponent(agent.id)}`;
  const getConfig = async () => {
    const response = await request.get(`${agentPath}/config`, { headers });
    expect(response.ok()).toBe(true);
    return response.json();
  };

  await page.goto(`/chat?agentId=${encodeURIComponent(agent.id)}`);
  const tools = page.getByRole('region', { name: '工具设置' });
  await expect(tools.getByText('即将上线').first()).toBeVisible();
  const timeDescription = '返回当前日期与时间';
  await expect(tools.getByText(timeDescription)).toHaveCount(0);
  const enabled = tools.getByRole('button', { name: '启用当前时间' });
  await expect(enabled).toHaveAttribute('aria-pressed', 'true');
  await enabled.click();
  await expect(enabled).toHaveAttribute('aria-pressed', 'false');
  await enabled.click();
  await expect(enabled).toHaveAttribute('aria-pressed', 'true');
  await tools.getByRole('button', { name: '当前时间：自动运行' }).click();
  await expect(
    tools.getByRole('button', { name: '当前时间：需确认' }),
  ).toBeVisible();
  await shot('01-tool-list', tools);
  await tools.getByRole('button', { name: '配置当前时间' }).click();
  const tool = page.getByRole('dialog', { name: '配置当前时间' });
  await expect(tool.getByText(timeDescription)).toBeVisible();
  await expect(
    tool.getByRole('combobox', { name: '审批', exact: true }),
  ).toHaveValue('always');
  const timezone = tool.getByRole('combobox', { name: '时区', exact: true });
  await timezone.selectOption('UTC');
  await tool.getByLabel('每次运行最多调用次数', { exact: true }).fill('3');
  await tool.getByLabel('单次超时（毫秒）', { exact: true }).fill('5000');
  await shot('02-tool-timezone', tool);
  await tool.getByRole('button', { name: '保存', exact: true }).click();
  await expect(tool).toBeHidden();

  await tools.getByRole('button', { name: '运行限制' }).click();
  const limits = page.getByRole('dialog', { name: '运行限制' });
  await limits.getByLabel('最大模型轮次', { exact: true }).fill('5');
  await limits.getByLabel('最大工具调用次数', { exact: true }).fill('7');
  await limits.getByLabel('最大并行工具数', { exact: true }).fill('2');
  await shot('03-runtime-limits', limits);
  await limits.getByRole('button', { name: '保存', exact: true }).click();
  await expect(limits).toBeHidden();
  const saved = await getConfig();
  expect(saved.id).not.toBe(agent.current_version_id);
  expect(saved.configuration.tools).toEqual([
    {
      name: 'current_time',
      enabled: true,
      approval: 'always',
      max_calls: 3,
      timeout_ms: 5000,
      allowed_roles: ['user', 'admin'],
      configuration: { timezone: 'UTC', delay_ms: 0 },
    },
    { name: 'memory_search', enabled: false },
  ]);
  expect(saved.configuration.runtime_limits).toMatchObject({
    max_wall_time_ms: 90000,
    max_model_turns: 5,
    max_tool_calls: 7,
    max_parallel_tools: 2,
  });

  await tools.getByRole('button', { name: '配置当前时间' }).click();
  await expect(timezone).toHaveValue('UTC');
  const update = await request.put(`${agentPath}/tools`, {
    headers,
    data: {
      expected_version_id: saved.id,
      tools: [
        { ...saved.configuration.tools[0], timeout_ms: 6000 },
        saved.configuration.tools[1],
      ],
    },
  });
  expect(update.ok()).toBe(true);
  const newer = await update.json();
  const stale = await request.put(`${agentPath}/tools`, {
    headers,
    data: { expected_version_id: saved.id, tools: saved.configuration.tools },
  });
  expect(stale.status()).toBe(409);
  await tool.getByLabel('单次超时（毫秒）', { exact: true }).fill('7000');
  await tool.getByRole('button', { name: '保存', exact: true }).click();
  await expect(tools.getByRole('alert')).toHaveText('配置已被更新，请刷新');
  expect((await getConfig()).configuration.tools[0].timeout_ms).toBe(6000);
  await tools.getByRole('button', { name: '刷新', exact: true }).click();
  await tools.getByRole('button', { name: '配置当前时间' }).click();
  await expect(
    tool.getByLabel('单次超时（毫秒）', { exact: true }),
  ).toHaveValue('6000');

  const memoryOff = await request.put(`${agentPath}/memory-policy`, {
    headers,
    data: { expected_version_id: newer.current_version_id, enabled: false },
  });
  expect(memoryOff.ok()).toBe(true);
  await page.reload();
  const memorySearch = tools.getByRole('button', { name: '启用搜索记忆' });
  await expect(memorySearch).toHaveText('需开启记忆');
  await expect(memorySearch).toBeDisabled();

  const publicID = process.env.XA_E2E_PUBLIC_ID;
  if (publicID) {
    const publicPath = `/api/v1/agents/${encodeURIComponent(publicID)}/tools`;
    const denied = await request.get(publicPath, { headers });
    expect([403, 404]).toContain(denied.status());
    const editDenied = await request.put(publicPath, {
      headers,
      data: { expected_version_id: newer.current_version_id, tools: [] },
    });
    expect([403, 404]).toContain(editDenied.status());
  }
});
