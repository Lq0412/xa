import { history, Link, useLocation, useModel } from '@umijs/max';
import { type FormEvent, useState } from 'react';
import { flushSync } from 'react-dom';
import { Icon } from '@/components/Icon';
import { Button } from '@/components/ui/button';
import {
  errorMessage,
  login,
  loginRedirect,
  register,
  TOKEN_KEY,
} from '@/services/api';

const fieldClass =
  'w-full rounded-xl border border-zinc-200 bg-white px-3 py-2 text-sm outline-none focus:border-zinc-400';

/** 邮箱认证使用浏览器表单校验，服务端保留最终的身份判定。 */
export default function AuthPage() {
  const location = useLocation();
  const registering = location.pathname === '/register';
  const { setInitialState } = useModel('@@initialState');
  const [error, setError] = useState('');
  const [pending, setPending] = useState(false);
  const submit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const data = new FormData(event.currentTarget);
    setPending(true);
    setError('');
    try {
      const email = String(data.get('email')).trim();
      const password = String(data.get('password'));
      if (registering)
        await register(
          email,
          password,
          String(data.get('display_name') || '').trim(),
        );
      const result = await login(email, password);
      localStorage.setItem(TOKEN_KEY, result.token);
      flushSync(() => {
        void setInitialState({ currentUser: result.user });
      });
      history.replace(loginRedirect());
    } catch (cause) {
      setError(errorMessage(cause));
    } finally {
      setPending(false);
    }
  };
  return (
    <div className="relative flex min-h-dvh items-center justify-center bg-white px-6 py-16">
      <Link
        to="/explore"
        className="absolute top-6 left-6 inline-flex items-center gap-1 text-sm text-zinc-500"
      >
        <Icon name="arrow-left" size={16} />
        浏览 Agent
      </Link>
      <div className="w-full max-w-[360px]">
        <h1 className="mb-8 text-2xl font-medium">
          {registering ? '创建账号' : '登录'}
        </h1>
        <form key={location.pathname} onSubmit={submit}>
          {error && (
            <div
              className="mb-4 rounded-xl bg-rose-50 px-3.5 py-3 text-sm text-rose-800"
              role="alert"
            >
              {error}
            </div>
          )}
          <fieldset className="grid gap-4 border-0 p-0" disabled={pending}>
            {registering && (
              <label className="grid gap-2 text-sm text-zinc-600">
                昵称
                <input
                  className={fieldClass}
                  name="display_name"
                  maxLength={80}
                  autoComplete="nickname"
                />
              </label>
            )}
            <label className="grid gap-2 text-sm text-zinc-600">
              邮箱
              <input
                className={fieldClass}
                name="email"
                type="email"
                required
                maxLength={320}
                autoComplete="email"
              />
            </label>
            <label className="grid gap-2 text-sm text-zinc-600">
              密码
              <input
                className={fieldClass}
                name="password"
                type="password"
                required
                minLength={8}
                maxLength={128}
                autoComplete={registering ? 'new-password' : 'current-password'}
              />
            </label>
            <Button type="submit" className="w-full">
              {pending ? '正在登录…' : registering ? '注册并登录' : '登录'}
            </Button>
          </fieldset>
        </form>
        <div className="mt-6 text-center text-sm text-zinc-500">
          <Link
            onClick={() => setError('')}
            to={`${registering ? '/login' : '/register'}${location.search}`}
          >
            {registering ? '已有账号？登录' : '创建账号'}
          </Link>
        </div>
      </div>
    </div>
  );
}
