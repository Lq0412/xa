import { adminModules } from './modules';

// 管理端独立导航；未实现模块也有可访问页面，便于检查完整产品结构。
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
  ...adminModules.map((item) => ({
    path: item.path,
    name: item.name,
    icon: item.icon,
    component: item.available ? './agents' : './module',
  })),
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
