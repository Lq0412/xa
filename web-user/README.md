# xa Web 用户端

独立 React 19、Ant Design 6、Umi Max 4 工程。通过 Go API 提供公共 Agent 浏览、登录注册、私有 Agent 列表与创建；对话、历史、记忆、反馈和账号设置按完整产品结构提供页面。

## 本地运行

先按 [启动说明](../docs/startup.md) 启动后端，再在根目录执行 `make web-user-install` 和 `make web-user`。访问 `http://localhost:8002`。默认代理地址为 `127.0.0.1:18080`，通过 `XA_API_PROXY` 覆盖。

## 验证与构建

在本目录执行 `npm run lint`、`npm run lint:antd`、`npm run build`。两个前端的真实浏览器用例集中在 `web/tests/`，运行方式见启动说明。日常检查不执行 `max setup`，避免清空运行中的生成目录。

构建产物为本目录 `dist/`。静态服务器需为页面路由回退至 `index.html`，将 `/api/` 代理至 Go server。管理员的后台链接使用构建环境变量 `XA_ADMIN_WEB_URL`；仅开发环境默认 `http://localhost:8000`。

尚未实现的模块使用明确的待开发状态，不提交模拟消息或伪造数据。页面和业务边界见 [前端结构](../docs/frontend-structure.md)。
