import { mkdir } from 'node:fs/promises';
import { join } from 'node:path';
import { expect, test } from '@playwright/test';

// 使用生产前端、真实审批 API、PostgreSQL 与百炼。截图仅在指定目录时保存。
test.skip(
  process.env.XA_CHAT_INTEGRATION !== '1',
  '通过 make integration 启动真实服务',
);

test('过程轨审批在刷新后恢复，批准后执行，拒绝后保留记忆', async ({
  page,
  request,
  browser,
}) => {
  const shotDir = process.env.XA_P5_SHOTS;
  if (shotDir) await mkdir(shotDir, { recursive: true });
  const shot = async (name: string, target = page) => {
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
  const agentName = '审批验收 Agent';
  const agentResponse = await request.post('/api/v1/agents', {
    headers,
    data: {
      name: agentName,
      visibility: 'private',
      configuration: {
        model: process.env.XA_P5_MODEL || 'qwen3.8-flash',
        enable_thinking: false,
        soul_md:
          '用户提供记忆 ID 并要求忘掉时，立即调用 memory_forget，传入该 ID 和理由。成功后说「已经删除这条记忆」。如果用户拒绝，不重试，直接说「没有删除这条记忆」。',
        tools: [{ name: 'memory_forget', enabled: true, approval: 'always' }],
      },
    },
  });
  expect(agentResponse.status()).toBe(201);
  const { agent } = await agentResponse.json();
  await page.goto(`/chat?agentId=${encodeURIComponent(agent.id)}`);
  const composer = page.getByRole('textbox', { name: '消息', exact: true });
  const stop = page.getByRole('button', { name: '停止生成' });
  const rail = page.getByTestId('process-rail').last();
  const summary = rail.getByTestId('process-summary');
  const actions = rail.getByTestId('approval-actions');
  const reply = page
    .locator('.message-row.assistant')
    .last()
    .getByTestId('message-text-part');
  const approve = actions.getByRole('button', { name: /^允许本次遗忘记忆/ });
  const reject = actions.getByRole('button', { name: /^拒绝遗忘记忆/ });
  const tool = rail.getByTestId('tool-call-chip').last();
  const add = async (content: string) => {
    const result = await request.post('/api/v1/memories', {
      headers,
      data: { content, namespace: 'preferences', importance: 6 },
    });
    expect(result.status()).toBe(201);
    const memory = await result.json();
    return String(memory.id);
  };
  const forget = async (content: string, id: string) => {
    await composer.fill(
      `请忘掉「${content}」这条记忆。直接用 memory_forget，记忆 ID 为 ${id}。`,
    );
    await composer.press('Enter');
    await expect(actions).toContainText(content, { timeout: 90000 });
    await expect(summary).toContainText('需要确认：遗忘记忆');
    await expect(composer).toHaveAttribute('placeholder', '请先确认上方的操作');
    await expect(stop).toBeVisible();
    await expect(page.getByRole('button', { name: '发送消息' })).toHaveCount(0);
    await expect(approve).toBeFocused();
    await expect(actions).toHaveCSS('border-top-width', '0px');
  };

  const coffee = await add('我喜欢手冲咖啡');
  await page.reload({ waitUntil: 'domcontentloaded' });
  await expect(page.getByRole('button', { name: '查看记忆' })).toContainText(
    '1 条',
  );
  await forget('我喜欢手冲咖啡', coffee);
  await shot('01-waiting');
  await page.reload({ waitUntil: 'domcontentloaded' });
  await expect(approve).toBeVisible({ timeout: 20000 });
  await expect(actions).toContainText('我喜欢手冲咖啡');
  await shot('02-refresh-waiting');
  await approve.click();
  await expect(reply).toContainText(/已经删除这条记忆/, {
    timeout: 90000,
  });
  await expect(stop).toBeHidden();
  if ((await summary.getAttribute('aria-expanded')) !== 'true')
    await summary.click();
  await expect(tool).toHaveAttribute('data-tool-status', 'succeeded');
  await expect(tool).toContainText('已允许');
  const deleted = await request.get('/api/v1/memories?status=deleted', {
    headers,
  });
  expect((await deleted.json()).items).toEqual(
    expect.arrayContaining([expect.objectContaining({ id: coffee })]),
  );
  await expect(page.getByRole('button', { name: '查看记忆' })).toContainText(
    '0 条',
  );
  await shot('03-approved');

  const tea = await add('我喜欢乌龙茶');
  await forget('我喜欢乌龙茶', tea);
  await page.setViewportSize({ width: 390, height: 844 });
  await expect(approve).toBeVisible();
  const buttons = await Promise.all([
    approve.boundingBox(),
    reject.boundingBox(),
  ]);
  expect(buttons[0]?.height).toBeGreaterThanOrEqual(44);
  expect(buttons[1]?.height).toBeGreaterThanOrEqual(44);
  expect(buttons[1]?.y).toBeGreaterThan(buttons[0]?.y ?? 0);
  await shot('04-narrow-waiting');
  await page.setViewportSize({ width: 1440, height: 1000 });
  await reject.click();
  await expect(reply).toContainText(/没有删除这条记忆/, {
    timeout: 90000,
  });
  await expect(stop).toBeHidden();
  await expect(tool).toHaveAttribute('data-tool-status', 'rejected');
  await shot('05-rejected');
  const active = await request.get('/api/v1/memories?status=active', {
    headers,
  });
  expect((await active.json()).items).toEqual(
    expect.arrayContaining([expect.objectContaining({ id: tea })]),
  );

  const setting = page.getByRole('switch', { name: '遗忘记忆需要确认' });
  await expect(setting).toBeChecked();
  await setting.click();
  await expect(setting).not.toBeChecked();
  await shot('06-agent-tool-switch');
  await setting.click();
  await expect(setting).toBeChecked();

  const adminURL = process.env.XA_P5_ADMIN_URL;
  if (adminURL) {
    const admin = await browser.newPage({
      viewport: { width: 1600, height: 1000 },
    });
    await admin.goto(`${adminURL}/approvals`);
    await admin
      .getByRole('textbox', { name: '邮箱' })
      .fill(process.env.XA_P5_ADMIN_EMAIL || '');
    await admin
      .getByLabel('密码', { exact: true })
      .fill(process.env.XA_P5_ADMIN_PASSWORD || '');
    await admin.getByRole('button', { name: /登\s*录/ }).click();
    await expect(admin).toHaveURL(/\/approvals$/);
    const record = admin.getByRole('row').filter({ hasText: agentName });
    await expect(record.filter({ hasText: '已允许' })).toContainText(/\d+ 秒/);
    await expect(
      record.filter({ hasText: '已允许' }).getByRole('link'),
    ).toHaveCount(2);
    await expect(record.filter({ hasText: '未执行' })).toContainText(
      '遗忘记忆',
    );
    await shot('07-admin-approval-records', admin);
    await admin.close();
  }
});
