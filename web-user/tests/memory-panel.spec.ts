import { expect, type Page, type Route, test } from '@playwright/test';

// 驱动真实记忆页，接口由测试拦截。这不是后端联调，也不接入生产代码。
test.skip(
  process.env.XA_MEMORY_UI !== '1',
  '设置 XA_MEMORY_UI=1 后才对真实页面做接口拦截',
);

const userID = 'user_1';
const agentID = 'agent_1';
const conversationID = 'conv_1';

type DeadJob = {
  id: string;
  kind: string;
  last_error_summary: string;
  updated_at: string;
};

type Memory = {
  id: string;
  content: string;
  namespace: string;
  subject_key: string;
  importance: number;
};

type ApiState = {
  dead: number;
  jobs: DeadJob[];
  memories: Memory[];
  retryCalls: string[];
  createBodies: Record<string, unknown>[];
  statsReads: number;
  failStatsOnRefresh: boolean;
  nextAfterBatch: { dead: number; jobs: DeadJob[] } | null;
  retryStatus: (id: string, index: number) => number | Promise<number>;
};

function job(id: string, at: string): DeadJob {
  return {
    id,
    kind: 'extract',
    last_error_summary: '模型服务请求失败',
    updated_at: at,
  };
}

function memory(item: Memory) {
  return {
    id: item.id,
    workspace_id: 'workspace_1',
    owner_user_id: userID,
    scope: 'user',
    agent_id: null,
    conversation_id: null,
    namespace: item.namespace,
    subject_key: item.subject_key,
    content: item.content,
    kind: 'fact',
    importance: item.importance,
    pinned: false,
    status: 'active',
    source: 'user_edited',
    source_type: 'user_edit',
    source_ref: null,
    supersedes_id: null,
    created_at: '2026-11-06T00:00:00Z',
    updated_at: '2026-11-06T00:00:00Z',
    last_used_at: null,
    retention: 1,
    strength_days: 60,
    sensitive: false,
  };
}

function statsBody(state: ApiState) {
  return {
    status: { active: state.memories.length },
    namespace: {},
    pending: 0,
    failed: 0,
    dead: state.dead,
    dead_jobs: state.jobs,
  };
}

function state(partial: Partial<ApiState> = {}): ApiState {
  return {
    dead: 0,
    jobs: [],
    memories: [
      {
        id: 'mem_pref',
        content: '喜欢先看结论。',
        namespace: 'preferences',
        subject_key: 'answer.order',
        importance: 8,
      },
    ],
    retryCalls: [],
    createBodies: [],
    statsReads: 0,
    failStatsOnRefresh: false,
    nextAfterBatch: null,
    retryStatus: () => 204,
    ...partial,
  };
}

async function install(page: Page, current: ApiState) {
  await page.addInitScript(() => {
    localStorage.setItem('xa.user.session', 'test-token');
    localStorage.setItem('xa.user.agent-details.open', 'true');
  });
  await page.route('**/api/v1/**', async (route: Route) => {
    const request = route.request();
    const url = new URL(request.url());
    const path = url.pathname;
    if (path === '/api/v1/auth/me') {
      await route.fulfill({
        json: {
          id: userID,
          email: 'user@example.com',
          display_name: '用户',
          role: 'user',
        },
      });
      return;
    }
    if (path === '/api/v1/agent-library') {
      await route.fulfill({
        json: {
          agents: [
            {
              id: agentID,
              name: '主 Agent',
              description: '日常助手',
              avatar: '',
              owner_id: userID,
              owner_user_id: userID,
              owner_type: 'user',
              current_version_id: 'version_1',
              is_default: true,
              is_primary: true,
              created_at: '2026-11-01T00:00:00Z',
              visibility: 'private',
              status: 'active',
              tags: [],
            },
          ],
        },
      });
      return;
    }
    if (path === `/api/v1/agents/${agentID}` && request.method() === 'GET') {
      await route.fulfill({
        json: {
          id: agentID,
          name: '主 Agent',
          description: '日常助手',
          avatar: '',
          owner_id: userID,
          owner_user_id: userID,
          owner_type: 'user',
          current_version_id: 'version_1',
          is_default: true,
          is_primary: true,
          created_at: '2026-11-01T00:00:00Z',
          visibility: 'private',
          status: 'active',
          tags: [],
        },
      });
      return;
    }
    if (path === `/api/v1/agents/${agentID}/config`) {
      await route.fulfill({
        json: {
          id: 'version_1',
          version_number: 1,
          available_models: ['qwen3.8-flash'],
          configuration: {
            soul_md: '请用中文回答。',
            model: 'qwen3.8-flash',
            memory_policy: {
              enabled: true,
              core_budget_tokens: 800,
              recall_top_k: 5,
              decay_enabled: true,
            },
          },
        },
      });
      return;
    }
    if (path === '/api/v1/conversations' && request.method() === 'GET') {
      await route.fulfill({
        json: {
          conversations: [
            {
              id: conversationID,
              agent_id: agentID,
              title: '日常',
              updated_at: '2026-11-06T00:00:00Z',
            },
          ],
        },
      });
      return;
    }
    if (path === `/api/v1/conversations/${conversationID}/messages`) {
      await route.fulfill({ json: { messages: [], has_more: false } });
      return;
    }
    if (path === `/api/v1/conversations/${conversationID}/summary`) {
      await route.fulfill({ json: { summary: null } });
      return;
    }
    if (path === '/api/v1/memory/stats') {
      current.statsReads += 1;
      if (current.failStatsOnRefresh && current.retryCalls.length > 0) {
        await route.fulfill({
          status: 500,
          json: { message: '统计暂时不可用' },
        });
        return;
      }
      if (current.nextAfterBatch && current.retryCalls.length >= 20) {
        current.dead = current.nextAfterBatch.dead;
        current.jobs = current.nextAfterBatch.jobs;
        current.nextAfterBatch = null;
      }
      await route.fulfill({ json: statsBody(current) });
      return;
    }
    if (path === '/api/v1/memories' && request.method() === 'GET') {
      await route.fulfill({
        json: {
          items: current.memories.map(memory),
          next_cursor: '',
        },
      });
      return;
    }
    if (path === '/api/v1/memories' && request.method() === 'POST') {
      const body = request.postDataJSON() as Record<string, unknown>;
      current.createBodies.push(body);
      const created: Memory = {
        id: `mem_${current.memories.length + 1}`,
        content: String(body.content),
        namespace: String(body.namespace),
        subject_key: `server_${current.memories.length + 1}`,
        importance: Number(body.importance),
      };
      current.memories = [created, ...current.memories];
      await route.fulfill({ status: 201, json: memory(created) });
      return;
    }
    const retry = path.match(/^\/api\/v1\/memory\/jobs\/([^/]+)\/retry$/);
    if (retry && request.method() === 'POST') {
      const id = decodeURIComponent(retry[1]);
      const index = current.retryCalls.length;
      current.retryCalls.push(id);
      const status = await current.retryStatus(id, index);
      if (status === 204) {
        current.jobs = current.jobs.filter((item) => item.id !== id);
        current.dead = Math.max(0, current.dead - 1);
        await route.fulfill({ status: 204, body: '' });
      } else {
        await route.fulfill({
          status,
          json: { message: status === 404 ? '任务不存在' : '请求失败' },
        });
      }
      return;
    }
    await route.fulfill({
      status: 500,
      json: { message: `未拦截 ${request.method()} ${path}` },
    });
  });
}

async function openMemory(page: Page) {
  await page.goto(`/chat?agentId=${agentID}&conversationId=${conversationID}`);
  const panel = page.getByRole('region', { name: 'Agent 配置' });
  await panel.getByRole('button', { name: '查看记忆' }).click();
  await expect(
    panel.getByRole('heading', { name: '记忆', exact: true }),
  ).toBeVisible();
  return panel;
}

test('无失败任务时不显示重试', async ({ page }) => {
  await install(page, state());
  const panel = await openMemory(page);
  await expect(panel.getByText('喜欢先看结论。')).toBeVisible();
  await expect(panel.getByText('偏好')).toBeVisible();
  await expect(panel.getByText('个整理任务失败')).toHaveCount(0);
  await expect(
    panel.getByRole('button', { name: '重试', exact: true }),
  ).toHaveCount(0);
});

test('重复点击只发一轮，失败提示和部分失败停在刷新结果', async ({ page }) => {
  let release: ((status: number) => void) | null = null;
  const held = new Promise<number>((resolve) => {
    release = resolve;
  });
  const current = state({
    dead: 3,
    jobs: [
      job('job_1', '2026-11-06T00:03:00Z'),
      job('job_2', '2026-11-06T00:02:00Z'),
      job('job_3', '2026-11-06T00:01:00Z'),
    ],
    retryStatus: async (_id, index) => {
      if (index === 0) return held;
      return index === 1 ? 500 : 204;
    },
  });
  await install(page, current);
  const panel = await openMemory(page);
  await expect(panel.getByText('3 个整理任务失败')).toBeVisible();
  const button = panel.getByRole('button', { name: '重试', exact: true });
  await button.evaluate((node: HTMLButtonElement) => {
    node.click();
    node.click();
  });
  await expect(panel.getByRole('button', { name: '重试中…' })).toBeDisabled();
  await expect.poll(() => current.retryCalls.length).toBe(1);
  await panel.screenshot({
    path: '/opt/cursor/artifacts/memory_retry_in_progress.png',
  });
  if (!release) throw new Error('重试没有挂起');
  release(204);
  await expect.poll(() => current.retryCalls.length).toBe(3);
  await expect(panel.getByRole('alert')).toContainText(
    '已重新排队 2 个，1 个未能重试，还有 1 个失败任务，可继续重试',
  );
  await expect(panel.getByText('1 个整理任务失败')).toBeVisible();
  expect(current.retryCalls).toEqual(['job_1', 'job_2', 'job_3']);
  await panel.screenshot({
    path: '/opt/cursor/artifacts/memory_retry_partial.png',
  });
});

test('超过 20 条只重试当前这批，并显示刷新后的剩余', async ({ page }) => {
  const first = Array.from({ length: 20 }, (_, index) =>
    job(
      `job_${String(index + 1).padStart(2, '0')}`,
      `2026-11-06T00:${String(20 - index).padStart(2, '0')}:00Z`,
    ),
  );
  const current = state({
    dead: 25,
    jobs: first,
    nextAfterBatch: {
      dead: 5,
      jobs: Array.from({ length: 5 }, (_, index) =>
        job(
          `job_${21 + index}`,
          `2026-11-06T00:00:${String(5 - index).padStart(2, '0')}Z`,
        ),
      ),
    },
  });
  await install(page, current);
  const panel = await openMemory(page);
  await panel.getByRole('button', { name: '重试', exact: true }).click();
  await expect(panel.getByRole('status')).toContainText(
    '已重新排队 20 个，还有 5 个失败任务，可继续重试',
  );
  expect(current.retryCalls).toEqual(first.map((item) => item.id));
  await panel.screenshot({
    path: '/opt/cursor/artifacts/memory_retry_remaining.png',
  });
});

test('统计刷新失败时不宣称已经处理完', async ({ page }) => {
  const current = state({
    dead: 2,
    jobs: [
      job('job_1', '2026-11-06T00:02:00Z'),
      job('job_2', '2026-11-06T00:01:00Z'),
    ],
    failStatsOnRefresh: true,
    retryStatus: () => 500,
  });
  await install(page, current);
  const panel = await openMemory(page);
  await panel.getByRole('button', { name: '重试', exact: true }).click();
  await expect(panel.getByRole('alert')).toContainText(
    '2 个未能重试。统计刷新失败，不能确认剩余失败任务',
  );
  await expect(panel.getByText('统计未更新')).toBeVisible();
  await expect(panel.getByRole('button', { name: '重新读取' })).toBeVisible();
  expect(current.retryCalls).toEqual(['job_1', 'job_2']);
  await panel.screenshot({
    path: '/opt/cursor/artifacts/memory_retry_refresh_failed.png',
  });
});

test('连续新增两条事实的请求没有 subject_key', async ({ page }) => {
  const current = state();
  await install(page, current);
  const panel = await openMemory(page);
  const facts = ['我住在杭州。', '我周末喜欢骑自行车。'];
  for (const fact of facts) {
    await panel.getByRole('button', { name: '+ 新增记忆' }).click();
    await panel.getByLabel('记忆内容').fill(fact);
    await expect(panel.getByLabel('分类')).toContainText('个人资料');
    await panel.locator('form').getByRole('button', { name: '保存' }).click();
    await expect(panel.getByText(fact)).toBeVisible();
  }
  expect(current.createBodies).toHaveLength(2);
  for (const body of current.createBodies) {
    expect(body).not.toHaveProperty('subject_key');
  }
  expect(current.createBodies[0].content).toBe(facts[0]);
  expect(current.createBodies[1].content).toBe(facts[1]);
  expect(current.createBodies[0].content).not.toBe(
    current.createBodies[1].content,
  );
  await expect(panel.getByText('个人资料')).toBeVisible();
  await expect(panel.getByText('偏好')).toBeVisible();
  await expect(
    panel.getByRole('status').filter({ hasText: '已添加' }),
  ).toBeHidden();
  await panel.screenshot({
    path: '/opt/cursor/artifacts/memory_manual_facts.png',
  });
});
