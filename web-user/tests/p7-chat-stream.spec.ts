import { once } from 'node:events';
import { mkdir } from 'node:fs/promises';
import {
  createServer,
  request as forward,
  type ServerResponse,
} from 'node:http';
import { join } from 'node:path';
import { test as base, expect } from '@playwright/test';

const test = base.extend<{ disconnectStream: () => void }>({
  disconnectStream: async ({ page }, use) => {
    let target: URL | undefined;
    let stream: ServerResponse | undefined;
    // 转发首个 POST 的真实字节，测试可以关闭浏览器连接来制造网络故障。
    const proxy = createServer((incoming, outgoing) => {
      if (!target) throw new Error('缺少 SSE 上游地址');
      stream = outgoing;
      const upstream = forward(
        target,
        {
          method: incoming.method,
          headers: { ...incoming.headers, host: target.host },
        },
        (response) => {
          outgoing.writeHead(response.statusCode ?? 502, response.headers);
          response.pipe(outgoing);
        },
      );
      outgoing.on('close', () => upstream.destroy());
      upstream.on('error', () => outgoing.destroy());
      incoming.pipe(upstream);
    });
    try {
      await once(proxy.listen(0, '127.0.0.1'), 'listening');
      const address = proxy.address();
      if (!address || typeof address === 'string')
        throw new Error('SSE 中转缺少端口');
      await page.route(
        /\/api\/v1\/conversations\/[^/]+\/messages$/,
        (route) => {
          if (target || route.request().method() !== 'POST')
            return route.fallback();
          target = new URL(route.request().url());
          return route.continue({
            url: `http://127.0.0.1:${address.port}${target.pathname}`,
          });
        },
      );
      await use(() => {
        if (!stream) throw new Error('SSE 连接尚未建立');
        stream.destroy();
      });
    } finally {
      proxy.closeAllConnections();
      await new Promise<void>((resolve) => proxy.close(() => resolve()));
    }
  },
});

test.skip(
  process.env.XA_CHAT_INTEGRATION !== '1',
  '需要真实用户端、认证 API、PostgreSQL 和百炼',
);

/** 真实工具在审批时挂起，给浏览器确定的断线、重连和取消时机。 */
test('对话断线后续订，刷新后恢复审批，并保留完整工具结果和停止状态', async ({
  disconnectStream,
  page,
  request,
}) => {
  const pageErrors: string[] = [];
  page.on('pageerror', (error) => {
    pageErrors.push(error.message);
    console.error('页面异常', error.message);
  });
  const shotDir = process.env.XA_P7_SHOTS;
  if (shotDir) await mkdir(shotDir, { recursive: true });
  const shot = async (name: string) => {
    if (shotDir) await page.screenshot({ path: join(shotDir, `${name}.png`) });
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
  expect(token).toBeTruthy();
  const headers = { Authorization: `Bearer ${token}` };
  const created = await request.post('/api/v1/agents', {
    headers,
    data: {
      name: '对话续订验收 Agent',
      visibility: 'private',
      configuration: {
        model: process.env.XA_P5_MODEL || 'qwen3.8-flash',
        enable_thinking: false,
        soul_md:
          '每次用户询问当前时间时，都必须调用 current_time 一次。工具返回后只回答「时间查询完成」，不要添加 Markdown。',
        tools: [{ name: 'current_time', enabled: true, approval: 'always' }],
      },
    },
  });
  expect(created.status()).toBe(201);
  const { agent } = (await created.json()) as { agent: { id: string } };
  await page.goto(`/chat?agentId=${encodeURIComponent(agent.id)}`);
  const composer = page.getByRole('textbox', { name: '消息', exact: true });
  const dock = page.getByTestId('approval-dock');
  const stop = page.getByRole('button', { name: '停止生成' });
  const send = async (text: string) => {
    await composer.fill(text);
    await composer.press('Enter');
    await expect(dock).toBeVisible({ timeout: 60000 });
    const assistant = page.locator('article.message-row.assistant').last();
    const id = await assistant.getAttribute('id');
    if (!id?.startsWith('run-')) throw new Error('回复缺少 run ID');
    return { assistant, runId: id.slice(4) };
  };
  const { assistant, runId } = await send('现在几点？请用 current_time 查询。');
  const eventsPath = `/api/v1/runs/${runId}/events`;
  const convId = new URL(page.url()).searchParams.get('conversationId');
  expect(convId).toBeTruthy();
  const messagesPath = `/api/v1/conversations/${convId}/messages`;
  const history = async () => {
    const response = await request.get(messagesPath, { headers });
    expect(response.ok()).toBe(true);
    return (await response.json()) as {
      messages: {
        id: string;
        role: string;
        run_id?: string;
        status: string;
        content: string;
        parts?: {
          type: string;
          call_id?: string;
          name?: string;
          text?: string;
        }[];
      }[];
    };
  };

  const disconnected = page.waitForEvent('requestfailed', {
    predicate: (event) =>
      event.method() === 'POST' &&
      new URL(event.url()).pathname === messagesPath,
    timeout: 10000,
  });
  const resumedResponse = page.waitForResponse((response) => {
    const url = new URL(response.url());
    return (
      url.pathname === eventsPath &&
      Number(url.searchParams.get('after')) > 0 &&
      response.ok()
    );
  });
  disconnectStream();
  await disconnected;
  const resumed = await resumedResponse;
  const after = new URL(resumed.url()).searchParams.get('after');
  expect(await resumed.request().headerValue('last-event-id')).toBe(after);
  await expect(dock).toBeVisible();
  await dock.getByRole('button', { name: /^允许本次，/ }).click();
  await expect(assistant).toHaveAttribute('data-message-status', 'completed', {
    timeout: 60000,
  });
  await expect(stop).toBeHidden();
  await expect(dock).toBeHidden();
  const saved = (await history()).messages.find(
    (message) => message.role === 'assistant' && message.run_id === runId,
  );
  if (!saved) throw new Error('历史中没有已保存的回复');
  expect(saved.content.trim()).toBeTruthy();
  await expect(page.locator(`article[id="run-${runId}"]`)).toHaveCount(1);
  const savedParts = (saved.parts ?? []).flatMap((part) =>
    part.type === 'text' && typeof part.text === 'string' ? [part.text] : [],
  );
  const expectedParts = savedParts.length ? savedParts : [saved.content];
  // 服务端 content 是各段原文连接，界面将相邻正文段分开显示。
  expect(expectedParts.join('').trim()).toBe(saved.content.trim());
  const joined = (parts: string[]) =>
    parts
      .map((part) => part.trim())
      .filter(Boolean)
      .join('\n\n');
  await expect
    .poll(async () =>
      joined(
        await assistant.getByTestId('message-text-part').allTextContents(),
      ),
    )
    .toBe(joined(expectedParts));
  const callId = saved.parts?.find(
    (part) => part.type === 'tool' && part.name === 'current_time',
  )?.call_id;
  expect(callId).toBeTruthy();
  const chip = assistant.getByTestId('tool-call-chip');
  await expect(chip).toHaveCount(1);
  await expect(chip.getByTestId('approval-receipt')).toHaveText('已允许本次');
  await expect(chip).toHaveAttribute('data-tool-status', 'succeeded');
  await chip.getByRole('button', { name: '当前时间', exact: false }).click();
  const toolPath = `/api/v1/runs/${runId}/tool-results/${callId}`;
  const result = await request.get(toolPath, { headers });
  expect(result.ok()).toBe(true);
  const { data, arguments: toolArguments } = await result.json();
  expect(data).toHaveProperty('now_rfc3339');
  await chip.getByRole('button', { name: '查看详情', exact: true }).click();
  const details = chip.getByRole('region', { name: '工具结果详情' });
  await expect(details.locator('pre')).toHaveCount(2);
  const shown = await details.locator('pre').allTextContents();
  expect(JSON.parse(shown[0])).toEqual(toolArguments);
  expect(JSON.parse(shown[1])).toEqual(data);
  await shot('tool-result-details');

  const { runId: stoppedId } = await send(
    '请重新调用 current_time 查询当前时间。',
  );
  const stoppedEvents = `/api/v1/runs/${stoppedId}/events`;
  const matchReplay = (url: URL) =>
    url.pathname === stoppedEvents && url.searchParams.get('after') === '0';
  let dropped = false;
  // 仅中断一次重放请求，其余消息、审批和停止走真实 API。
  await page.route(matchReplay, async (route) => {
    if (dropped) return route.continue();
    dropped = true;
    await route.abort('connectionreset');
  });
  const replayed = page.waitForResponse(
    (response) => matchReplay(new URL(response.url())) && response.ok(),
  );
  await page.reload({ waitUntil: 'domcontentloaded' });
  await replayed;
  expect(dropped).toBe(true);
  await expect(dock).toBeVisible();
  await expect(composer).toBeDisabled();
  await shot('approval-after-reconnect');
  const cancelled = page.waitForResponse(
    (response) =>
      new URL(response.url()).pathname === `/api/v1/runs/${stoppedId}/cancel` &&
      response.request().method() === 'POST',
  );
  await stop.click();
  expect((await cancelled).ok()).toBe(true);
  const stopped = page.locator(`article[id="run-${stoppedId}"]`);
  await expect(stopped).toHaveAttribute('data-message-status', 'stopped');
  await expect(dock).toBeHidden();
  await expect(composer).toBeEnabled();
  await page.reload({ waitUntil: 'domcontentloaded' });
  await expect(stopped).toHaveAttribute('data-message-status', 'stopped');
  await expect(stopped).toContainText('已停止生成');
  await expect(dock).toBeHidden();
  expect(pageErrors).toEqual([]);
});
