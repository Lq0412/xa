import * as agents from './generated/agents';
import * as auth from './generated/auth';
import { request } from './http';

export { ApiError, errorMessage, TOKEN_KEY } from './http';

/** 身份和归属只信任后端返回的数据。 */
export type User = Awaited<ReturnType<typeof auth.getMe>>;
export type Agent = Awaited<ReturnType<typeof agents.getAgent>>;
/** SOUL 和模型是不可变版本字段；保存时必须携带当前版本号。 */
export type AgentVersion = Awaited<ReturnType<typeof agents.getAgentConfig>>;
export const listAgentLibrary = (signal?: AbortSignal) =>
  request<{ agents: Agent[] }>('/agent-library', { signal });
export const addPublicAgent = (id: string, signal?: AbortSignal) =>
  request<void>(`/agent-library/${encodeURIComponent(id)}`, {
    method: 'PUT',
    signal,
  });
export const getAgentConfig = (id: string, signal?: AbortSignal) =>
  agents.getAgentConfig({ agent_id: encodeURIComponent(id) }, { signal });

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
  agents.saveAgent(
    { agent_id: encodeURIComponent(agent.id) },
    {
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
    },
  );
export const { getMe, logout } = auth;
export const login = (email: string, password: string) =>
  auth.login({ email, password });
export const register = (
  email: string,
  password: string,
  display_name: string,
) => auth.register({ email, password, display_name });
export const listAgents = (mine: boolean) =>
  mine ? agents.listAgents() : agents.availableAgents();
export const getAgent = (id: string, signal?: AbortSignal) =>
  agents.getAgent({ agent_id: encodeURIComponent(id) }, { signal });
/** 新建默认模型由服务端配置决定，客户端不猜测白名单。 */
export const createAgent = (
  name: string,
  description: string,
  soul_md: string,
) =>
  agents.createAgent({
    name,
    description,
    avatar: '',
    tags: [],
    visibility: 'private',
    configuration: { soul_md },
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
