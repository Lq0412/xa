import type { AxiosError, RequestConfig } from '@umijs/max';
import { redirectToLogin, TOKEN_KEY } from '@/services/xa/session';

/** 服务端错误信息，用于展示具体原因和定位对应请求。 */
type ErrorBody = { code?: string; message?: string; request_id?: string };

/** 保留 HTTP 状态，调用方只处理明确的业务错误，其他异常继续抛出。 */
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

/** 将 xa 错误结构传给页面，由 App.useApp 或表单 Alert 展示，避免重复提示。 */
export const errorConfig: RequestConfig = {
  errorConfig: {
    errorHandler(error, options) {
      const cause = error as AxiosError<ErrorBody>;
      const status = cause.response?.status ?? 0;
      const body = cause.response?.data;
      if (status === 401 && !options.skipErrorHandler) {
        window.localStorage.removeItem(TOKEN_KEY);
        redirectToLogin();
      }
      throw new ApiError(
        status,
        body?.code ?? 'request_failed',
        body?.message ??
          (status ? `请求失败（${status}）` : '无法连接服务，请检查网络后重试'),
        body?.request_id,
      );
    },
  },
};
