import { request } from './http';
import { type DeadJob, manualMemoryBody, parseDeadJobs } from './memoryRetry';

/** 记忆条目。编辑和恢复都不会原地修改，成功后返回新的 id。 */
export type MemoryItem = {
  id: string;
  workspace_id: string;
  owner_user_id: string;
  scope: 'user' | 'agent' | 'session';
  agent_id: string | null;
  conversation_id: string | null;
  namespace: string;
  subject_key: string;
  content: string;
  kind: string;
  importance: number;
  pinned: boolean;
  status: 'active' | 'superseded' | 'deleted' | 'archived';
  source: 'auto' | 'user_edited';
  source_type: string;
  source_ref: string | null;
  supersedes_id: string | null;
  created_at: string;
  updated_at: string;
  last_used_at: string | null;
  retention: number;
  strength_days: number;
  sensitive: boolean;
};

/** 一条记忆的审计事件，沿取代链和合并父版本返回。 */
export type MemoryEvent = {
  id: number;
  memory_id: string | null;
  source_ref: string | null;
  op: string;
  actor_type: string;
  actor_id: string;
  before: unknown;
  after: unknown;
  reason: string;
  created_at: string;
};

/**
 * Agent 版本上的记忆策略。
 * 界面只编辑遗忘开关和召回条数，其余预算字段保留给服务端，不在页面展示。
 */
export type MemoryPolicy = {
  enabled: boolean;
  core_budget_tokens: number;
  recall_top_k: number;
  decay_enabled: boolean;
};

/** memory.md 是按预算渲染的只读视图，不是另一份存储。 */
export type MemoryView = {
  markdown: string;
  estimated_tokens: number;
  item_ids: string[];
};

/** 保存策略后的新版本。再次修改必须使用 current_version_id。 */
export type MemoryPolicyResult = {
  current_version_id: string;
  version_number: number;
  memory_policy: MemoryPolicy;
};

/** 用户编辑可改的字段。归属、kind 和 subject_key 由服务端保留。 */
export type MemoryEdit = {
  content: string;
  namespace: string;
  importance: number;
  pinned: boolean;
};

/** 手动新增。不传 subject_key，由服务端按内容生成，前端不补固定键。 */
export type MemoryCreate = {
  content: string;
  namespace: string;
  importance: number;
  kind?: string;
  pinned?: boolean;
  scope?: 'user' | 'agent';
  agent_id?: string;
};

/** 当前用户的记忆统计。deadJobs 最多 20 条，dead 才是失败任务总数。 */
export type MemoryStats = {
  status: Record<string, number>;
  namespace: Record<string, number>;
  pending: number;
  failed: number;
  dead: number;
  latest_summary_at?: string;
  latest_maintain_at?: string;
  deadJobs: DeadJob[];
};

export type MemoryPage = {
  items: MemoryItem[];
  nextCursor: string;
};

const defaults: MemoryPolicy = {
  enabled: true,
  core_budget_tokens: 800,
  recall_top_k: 5,
  decay_enabled: true,
};

/** 与服务端写入闸门一致，用来把凭证类内容说成人话。 */
const credentialPattern =
  /密码|password|api[_ -]?key|银行卡|身份证|\bsk-[a-z0-9]{12,}|\b[0-9]{16,19}\b|\b[0-9]{17}[0-9x]\b/i;

export function containsCredential(content: string) {
  return credentialPattern.test(content);
}

/** 读取配置里的 memory_policy。缺省字段与服务端默认值一致：开启，并允许遗忘。 */
export function readMemoryPolicy(raw: unknown): MemoryPolicy {
  if (raw == null) return defaults;
  if (typeof raw !== 'object' || Array.isArray(raw)) {
    throw new Error('memory_policy 格式不正确');
  }
  const value = raw as Record<string, unknown>;
  if (
    (value.enabled != null && typeof value.enabled !== 'boolean') ||
    (value.core_budget_tokens != null &&
      typeof value.core_budget_tokens !== 'number') ||
    (value.recall_top_k != null && typeof value.recall_top_k !== 'number') ||
    (value.decay_enabled != null && typeof value.decay_enabled !== 'boolean')
  ) {
    throw new Error('memory_policy 格式不正确');
  }
  return {
    enabled:
      typeof value.enabled === 'boolean' ? value.enabled : defaults.enabled,
    core_budget_tokens:
      typeof value.core_budget_tokens === 'number'
        ? value.core_budget_tokens
        : defaults.core_budget_tokens,
    recall_top_k:
      typeof value.recall_top_k === 'number'
        ? value.recall_top_k
        : defaults.recall_top_k,
    decay_enabled:
      typeof value.decay_enabled === 'boolean'
        ? value.decay_enabled
        : defaults.decay_enabled,
  };
}

function readCountMap(value: unknown) {
  if (typeof value !== 'object' || value == null || Array.isArray(value)) {
    throw new Error('记忆统计格式不正确');
  }
  const counts: Record<string, number> = {};
  for (const [key, count] of Object.entries(value)) {
    if (typeof count !== 'number') throw new Error('记忆统计格式不正确');
    counts[key] = count;
  }
  return counts;
}

/**
 * 分页读取记忆。all_visible 会合并用户层和指定 Agent 层。
 * 显式带上 limit 与 cursor，避免退回旧的完整列表。
 */
export async function listMemories(
  filter: {
    scope: string;
    agent_id?: string;
    namespace?: string;
    status?: string;
    limit: number;
    cursor?: string;
  },
  signal?: AbortSignal,
) {
  const params = new URLSearchParams();
  params.set('scope', filter.scope);
  params.set('limit', String(filter.limit));
  params.set('cursor', filter.cursor ?? '');
  if (filter.agent_id) params.set('agent_id', filter.agent_id);
  if (filter.namespace) params.set('namespace', filter.namespace);
  if (filter.status) params.set('status', filter.status);
  const result = await request<{ items: MemoryItem[]; next_cursor?: string }>(
    `/memories?${params.toString()}`,
    { signal },
  );
  if (!Array.isArray(result.items)) throw new Error('记忆列表格式不正确');
  if (result.next_cursor != null && typeof result.next_cursor !== 'string') {
    throw new Error('记忆列表格式不正确');
  }
  return {
    items: result.items,
    nextCursor: result.next_cursor ?? '',
  } satisfies MemoryPage;
}

/** 沿游标数完当前筛选下的全部条目，供卡片显示总数。 */
export async function countMemories(
  filter: {
    scope: string;
    agent_id?: string;
    namespace?: string;
    status?: string;
  },
  signal?: AbortSignal,
) {
  let cursor = '';
  let total = 0;
  for (;;) {
    const page = await listMemories({ ...filter, limit: 200, cursor }, signal);
    total += page.items.length;
    if (!page.nextCursor) return total;
    if (page.nextCursor === cursor) throw new Error('记忆分页游标没有前进');
    cursor = page.nextCursor;
  }
}

/** 当前 Agent 可读的 memory.md。关闭或没有条目时 markdown 为空字符串。 */
export async function getMemoryMarkdown(agentID: string, signal?: AbortSignal) {
  const result = await request<MemoryView>(
    `/agents/${encodeURIComponent(agentID)}/memory`,
    { signal },
  );
  if (
    typeof result.markdown !== 'string' ||
    typeof result.estimated_tokens !== 'number' ||
    !Array.isArray(result.item_ids)
  ) {
    throw new Error('memory.md 格式不正确');
  }
  return result;
}

export const createMemory = (input: MemoryCreate) =>
  request<MemoryItem>('/memories', {
    method: 'POST',
    body: JSON.stringify(manualMemoryBody(input)),
  });

/** 用新条目取代旧条目。旧 id 再编辑会返回 409。 */
export const updateMemory = (id: string, input: MemoryEdit) =>
  request<MemoryItem>(`/memories/${encodeURIComponent(id)}`, {
    method: 'PUT',
    body: JSON.stringify(input),
  });

export const deleteMemory = (id: string) =>
  request<void>(`/memories/${encodeURIComponent(id)}`, { method: 'DELETE' });

/** 把已归档或已删除的版本恢复成新的生效条目。 */
export const restoreMemory = (id: string) =>
  request<MemoryItem>(`/memories/${encodeURIComponent(id)}/restore`, {
    method: 'POST',
  });

export async function listMemoryEvents(id: string, signal?: AbortSignal) {
  const result = await request<{ events: MemoryEvent[] }>(
    `/memories/${encodeURIComponent(id)}/events`,
    { signal },
  );
  if (!Array.isArray(result.events)) throw new Error('记忆历史格式不正确');
  return result.events;
}

export async function getMemoryStats(signal?: AbortSignal) {
  const result = await request<Record<string, unknown>>('/memory/stats', {
    signal,
  });
  if (
    typeof result.pending !== 'number' ||
    typeof result.failed !== 'number' ||
    typeof result.dead !== 'number'
  ) {
    throw new Error('记忆统计格式不正确');
  }
  if (
    result.latest_summary_at != null &&
    typeof result.latest_summary_at !== 'string'
  ) {
    throw new Error('记忆统计格式不正确');
  }
  if (
    result.latest_maintain_at != null &&
    typeof result.latest_maintain_at !== 'string'
  ) {
    throw new Error('记忆统计格式不正确');
  }
  return {
    status: readCountMap(result.status),
    namespace: readCountMap(result.namespace),
    pending: result.pending,
    failed: result.failed,
    dead: result.dead,
    latest_summary_at:
      typeof result.latest_summary_at === 'string'
        ? result.latest_summary_at
        : undefined,
    latest_maintain_at:
      typeof result.latest_maintain_at === 'string'
        ? result.latest_maintain_at
        : undefined,
    deadJobs: parseDeadJobs(result.dead_jobs),
  } satisfies MemoryStats;
}

/** 只允许重试已经失败到 dead 的整理任务。成功时没有响应体。 */
export const retryMemoryJob = (id: string) =>
  request<void>(`/memory/jobs/${encodeURIComponent(id)}/retry`, {
    method: 'POST',
  });

/**
 * 提交开关，以及调用方显式修改的遗忘开关和召回条数。
 * 未出现的预算字段由服务端保留当前值。
 */
export async function saveMemoryPolicy(
  agentID: string,
  input: {
    expected_version_id: string;
    enabled: boolean;
    recall_top_k?: number;
    decay_enabled?: boolean;
  },
) {
  const result = await request<MemoryPolicyResult>(
    `/agents/${encodeURIComponent(agentID)}/memory-policy`,
    { method: 'PUT', body: JSON.stringify(input) },
  );
  if (
    typeof result.current_version_id !== 'string' ||
    typeof result.version_number !== 'number' ||
    result.memory_policy == null ||
    typeof result.memory_policy.enabled !== 'boolean' ||
    typeof result.memory_policy.recall_top_k !== 'number' ||
    typeof result.memory_policy.decay_enabled !== 'boolean'
  ) {
    throw new Error('记忆开关返回格式不正确');
  }
  return result;
}
