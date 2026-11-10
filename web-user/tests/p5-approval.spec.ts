import { mkdir } from 'node:fs/promises';
import { join } from 'node:path';
import { expect, test } from '@playwright/test';

// 使用生产前端、真实审批 API、PostgreSQL 与百炼。截图仅在指定目录时保存。
test.skip(
  process.env.XA_CHAT_INTEGRATION !== '1',
  '通过 make integration 启动真实服务',
);

test('输入区审批可刷新和键盘操作，历史留下收据，工具状态可切换', async ({
  page,
  request,
  browser,
}) => {
  await page.setViewportSize({ width: 1280, height: 800 });
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
  const actions = page.getByRole('region', { name: '需要确认遗忘记忆' });
  // 一个回复容器可以包含多个正文段，按整条回复断言。
  const reply = page
    .locator('.message-row.assistant')
    .last()
    .locator('.assistant-bubble');
  const approve = actions.getByRole('button', { name: /^允许本次，忘掉/ });
  const reject = actions.getByRole('button', { name: /^拒绝，忘掉/ });
  const conversationOption = actions.getByRole('button', {
    name: /^本对话不再询问，忘掉/,
  });
  const agentOption = actions.getByRole('button', {
    name: /^以后在这个 Agent 上都允许，忘掉/,
  });
  const receipt = rail.getByTestId('approval-receipt');
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
    await expect(actions.getByTestId('approval-sentence')).toHaveText(
      `要忘掉「${content}」这条记忆。`,
      { timeout: 90000 },
    );
    await expect(summary).toHaveText('等待你确认：遗忘记忆');
    await expect(summary).toHaveAttribute('aria-expanded', 'false');
    await expect(rail.getByRole('button')).toHaveCount(1);
    await expect(
      page.getByTestId('composer-stack').getByTestId('approval-dock'),
    ).toBeVisible();
    await expect(
      page.getByTestId('composer-stack').getByRole('textbox', { name: '消息' }),
    ).toBeDisabled();
    await expect(composer).toHaveAttribute('placeholder', '请先确认上方的操作');
    await expect(stop).toBeVisible();
    await expect(page.getByRole('button', { name: '发送消息' })).toHaveCount(0);
    await expect(approve).toBeFocused();
    await expect(approve).toHaveCSS('outline-style', 'none');
    await expect(
      actions
        .getByRole('button')
        .filter({ has: page.locator('.approval-option-number') }),
    ).toHaveText([
      '1允许本次',
      '2本对话不再询问',
      '3以后在这个 Agent 上都允许',
      '4拒绝',
    ]);
    await expect(actions.getByTestId('approval-countdown')).toHaveCount(0);
    await expect(actions.getByTestId('approval-arguments')).toHaveCount(0);
    await expect(actions).not.toContainText(id);
    await expect(agentOption).toHaveAccessibleDescription(/只影响之后的对话/);
  };

  const coffee = await add('我喜欢手冲咖啡');
  await page.reload({ waitUntil: 'domcontentloaded' });
  await expect(page.getByRole('button', { name: '查看记忆' })).toContainText(
    '1 条',
  );
  await forget('我喜欢手冲咖啡', coffee);
  await expect(
    page.getByRole('heading', { name: agentName, level: 1 }),
  ).toHaveCount(0);
  const modelLabel = page
    .getByRole('combobox', { name: '模型' })
    .locator('span')
    .first();
  await expect
    .poll(() =>
      modelLabel.evaluate((node) => node.scrollWidth <= node.clientWidth),
    )
    .toBe(true);
  await shot('01-waiting');
  await page.reload({ waitUntil: 'domcontentloaded' });
  await expect(approve).toBeVisible({ timeout: 20000 });
  await expect(actions).toContainText('我喜欢手冲咖啡');
  await shot('02-refresh-waiting');
  const details = actions.getByRole('button', { name: '详情' });
  await expect(details).toHaveAttribute('aria-expanded', 'false');
  await details.click();
  await expect(actions.getByTestId('approval-arguments')).toContainText(coffee);
  await details.press('4');
  await expect(actions).toBeVisible();
  await details.click();
  await approve.focus();
  await approve.press('ArrowDown');
  await expect(conversationOption).toBeFocused();
  await expect(conversationOption).toHaveCSS('outline-style', 'solid');
  await conversationOption.press('ArrowUp');
  await expect(approve).toBeFocused();
  await approve.press('Enter');
  await expect(reply).toContainText(/已经删除这条记忆/, {
    timeout: 90000,
  });
  await expect(stop).toBeHidden();
  await expect(actions).toHaveCount(0);
  await expect(tool).toHaveAttribute('data-tool-status', 'succeeded');
  await expect(receipt).toHaveText('已允许本次');
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
  await page.reload({ waitUntil: 'domcontentloaded' });
  await expect(approve).toBeFocused();
  await expect(approve).toHaveCSS('outline-style', 'none');
  const stack = await page.getByTestId('composer-stack').boundingBox();
  expect(stack?.width).toBeGreaterThanOrEqual(360);
  const buttons = await Promise.all(
    [approve, conversationOption, agentOption, reject].map((option) =>
      option.boundingBox(),
    ),
  );
  for (const [index, box] of buttons.entries()) {
    expect(box?.height).toBeGreaterThanOrEqual(44);
    expect(box?.width).toBe(buttons[0]?.width);
    if (index > 0) expect(box?.y).toBeGreaterThan(buttons[index - 1]?.y ?? 0);
  }
  await shot('04-narrow-waiting');
  await page.setViewportSize({ width: 1280, height: 800 });
  await approve.press('4');
  await expect(reply).toContainText(/没有删除这条记忆/, {
    timeout: 90000,
  });
  await expect(stop).toBeHidden();
  await expect(actions).toHaveCount(0);
  await expect(tool).toHaveAttribute('data-tool-status', 'rejected');
  await expect(receipt).toHaveText('已拒绝忘掉「我喜欢乌龙茶」');
  await expect(rail.locator('pre')).toHaveCount(0);
  await expect(rail).not.toContainText(tea);
  await shot('05-rejected');
  const active = await request.get('/api/v1/memories?status=active', {
    headers,
  });
  expect((await active.json()).items).toEqual(
    expect.arrayContaining([expect.objectContaining({ id: tea })]),
  );

  const toolSettings = page.getByRole('region', { name: '工具设置' });
  const ask = toolSettings.getByRole('button', { name: '遗忘记忆：需确认' });
  const auto = toolSettings.getByRole('button', { name: '遗忘记忆：自动运行' });
  await expect(toolSettings.getByRole('checkbox')).toHaveCount(0);
  await ask.click();
  await expect(auto).toBeEnabled();
  await shot('06-agent-tool-switch');
  await auto.click();
  await expect(ask).toBeEnabled();

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

  const extra = await add('我喜欢冷萃咖啡');
  await forget('我喜欢冷萃咖啡', extra);
  await approve.press('3');
  await expect(receipt).toHaveText(
    '已允许，以后在这个 Agent 上都不再询问遗忘记忆',
    { timeout: 90000 },
  );
  await expect(auto).toBeEnabled();
  await expect(stop).toBeHidden();
  await page.reload({ waitUntil: 'domcontentloaded' });
  await expect(receipt).toHaveText(
    '已允许，以后在这个 Agent 上都不再询问遗忘记忆',
  );
});
