import * as Dropdown from '@radix-ui/react-dropdown-menu';
import {
  Activity,
  ArrowUpRight,
  Bot,
  ChevronDown,
  Inbox,
  LayoutGrid,
  LogOut,
  Menu,
  X,
} from 'lucide-react';
import { useEffect, useState } from 'react';
import { history, Link, NavLink, Outlet, useOutletContext } from 'umi';
import access from '@/access';
import { Button } from '@/components/ui/button';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogTitle,
} from '@/components/ui/dialog';
import Loading from '@/loading';
import { getCurrentUser, logout } from '@/services/xa/api';
import { ApiError } from '@/services/xa/request';
import { LOGIN_PATH, redirectToLogin, TOKEN_KEY } from '@/services/xa/session';
import { ErrorNotice, ResultPage } from './Page';

const userWeb = process.env.XA_USER_WEB_URL;
const navigation = [
  { path: '/overview', label: '今天', Icon: LayoutGrid },
  { path: '/agents', label: 'Agent', Icon: Bot },
  { path: '/approvals', label: '确认记录', Icon: Inbox },
];
type Session = { user: XA.User } | { error: string };

/** 页面权限沿用服务端确认的账号与原有归属规则。 */
export function useAdminAccess() {
  return access({ currentUser: useOutletContext<XA.User>() });
}

/** 未完成身份核验前不挂载管理页面。 */
export default function AdminLayout() {
  const [session, setSession] = useState<Session>();
  useEffect(() => {
    if (!localStorage.getItem(TOKEN_KEY)) {
      redirectToLogin();
      return;
    }
    let current = true;
    void getCurrentUser().then(
      (user) => {
        if (current) setSession({ user });
      },
      (cause: unknown) => {
        if (!(cause instanceof ApiError)) throw cause;
        if (current && cause.status !== 401)
          setSession({ error: cause.message });
      },
    );
    return () => {
      current = false;
    };
  }, []);
  if (!session) return <Loading />;
  if ('error' in session)
    return (
      <ResultPage title="读取登录状态失败" description={session.error}>
        <Button onClick={() => window.location.reload()}>重新加载</Button>
      </ResultPage>
    );
  return <AdminShell user={session.user} />;
}

function AdminShell({ user }: { user: XA.User }) {
  const [open, setOpen] = useState(false);
  const [signingOut, setSigningOut] = useState(false);
  const [error, setError] = useState('');
  const { canAdmin } = access({ currentUser: user });
  const name = user.display_name || user.email;
  async function signOut() {
    setSigningOut(true);
    setError('');
    try {
      await logout();
      localStorage.removeItem(TOKEN_KEY);
      history.replace(LOGIN_PATH);
    } catch (cause) {
      if (!(cause instanceof ApiError)) throw cause;
      if (cause.status !== 401) setError(cause.message);
    } finally {
      setSigningOut(false);
    }
  }
  const links = (
    <nav aria-label="主导航" className="admin-nav">
      {navigation.map(({ path, label, Icon }) => (
        <NavLink key={path} to={path} onClick={() => setOpen(false)}>
          <Icon size={18} aria-hidden="true" />
          {label}
        </NavLink>
      ))}
      <button type="button" disabled>
        <Activity size={18} aria-hidden="true" />
        运行追踪<span className="ml-auto text-[10px]">即将上线</span>
      </button>
    </nav>
  );
  const brand = (
    <Link to="/overview" className="admin-brand" onClick={() => setOpen(false)}>
      <img alt="" src="/default-agent-avatar.png" />
      管理
    </Link>
  );
  const account = (
    <Dropdown.Root>
      <Dropdown.Trigger asChild>
        <button
          type="button"
          className="account-button"
          aria-label="账号菜单"
          disabled={signingOut}
        >
          <span className="account-avatar">
            {Array.from(name)[0]?.toUpperCase()}
          </span>
          <span className="truncate">{name}</span>
          <ChevronDown size={13} className="ml-auto shrink-0 text-zinc-400" />
        </button>
      </Dropdown.Trigger>
      <Dropdown.Portal>
        <Dropdown.Content
          align="start"
          sideOffset={8}
          className="z-[70] min-w-40 rounded-xl border border-zinc-200 bg-white p-1 shadow-sm"
        >
          <Dropdown.Item
            className="flex cursor-pointer items-center gap-2 rounded-lg px-3 py-2 text-sm outline-none data-[highlighted]:bg-zinc-100"
            onSelect={() => void signOut()}
          >
            <LogOut size={15} />
            退出登录
          </Dropdown.Item>
        </Dropdown.Content>
      </Dropdown.Portal>
    </Dropdown.Root>
  );
  const bottom = (
    <div className="sidebar-bottom">
      {userWeb && (
        <a
          href={userWeb}
          target="_blank"
          rel="noreferrer"
          className="flex items-center gap-2 text-xs text-zinc-500"
        >
          打开用户端
          <ArrowUpRight size={14} />
        </a>
      )}
      {account}
    </div>
  );
  return (
    <div className="admin-shell">
      <aside className="admin-sidebar">
        {brand}
        {canAdmin && links}
        {bottom}
      </aside>
      <div className="admin-workspace">
        <header className="mobile-header">
          <Button
            variant="ghost"
            size="icon"
            aria-label="打开导航"
            onClick={() => setOpen(true)}
          >
            <Menu size={18} />
          </Button>
          {brand}
        </header>
        {error && (
          <div className="px-8 pt-4">
            <ErrorNotice>{error}</ErrorNotice>
          </div>
        )}
        <main>
          {canAdmin ? (
            <Outlet context={user} />
          ) : (
            <ResultPage title="此入口仅供管理员使用">
              {userWeb && (
                <Button asChild>
                  <a href={userWeb}>前往用户端</a>
                </Button>
              )}
            </ResultPage>
          )}
        </main>
      </div>
      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent className="inset-y-0 left-0 flex w-60 flex-col bg-zinc-50 p-4">
          <DialogTitle className="sr-only">管理导航</DialogTitle>
          <DialogDescription className="sr-only">
            切换管理页面和账号
          </DialogDescription>
          <div className="flex items-start justify-between">
            {brand}
            <Button
              variant="ghost"
              size="icon"
              aria-label="关闭导航"
              onClick={() => setOpen(false)}
            >
              <X size={18} />
            </Button>
          </div>
          {canAdmin && links}
          {bottom}
        </DialogContent>
      </Dialog>
    </div>
  );
}
