import { history, Link, Outlet, useLocation, useModel } from '@umijs/max';
import type { ReactNode } from 'react';
import { useEffect, useState } from 'react';
import { Dialog } from '@/components/Dialog';
import { Icon, type IconName } from '@/components/Icon';
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip';
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
      <TooltipProvider delayDuration={300}>
        <WorkspaceLayout />
      </TooltipProvider>
    </DetailsPreference>
  );
}

function RailTip({ label, children }: { label: string; children: ReactNode }) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>{children}</TooltipTrigger>
      <TooltipContent side="right">{label}</TooltipContent>
    </Tooltip>
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
    <div className="user-app flex h-dvh overflow-hidden bg-white text-zinc-900">
      <nav
        className="icon-rail relative flex h-full w-14 shrink-0 flex-col border-r border-zinc-200 bg-[#f6f6f7]"
        aria-label="主导航"
      >
        <div className="rail-links">
          {navigation.map((item) => (
            <RailTip key={item.path} label={item.name}>
              <Link
                className="rail-link"
                to={item.path}
                aria-label={item.name}
                title={item.name}
                aria-current={pathname === item.path ? 'page' : undefined}
              >
                <Icon name={item.icon} size={20} />
              </Link>
            </RailTip>
          ))}
        </div>
        <div className="rail-bottom">
          <RailTip label="设置">
            <button
              type="button"
              className="rail-link"
              onClick={() => setSettings(true)}
              aria-label="设置"
              title="设置"
            >
              <Icon name="settings" size={20} />
            </button>
          </RailTip>
          {user ? (
            <RailTip label={user.display_name || user.email}>
              <button
                type="button"
                className="rail-link"
                onClick={() => setSettings(true)}
                aria-label="当前账号"
                title={user.display_name || user.email}
              >
                <span className="flex size-7 items-center justify-center rounded-full bg-zinc-200 text-xs text-zinc-600">
                  {(user.display_name || user.email).charAt(0)}
                </span>
              </button>
            </RailTip>
          ) : (
            <RailTip label="登录">
              <Link
                to="/login"
                className="rail-link"
                aria-label="登录"
                title="登录"
              >
                <Icon name="user" size={20} />
              </Link>
            </RailTip>
          )}
          {user && (
            <RailTip label="退出登录">
              <button
                type="button"
                className="rail-link"
                aria-label="退出登录"
                title="退出登录"
                disabled={signingOut}
                onClick={() => void signOut()}
              >
                <Icon name="logout" size={20} />
              </button>
            </RailTip>
          )}
        </div>
      </nav>
      <main
        className={`flex min-h-0 min-w-0 flex-1 flex-col ${pathname === '/chat' ? 'overflow-hidden' : 'overflow-auto'}`}
      >
        {error && (
          <div
            className="m-4 flex items-center justify-between gap-3 rounded-xl bg-rose-50 px-3.5 py-3 text-sm text-rose-800"
            role="alert"
          >
            {error}
            <button
              type="button"
              className="shrink-0 underline"
              onClick={() => setError('')}
            >
              关闭
            </button>
          </div>
        )}
        {initialState?.sessionError ? (
          <div className="px-10 py-12">
            <div
              className="flex items-center justify-between gap-3 rounded-xl bg-rose-50 px-3.5 py-3 text-sm text-rose-800"
              role="alert"
            >
              {initialState.sessionError}
              <button
                type="button"
                className="shrink-0 underline"
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
          <div className="px-5 pb-5 text-sm">
            {user && <p className="mb-4 text-zinc-500">{user.email}</p>}
            <label className="flex items-center justify-between gap-4 py-3">
              展开 Agent 配置
              <input
                type="checkbox"
                role="switch"
                className="size-4 accent-[#f5c518]"
                aria-checked={details.open}
                checked={details.open}
                onChange={(event) => details.setOpen(event.target.checked)}
              />
            </label>
            {user?.role === 'admin' && process.env.XA_ADMIN_WEB_URL && (
              <a
                className="flex items-center justify-between py-3"
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
