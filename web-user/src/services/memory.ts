import { request } from './http';

/** 记忆条目。编辑不会原地修改，成功后返回新的 id。 */
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
  importance: number;
  pinned: boolean;
  status: 'active' | 'superseded' | 'deleted';
  source: 'auto' | 'user_edited';
  source_ref: string | null;
  supersedes_id: string | null;
  created_at: string;
  updated_at: string;
  last_used_at: string | null;
};

/** 一条记忆的审计事件，沿取代链返回。 */
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

/** Agent 版本上的记忆开关。缺省字段与服务端默认值一致。 */
export type MemoryPolicy = {
  enabled: boolean;
  core_budget_tokens: number;
  recall_top_k: number;
};

/** memory.md 是按预算渲染的只读视图，不是另一份存储。 */
export type MemoryView = {
  markdown: string;
  estimated_tokens: number;
  item_ids: string[];
};

/** 保存开关后的新版本。再次修改必须使用 current_version_id。 */
export type MemoryPolicyResult = {
  current_version_id: string;
  version_number: number;
  memory_policy: MemoryPolicy;
};

/** 用户编辑可改的字段。归属和 subject_key 由服务端保留。 */
export type MemoryEdit = {
  content: string;
  namespace: string;
  importance: number;
  pinned: boolean;
};

const defaults: MemoryPolicy = {
  enabled: true,
  core_budget_tokens: 800,
  recall_top_k: 5,
};

/** 读取配置里的 memory_policy。null 表示沿用服务端默认：开启。 */
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
    (value.recall_top_k != null && typeof value.recall_top_k !== 'number')
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
  };
}

/** agent_id 只匹配该 Agent 的私有记忆，不包含 scope=user。 */
export async function listMemories(
  filter: {
    scope: string;
    agent_id?: string;
    namespace?: string;
    status?: string;
  },
  signal?: AbortSignal,
) {
  const params = new URLSearchParams();
  params.set('scope', filter.scope);
  if (filter.agent_id) params.set('agent_id', filter.agent_id);
  if (filter.namespace) params.set('namespace', filter.namespace);
  if (filter.status) params.set('status', filter.status);
  const result = await request<{ items: MemoryItem[] }>(
    `/memories?${params.toString()}`,
    { signal },
  );
  if (!Array.isArray(result.items)) throw new Error('记忆列表格式不正确');
  return result.items;
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

/** 用新条目取代旧条目。旧 id 再编辑会返回 409。 */
export const updateMemory = (id: string, input: MemoryEdit) =>
  request<MemoryItem>(`/memories/${encodeURIComponent(id)}`, {
    method: 'PUT',
    body: JSON.stringify(input),
  });

export const deleteMemory = (id: string) =>
  request<void>(`/memories/${encodeURIComponent(id)}`, { method: 'DELETE' });

export async function listMemoryEvents(id: string, signal?: AbortSignal) {
  const result = await request<{ events: MemoryEvent[] }>(
    `/memories/${encodeURIComponent(id)}/events`,
    { signal },
  );
  if (!Array.isArray(result.events)) throw new Error('记忆历史格式不正确');
  return result.events;
}

/** 只提交开关和当前版本号，预算保持服务端已有值。 */
export async function saveMemoryPolicy(
  agentID: string,
  input: { expected_version_id: string; enabled: boolean },
) {
  const result = await request<MemoryPolicyResult>(
    `/agents/${encodeURIComponent(agentID)}/memory-policy`,
    { method: 'PUT', body: JSON.stringify(input) },
  );
  if (
    typeof result.current_version_id !== 'string' ||
    typeof result.version_number !== 'number' ||
    result.memory_policy == null ||
    typeof result.memory_policy.enabled !== 'boolean'
  ) {
    throw new Error('记忆开关返回格式不正确');
  }
  return result;
}
