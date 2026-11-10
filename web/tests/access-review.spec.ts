import { expect, type Page, test } from '@playwright/test';

const tokenKey = 'xa.web.session';
const loginPath = '/user/login';
const loginAPI = '/api/v1/auth/login';
const logoutAPI = '/api/v1/auth/logout';
const meAPI = '/api/v1/auth/me';

function loginURL(redirect: string) {
  return `${loginPath}?redirect=${encodeURIComponent(redirect)}`;
}

async function signIn(page: Page) {
  const email = process.env.XA_E2E_EMAIL;
  const password = process.env.XA_E2E_PASSWORD;
  if (!email || !password)
    throw new Error('请设置 XA_E2E_EMAIL 和 XA_E2E_PASSWORD');

  await page.getByLabel('邮箱', { exact: true }).fill(email);
  await page.getByLabel('密码', { exact: true }).fill(password);
  const login = page.waitForResponse(
    (response) => new URL(response.url()).pathname === loginAPI,
  );
  await page.getByRole('button', { name: '登录', exact: true }).click();
  const response = await login;
  expect(response.status()).toBe(200);
  const { token } = await response.json();
  if (typeof token !== 'string' || !token)
    throw new Error('登录成功后缺少会话令牌');
  return token;
}

async function clickSignOut(page: Page) {
  await page.getByRole('button', { name: '账号菜单' }).click();
  await page.getByRole('menuitem', { name: '退出登录' }).click();
}

test('匿名直达和令牌失效均保留确认记录的查询条件与锚点', async ({
  page,
  request,
}) => {
  const target = '/approvals?status=pending#review';
  await page.goto(target);
  await expect(page).toHaveURL(loginURL(target));
  expect((await request.get('/api/v1/admin/approvals')).status()).toBe(401);

  const token = await signIn(page);
  const headers = { Authorization: `Bearer ${token}` };
  try {
    await expect(page).toHaveURL(target);
    await expect(page.getByRole('heading', { name: '确认记录' })).toBeVisible();
    expect((await request.post(logoutAPI, { headers })).status()).toBe(204);
    await page.reload();
    await expect(page).toHaveURL(loginURL(target));
    expect(
      await page.evaluate(
        (key) => localStorage.getItem(key) === null,
        tokenKey,
      ),
    ).toBe(true);
    expect((await request.get(meAPI, { headers })).status()).toBe(401);
  } finally {
    await request.post(logoutAPI, { headers });
  }
});

for (const target of [
  '//example.invalid/x',
  '/\\example.invalid/x',
  '/approvals/..//example.invalid/x',
  '/user/login/?redirect=/agents',
  '/user/register/?source=login',
]) {
  test(`登录忽略不安全的回跳地址 ${target}`, async ({ page, request }) => {
    const errors: string[] = [];
    page.on('pageerror', (error) => errors.push(error.name));
    await page.goto(loginURL(target));
    const token = await signIn(page);
    const headers = { Authorization: `Bearer ${token}` };
    try {
      await expect(page).toHaveURL('/overview');
      expect(errors).toEqual([]);
    } finally {
      expect((await request.post(logoutAPI, { headers })).status()).toBe(204);
    }
  });
}

test('有未保存的 Agent 草稿时先确认，再撤销会话', async ({ page, request }) => {
  await page.goto(loginURL('/agents/new'));
  const token = await signIn(page);
  const headers = { Authorization: `Bearer ${token}` };
  try {
    const name = page.getByLabel('名称', { exact: true });
    await name.fill('未保存的草稿');
    page.once('dialog', (dialog) => dialog.dismiss());
    await clickSignOut(page);
    await expect(page).toHaveURL('/agents/new');
    await expect(name).toHaveValue('未保存的草稿');
    expect((await request.get(meAPI, { headers })).status()).toBe(200);

    const dialogs: string[] = [];
    page.on('dialog', async (dialog) => {
      dialogs.push(dialog.type());
      await dialog.accept();
    });
    await clickSignOut(page);
    await expect(page).toHaveURL(loginPath);
    await expect(name).toHaveCount(0);
    expect(dialogs).toEqual(['confirm']);
    expect((await request.get(meAPI, { headers })).status()).toBe(401);
  } finally {
    await request.post(logoutAPI, { headers });
  }
});

test('编辑 Agent 时收到会话失效响应，离开草稿并回到登录', async ({
  page,
  request,
}) => {
  const target = '/agents/new';
  await page.goto(loginURL(target));
  const token = await signIn(page);
  const headers = { Authorization: `Bearer ${token}` };
  try {
    const name = page.getByLabel('名称', { exact: true });
    await name.fill('未保存的草稿');
    expect((await request.post(logoutAPI, { headers })).status()).toBe(204);

    const dialogs: string[] = [];
    page.on('dialog', async (dialog) => {
      dialogs.push(dialog.type());
      await dialog.accept();
    });
    const expired = page.waitForResponse(
      (response) => new URL(response.url()).pathname === logoutAPI,
    );
    await clickSignOut(page);
    expect((await expired).status()).toBe(401);
    await expect(page).toHaveURL(loginURL(target));
    await expect(name).toHaveCount(0);
    expect(dialogs).toEqual(['confirm']);
  } finally {
    await request.post(logoutAPI, { headers });
  }
});
