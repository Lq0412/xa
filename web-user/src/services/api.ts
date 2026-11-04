import { history } from '@umijs/max';

/** 用户端会话独立保存，不向管理端 URL 传递令牌。 */
export const TOKEN_KEY = 'xa.user.session';

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
  visibility: 'public' | 'private';
  status: 'active' | 'offline' | 'deleted';
  tags: string[] | null;
};
/** 保留状态码，供页面区分登录失效与服务错误。 */
export class ApiError extends Error {
  constructor(
    readonly status: number,
    message: string,
  ) {
    super(message);
    this.name = 'ApiError';
  }
}

/** 统一处理真实请求，错误由调用页面展示；401 清除失效令牌并返回登录。 */
async function request<T>(
  path: string,
  options: RequestInit = {},
  checkSession = true,
): Promise<T> {
  const token = localStorage.getItem(TOKEN_KEY);
  const response = await fetch(`/api/v1${path}`, {
    ...options,
    headers: {
      'Content-Type': 'application/json',
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
      ...options.headers,
    },
  });
  if (!response.ok) {
    const body = await response.json();
    if (response.status === 401 && checkSession) {
      localStorage.removeItem(TOKEN_KEY);
      history.replace(
        `/login?redirect=${encodeURIComponent(location.pathname + location.search)}`,
      );
    }
    throw new ApiError(
      response.status,
      body.message || `请求失败（${response.status}）`,
    );
  }
  return response.status === 204 ? (undefined as T) : response.json();
}

/** 将网络失败或明确业务错误转换成可见提示，程序错误继续抛出。 */
export function errorMessage(cause: unknown): string {
  if (cause instanceof ApiError) return cause.message;
  if (cause instanceof TypeError && /fetch|network/i.test(cause.message))
    return '无法连接服务，请检查网络后重试';
  throw cause;
}

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
export const getAgent = (id: string) =>
  request<Agent>(`/agents/${encodeURIComponent(id)}`);
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
