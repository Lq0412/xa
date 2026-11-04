import { UserOutlined } from '@ant-design/icons';
import type {
  RequestConfig,
  RequestOptions,
  RunTimeLayoutConfig,
} from '@umijs/max';
import { Link } from '@umijs/max';
import { Button, Result } from 'antd';
import { AvatarDropdown, Footer } from '@/components';
import { getCurrentUser } from '@/services/xa/api';
import { isAuthPage, redirectToLogin, TOKEN_KEY } from '@/services/xa/session';
import defaultSettings from '../config/defaultSettings';
import { ApiError, errorConfig } from './requestErrorConfig';

/** 应用启动时恢复 Redis 会话；网络故障明确展示，不能误判为会话过期。 */
export async function getInitialState(): Promise<{
  currentUser?: XA.User;
  sessionError?: string;
}> {
  if (isAuthPage(window.location.pathname)) return {};
  if (!window.localStorage.getItem(TOKEN_KEY)) {
    redirectToLogin();
    return {};
  }
  try {
    return { currentUser: await getCurrentUser() };
  } catch (cause) {
    if (!(cause instanceof ApiError)) throw cause;
    if (cause.status === 401) return {};
    return { sessionError: cause.message };
  }
}

/** 使用 ProLayout 提供导航、账号菜单、折叠侧栏和响应式布局。 */
export const layout: RunTimeLayoutConfig = ({ initialState }) => ({
  ...defaultSettings,
  menu: { locale: true },
  avatarProps: {
    icon: <UserOutlined />,
    title:
      initialState?.currentUser?.display_name ||
      initialState?.currentUser?.email,
    render: (_, children) => <AvatarDropdown>{children}</AvatarDropdown>,
  },
  menuItemRender: (item, dom) =>
    item.path ? <Link to={item.path}>{dom}</Link> : dom,
  footerRender: () => <Footer />,
  onPageChange: () => {
    if (!initialState?.currentUser && !initialState?.sessionError)
      redirectToLogin();
  },
  childrenRender: (children) => {
    if (initialState?.sessionError) {
      return (
        <Result
          status="error"
          title="读取登录状态失败"
          subTitle={initialState.sessionError}
          extra={
            <Button type="primary" onClick={() => window.location.reload()}>
              重新加载
            </Button>
          }
        />
      );
    }
    return initialState?.currentUser ? children : null;
  },
});

/** 所有业务请求统一携带 Bearer 令牌，错误交给页面的可见反馈处理。 */
export const request: RequestConfig = {
  ...errorConfig,
  timeout: 15000,
  requestInterceptors: [
    (options: RequestOptions) => {
      const token = window.localStorage.getItem(TOKEN_KEY);
      return {
        ...options,
        headers: {
          ...options.headers,
          ...(token ? { Authorization: `Bearer ${token}` } : {}),
        },
      };
    },
  ],
};
