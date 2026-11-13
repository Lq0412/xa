# 管理端验收

管理端使用 Umi 4、React 19、Tailwind CSS、Radix UI、Lucide 和 TanStack Table。页面与命令以 [Web 说明](../web/README.md) 和 [前端结构](frontend-structure.md) 为准。

## 验收范围

- 管理员登录、刷新恢复会话、退出；普通用户进入独立用户端。
- 今天页、Agent 列表、整页创建与配置、未保存退出保护、配置版本冲突、启停和删除。
- 只读确认记录、运行列表和详情；运行查询不跨用户授权。
- 窄屏布局、键盘与焦点交互，以及真实后端的失败提示。

## 验证方式

使用 Node.js 24，按 [启动说明](startup.md) 启动真实依赖、server 和前端。在 `web/` 执行 `npm run lint`、`npm run build`；`npm run test:smoke` 执行管理端核心浏览器场景，`npm test` 执行现有完整浏览器用例。

通过环境变量 `XA_E2E_EMAIL`、`XA_E2E_PASSWORD` 注入管理员测试账号，不打印凭证。`XA_WEB_URL` 和 `XA_USER_WEB_URL` 可覆盖访问地址。测试输出位于项目根目录 `.output/web-e2e/`，可选 `XA_ADMIN_SHOTS` 指定截图目录。

本文规定验收方式，不作为某次执行成功的记录；实际命令、结果和截图随对应 PR 提交。
