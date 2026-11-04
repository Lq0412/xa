import {
  AppstoreOutlined,
  CommentOutlined,
  LogoutOutlined,
  SettingOutlined,
  TeamOutlined,
  UserOutlined,
} from '@ant-design/icons';
import { history, Link, Outlet, useLocation, useModel } from '@umijs/max';
import { Alert, App, Avatar, Button, Tooltip } from 'antd';
import { useEffect } from 'react';
import { errorMessage, logout, TOKEN_KEY } from '@/services/api';

/** 用户端导航只放使用者功能；聊天采用独立会话栏，其他页面使用文字导航。 */
const navigation = [
  { path: '/chat', name: '对话', icon: <CommentOutlined /> },
  { path: '/explore', name: '发现 Agent', icon: <AppstoreOutlined /> },
  { path: '/my-agents', name: '我的 Agent', icon: <TeamOutlined /> },
];

/** 所有受保护页面共用身份守卫；接口仍由服务端校验最终权限。 */
export default function AppLayout() {
  const { pathname, search } = useLocation();
  const { initialState, setInitialState } = useModel('@@initialState');
  const user = initialState?.currentUser;
  const { message } = App.useApp();
  const requiresLogin = ['/my-agents', '/chat'].includes(pathname);
  useEffect(() => {
    if (requiresLogin && !user && !initialState?.sessionError)
      history.replace(
        `/login?redirect=${encodeURIComponent(pathname + search)}`,
      );
  }, [requiresLogin, user, pathname, search, initialState?.sessionError]);

  const signOut = async () => {
    try {
      await logout();
      localStorage.removeItem(TOKEN_KEY);
      await setInitialState({});
      history.replace('/explore');
    } catch (cause) {
      message.error(errorMessage(cause));
    }
  };

  return (
    <div className="user-app">
      <nav className="icon-rail" aria-label="主导航">
        <Link to="/explore" className="brand-mark" aria-label="xa 首页">
          xa
        </Link>
        <div className="rail-links">
          {navigation.map((item) => (
            <Tooltip key={item.path} title={item.name} placement="right">
              <Link
                className={`rail-link ${pathname === item.path ? 'active' : ''}`}
                to={item.path}
                aria-label={item.name}
                aria-current={pathname === item.path ? 'page' : undefined}
              >
                {item.icon}
              </Link>
            </Tooltip>
          ))}
        </div>
        <div className="rail-bottom">
          {user?.role === 'admin' && process.env.XA_ADMIN_WEB_URL && (
            <Tooltip title="管理后台" placement="right">
              <a
                className="rail-link"
                href={process.env.XA_ADMIN_WEB_URL}
                target="_blank"
                rel="noreferrer"
                aria-label="管理后台"
              >
                <SettingOutlined />
              </a>
            </Tooltip>
          )}
          {user ? (
            <Tooltip title={user.display_name || user.email} placement="right">
              <span className="rail-link" role="img" aria-label="当前账号">
                <Avatar size={30} icon={<UserOutlined />} />
              </span>
            </Tooltip>
          ) : (
            <Tooltip title="登录" placement="right">
              <Link to="/login" className="rail-link" aria-label="登录">
                <Avatar size={30} icon={<UserOutlined />} />
              </Link>
            </Tooltip>
          )}
          {user && (
            <Tooltip title="退出登录" placement="right">
              <Button
                type="text"
                className="rail-link"
                icon={<LogoutOutlined />}
                aria-label="退出登录"
                onClick={() => void signOut()}
              />
            </Tooltip>
          )}
        </div>
      </nav>
      {pathname !== '/chat' && (
        <aside className="workspace-nav">
          <div className="workspace-title">我的工作空间</div>
          <nav aria-label="工作空间导航">
            {navigation.map((item) => (
              <Link
                key={item.path}
                to={item.path}
                className={pathname === item.path ? 'selected' : ''}
              >
                {item.icon}
                <span>{item.name}</span>
              </Link>
            ))}
          </nav>
          <div className="workspace-account">
            {user ? (
              <span>{user.display_name || user.email}</span>
            ) : (
              <Link to="/login">登录或创建账号</Link>
            )}
          </div>
        </aside>
      )}
      <main className={`user-main ${pathname === '/chat' ? 'chat-main' : ''}`}>
        {initialState?.sessionError ? (
          <div className="page-content">
            <Alert
              type="error"
              title="读取登录状态失败"
              description={initialState.sessionError}
              showIcon
              action={<Button onClick={() => location.reload()}>重试</Button>}
            />
          </div>
        ) : requiresLogin && !user ? null : (
          <Outlet />
        )}
      </main>
    </div>
  );
}
