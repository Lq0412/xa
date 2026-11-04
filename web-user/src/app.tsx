import {
  ApiError,
  errorMessage,
  getMe,
  TOKEN_KEY,
  type User,
} from '@/services/api';

/** 公共列表无需登录；存在令牌时向后端核验，服务故障明确保留为错误状态。 */
export async function getInitialState(): Promise<{
  currentUser?: User;
  sessionError?: string;
}> {
  if (!localStorage.getItem(TOKEN_KEY)) return {};
  try {
    return { currentUser: await getMe() };
  } catch (cause) {
    if (cause instanceof ApiError && cause.status === 401) return {};
    return { sessionError: errorMessage(cause) };
  }
}
