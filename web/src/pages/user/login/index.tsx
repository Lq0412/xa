import { type FormEvent, useState } from 'react';
import { history, Link } from 'umi';
import { ErrorNotice } from '@/components/Page';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/fields';
import { login } from '@/services/xa/api';
import { ApiError } from '@/services/xa/request';
import { getLoginRedirect, TOKEN_KEY } from '@/services/xa/session';

/** 登录失败显示在表单里，登录成功后回到原站内地址。 */
export default function LoginPage() {
  const [error, setError] = useState('');
  const [submitting, setSubmitting] = useState(false);

  async function signIn(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setError('');
    setSubmitting(true);
    const form = new FormData(event.currentTarget);
    try {
      const result = await login(
        String(form.get('email')).trim(),
        String(form.get('password')),
      );
      localStorage.setItem(TOKEN_KEY, result.token);
      history.replace(getLoginRedirect());
    } catch (cause) {
      if (!(cause instanceof ApiError)) throw cause;
      setError(cause.message);
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <main className="grid min-h-dvh place-items-center bg-white px-5 py-12">
      <div className="w-full max-w-85">
        <img
          alt=""
          src="/default-agent-avatar.png"
          className="mb-5 size-10 rounded-xl"
        />
        <h1 className="mb-8 text-3xl font-semibold tracking-tight">
          登录管理端
        </h1>
        <form className="space-y-5" onSubmit={signIn}>
          {error && <ErrorNotice>{error}</ErrorNotice>}
          <label
            htmlFor="login-email"
            className="grid gap-2 text-sm font-medium"
          >
            <span>邮箱</span>
            <Input
              id="login-email"
              type="email"
              name="email"
              autoComplete="email"
              required
              className="h-11"
            />
          </label>
          <label
            htmlFor="login-password"
            className="grid gap-2 text-sm font-medium"
          >
            <span>密码</span>
            <Input
              id="login-password"
              type="password"
              name="password"
              autoComplete="current-password"
              required
              minLength={8}
              className="h-11"
            />
          </label>
          <Button type="submit" disabled={submitting} className="h-11 w-full">
            {submitting ? '登录中…' : '登录'}
          </Button>
        </form>
        <Link
          to="/user/register"
          className="mt-6 inline-block rounded-sm text-sm text-zinc-600 hover:text-zinc-900 hover:underline"
        >
          用户注册与使用入口
        </Link>
      </div>
    </main>
  );
}
