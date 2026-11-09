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

async function prepareAgent(
  request: Parameters<typeof test>[0]['request'],
  api: string,
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
          configuration: { timezone: 'Asia/Shanghai', delay_ms: 5000 },
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

test('P4 流式：思考折叠、工具 chip、刷新续传', async ({
  page,
  context,
  request,
}) => {
  test.setTimeout(480000);
  const api = process.env.XA_API_BASE || 'http://127.0.0.1:18080';
  const { primaryId } = await prepareAgent(request, api);

  await context.grantPermissions(['clipboard-read', 'clipboard-write']);
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

  const composer = page.getByPlaceholder('发消息');
  await expect(composer).toBeEnabled({ timeout: 60000 });

  await composer.fill(
    '必须先调用 current_time。在思考里写清你要先查时间再回答；正文两句话说明当前时间即可。',
  );
  await composer.press('Enter');

  const stopBtn = page.getByRole('button', { name: '停止生成' });
  await expect(page).toHaveURL(/conversationId=/, { timeout: 120000 });

  const toolChip = page.locator('[data-testid="tool-call-chip"]');
  const assistant = lastAssistant(page);
  const reasoning = page.locator('[data-testid="reasoning-fold"]').last();
  await expect(stopBtn).toBeVisible({ timeout: 120000 });

  await Promise.all([
    page
      .waitForFunction(
        () => {
          const folds = document.querySelectorAll(
            '[data-testid="reasoning-fold"]',
          );
          for (const node of folds) {
            const fold = node as HTMLElement;
            const title = fold.querySelector('button span')?.textContent || '';
            if (!title.includes('思考中')) continue;
            const live =
              fold.getAttribute('data-live-run') === '1' ||
              fold.getAttribute('data-reasoning-status') === 'streaming';
            if (!live) continue;
            const body = fold.querySelector(
              '[data-testid="reasoning-fold-body"]',
            );
            const text = body?.textContent?.replace(/\s/g, '') || '';
            if (
              text.length > 8 &&
              !fold.textContent?.includes('思考内容未保留')
            )
              return true;
          }
          return false;
        },
        undefined,
        { timeout: 120000, polling: 30 },
      )
      .then(async () => {
        const fold = page.locator('[data-testid="reasoning-fold"]').last();
        await fold.getByRole('button').click();
        await expect(stopBtn).toBeVisible();
        await page.screenshot({
          path: path.join(artifactDir, 'p4-chat-reasoning-fold.png'),
          fullPage: true,
        });
      }),
    page
      .waitForFunction(
        () => {
          const row = [...document.querySelectorAll('article.assistant')].pop();
          if (!row) return false;
          const running = row.querySelector(
            '[data-testid="tool-call-chip"][data-status="running"]',
          );
          if (!running) return false;
          const finals = row.querySelectorAll(
            '[data-testid="message-text-part"]',
          );
          for (const el of finals) {
            if (/年.*月.*日.*星期/.test(el.textContent || '')) return false;
          }
          return true;
        },
        undefined,
        { timeout: 120000, polling: 20 },
      )
      .then(async () => {
        await expect(stopBtn).toBeVisible();
        await expect(
          assistant.locator('[data-testid="message-text-part"]').filter({
            hasText: /现在是.*年|年.*月.*日.*星期/,
          }),
        ).toHaveCount(0);
        await page.screenshot({
          path: path.join(artifactDir, 'p4-chat-tool-running.png'),
          fullPage: true,
        });
      }),
  ]);
  await expect(reasoning).toBeVisible();

  await expect(
    page.locator('[data-testid="tool-call-chip"][data-status="done"]').first(),
  ).toBeVisible({ timeout: 180000 });
  await expect(composer).toBeEnabled({ timeout: 120000 });
  const convId = new URL(page.url()).searchParams.get('conversationId');
  const token = await page.evaluate(() =>
    localStorage.getItem('xa.user.session'),
  );
  await expect
    .poll(
      async () => {
        const hist = await request.get(
          `${api}/api/v1/conversations/${convId}/messages`,
          { headers: { Authorization: `Bearer ${token}` } },
        );
        const histBody = (await hist.json()) as {
          messages: {
            role: string;
            content?: string;
            parts?: { type: string }[];
          }[];
        };
        const assistantMsg = histBody.messages.find(
          (m) => m.role === 'assistant',
        );
        const hasParts = assistantMsg?.parts?.some(
          (p) => p.type === 'tool' || p.type === 'reasoning',
        );
        const hasText = (assistantMsg?.content?.trim().length ?? 0) > 0;
        return hasParts || hasText;
      },
      { timeout: 60000 },
    )
    .toBeTruthy();
  await page.screenshot({
    path: path.join(artifactDir, 'p4-chat-tool-done.png'),
    fullPage: true,
  });

  await composer.fill(
    '【续传】再次调用 current_time，然后写至少八百字散文，分四段输出，不要提前结束。',
  );
  await composer.press('Enter');
  await expect(stopBtn).toBeVisible({ timeout: 120000 });
  await expect
    .poll(
      async () =>
        (await lastAssistant(page).innerText()).replace(/\s/g, '').length,
      { timeout: 90000 },
    )
    .toBeGreaterThan(40);
  await page.reload({ waitUntil: 'domcontentloaded' });
  await expect(composer).toBeVisible();
  const resumed = lastAssistant(page);
  await expect
    .poll(async () => (await resumed.innerText()).replace(/\s/g, '').length, {
      timeout: 30000,
    })
    .toBeGreaterThan(20);
  await expect(
    stopBtn.or(page.locator('.streaming-caret')).first(),
  ).toBeVisible({ timeout: 120000 });
  await expect(toolChip.first()).toBeVisible({ timeout: 120000 });
  await page.screenshot({
    path: path.join(artifactDir, 'p4-chat-resume.png'),
    fullPage: true,
  });
  await expect(composer).toBeEnabled({ timeout: 300000 });

  const files = [
    'p4-chat-reasoning-fold.png',
    'p4-chat-tool-running.png',
    'p4-chat-tool-done.png',
    'p4-chat-resume.png',
  ].map((n) => path.join(artifactDir, n));
  expect(new Set(files.map(md5File)).size).toBe(4);
});
