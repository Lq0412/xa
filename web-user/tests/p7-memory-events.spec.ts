import { expect, type Response, test } from '@playwright/test';

// 通过生产前端、真实工具、PostgreSQL 和百炼验证流式记忆联动。
test.skip(
  process.env.XA_CHAT_INTEGRATION !== '1',
  '通过 make integration 启动真实服务',
);

test('工具写入和遗忘在回复结束前刷新记忆，恢复时不重复显示', async ({
  page,
  request,
}) => {
  test.setTimeout(300000);
  const pageErrors: string[] = [];
  page.on('pageerror', (error) => pageErrors.push(error.message));
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
  const response = await request.post('/api/v1/agents', {
    headers,
    data: {
      name: '记忆联动验收 Agent',
      visibility: 'private',
      configuration: {
        model: process.env.XA_P5_MODEL || 'qwen3.8-flash',
        enable_thinking: false,
        soul_md:
          '按用户指定顺序逐个调用工具。收到第一个工具的结果后，下一步只调用 current_time，等到结果后才回复。每个工具仅调用一次。',
        tools: [
          { name: 'memory_write', enabled: true, approval: 'never' },
          { name: 'memory_forget', enabled: true, approval: 'never' },
          { name: 'current_time', enabled: true, approval: 'always' },
        ],
      },
    },
  });
  expect(response.status()).toBe(201);
  const { agent } = await response.json();
  await page.goto(`/chat?agentId=${encodeURIComponent(agent.id)}`);

  const panel = page.getByRole('region', { name: 'Agent 配置' });
  const card = panel.getByRole('button', { name: '查看记忆' });
  const composer = page.getByRole('textbox', { name: '消息', exact: true });
  const stop = page.getByRole('button', { name: '停止生成' });
  // 用真实审批挂起下一次工具调用，确保断言发生在回复结束前。
  const approval = page.getByTestId('approval-dock');
  const approve = approval.getByRole('button', { name: /^允许本次/ });
  const preview = panel.getByRole('switch', { name: 'memory.md 预览' });
  const stats = panel.getByText(/^生效 \d+ · 归档/);
  const content = '我喜欢用木质书签标记读到的章节。';
  const row = panel.getByText(content, { exact: true });
  const send = async (message: string) => {
    await composer.fill(message);
    await composer.press('Enter');
  };
  const getMemories = async (status: string) => {
    const result = await request.get('/api/v1/memories', {
      headers,
      params: {
        scope: 'all_visible',
        agent_id: agent.id,
        status,
        limit: '200',
      },
    });
    expect(result.ok()).toBeTruthy();
    const { items } = (await result.json()) as {
      items: { id: string; content: string; source: string }[];
    };
    return items;
  };

  await expect(card).toContainText(/\d+ 条/);
  const initialCount = Number((await card.innerText()).match(/(\d+) 条/)?.[1]);
  await card.click();
  await expect(stats).toBeVisible();
  const initialActive = Number(
    (await stats.innerText()).match(/生效 (\d+)/)?.[1],
  );
  await expect(row).toHaveCount(0);

  await send(
    `先调用 memory_write，参数为 ${JSON.stringify({ content, namespace: 'preferences', subject_key: 'reading.bookmark', kind: 'preference', importance: 10 })}。必须等它成功后再调用 current_time。`,
  );
  await expect(approve).toBeVisible({ timeout: 90000 });
  await expect(row).toHaveCount(1);
  await expect(stats).toContainText(`生效 ${initialActive + 1} ·`);
  await expect(stop).toBeVisible();
  const written = (await getMemories('active')).filter(
    (item) => item.content === content,
  );
  expect(written).toEqual([expect.objectContaining({ source: 'tool' })]);

  await panel.getByRole('button', { name: '返回' }).click();
  await expect(card).toHaveText(new RegExp(`^${initialCount + 1} 条`));
  const replay = page.waitForRequest(/\/api\/v1\/runs\/[^/]+\/events\?after=/);
  await page.reload();
  const runPath = new URL((await replay).url()).pathname.replace(
    /\/events$/,
    '',
  );
  await expect(approve).toBeVisible();
  await card.click();
  await expect(row).toHaveCount(1);
  await expect(stats).toContainText(`生效 ${initialActive + 1} ·`);
  await preview.click();
  await expect(panel.getByText(content, { exact: false })).toBeVisible();
  const previewPath = `/api/v1/agents/${agent.id}/memory`;
  const previewStatuses: number[] = [];
  const recordPreview = (result: Response) => {
    if (new URL(result.url()).pathname === previewPath)
      previewStatuses.push(result.status());
  };
  page.on('response', recordPreview);
  try {
    await approve.click();
    await expect(stop).toBeHidden({ timeout: 90000 });
    const runResponse = await request.get(runPath, { headers });
    expect(runResponse.ok()).toBeTruthy();
    const { status, error_code, error } = await runResponse.json();
    expect({
      status,
      error_code,
      error,
      visibleErrors: await page.getByRole('alert').allTextContents(),
    }).toEqual({
      status: 'completed',
      error_code: '',
      error: '',
      visibleErrors: [],
    });
    expect(pageErrors, '轮次完成时的浏览器异常').toEqual([]);
    await expect(page.getByRole('log', { name: '对话消息' })).toBeVisible();
    await expect(composer).toBeVisible();
    await expect(
      panel.getByRole('heading', { name: '记忆', exact: true }),
    ).toBeVisible();
    await expect(preview).toBeChecked();
    await expect
      .poll(() => previewStatuses, {
        timeout: 12000,
        message: '轮次完成后重新读取已打开的 Markdown 预览',
      })
      .toContain(200);
  } finally {
    page.off('response', recordPreview);
  }

  // 仅断开这一次浏览器的预览请求，其余请求继续访问真实服务。
  await page.route(`**${previewPath}`, (route) => route.abort(), { times: 1 });
  await send(
    `先调用 memory_forget，使用记忆 ID ${written[0].id}，原因填写「用户要求删除」。必须等它成功后再调用 current_time。`,
  );
  await expect(approve).toBeVisible({ timeout: 90000 });
  await expect(panel.getByRole('alert')).toContainText('无法连接服务');
  await expect(stats).toContainText(`生效 ${initialActive} ·`);
  await panel.getByRole('button', { name: '重新读取' }).click();
  await expect(panel.getByRole('alert')).toHaveCount(0);
  await expect(panel.getByText(content, { exact: false })).toHaveCount(0);
  await expect(stop).toBeVisible();
  expect(await getMemories('deleted')).toEqual(
    expect.arrayContaining([expect.objectContaining({ id: written[0].id })]),
  );

  await preview.click();
  await expect(row).toHaveCount(0);
  await panel.getByRole('button', { name: '已删除' }).click();
  await expect(row).toHaveCount(1);
  await panel.getByRole('button', { name: '返回' }).click();
  await expect(card).toHaveText(new RegExp(`^${initialCount} 条`));
  await approve.click();
  await expect(stop).toBeHidden({ timeout: 90000 });
  expect(pageErrors, '浏览器异常').toEqual([]);
});
