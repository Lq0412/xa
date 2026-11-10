import { history, matchRoutes } from 'umi';

/** 与已有 Web 登录保持相同的存储键；有效身份始终以服务端为准。 */
export const TOKEN_KEY = 'xa.web.session';
export const LOGIN_PATH = '/user/login';
export const BEFORE_SIGN_OUT = 'xa:before-signout';

const authRoutes = [{ path: LOGIN_PATH }, { path: '/user/register' }];

function isAuthPage(pathname: string) {
  return Boolean(matchRoutes(authRoutes, { pathname }));
}

/** URL 规范化可能产生双斜线，返回地址前再次检查，避免被 history 解释为外站。 */
function safeRedirect(value: string | null) {
  const origin = window.location.origin;
  if (
    !value?.startsWith('/') ||
    value.startsWith('//') ||
    value.includes('\\') ||
    !URL.canParse(value, origin)
  )
    return undefined;
  const url = new URL(value, origin);
  if (
    url.origin === origin &&
    !url.pathname.startsWith('//') &&
    !isAuthPage(url.pathname)
  )
    return url.pathname + url.search + url.hash;
  return undefined;
}

/** 会话失效时保留原站内地址，避免回跳到登录页本身。 */
export function redirectToLogin() {
  const { pathname, search, hash } = history.location;
  if (isAuthPage(pathname)) return;
  const redirect = safeRedirect(pathname + search + hash);
  history.replace(
    redirect
      ? `${LOGIN_PATH}?redirect=${encodeURIComponent(redirect)}`
      : LOGIN_PATH,
  );
}

/** 回到原站内地址，拒绝不安全的地址和登录、注册页面。 */
export function getLoginRedirect() {
  const value = new URLSearchParams(window.location.search).get('redirect');
  return safeRedirect(value) ?? '/overview';
}
