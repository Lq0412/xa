import { request } from './request';

/** 登录失败交给表单展示，不触发会话过期跳转。 */
export async function login(email: string, password: string) {
  return request<XA.LoginResult>('/api/v1/auth/login', {
    method: 'POST',
    data: { email, password },
    anonymous: true,
  });
}

/** 注册接口只创建普通用户，角色由服务端确定。 */
export async function register(
  email: string,
  password: string,
  displayName: string,
) {
  return request<XA.User>('/api/v1/auth/register', {
    method: 'POST',
    data: { email, password, display_name: displayName },
    anonymous: true,
  });
}

/** 撤销当前 Redis 会话；成功后才能清除本地登录状态。 */
export async function logout() {
  return request<void>('/api/v1/auth/logout', { method: 'POST' });
}

/** 刷新页面时向服务端核对令牌和用户身份。 */
export async function getCurrentUser() {
  return request<XA.User>('/api/v1/auth/me');
}

/** 获取当前用户有权浏览的公共和私有 Agent。 */
export async function listAgents(signal?: AbortSignal) {
  return request<{ agents: XA.Agent[] | null }>('/api/v1/agents', { signal });
}

/** 读取编辑时的最新元数据，避免直接沿用列表缓存。 */
export async function getAgent(agentId: string) {
  return request<XA.Agent>(`/api/v1/agents/${encodeURIComponent(agentId)}`);
}

/** 获取完整配置；编辑器必须保留暂未展示的绑定和策略。 */
export async function getAgentConfig(agentId: string) {
  return request<XA.AgentVersion>(
    `/api/v1/agents/${encodeURIComponent(agentId)}/config`,
  );
}

/** 与后端保存接口一致的字段；头像和完整配置不能因表单省略而丢失。 */
export type AgentSaveInput = {
  name: string;
  avatar: string;
  description: string;
  tags: string[];
  visibility: XA.Agent['visibility'];
  configuration: XA.Configuration;
};

/** 首次保存创建 Agent 及其首个配置版本。 */
export async function createAgent(data: AgentSaveInput) {
  return request<{ agent: XA.Agent; current_version_id: string }>(
    '/api/v1/agents',
    { method: 'POST', data },
  );
}

/** 传递读取时的版本号，过期编辑由后端返回 409，禁止覆盖他人的修改。 */
export async function saveAgent(
  agentId: string,
  expectedVersionId: string,
  data: AgentSaveInput,
) {
  return request<{ agent: XA.Agent; current_version_id: string }>(
    `/api/v1/agents/${encodeURIComponent(agentId)}/config`,
    {
      method: 'PUT',
      data: { ...data, expected_version_id: expectedVersionId },
    },
  );
}

/** 启停操作使用后端权限与状态校验。 */
export async function setAgentStatus(agentId: string, enabled: boolean) {
  return request<void>(
    `/api/v1/agents/${encodeURIComponent(agentId)}/${enabled ? 'enable' : 'offline'}`,
    { method: 'POST' },
  );
}

/** 通过服务端执行软删除，页面仅在成功后刷新。 */
export async function deleteAgent(agentId: string) {
  return request<void>(`/api/v1/agents/${encodeURIComponent(agentId)}`, {
    method: 'DELETE',
  });
}

/** 管理端的审批摘要不包含工具原始参数。 */
export type ApprovalRecord = {
  id: string;
  created_at: string;
  user: string;
  conversation_id: string;
  agent_id: string;
  agent: string;
  tool_name: string;
  title: string | null;
  summary: string | null;
  status: string;
  wait_ms: number;
  decider: string;
  remember: string;
  run_id: string;
};

/** 由服务端筛选并返回下一页游标。 */
export function listApprovals(
  filters: {
    status: string;
    tool_name: string;
    cursor: string;
  },
  signal?: AbortSignal,
) {
  const query = new URLSearchParams({ limit: '50' });
  for (const [name, value] of Object.entries(filters)) {
    if (value) query.set(name, value);
  }
  return request<{ approvals: ApprovalRecord[]; next_cursor: string }>(
    `/api/v1/admin/approvals?${query}`,
    { signal },
  );
}

/** 目录可供管理员读取，公共 Agent 不调用仅限所有者的工具视图接口。 */
export type CatalogTool = {
  name: string;
  title: string;
  available: boolean;
  default_enabled: boolean;
  approval: { default: 'always' | 'never'; overridable: boolean };
};

export function listTools() {
  return request<{ tools: CatalogTool[] }>('/api/v1/tools');
}

/** 概览统计遍历全部游标，不能把第一页的条数当成总数。 */
export async function allApprovals(signal: AbortSignal) {
  const rows: ApprovalRecord[] = [];
  let cursor = '';
  do {
    signal.throwIfAborted();
    const result = await listApprovals(
      { status: '', tool_name: '', cursor },
      signal,
    );
    rows.push(...result.approvals);
    cursor = result.next_cursor;
  } while (cursor);
  return rows;
}
