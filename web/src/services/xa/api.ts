import { request } from '@umijs/max';

/** 登录失败交给表单展示，不触发会话过期跳转。 */
export async function login(email: string, password: string) {
  return request<XA.LoginResult>('/api/v1/auth/login', {
    method: 'POST',
    data: { email, password },
    skipErrorHandler: true,
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
    skipErrorHandler: true,
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
export async function listAgents() {
  return request<{ agents: XA.Agent[] | null }>('/api/v1/agents');
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
