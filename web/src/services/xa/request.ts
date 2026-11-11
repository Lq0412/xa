import { redirectToLogin, TOKEN_KEY } from './session';

type ErrorBody = Partial<API.ApiError>;
type Options = {
  method?: string;
  data?: unknown;
  anonymous?: boolean;
  signal?: AbortSignal;
  headers?: HeadersInit;
  params?: Record<string, string | number | boolean | undefined>;
};

/** 保留 HTTP 状态和服务端错误，页面据此显示表单或列表反馈。 */
export class ApiError extends Error {
  constructor(
    readonly status: number,
    readonly code: string,
    message: string,
    readonly requestId?: string,
  ) {
    super(message);
    this.name = 'ApiError';
  }
}

/** 请求统一走同源代理，会话失效时回到登录并保留站内地址。 */
export async function request<T>(
  path: string,
  options: Options = {},
): Promise<T> {
  const headers = new Headers(options.headers);
  headers.set('Accept', 'application/json');
  const token = options.anonymous ? null : localStorage.getItem(TOKEN_KEY);
  if (token) headers.set('Authorization', `Bearer ${token}`);
  if (options.data !== undefined)
    headers.set('Content-Type', 'application/json');
  const query = new URLSearchParams();
  for (const [key, value] of Object.entries(options.params ?? {})) {
    if (value !== undefined) query.append(key, String(value));
  }
  const url = `${path}${query.size ? `?${query}` : ''}`;
  const timeout = AbortSignal.timeout(15000);
  const signal = options.signal
    ? AbortSignal.any([timeout, options.signal])
    : timeout;
  let response: Response;
  try {
    response = await fetch(url, {
      method: options.method,
      headers,
      body:
        options.data === undefined ? undefined : JSON.stringify(options.data),
      signal,
    });
  } catch (cause) {
    if (!(cause instanceof TypeError) && !signal.aborted) throw cause;
    throw new ApiError(
      0,
      'network_error',
      signal.aborted ? '请求超时，请重试' : '无法连接服务，请检查网络后重试',
    );
  }
  if (response.status === 401 && !options.anonymous) {
    localStorage.removeItem(TOKEN_KEY);
    redirectToLogin();
  }
  if (!response.ok) {
    const body: ErrorBody | undefined = response.headers
      .get('content-type')
      ?.includes('application/json')
      ? await response.json()
      : undefined;
    throw new ApiError(
      response.status,
      body?.code ?? 'request_failed',
      body?.message ?? `请求失败（${response.status}）`,
      body?.request_id,
    );
  }
  return response.status === 204 ? (undefined as T) : response.json();
}
