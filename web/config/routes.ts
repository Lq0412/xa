// 登录、注册和 404 保留独立页面，业务路由共用管理布局。
export default [
  { path: '/user/login', component: './user/login' },
  { path: '/user/register', component: './user/register' },
  {
    path: '/',
    component: '@/components/AdminLayout',
    routes: [
      { path: '/', redirect: '/overview' },
      { path: '/overview', component: './overview' },
      { path: '/agents', component: './agents' },
      { path: '/agents/new', component: './agents/AgentEditor' },
      { path: '/agents/:id', component: './agents/AgentEditor' },
      { path: '/approvals', component: './approvals' },
      { path: '/runs', component: './runs' },
      { path: '/runs/:id', component: './runs/detail' },
    ],
  },
  {
    component: './exception/404',
    path: '*',
  },
];
