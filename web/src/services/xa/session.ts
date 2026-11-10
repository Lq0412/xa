import { history } from 'umi';

/** 与已有 Web 登录保持相同的存储键；有效身份始终以服务端为准。 */
export const TOKEN_KEY = 'xa.web.session';
export const LOGIN_PATH = '/user/login';

/** 登录和注册可直接访问，不受管理页登录检查影响。 */
export function isAuthPage(path: string) {
  return path === LOGIN_PATH || path === '/user/register';
}

/** 会话失效时保留原站内地址，避免回跳到登录页本身。 */
export function redirectToLogin() {
  const { pathname, search, hash } = history.location;
  if (!isAuthPage(pathname)) {
    history.replace(
      `${LOGIN_PATH}?redirect=${encodeURIComponent(pathname + search + hash)}`,
    );
  }
}

/** 只接受同源站内回跳，阻止双斜线和反斜线被浏览器解释为外站地址。 */
export function getLoginRedirect() {
  const value = new URLSearchParams(window.location.search).get('redirect');
  if (!value?.startsWith('/') || value.startsWith('//') || value.includes('\\'))
    return '/overview';
  const url = new URL(value, window.location.origin);
  return url.origin === window.location.origin && !isAuthPage(url.pathname)
    ? url.pathname + url.search + url.hash
    : '/overview';
}
