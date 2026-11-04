// 管理端导航只列出已接入的页面。
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
    path: '/overview',
    name: '平台概览',
    icon: 'dashboard',
    component: './overview',
  },
  {
    path: '/agents',
    name: 'Agent 管理',
    icon: 'robot',
    component: './agents',
  },
  {
    path: '/',
    redirect: '/overview',
  },
  {
    component: './exception/404',
    layout: false,
    path: '*',
  },
];
