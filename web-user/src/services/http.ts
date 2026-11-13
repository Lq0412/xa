import { history } from '@umijs/max';

/** 用户端身份只在本站保存，JSON 与 SSE 请求共用认证和错误处理。 */
export const TOKEN_KEY = 'xa.user.session';

/** 保留 HTTP 状态码，让页面区分权限、业务和连接错误。 */
export class ApiError extends Error {
  constructor(
    readonly status: number,
    message: string,
  ) {
    super(message);
    this.name = 'ApiError';
  }
}

function streamApiRoot(path: string, method: string) {
  if (path.startsWith('/health/')) return '';
  const base = String(process.env.XA_API_STREAM_BASE || '').replace(/\/$/, '');
  if (!base) return '/api/v1';
  const stream =
    (method === 'POST' && /\/conversations\/[^/]+\/messages$/.test(path)) ||
    /\/runs\/[^/]+\/events/.test(path);
  return stream ? `${base}/api/v1` : '/api/v1';
}

/** 发起认证请求；调用者决定读取 JSON 还是 SSE 响应体。 */
export async function fetchApi(
  path: string,
  options: RequestInit = {},
  checkSession = true,
) {
  const token = localStorage.getItem(TOKEN_KEY);
  const method = (options.method || 'GET').toUpperCase();
  const response = await fetch(`${streamApiRoot(path, method)}${path}`, {
    ...options,
    headers: {
      ...(options.body instanceof FormData
        ? {}
        : { 'Content-Type': 'application/json' }),
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
      ...options.headers,
    },
  });
  if (!response.ok) {
    if (response.status === 401 && checkSession) {
      localStorage.removeItem(TOKEN_KEY);
      history.replace(
        `/login?redirect=${encodeURIComponent(location.pathname + location.search)}`,
      );
    }
    const body = response.headers
      .get('content-type')
      ?.includes('application/json')
      ? await response.json()
      : undefined;
    throw new ApiError(
      response.status,
      body?.message || `请求失败（${response.status}）`,
    );
  }
  return response;
}

/** JSON 接口保留 204 的空响应语义。 */
export async function request<T>(
  path: string,
  options: RequestInit = {},
  checkSession = true,
): Promise<T> {
  const response = await fetchApi(path, options, checkSession);
  return response.status === 204 ? (undefined as T) : response.json();
}

/** 预期的服务错误显示给用户，程序错误继续抛出。 */
export function errorMessage(cause: unknown): string {
  if (cause instanceof ApiError) return cause.message;
  if (
    cause instanceof TypeError &&
    /fetch|network|load failed|terminated/i.test(cause.message)
  )
    return '无法连接服务，请检查网络后重试';
  throw cause;
}
