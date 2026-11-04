# xa Web

基于 Ant Design Pro v6.0.3 官方模板，使用 React 19、Ant Design 6、Umi Max 4 和 ProComponents 3。保留上游 MIT 许可证（LICENSE）。

页面使用标准 ProLayout、ProTable、ModalForm 和 LoginForm。业务路由为登录、注册与 Agent 管理，连接 xa 的真实账号、Redis 会话和 Agent 管理接口。

## 启动

需要 Node.js 22 或更新版本。先按 [启动说明](../docs/startup.md) 启动基础依赖和 server，再在仓库根目录运行：

```bash
make web-install
make web
```

访问 `http://127.0.0.1:8000`。开发代理默认连接 `http://127.0.0.1:18080`，可通过 `XA_API_PROXY` 修改。管理员使用部署配置中的账号登录；普通用户可注册，公共 Agent 对普通用户只读。

## 开发与验证

```bash
cd web
npm run lint
npm run lint:antd
npm run build
```

`config/` 管理路由、布局及开发代理；`src/pages/` 存放业务页面；`src/services/xa/` 封装真实接口。生成目录 `src/.umi*`、依赖和构建产物不提交。

`src/services/ant-design-pro/` 和 `config/oneapi.json` 保留上游生成示例供模板参考，业务页面不会调用其中的演示接口。改造基于官方 v6.0.3 发布源码，未接入其演示 Worker、Mock、统计服务和部署工作流。

## 浏览器验收

`npm test` 使用 Playwright 连接已经启动的 Web 和 server，不替换网络响应。首次运行先执行 `npx playwright install --with-deps chromium`，需要安装 Chromium 及系统依赖。测试使用环境变量 `XA_E2E_EMAIL`、`XA_E2E_PASSWORD` 指定已有的测试账号；凭证不得写入文件或提交。默认地址为 `http://127.0.0.1:8000`，可用 `XA_WEB_URL` 修改。

测试会创建并删除自己的 Agent，覆盖登录、刷新恢复会话、筛选、编辑配置保留、启停、删除和退出。输出保存在仓库根目录 `.output/web-e2e/`，不采集截图、录屏或包含凭证的 trace。

## 构建产物

`npm run build` 输出 `dist/`，开发资源输出到仓库根目录 `.output/web-dev/`，两者可同时运行。安装时 `prepare` 生成 Umi 类型；日常 lint 不重新清空生成目录，以免干扰运行中的开发服务。

生产静态服务器需要将页面路由回退到 `index.html`，并将 `/api/` 和 `/health/` 代理到 server；开发代理不会包含在构建产物中。
