import { history, Link, Outlet, useLocation, useModel } from '@umijs/max';
import { useEffect, useState } from 'react';
import { Dialog } from '@/components/Dialog';
import { Icon, type IconName } from '@/components/Icon';
import { errorMessage, logout, TOKEN_KEY } from '@/services/api';
import { DetailsPreference, useDetailsPreference } from './DetailsPreference';

const navigation: { path: string; name: string; icon: IconName }[] = [
  { path: '/chat', name: '对话', icon: 'chat' },
  { path: '/explore', name: '发现 Agent', icon: 'grid' },
  { path: '/my-agents', name: '我的 Agent', icon: 'team' },
];

/** 用户端导航和身份守卫，不引入管理端组件或样式。 */
export default function AppLayout() {
  return (
    <DetailsPreference>
      <WorkspaceLayout />
    </DetailsPreference>
  );
}

function WorkspaceLayout() {
  const { pathname, search } = useLocation();
  const { initialState, setInitialState } = useModel('@@initialState');
  const user = initialState?.currentUser;
  const details = useDetailsPreference();
  const [settings, setSettings] = useState(false);
  const [error, setError] = useState('');
  const [signingOut, setSigningOut] = useState(false);
  const requiresLogin = ['/my-agents', '/chat'].includes(pathname);
  useEffect(() => {
    if (requiresLogin && !user && !initialState?.sessionError)
      history.replace(
        `/login?redirect=${encodeURIComponent(pathname + search)}`,
      );
  }, [requiresLogin, user, pathname, search, initialState?.sessionError]);
  const signOut = async () => {
    setSigningOut(true);
    setError('');
    try {
      await logout();
      localStorage.removeItem(TOKEN_KEY);
      await setInitialState({});
      history.replace('/explore');
    } catch (cause) {
      setError(errorMessage(cause));
    } finally {
      setSigningOut(false);
    }
  };
  return (
    <div className="user-app">
      <nav className="icon-rail" aria-label="主导航">
        <div className="rail-links">
          {navigation.map((item) => (
            <Link
              key={item.path}
              className={`rail-link ${pathname === item.path ? 'active' : ''}`}
              to={item.path}
              aria-label={item.name}
              title={item.name}
              aria-current={pathname === item.path ? 'page' : undefined}
            >
              <Icon name={item.icon} size={23} />
            </Link>
          ))}
        </div>
        <div className="rail-bottom">
          <button
            type="button"
            className="rail-link"
            onClick={() => setSettings(true)}
            aria-label="设置"
            title="设置"
          >
            <Icon name="settings" size={22} />
          </button>
          {user ? (
            <button
              type="button"
              className="rail-link"
              onClick={() => setSettings(true)}
              aria-label="当前账号"
              title={user.display_name || user.email}
            >
              <span className="account-avatar">
                {Array.from(user.display_name || user.email)[0]}
              </span>
            </button>
          ) : (
            <Link
              to="/login"
              className="rail-link"
              aria-label="登录"
              title="登录"
            >
              <Icon name="user" />
            </Link>
          )}
          {user && (
            <button
              type="button"
              className="rail-link"
              aria-label="退出登录"
              title="退出登录"
              disabled={signingOut}
              onClick={() => void signOut()}
            >
              <Icon name="logout" size={22} />
            </button>
          )}
        </div>
      </nav>
      <main className={`user-main ${pathname === '/chat' ? 'chat-main' : ''}`}>
        {error && (
          <div className="error-banner" role="alert">
            {error}
            <button
              type="button"
              className="text-button"
              onClick={() => setError('')}
            >
              关闭
            </button>
          </div>
        )}
        {initialState?.sessionError ? (
          <div className="page-content">
            <div className="error-banner" role="alert">
              {initialState.sessionError}
              <button
                type="button"
                className="text-button"
                onClick={() => location.reload()}
              >
                重试
              </button>
            </div>
          </div>
        ) : requiresLogin && !user ? null : (
          <Outlet />
        )}
      </main>
      {settings && (
        <Dialog title="设置" onClose={() => setSettings(false)}>
          <div className="settings-content">
            {user && <p className="account-email">{user.email}</p>}
            <label className="settings-row">
              展开 Agent 配置
              <input
                type="checkbox"
                role="switch"
                aria-checked={details.open}
                checked={details.open}
                onChange={(event) => details.setOpen(event.target.checked)}
              />
            </label>
            {user?.role === 'admin' && process.env.XA_ADMIN_WEB_URL && (
              <a
                className="settings-row"
                href={process.env.XA_ADMIN_WEB_URL}
                target="_blank"
                rel="noreferrer"
              >
                管理后台 <Icon name="arrow-right" size={16} />
              </a>
            )}
          </div>
        </Dialog>
      )}
    </div>
  );
}
