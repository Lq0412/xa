import { createHash } from 'node:crypto';
import fs from 'node:fs';
import path from 'node:path';
import { expect, test } from '@playwright/test';

if (process.env.XA_P4_CHAT_SCREENSHOTS !== '1') {
  throw new Error('请设置 XA_P4_CHAT_SCREENSHOTS=1');
}

const artifactDir = '/opt/cursor/artifacts';

function md5File(file: string) {
  return createHash('md5').update(fs.readFileSync(file)).digest('hex');
}

async function login(page: import('@playwright/test').Page, primaryId: string) {
  await page.goto('/login', { waitUntil: 'networkidle' });
  await page.locator('input[name=email]').fill(process.env.XA_E2E_EMAIL || '');
  await page
    .locator('input[name=password]')
    .fill(process.env.XA_E2E_PASSWORD || '');
  await page.getByRole('button', { name: '登录', exact: true }).click();
  await expect(page).toHaveURL(/\/chat/, { timeout: 60000 });
  await page.goto(`/chat?agentId=${encodeURIComponent(primaryId)}`, {
    waitUntil: 'networkidle',
  });
  await expect(page.getByPlaceholder('发消息')).toBeEnabled({ timeout: 60000 });
}

async function prepareAgent(
  request: Parameters<typeof test>[0]['request'],
  api: string,
  toolConfig: Record<string, unknown> = {
    timezone: 'Asia/Shanghai',
    delay_ms: 5000,
  },
) {
  const tokenRes = await request.post(`${api}/api/v1/auth/login`, {
    data: {
      email: process.env.XA_E2E_EMAIL,
      password: process.env.XA_E2E_PASSWORD,
    },
  });
  expect(tokenRes.ok()).toBeTruthy();
  const { token } = (await tokenRes.json()) as { token: string };
  const agentsRes = await request.get(`${api}/api/v1/agents`, {
    headers: { Authorization: `Bearer ${token}` },
  });
  const agents = (await agentsRes.json()) as {
    agents: {
      id: string;
      current_version_id: string;
      is_primary?: boolean;
      name: string;
    }[];
  };
  const primaryAgent =
    agents.agents.find((a) => a.is_primary) ?? agents.agents[0];
  if (!primaryAgent) throw new Error('未找到可用 Agent');
  const saveRes = await request.put(
    `${api}/api/v1/agents/${primaryAgent.id}/config`,
    {
      headers: { Authorization: `Bearer ${token}` },
      data: {
        agent_id: primaryAgent.id,
        expected_version_id: primaryAgent.current_version_id,
        name: primaryAgent.name || '主',
        visibility: 'private',
        configuration: {
          model: 'qwen3.7-plus',
          soul_md:
            '思考时必须写出 Markdown：一行加粗标题，再加一个至少两项的无序列表。先写完思考再调用 current_time，工具返回前不要写正文。',
          enable_thinking: true,
        },
      },
    },
  );
  const saved = (await saveRes.json()) as { current_version_id: string };
  await request.post(`${api}/api/v1/agents/${primaryAgent.id}/enable`, {
    headers: { Authorization: `Bearer ${token}` },
  });
  await request.put(`${api}/api/v1/agents/${primaryAgent.id}/tools`, {
    headers: { Authorization: `Bearer ${token}` },
    data: {
      expected_version_id: saved.current_version_id,
      tools: [
        {
          name: 'current_time',
          enabled: true,
          configuration: toolConfig,
        },
      ],
      runtime_limits: { max_tool_calls: 8 },
    },
  });
  return { token, primaryId: primaryAgent.id };
}

function lastAssistant(page: import('@playwright/test').Page) {
  return page
    .locator('article.message-row.assistant, article.assistant')
    .last();
}

function processRail(page: import('@playwright/test').Page) {
  return lastAssistant(page).locator('[data-testid="process-rail"]');
}

test('P4.1 流式：一行过程、时间线、历史用时、失败', async ({
  page,
  context,
  request,
}) => {
  test.setTimeout(600000);
  const api = process.env.XA_API_BASE || 'http://127.0.0.1:18080';
  const { primaryId, token } = await prepareAgent(request, api);

  await context.grantPermissions(['clipboard-read', 'clipboard-write']);
  await login(page, primaryId);

  const composer = page.getByPlaceholder('发消息');
  const stopBtn = page.getByRole('button', { name: '停止生成' });

  await composer.fill(
    '请先在思考里用加粗标题和无序列表说明你要查询当前时间，然后调用 current_time。工具返回后再用两句话回答现在的时间。',
  );
  await composer.press('Enter');
  await expect(stopBtn).toBeVisible({ timeout: 120000 });
  await expect(page).toHaveURL(/conversationId=/, { timeout: 120000 });

  const assistant = lastAssistant(page);
  const rail = processRail(page);
  await expect(rail).toBeVisible({ timeout: 120000 });

  await Promise.all([
    page
      .waitForFunction(
        () => {
          const node = document.querySelector('[data-testid="process-rail"]');
          if (node?.getAttribute('data-phase') !== 'thinking') return false;
          const title =
            node.querySelector('[data-testid="process-summary"]')
              ?.textContent || '';
          const shimmer = node.querySelector('.process-summary-shimmer');
          return /正在思考/.test(title) && /秒/.test(title) && Boolean(shimmer);
        },
        undefined,
        { timeout: 120000, polling: 80 },
      )
      .then(async () => {
        await page.screenshot({
          path: path.join(artifactDir, 'p41-reasoning-streaming.png'),
          fullPage: true,
        });
      }),
    page
      .waitForFunction(
        () => {
          const node = document.querySelector('[data-testid="process-rail"]');
          if (node?.getAttribute('data-phase') !== 'tool') return false;
          const title =
            node.querySelector('[data-testid="process-summary"]')
              ?.textContent || '';
          return title.includes('正在调用');
        },
        undefined,
        { timeout: 180000, polling: 50 },
      )
      .then(async () => {
        await page.screenshot({
          path: path.join(artifactDir, 'p41-tool-running.png'),
          fullPage: true,
        });
      }),
  ]);

  await expect(composer).toBeEnabled({ timeout: 240000 });
  const summary = assistant.locator('[data-testid="process-summary"]');
  await expect
    .poll(async () => {
      const expanded = await summary.getAttribute('aria-expanded');
      const title = await summary.innerText();
      return `${expanded}:${title}`;
    })
    .toMatch(/^false:.*(已思考|思考并调用了|调用了)/);

  const bubble = assistant.locator('.assistant-bubble').first();
  const bubbleY = (await bubble.boundingBox())?.y ?? 0;
  const railY = (await rail.boundingBox())?.y ?? 0;
  expect(railY).toBeLessThan(bubbleY);
  await expect(rail.locator('[data-testid="process-preview"]')).toHaveCount(0);

  await page.screenshot({
    path: path.join(artifactDir, 'p41-complete-folded.png'),
    fullPage: true,
  });

  await summary.click();
  await expect(summary).toHaveAttribute('aria-expanded', 'true');
  const reasoningBodies = rail.locator('[data-testid="reasoning-body"]');
  await expect(reasoningBodies.first()).toBeVisible();
  const bodyCount = await reasoningBodies.count();
  let sawMarkdown = false;
  for (let index = 0; index < bodyCount; index += 1) {
    const body = reasoningBodies.nth(index);
    const bodyText = (await body.innerText()) || '';
    expect(bodyText).not.toMatch(/\*\*[^*]+\*\*/);
    if ((await body.locator('strong, li').count()) > 0) sawMarkdown = true;
  }
  expect(sawMarkdown).toBe(true);
  await page.screenshot({
    path: path.join(artifactDir, 'p41-reasoning-markdown.png'),
    fullPage: true,
  });

  const convId = new URL(page.url()).searchParams.get('conversationId');
  await page.reload({ waitUntil: 'domcontentloaded' });
  await expect(composer).toBeVisible({ timeout: 60000 });
  const histAssistant = lastAssistant(page);
  const histSummary = histAssistant.locator('[data-testid="process-summary"]');
  await expect(histSummary).toBeVisible({ timeout: 60000 });
  await expect(histSummary).toHaveAttribute('aria-expanded', 'false');
  expect(await histSummary.innerText()).toMatch(/已思考|思考并调用了|调用了/);
  await histSummary.click();
  const histSegment = histAssistant
    .locator('[data-testid="reasoning-segment"]')
    .first();
  await expect(histSegment).toHaveAttribute('data-expandable', '0');
  await expect(histSegment).toContainText('已思考');
  await expect(
    histAssistant.locator('[data-testid="reasoning-body"]'),
  ).toHaveCount(0);
  const histTool = histAssistant
    .locator('[data-testid="tool-call-chip"]')
    .first();
  await expect(histTool).toBeVisible();
  await histTool.getByRole('button').click();
  await expect(
    histTool.locator('[data-testid="tool-summary"], pre').first(),
  ).toBeVisible();
  await page.screenshot({
    path: path.join(artifactDir, 'p41-history-refresh.png'),
    fullPage: true,
  });

  await prepareAgent(request, api, { timezone: 'Not/A_Valid_Zone' });
  await page.goto(`/chat?agentId=${encodeURIComponent(primaryId)}`, {
    waitUntil: 'networkidle',
  });
  await composer.fill('只调用 current_time 一次，然后一句话说明结果。');
  await composer.press('Enter');
  const failedChip = page
    .locator('[data-testid="tool-call-chip"][data-tool-status="failed"]')
    .last();
  await expect(failedChip).toBeVisible({ timeout: 180000 });
  await expect(composer).toBeEnabled({ timeout: 180000 });
  const failedRail = processRail(page);
  await expect(
    failedRail.locator('[data-testid="process-summary"]'),
  ).toHaveAttribute('aria-expanded', 'true');
  await expect(failedChip.locator('[data-testid="tool-error"]')).toBeVisible();
  await page.screenshot({
    path: path.join(artifactDir, 'p41-tool-failed.png'),
    fullPage: true,
  });

  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto(
    `/chat?agentId=${encodeURIComponent(primaryId)}&conversationId=${convId}`,
    { waitUntil: 'domcontentloaded' },
  );
  await expect(
    lastAssistant(page).locator('[data-testid="process-summary"]'),
  ).toBeVisible({
    timeout: 60000,
  });
  await page.screenshot({
    path: path.join(artifactDir, 'p41-narrow-viewport.png'),
    fullPage: true,
  });

  await page.emulateMedia({ colorScheme: 'dark' });
  await page.screenshot({
    path: path.join(artifactDir, 'p41-dark.png'),
    fullPage: true,
  });

  const hist = await request.get(
    `${api}/api/v1/conversations/${convId}/messages`,
    { headers: { Authorization: `Bearer ${token}` } },
  );
  const histBody = (await hist.json()) as {
    messages: {
      role: string;
      run_id?: string;
      parts?: { type: string; duration_ms?: number; text?: string }[];
    }[];
  };
  // 接口按时间从早到晚返回。失败轮也在这条会话里，取最后一条会读到那一轮较短的思考。
  const firstAssistant = histBody.messages.find((m) => m.role === 'assistant');
  expect(firstAssistant?.run_id).toBeTruthy();
  const reasoning = firstAssistant?.parts?.find((p) => p.type === 'reasoning');
  expect(reasoning?.text ?? '').toBe('');
  expect(reasoning?.duration_ms ?? 0).toBeGreaterThan(1000);

  const files = [
    'p41-reasoning-streaming.png',
    'p41-tool-running.png',
    'p41-complete-folded.png',
    'p41-reasoning-markdown.png',
    'p41-history-refresh.png',
    'p41-tool-failed.png',
    'p41-narrow-viewport.png',
    'p41-dark.png',
  ].map((n) => path.join(artifactDir, n));
  expect(new Set(files.map(md5File)).size).toBe(files.length);
});
