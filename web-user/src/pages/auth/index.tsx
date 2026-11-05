import { history, Link, useLocation, useModel } from '@umijs/max';
import { type FormEvent, useState } from 'react';
import { flushSync } from 'react-dom';
import { Icon } from '@/components/Icon';
import {
  errorMessage,
  login,
  loginRedirect,
  register,
  TOKEN_KEY,
} from '@/services/api';

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
    <div className="auth-page">
      <Link to="/explore" className="auth-back">
        <Icon name="arrow-left" size={16} />
        浏览 Agent
      </Link>
      <div className="auth-card">
        <h1>{registering ? '创建账号' : '登录'}</h1>
        <form key={location.pathname} className="editor-form" onSubmit={submit}>
          {error && (
            <div className="error-banner" role="alert">
              {error}
            </div>
          )}
          <fieldset disabled={pending}>
            {registering && (
              <label>
                昵称
                <input
                  name="display_name"
                  maxLength={80}
                  autoComplete="nickname"
                />
              </label>
            )}
            <label>
              邮箱
              <input
                name="email"
                type="email"
                required
                maxLength={320}
                autoComplete="email"
              />
            </label>
            <label>
              密码
              <input
                name="password"
                type="password"
                required
                minLength={8}
                maxLength={128}
                autoComplete={registering ? 'new-password' : 'current-password'}
              />
            </label>
            <button type="submit" className="button primary full-width">
              {pending ? '正在登录…' : registering ? '注册并登录' : '登录'}
            </button>
          </fieldset>
        </form>
        <div className="auth-switch">
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
