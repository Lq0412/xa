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
            '先一句思考说明将调用 current_time，立刻调用工具，禁止在工具完成前写正文。',
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

test('P4.1 流式：过程轨、思考折叠、工具卡、历史用时', async ({
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
    '必须先调用 current_time。在思考里写清你要先查时间再回答；正文两句话说明当前时间即可。',
  );
  await composer.press('Enter');
  await expect(stopBtn).toBeVisible({ timeout: 120000 });
  await expect(page).toHaveURL(/conversationId=/, { timeout: 120000 });

  const assistant = lastAssistant(page);
  const rail = processRail(page);

  await expect(rail).toBeVisible({ timeout: 120000 });

  await page.waitForFunction(
    () => {
      const fold = [
        ...document.querySelectorAll('[data-testid="reasoning-fold"]'),
      ].pop();
      if (!fold) return false;
      const status = fold.getAttribute('data-reasoning-status');
      if (status !== 'streaming') return false;
      const title = fold.querySelector('button')?.textContent || '';
      return title.includes('思考中') && /\d+\s*秒/.test(title);
    },
    undefined,
    { timeout: 120000, polling: 200 },
  );
  await page.screenshot({
    path: path.join(artifactDir, 'p41-reasoning-streaming.png'),
    fullPage: true,
  });

  await page.waitForFunction(
    () => {
      const row = [...document.querySelectorAll('article.assistant')].pop();
      if (!row) return false;
      const fold = row.querySelector('[data-testid="reasoning-fold"]');
      const foldStatus = fold?.getAttribute('data-reasoning-status');
      const foldCollapsed =
        fold &&
        fold.querySelector('button')?.getAttribute('aria-expanded') === 'false';
      const toolRunning = row.querySelector(
        '[data-testid="tool-call-chip"][data-status="running"]',
      );
      const bubble = row.querySelector('[data-testid="message-text-part"]');
      const bubbleText = bubble?.textContent || '';
      const noFinalTime = !/年.*月.*日/.test(bubbleText);
      return Boolean(
        toolRunning &&
          (foldCollapsed || foldStatus === 'completed') &&
          noFinalTime,
      );
    },
    undefined,
    { timeout: 180000, polling: 300 },
  );
  await page.screenshot({
    path: path.join(artifactDir, 'p41-tool-running.png'),
    fullPage: true,
  });

  await expect(
    page.locator('[data-testid="tool-call-chip"][data-status="done"]').first(),
  ).toBeVisible({ timeout: 240000 });
  await expect(composer).toBeEnabled({ timeout: 180000 });

  const bubbleBox = assistant.locator('.rounded-2xl.bg-zinc-100').first();
  const railBox = rail.first();
  const bubbleY = (await bubbleBox.boundingBox())?.y ?? 0;
  const railY = (await railBox.boundingBox())?.y ?? 0;
  expect(railY).toBeLessThan(bubbleY);

  await expect
    .poll(async () => {
      const fold = assistant.locator('[data-testid="reasoning-fold"]').last();
      const expanded = await fold
        .getByRole('button')
        .getAttribute('aria-expanded');
      const title = await fold.getByRole('button').innerText();
      return `${expanded}:${title}`;
    })
    .toMatch(/^false:.*已思考/);

  await page.screenshot({
    path: path.join(artifactDir, 'p41-complete-folded.png'),
    fullPage: true,
  });

  await assistant
    .locator('[data-testid="reasoning-fold"]')
    .last()
    .getByRole('button')
    .click();
  await expect(
    assistant.locator('[data-testid="reasoning-fold-body"]'),
  ).toBeVisible();
  const bodyText =
    (await assistant
      .locator('[data-testid="reasoning-fold-body"]')
      .innerText()) || '';
  expect(bodyText).not.toMatch(/\*\*[^*]+\*\*/);
  await page.screenshot({
    path: path.join(artifactDir, 'p41-reasoning-markdown.png'),
    fullPage: true,
  });

  const convId = new URL(page.url()).searchParams.get('conversationId');
  await page.reload({ waitUntil: 'domcontentloaded' });
  await expect(composer).toBeVisible({ timeout: 60000 });
  const histAssistant = lastAssistant(page);
  await expect(
    histAssistant.locator('[data-testid="process-rail"]'),
  ).toBeVisible({
    timeout: 60000,
  });
  const histTitle = await histAssistant
    .locator('[data-testid="reasoning-fold"] button')
    .first()
    .innerText();
  expect(histTitle).toMatch(/已思考/);
  await histAssistant
    .locator('[data-testid="reasoning-fold"] button')
    .first()
    .click();
  await expect(
    histAssistant.locator('[data-testid="reasoning-fold-placeholder"]'),
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
  await expect(
    page
      .locator('[data-testid="tool-call-chip"][data-tool-status="failed"]')
      .last(),
  ).toBeVisible({ timeout: 180000 });
  await page.screenshot({
    path: path.join(artifactDir, 'p41-tool-failed.png'),
    fullPage: true,
  });

  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto(
    `/chat?agentId=${encodeURIComponent(primaryId)}&conversationId=${convId}`,
    { waitUntil: 'domcontentloaded' },
  );
  await expect(lastAssistant(page)).toBeVisible({ timeout: 60000 });
  await page.screenshot({
    path: path.join(artifactDir, 'p41-narrow-viewport.png'),
    fullPage: true,
  });

  const hist = await request.get(
    `${api}/api/v1/conversations/${convId}/messages`,
    { headers: { Authorization: `Bearer ${token}` } },
  );
  const histBody = (await hist.json()) as {
    messages: {
      role: string;
      parts?: { type: string; duration_ms?: number }[];
    }[];
  };
  const assistantMsg = histBody.messages.find((m) => m.role === 'assistant');
  const reasoning = assistantMsg?.parts?.find((p) => p.type === 'reasoning');
  expect(reasoning?.duration_ms).toBeGreaterThan(0);

  const files = [
    'p41-reasoning-streaming.png',
    'p41-tool-running.png',
    'p41-complete-folded.png',
    'p41-reasoning-markdown.png',
    'p41-history-refresh.png',
    'p41-tool-failed.png',
    'p41-narrow-viewport.png',
  ].map((n) => path.join(artifactDir, n));
  expect(new Set(files.map(md5File)).size).toBe(files.length);
});
