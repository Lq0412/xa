import { request } from './http';

export { ApiError, errorMessage, TOKEN_KEY } from './http';

/** 已认证用户；角色和归属只能由服务端确定。 */
export type User = {
  id: string;
  email: string;
  display_name: string;
  role: 'admin' | 'user';
};
/** 用户可见的 Agent 资料，列表和对话详情共用。 */
export type Agent = {
  id: string;
  name: string;
  description: string;
  avatar: string;
  owner_id: string;
  owner_type: 'user' | 'system';
  current_version_id: string;
  is_default: boolean;
  created_at: string;
  visibility: 'public' | 'private';
  status: 'active' | 'offline' | 'deleted';
  tags: string[] | null;
};

/** 不可变配置版本用于详情展示和编辑冲突检测，未编辑的配置原样保留。 */
export type AgentVersion = {
  id: string;
  runtime_model?: string;
  configuration: {
    prompts: PromptSlot[] | null;
    [key: string]: unknown;
  };
};

/** 每个人格片段的启用状态和顺序均来自服务端。 */
export type PromptSlot = {
  name: string;
  content: string;
  enabled: boolean;
  order: number;
  variables: string[];
  scope: string;
};

/** 已添加公共 Agent 与自有 Agent 由服务端按当前用户汇总。 */
export const listAgentLibrary = (signal?: AbortSignal) =>
  request<{ agents: Agent[] }>('/agent-library', { signal });

/** 添加操作可重复调用，使用用户账号持久保存。 */
export const addPublicAgent = (id: string, signal?: AbortSignal) =>
  request<void>(`/agent-library/${encodeURIComponent(id)}`, {
    method: 'PUT',
    signal,
  });

/** 当前版本包含真实提示词及当前对话模型，不在客户端猜测默认值。 */
export const getAgentConfig = (id: string, signal?: AbortSignal) =>
  request<AgentVersion>(`/agents/${encodeURIComponent(id)}/config`, { signal });

/** 编辑复用现有版本保存接口，保留头像、标签和未在表单中展示的配置。 */
export const saveAgent = (
  agent: Agent,
  version: AgentVersion,
  values: { name: string; description: string; prompt: string },
  promptIndex: number,
) => {
  const prompts = [...(version.configuration.prompts ?? [])];
  if (promptIndex >= 0)
    prompts[promptIndex] = { ...prompts[promptIndex], content: values.prompt };
  else if (values.prompt.trim())
    prompts.push({
      name: 'system',
      content: values.prompt,
      enabled: true,
      order: 0,
      variables: [],
      scope: 'agent',
    });
  return request<{ agent: Agent }>(
    `/agents/${encodeURIComponent(agent.id)}/config`,
    {
      method: 'PUT',
      body: JSON.stringify({
        name: values.name,
        description: values.description,
        avatar: agent.avatar,
        tags: agent.tags ?? [],
        visibility: agent.visibility,
        expected_version_id: version.id,
        configuration: { ...version.configuration, prompts },
      }),
    },
  );
};
/** 恢复当前身份。 */
export const getMe = () => request<User>('/auth/me');
/** 使用邮箱登录现有账号。 */
export const login = (email: string, password: string) =>
  request<{ user: User; token: string }>(
    '/auth/login',
    { method: 'POST', body: JSON.stringify({ email, password }) },
    false,
  );
/** 公开注册只创建普通账号。 */
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
/** 成功撤销 Redis 会话后才能清除本地状态。 */
export const logout = () => request<void>('/auth/logout', { method: 'POST' });
/** 公共列表可匿名访问；我的 Agent 由后端按登录身份过滤。 */
export const listAgents = (mine: boolean) =>
  request<{ agents: Agent[] | null }>(mine ? '/agents' : '/available-agents');
/** 对话页按 ID 从后端读取可见 Agent，禁止相信 URL 中的名称或归属。 */
export const getAgent = (id: string, signal?: AbortSignal) =>
  request<Agent>(`/agents/${encodeURIComponent(id)}`, { signal });
/** 用户端创建仅限私有 Agent，平台资源凭证不进入表单。 */
export const createAgent = (
  name: string,
  description: string,
  prompt: string,
) =>
  request<{ agent: Agent }>('/agents', {
    method: 'POST',
    body: JSON.stringify({
      name,
      description,
      avatar: '',
      tags: [],
      visibility: 'private',
      configuration: {
        prompts: prompt.trim()
          ? [
              {
                name: 'system',
                content: prompt.trim(),
                enabled: true,
                order: 0,
                variables: [],
                scope: 'agent',
              },
            ]
          : [],
      },
    }),
  });

/** 登录只回跳本站路径，防止开放重定向。 */
export function loginRedirect() {
  const target = new URLSearchParams(location.search).get('redirect');
  if (
    !target?.startsWith('/') ||
    target.startsWith('//') ||
    target.includes('\\')
  )
    return '/explore';
  const url = new URL(target, location.origin);
  return url.origin === location.origin &&
    !['/login', '/register'].includes(url.pathname)
    ? url.pathname + url.search
    : '/explore';
}
