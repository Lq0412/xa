// 业务路由沿用 Pro 的登录页和管理页组织方式。
export default [
  {
    path: '/user',
    layout: false,
    routes: [
      {
        name: 'login',
        path: '/user/login',
        component: './user/login',
      },
      {
        name: 'register',
        path: '/user/register',
        component: './user/register',
      },
    ],
  },
  {
    path: '/agents',
    name: 'agents',
    icon: 'robot',
    component: './agents',
  },
  {
    path: '/',
    redirect: '/agents',
  },
  {
    component: './exception/404',
    layout: false,
    path: '*',
  },
];
