import { request as fetchJSON } from './http';

type RequestOptions = Pick<RequestInit, 'method' | 'headers' | 'signal'> & {
  data?: unknown;
  params?: Record<string, string | number | boolean | undefined>;
};

/** 将 Umi 生成的请求交给用户端已有的 Fetch、认证和错误处理。 */
export function request<T>(
  url: string,
  { method, data, params, headers, signal }: RequestOptions,
): Promise<T> {
  const path = url.replace(/^\/api\/v1(?=\/)/, '');
  const query = new URLSearchParams();
  for (const [key, value] of Object.entries(params ?? {})) {
    if (value !== undefined) query.append(key, String(value));
  }
  const search = query.size ? `?${query}` : '';
  return fetchJSON<T>(
    `${path}${search}`,
    { method, headers, signal, body: JSON.stringify(data) },
    path !== '/auth/login' && path !== '/auth/register',
  );
}
