import { expect, type Page, test } from '@playwright/test';

// 账号和真实 Agent 由 Go 集成测试创建、清理；模型调用只允许在 make integration 中运行。
if (process.env.XA_CHAT_INTEGRATION !== '1')
  throw new Error('请运行 XA_BROWSER_INTEGRATION=1 make integration');
const primaryID = process.env.XA_E2E_PRIMARY_ID || '';
const publicID = process.env.XA_E2E_PUBLIC_ID || '';
const publicName = process.env.XA_E2E_PUBLIC_NAME || '';

/** 基于浏览器布局坐标检查四栏、固定输入区和屏幕边界。 */
async function expectLayout(page: Page) {
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  ).toBe(true);
  const rects = await page
    .locator('.rail-links, .rail-bottom, .message-composer')
    .evaluateAll((nodes) =>
      nodes.map((node) => {
        const r = node.getBoundingClientRect();
        return { top: r.top, bottom: r.bottom, height: r.height };
      }),
    );
  expect(rects).toHaveLength(3);
  const height = page.viewportSize()?.height || 0;
  expect(
    Math.abs(rects[0].top + rects[0].height / 2 - height / 2),
  ).toBeLessThan(2);
  expect(rects[1].top).toBeGreaterThan(rects[0].bottom);
  expect(rects[1].bottom).toBeLessThanOrEqual(height);
  expect(rects[2].bottom).toBeLessThanOrEqual(height);
}

test('四栏布局、公共 Agent 添加、版本编辑、历史隔离和真实流式对话', async ({
  page,
  request,
  context,
}) => {
  const errors: string[] = [];
  page.on('pageerror', (cause) => errors.push(cause.message));
  await page.goto('/login');
  await page
    .getByLabel('邮箱', { exact: true })
    .fill(process.env.XA_E2E_EMAIL || '');
  await page
    .getByLabel('密码', { exact: true })
    .fill(process.env.XA_E2E_PASSWORD || '');
  await page.getByRole('button', { name: /登\s*录/ }).click();
  await expect(page).toHaveURL(/\/explore$/);
  const token = await page.evaluate(() =>
    localStorage.getItem('xa.user.session'),
  );
  const headers = { Authorization: `Bearer ${token}` };
  try {
    expect((await request.get('/api/v1/agent-library')).status()).toBe(401);
    expect(
      (
        await request.put(`/api/v1/agent-library/${primaryID}`, { headers })
      ).status(),
    ).toBe(404);
    const initial = (
      await (await request.get('/api/v1/agent-library', { headers })).json()
    ).agents;
    expect(initial.map((agent: { id: string }) => agent.id)).toEqual([
      primaryID,
    ]);
    await page
      .getByRole('navigation', { name: '主导航' })
      .getByRole('link', { name: '对话', exact: true })
      .click();
    await expect(page).toHaveURL(new RegExp(`agentId=${primaryID}`));
    const sidebar = page.getByRole('navigation', { name: 'Agent 列表' });
    await expect(
      sidebar.getByRole('button', { name: '我的主 Agent', exact: true }),
    ).toHaveAttribute('aria-current', 'page');
    const details = page.getByRole('region', { name: 'Agent 资料' });
    await expect(details).toContainText('qwen3.8-flash');
    await expect(details).toContainText('请按用户要求用中文回答。');
    await expect(details.locator('time')).toBeVisible();
    await expectLayout(page);
    const columns = await page
      .locator(
        '.icon-rail, .agent-sidebar, .conversation-main, .agent-details-panel',
      )
      .evaluateAll((nodes) =>
        nodes.map((node) => {
          const r = node.getBoundingClientRect();
          return { left: r.left, right: r.right };
        }),
      );
    expect(columns).toHaveLength(4);
    for (let i = 1; i < columns.length; i++)
      expect(columns[i].left).toBeGreaterThanOrEqual(columns[i - 1].right - 1);
    await details.getByRole('button', { name: '收起 Agent 详情' }).click();
    await page.reload();
    await expect(page.locator('.agent-details-panel')).toHaveCount(0);
    await page
      .getByRole('button', { name: '展开 Agent 详情', exact: true })
      .click();
    await page.reload();
    await expect(details).toBeVisible();

    await context.setOffline(true);
    await details.getByRole('button', { name: /编\s*辑/ }).click();
    const editor = page.getByRole('dialog', { name: '编辑 Agent' });
    await expect(editor.getByRole('alert')).toContainText('无法连接服务');
    await expect(editor.getByLabel('名称', { exact: true })).toBeDisabled();
    await expect(
      editor.getByRole('button', { name: /保\s*存/ }),
    ).toBeDisabled();
    await context.setOffline(false);
    await editor.getByRole('button', { name: '重新加载' }).click();
    await expect(editor.getByLabel('名称', { exact: true })).toHaveValue(
      '我的主 Agent',
    );
    await editor.getByLabel('介绍', { exact: true }).fill('已经编辑的真实介绍');
    await editor
      .getByLabel('系统提示词', { exact: true })
      .fill('请用中文回答，遵循用户要求的格式。');
    await editor.getByRole('button', { name: /保\s*存/ }).click();
    await expect(editor).toBeHidden();
    await expect(details).toContainText('已经编辑的真实介绍');
    await expect(details).toContainText('请用中文回答，遵循用户要求的格式。');
    const config = (
      await (
        await request.get(`/api/v1/agents/${primaryID}/config`, { headers })
      ).json()
    ).configuration;
    expect(config.prompts).toHaveLength(2);
    expect(config.prompts[1].content).toBe('保留这个未启用片段');

    await page.getByRole('button', { name: '新建 Agent' }).click();
    const create = page.getByRole('dialog', { name: '新建 Agent' });
    await create.getByLabel('名称', { exact: true }).fill('第二个助手');
    await create
      .getByLabel('系统提示词', { exact: true })
      .fill('请用中文回答。');
    await create.getByRole('button', { name: /创\s*建/ }).click();
    await expect(create).toBeHidden();
    await expect(page.locator('.conversation-identity')).toContainText(
      '第二个助手',
    );
    const secondAgentID = new URL(page.url()).searchParams.get('agentId');
    await expect(
      sidebar.locator('.agent-sidebar-select').first(),
    ).toContainText('我的主 Agent');
    await page.getByRole('textbox', { name: '搜索对话 Agent' }).fill('第二');
    await expect(sidebar.locator('.agent-sidebar-select')).toHaveCount(1);
    await page.getByRole('textbox', { name: '搜索对话 Agent' }).fill('');

    await page
      .getByRole('navigation', { name: '主导航' })
      .getByRole('link', { name: '发现 Agent', exact: true })
      .click();
    await expect(
      page.getByRole('navigation', { name: '工作空间导航' }),
    ).toHaveCount(0);
    await expect(
      page.getByRole('link', { name: '我的 Agent', exact: true }),
    ).toHaveCount(1);
    await page
      .getByRole('textbox', { name: '搜索 Agent', exact: true })
      .fill(publicName);
    await page
      .getByRole('article')
      .filter({ hasText: publicName })
      .getByRole('button', { name: '添加并对话' })
      .click();
    await expect(page).toHaveURL(new RegExp(`agentId=${publicID}`));
    await expect(details).toContainText('你是公共助手。');
    await expect(details.getByRole('button', { name: /编\s*辑/ })).toHaveCount(
      0,
    );
    expect(
      (
        await request.put(`/api/v1/agent-library/${publicID}`, { headers })
      ).status(),
    ).toBe(204);
    await page.reload();
    const stored = (
      await (await request.get('/api/v1/agent-library', { headers })).json()
    ).agents;
    expect(
      stored.filter((agent: { id: string }) => agent.id === publicID),
    ).toHaveLength(1);
    await expect(
      sidebar.getByRole('button', { name: publicName, exact: true }),
    ).toHaveAttribute('aria-current', 'page');
    await sidebar
      .getByRole('button', { name: '我的主 Agent', exact: true })
      .click();
    const input = page.getByRole('textbox', { name: '消息', exact: true });
    await expect(input).toBeEnabled();
    const firstPrompt =
      '请用 Markdown 写一个两行表格，再附一行 JavaScript 代码块。不要额外解释。';
    await input.fill(firstPrompt);
    await input.dispatchEvent('keydown', {
      key: 'Enter',
      isComposing: true,
      keyCode: 229,
    });
    await expect(page.locator('.message-row')).toHaveCount(0);
    await input.press('Enter');
    await input.press('Enter');
    await expect(page).toHaveURL(/conversationId=conv_/);
    const firstURL = page.url();
    await expect(page.locator('.message-row.assistant pre code')).toBeVisible({
      timeout: 60000,
    });
    await expect(page.getByRole('button', { name: '停止生成' })).toBeHidden({
      timeout: 60000,
    });
    await expect(page.locator('.message-row')).toHaveCount(2);
    await expect(page.locator('.message-row.assistant table')).toBeVisible();
    await input.fill('主 Agent 的草稿');
    await sidebar
      .getByRole('button', { name: '第二个助手', exact: true })
      .click();
    await expect(input).toHaveValue('');
    await expect(page.locator('.message-row')).toHaveCount(0);
    const longPrompt =
      '请列出100条前端可访问性建议，每条至少30字，使用编号列表，直接开始不要省略。';
    await input.fill(longPrompt);
    await input.press('Enter');
    await expect(page).toHaveURL(/conversationId=conv_/);
    const secondURL = page.url();
    expect(new URL(secondURL).searchParams.get('agentId')).toBe(secondAgentID);
    await expect
      .poll(
        async () =>
          (await page.locator('.message-row.assistant').textContent())
            ?.length || 0,
        { timeout: 60000 },
      )
      .toBeGreaterThan(800);
    await page.locator('.conversation-body').evaluate((node) => {
      node.scrollTop = 0;
    });
    await expect(
      page.getByRole('button', { name: '回到最新消息' }),
    ).toBeVisible();
    await sidebar
      .getByRole('button', { name: firstPrompt.slice(0, 30), exact: true })
      .click();
    await expect(page).toHaveURL(firstURL);
    await expect(input).toHaveValue('主 Agent 的草稿');
    await expect(page.locator('.message-row.user')).toHaveText(firstPrompt);
    await sidebar
      .getByRole('button', { name: longPrompt.slice(0, 30), exact: true })
      .click();
    await page.getByRole('button', { name: '停止生成' }).click();
    const secondID = new URL(secondURL).searchParams.get('conversationId');
    await expect
      .poll(
        async () =>
          (
            await (
              await request.get(`/api/v1/conversations/${secondID}/messages`, {
                headers,
              })
            ).json()
          ).messages.at(-1)?.error,
      )
      .toBe('已停止生成');
    await page.reload();
    await expect(page).toHaveURL(secondURL);
    await expect(page.getByText('已停止生成', { exact: true })).toBeVisible();
    await expect(
      sidebar.getByRole('button', {
        name: longPrompt.slice(0, 30),
        exact: true,
      }),
    ).toHaveAttribute('aria-current', 'page');
    await sidebar
      .getByRole('button', { name: '与 第二个助手 新建会话' })
      .click();
    await expect(page).not.toHaveURL(/conversationId=/);
    await expect(page.locator('.message-row')).toHaveCount(0);
    await sidebar
      .getByRole('button', { name: longPrompt.slice(0, 30), exact: true })
      .click();
    await expect(page).toHaveURL(secondURL);

    await page.setViewportSize({ width: 390, height: 844 });
    await expect(details).toBeHidden();
    await expectLayout(page);
    await page.getByRole('button', { name: '打开 Agent 列表' }).click();
    await expect(sidebar).toBeVisible();
    await page.getByRole('button', { name: '关闭 Agent 列表' }).click();
    await page
      .getByRole('button', { name: '展开 Agent 详情', exact: true })
      .click();
    await expect(details).toBeVisible();
    await details.getByRole('button', { name: '收起 Agent 详情' }).click();
    await page.setViewportSize({ width: 844, height: 390 });
    await expectLayout(page);
    await context.setOffline(true);
    await input.fill('断网时发送');
    await input.press('Enter');
    await expect(page.getByRole('alert')).toContainText('无法连接服务');
    await context.setOffline(false);
    await page.getByRole('button', { name: '重新加载记录' }).click();
    await expect(page.getByRole('alert')).toBeHidden();
    await expect(page.locator('.message-row')).toHaveCount(2);
    expect(errors).toEqual([]);
  } finally {
    await context.setOffline(false);
    expect(
      (await request.post('/api/v1/auth/logout', { headers })).status(),
    ).toBe(204);
  }
});
