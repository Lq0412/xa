import { expect, type Page, test } from '@playwright/test';

// 仅由 make integration 启动，使用真实 PostgreSQL、认证与低成本模型。
if (process.env.XA_CHAT_INTEGRATION !== '1')
  throw new Error('请运行 XA_BROWSER_INTEGRATION=1 make integration');
const primaryID = process.env.XA_E2E_PRIMARY_ID || '';
const publicID = process.env.XA_E2E_PUBLIC_ID || '';
const publicName = process.env.XA_E2E_PUBLIC_NAME || '';

async function layout(page: Page) {
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  ).toBe(true);
  const height = page.viewportSize()?.height || 0;
  const rail = await page.locator('.rail-links').boundingBox();
  const bottom = await page.locator('.rail-bottom').boundingBox();
  const composer = await page.locator('.message-composer').boundingBox();
  expect(rail).not.toBeNull();
  expect(bottom).not.toBeNull();
  expect(composer).not.toBeNull();
  if (!rail || !bottom || !composer) throw Error('布局缺少区域');
  if (height > 500)
    expect(Math.abs(rail.y + rail.height / 2 - height / 2)).toBeLessThan(2);
  expect(bottom.y).toBeGreaterThanOrEqual(rail.y + rail.height);
  expect(composer.y + composer.height).toBeLessThanOrEqual(height);
  expect(await page.locator('[class^="ant-"],[class*=" ant-"]').count()).toBe(
    0,
  );
}

test('原生四栏、SOUL 版本、唯一对话、历史分页、代码复制与窄屏焦点', async ({
  page,
  request,
  context,
}) => {
  test.setTimeout(240000);
  const errors: string[] = [];
  page.on('pageerror', (error) => errors.push(error.message));
  await context.grantPermissions(['clipboard-read', 'clipboard-write']);
  await page.goto('/login');
  await page
    .getByLabel('邮箱', { exact: true })
    .fill(process.env.XA_E2E_EMAIL || '');
  await page
    .getByLabel('密码', { exact: true })
    .fill(process.env.XA_E2E_PASSWORD || '');
  await page.getByRole('button', { name: '登录', exact: true }).click();
  await expect(page).toHaveURL(new RegExp(`agentId=${primaryID}`));
  const token = await page.evaluate(() =>
    localStorage.getItem('xa.user.session'),
  );
  const headers = { Authorization: `Bearer ${token}` };
  const sidebar = page.getByRole('navigation', { name: '可对话 Agent' });
  const details = page.getByRole('region', { name: 'Agent 配置' });
  await expect(details).toContainText('请按用户要求用中文回答。');
  await expect(details.locator('textarea')).toHaveCount(0);
  await layout(page);
  const columns = await page
    .locator(
      '.icon-rail,.agent-sidebar,.conversation-main,.agent-details-panel',
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
  await details.getByRole('button', { name: '收起 Agent 配置' }).click();
  await page.reload();
  await expect(page.locator('.agent-details-panel')).toHaveCount(0);
  await page.getByRole('button', { name: '展开 Agent 配置' }).click();
  await expect(details).toBeVisible();
  await details.getByRole('button', { name: '编辑基本信息' }).click();
  const editor = page.getByRole('dialog', { name: '编辑基本信息' });
  await editor.getByLabel('简介', { exact: true }).fill('这是我的助手');
  await editor.getByRole('button', { name: '保存', exact: true }).click();
  await expect(editor).toBeHidden();
  await expect(details).toContainText('这是我的助手');
  await details.getByRole('button', { name: '编辑 SOUL.md' }).click();
  await details
    .getByRole('textbox', { name: 'SOUL.md', exact: true })
    .fill('请用中文回答，按照用户要求的格式输出。');
  await details.getByRole('button', { name: '预览', exact: true }).click();
  await expect(details.locator('.soul-preview')).toContainText(
    '按照用户要求的格式输出',
  );
  await details.getByRole('button', { name: '保存', exact: true }).click();
  await expect(details.locator('textarea')).toHaveCount(0);
  await expect(details.locator('.version-label')).toHaveText('v4');
  const input = page.getByRole('textbox', { name: '消息', exact: true });
  await input.fill(
    '请输出一个两行 Markdown 表格和 JavaScript 代码块，代码是 console.log("hello");。不要其他说明。',
  );
  await input.dispatchEvent('keydown', {
    key: 'Enter',
    isComposing: true,
    keyCode: 229,
  });
  await expect(page.locator('.message-row')).toHaveCount(0);
  await input.press('Enter');
  await expect(page.getByRole('button', { name: '停止生成' })).toBeHidden({
    timeout: 60000,
  });
  await expect(page.locator('.message-row.assistant pre code')).toBeVisible({
    timeout: 60000,
  });
  await expect(page.locator('.message-row.assistant table')).toBeVisible();
  await expect(page.locator('.message-row')).toHaveCount(2);
  await expect(page.locator('.message-row .agent-avatar')).toHaveCount(0);
  const code = await page.locator('.message-row pre').textContent();
  await page.getByRole('button', { name: '复制代码' }).click();
  expect(await page.evaluate(() => navigator.clipboard.readText())).toBe(code);
  expect(
    await page.locator('.message-row code .hljs-string').count(),
  ).toBeGreaterThan(0);
  const firstURL = page.url();
  const conversationID = new URL(firstURL).searchParams.get('conversationId');
  expect(conversationID).toBeTruthy();
  const same = await (
    await request.post('/api/v1/conversations', {
      headers,
      data: { agent_id: primaryID },
    })
  ).json();
  expect(same.id).toBe(conversationID);
  // 经真实模型写入足够历史，验证分页和上滚位置，不能用替换接口填充消息。
  for (let i = 0; i < 10; i++) {
    const response = await request.post(
      `/api/v1/conversations/${conversationID}/messages`,
      { headers, data: { content: `第 ${i + 1} 次检查，请只回复「好」。` } },
    );
    expect(response.ok()).toBe(true);
    expect(await response.text()).toContain('event: done');
  }
  await page.goto(`/chat?agentId=${primaryID}`);
  await expect(page).toHaveURL(firstURL);
  await expect(page.locator('.message-row')).toHaveCount(20);
  await page.locator('.conversation-body').evaluate((node) => {
    node.scrollTop = 0;
  });
  await expect(page.locator('.message-row')).toHaveCount(22);
  await expect(page.locator('.message-row.assistant pre code')).toBeVisible();
  await input.fill('主 Agent 的草稿');
  await page.getByRole('button', { name: '新建 Agent', exact: true }).click();
  const create = page.getByRole('dialog', { name: '新建 Agent' });
  await create.getByLabel('名称', { exact: true }).fill('第二个助手');
  await create.getByLabel('SOUL.md', { exact: true }).fill('请用中文回答。');
  await create.getByRole('button', { name: '创建', exact: true }).click();
  await expect(create).toBeHidden();
  await expect(page.locator('.conversation-title')).toHaveText('第二个助手');
  await expect(page.locator('.agent-sidebar-select').first()).toContainText(
    '我的主 Agent',
  );
  await details
    .getByRole('combobox', { name: '模型' })
    .selectOption('qwen3.8-flash');
  await details.getByRole('button', { name: '保存', exact: true }).click();
  await expect(
    details.getByRole('button', { name: '保存', exact: true }),
  ).toBeHidden();
  await sidebar
    .getByRole('button', { name: '我的主 Agent', exact: true })
    .click();
  await expect(input).toHaveValue('主 Agent 的草稿');
  await sidebar
    .getByRole('button', { name: '第二个助手', exact: true })
    .click();
  await expect(input).toHaveValue('');
  await input.fill('请列出100条前端可访问性建议，每条至少30字。');
  await input.press('Enter');
  await expect
    .poll(
      async () =>
        (await page.locator('.message-row.assistant').textContent())?.length ||
        0,
      { timeout: 60000 },
    )
    .toBeGreaterThan(200);
  await page.getByRole('button', { name: '停止生成' }).click();
  const secondID = new URL(page.url()).searchParams.get('conversationId');
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
  await expect(page.getByText('已停止生成', { exact: true })).toBeVisible();
  await page
    .getByRole('navigation', { name: '主导航' })
    .getByRole('link', { name: '发现 Agent' })
    .click();
  await page
    .getByRole('textbox', { name: '搜索 Agent', exact: true })
    .fill(publicName);
  await page.getByRole('button', { name: '添加并对话', exact: true }).click();
  await expect(page).toHaveURL(new RegExp(`agentId=${publicID}`));
  await expect(details).toContainText('你是公共助手。');
  await expect(details.locator('textarea,select')).toHaveCount(0);
  await expect(details.getByRole('button', { name: /编辑/ })).toHaveCount(0);
  await page.setViewportSize({ width: 390, height: 844 });
  await expect(details).toBeHidden();
  await layout(page);
  await page.getByRole('button', { name: '打开 Agent 列表' }).click();
  const sheet = page.getByRole('dialog', { name: '我的 Agent' });
  await expect(sheet).toBeVisible();
  await sheet
    .getByRole('button', { name: '我的主 Agent', exact: true })
    .click();
  await expect(sheet).toBeHidden();
  await page.getByRole('button', { name: '展开 Agent 配置' }).click();
  await expect(page.getByRole('dialog', { name: 'Agent 配置' })).toBeVisible();
  await page.keyboard.press('Escape');
  await expect(page.getByRole('dialog')).toHaveCount(0);
  await expect(
    page.getByRole('button', { name: '展开 Agent 配置' }),
  ).toBeFocused();
  await page.setViewportSize({ width: 844, height: 390 });
  await layout(page);
  expect(errors).toEqual([]);
  expect(
    (await request.post('/api/v1/auth/logout', { headers })).status(),
  ).toBe(204);
});
