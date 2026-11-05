import { request } from './http';

export { ApiError, errorMessage, TOKEN_KEY } from './http';

/** 身份和归属只信任后端返回的数据。 */
export type User = {
  id: string;
  email: string;
  display_name: string;
  role: 'admin' | 'user';
};
export type Agent = {
  id: string;
  name: string;
  description: string;
  avatar: string;
  owner_id: string;
  owner_user_id?: string;
  owner_type: 'user' | 'system';
  current_version_id: string;
  is_default: boolean;
  is_primary: boolean;
  created_at: string;
  visibility: 'public' | 'private';
  status: 'active' | 'offline' | 'deleted';
  tags: string[] | null;
};
/** SOUL 和模型是不可变版本字段；保存时必须携带当前版本号。 */
export type AgentVersion = {
  id: string;
  version_number: number;
  available_models: string[];
  configuration: { soul_md: string; model: string; [key: string]: unknown };
};
export const listAgentLibrary = (signal?: AbortSignal) =>
  request<{ agents: Agent[] }>('/agent-library', { signal });
export const addPublicAgent = (id: string, signal?: AbortSignal) =>
  request<void>(`/agent-library/${encodeURIComponent(id)}`, {
    method: 'PUT',
    signal,
  });
export const getAgentConfig = (id: string, signal?: AbortSignal) =>
  request<AgentVersion>(`/agents/${encodeURIComponent(id)}/config`, { signal });

/** 基本资料与模块编辑共用版本接口，未编辑的字段原样保留。 */
export const saveAgent = (
  agent: Agent,
  version: AgentVersion,
  values: {
    name?: string;
    description?: string;
    soul_md?: string;
    model?: string;
  },
) =>
  request<{ agent: Agent }>(`/agents/${encodeURIComponent(agent.id)}/config`, {
    method: 'PUT',
    body: JSON.stringify({
      name: values.name ?? agent.name,
      description: values.description ?? agent.description,
      avatar: agent.avatar,
      tags: agent.tags ?? [],
      visibility: agent.visibility,
      expected_version_id: version.id,
      configuration: {
        ...version.configuration,
        // 旧管理端的 prompts 投影不能覆盖用户端的 soul_md 编辑。
        prompts: undefined,
        soul_md: values.soul_md ?? version.configuration.soul_md,
        model: values.model ?? version.configuration.model,
      },
    }),
  });
export const getMe = () => request<User>('/auth/me');
export const login = (email: string, password: string) =>
  request<{ user: User; token: string }>(
    '/auth/login',
    { method: 'POST', body: JSON.stringify({ email, password }) },
    false,
  );
export const register = (
  email: string,
  password: string,
  display_name: string,
) =>
  request<User>(
    '/auth/register',
    { method: 'POST', body: JSON.stringify({ email, password, display_name }) },
    false,
  );
export const logout = () => request<void>('/auth/logout', { method: 'POST' });
export const listAgents = (mine: boolean) =>
  request<{ agents: Agent[] | null }>(mine ? '/agents' : '/available-agents');
export const getAgent = (id: string, signal?: AbortSignal) =>
  request<Agent>(`/agents/${encodeURIComponent(id)}`, { signal });
/** 新建默认模型由服务端配置决定，客户端不猜测白名单。 */
export const createAgent = (
  name: string,
  description: string,
  soul_md: string,
) =>
  request<{ agent: Agent }>('/agents', {
    method: 'POST',
    body: JSON.stringify({
      name,
      description,
      avatar: '',
      tags: [],
      visibility: 'private',
      configuration: { soul_md },
    }),
  });

/** 只允许本站登录回跳，防止开放重定向。 */
export function loginRedirect() {
  const target = new URLSearchParams(location.search).get('redirect');
  if (
    !target?.startsWith('/') ||
    target.startsWith('//') ||
    target.includes('\\')
  )
    return '/chat';
  const url = new URL(target, location.origin);
  return url.origin === location.origin &&
    !['/login', '/register'].includes(url.pathname)
    ? url.pathname + url.search
    : '/chat';
}
